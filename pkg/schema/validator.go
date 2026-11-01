package schema

import (
	"fmt"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Validator validates resources against local schemas and lifecycle rules.
type Validator struct {
	Catalog *Catalog
}

// NewValidator initializes a Validator backed by a Catalog.
func NewValidator(c *Catalog) *Validator {
	if c == nil {
		c = NewDefaultCatalog()
	}
	return &Validator{Catalog: c}
}

// Validate validates an extracted resource against its schema and target Kubernetes version.
func (v *Validator) Validate(res *inventory.Resource, targetVersion string) []Finding {
	var findings []Finding

	// 1. API Version Lifecycle checks (DEP-API-001, DEP-API-002)
	if lfFinding, found := v.Catalog.CheckLifecycle(res.ID.Group, res.ID.Version, res.ID.Kind, targetVersion); found {
		lfFinding.Location = res.Location
		findings = append(findings, lfFinding)
	}

	// 2. Fetch Schema (K8S-GVK-009)
	resSchema, ok := v.Catalog.GetSchema(res.ID.Group, res.ID.Version, res.ID.Kind)
	if !ok {
		findings = append(findings, Finding{
			RuleID:        RuleUnknownGVK,
			Severity:      "warning",
			Message:       fmt.Sprintf("Resource GVK %q is not covered by offline schema catalog; skipping detailed schema checks", res.ID),
			Location:      res.Location,
			EvidenceClass: "deterministic",
		})
		return findings
	}

	// 3. Structural Validation
	schemaFindings := v.validateNode(res.RootNode, resSchema.Root, res.ID.Kind)
	findings = append(findings, schemaFindings...)

	return findings
}

func (v *Validator) validateNode(node *nir.Node, propSchema *PropertySchema, path string) []Finding {
	if node == nil || propSchema == nil {
		return nil
	}

	var findings []Finding

	switch node.Kind {
	case nir.KindMapping:
		if propSchema.Type != TypeObject && propSchema.Type != TypeAny {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected %s, got object (mapping)", path, propSchema.Type),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
			return findings
		}

		// Check required fields: K8S-SCHEMA-003
		for _, reqField := range propSchema.Required {
			if node.Get(reqField) == nil {
				fieldPath := reqField
				if path != "" {
					fieldPath = path + "." + reqField
				}
				findings = append(findings, Finding{
					RuleID:        RuleMissingRequired,
					Severity:      "error",
					Message:       fmt.Sprintf("Missing required field %q at %s", reqField, fieldPath),
					Location:      node.Location,
					EvidenceClass: "deterministic",
				})
			}
		}

		// Check known vs unknown fields: K8S-SCHEMA-001
		for _, entry := range node.Fields {
			if entry.Key == nil {
				continue
			}
			keyName := entry.Key.Value
			fieldPath := keyName
			if path != "" {
				fieldPath = path + "." + keyName
			}

			if childSchema, exists := propSchema.Properties[keyName]; exists {
				childFindings := v.validateNode(entry.Value, childSchema, fieldPath)
				findings = append(findings, childFindings...)
			} else if !propSchema.AdditionalProperties && !propSchema.PreserveUnknownFields && len(propSchema.Properties) > 0 {
				findings = append(findings, Finding{
					RuleID:        RuleUnknownField,
					Severity:      "warning",
					Message:       fmt.Sprintf("Unknown field %q at %s", keyName, fieldPath),
					Location:      entry.Key.Location,
					EvidenceClass: "deterministic",
					Details:       fmt.Sprintf("Field %s is not defined in the schema for %s", keyName, path),
				})
			}
		}

	case nir.KindSequence:
		if propSchema.Type != TypeArray && propSchema.Type != TypeAny {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected %s, got array (sequence)", path, propSchema.Type),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
			return findings
		}

		if propSchema.Items != nil {
			for i, item := range node.Items {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				itemFindings := v.validateNode(item, propSchema.Items, itemPath)
				findings = append(findings, itemFindings...)
			}
		}

	case nir.KindScalar:
		findings = append(findings, v.validateScalar(node, propSchema, path)...)

	case nir.KindNull:
		if propSchema.Type != TypeAny {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Null value not permitted at %q; expected %s", path, propSchema.Type),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	}

	return findings
}

func (v *Validator) validateScalar(node *nir.Node, propSchema *PropertySchema, path string) []Finding {
	var findings []Finding

	// Type matching
	switch propSchema.Type {
	case TypeString:
		if node.ScalarType != nir.TypeString {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected string, got %s", path, node.ScalarType),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	case TypeInteger:
		if node.ScalarType != nir.TypeInt {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected integer, got %s", path, node.ScalarType),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	case TypeNumber:
		if node.ScalarType != nir.TypeInt && node.ScalarType != nir.TypeFloat {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected number, got %s", path, node.ScalarType),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	case TypeBoolean:
		if node.ScalarType != nir.TypeBool {
			findings = append(findings, Finding{
				RuleID:        RuleWrongType,
				Severity:      "error",
				Message:       fmt.Sprintf("Wrong type at %q: expected boolean, got %s", path, node.ScalarType),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	case TypeObject, TypeArray:
		findings = append(findings, Finding{
			RuleID:        RuleWrongType,
			Severity:      "error",
			Message:       fmt.Sprintf("Wrong type at %q: expected %s, got scalar", path, propSchema.Type),
			Location:      node.Location,
			EvidenceClass: "deterministic",
		})
	}

	// Enum matching: K8S-SCHEMA-004
	if len(propSchema.Enum) > 0 {
		matched := false
		for _, allowed := range propSchema.Enum {
			if node.Value == allowed {
				matched = true
				break
			}
		}
		if !matched {
			findings = append(findings, Finding{
				RuleID:        RuleInvalidEnum,
				Severity:      "error",
				Message:       fmt.Sprintf("Invalid enum value %q at %q; allowed: [%s]", node.Value, path, strings.Join(propSchema.Enum, ", ")),
				Location:      node.Location,
				EvidenceClass: "deterministic",
			})
		}
	}

	return findings
}
