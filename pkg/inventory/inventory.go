package inventory

import (
	"fmt"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Diagnostic rule IDs for object-root and identity defects per STATIC_ANALYSIS_RULES.md.
const (
	RuleRootNotMapping     = "K8S-GVK-001"
	RuleMissingAPIVersion  = "K8S-GVK-002"
	RuleMissingKind        = "K8S-GVK-003"
	RuleAPIVersionNotStr   = "K8S-GVK-004"
	RuleKindNotStr         = "K8S-GVK-005"
	RuleInvalidAPIVersion  = "K8S-GVK-006"
	RuleMissingMetadata    = "K8S-GVK-007"
	RuleMetadataNotMapping = "K8S-GVK-008"
	RuleUnknownGVK         = "K8S-GVK-009"
	RuleMissingName        = "K8S-GVK-010"
	RuleDuplicateResource  = "MAINT-DUP-001"
)

// ClusterScopedKinds lists standard Kubernetes kinds that are cluster-scoped (do not have a namespace).
var ClusterScopedKinds = map[string]bool{
	"Namespace":                        true,
	"ClusterRole":                      true,
	"ClusterRoleBinding":               true,
	"StorageClass":                     true,
	"PersistentVolume":                 true,
	"CustomResourceDefinition":         true,
	"PriorityClass":                    true,
	"APIService":                       true,
	"Node":                             true,
	"ComponentStatus":                  true,
	"CertificateSigningRequest":        true,
	"IngressClass":                     true,
	"RuntimeClass":                     true,
	"VolumeAttachment":                 true,
	"MutatingWebhookConfiguration":     true,
	"ValidatingWebhookConfiguration":   true,
	"ValidatingAdmissionPolicy":        true,
	"ValidatingAdmissionPolicyBinding": true,
}

// ResourceID uniquely identifies a Kubernetes object within an API scope (GVKNN).
type ResourceID struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// String returns a human-readable identifier: [Group/]Kind/[Namespace/]Name.
func (id ResourceID) String() string {
	gvk := id.Kind
	if id.Group != "" {
		gvk = fmt.Sprintf("%s.%s", id.Kind, id.Group)
	}
	if id.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", gvk, id.Namespace, id.Name)
	}
	return fmt.Sprintf("%s/%s", gvk, id.Name)
}

// ScopeKey returns the unique declaration key for duplicate detection: (Group, Kind, Namespace, Name).
func (id ResourceID) ScopeKey() string {
	return fmt.Sprintf("%s|%s|%s|%s", id.Group, id.Kind, id.Namespace, id.Name)
}

// Resource represents a normalized, extracted Kubernetes object from an NIR document.
type Resource struct {
	ID              ResourceID        `json:"id"`
	APIVersion      string            `json:"api_version"`
	IsClusterScoped bool              `json:"is_cluster_scoped"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	Doc             *nir.Document     `json:"-"`
	RootNode        *nir.Node         `json:"-"`
	Location        nir.Location      `json:"location"`
}

// Finding represents a diagnostic issue detected during resource inventory extraction.
type Finding struct {
	RuleID        string       `json:"rule_id"`
	Severity      string       `json:"severity"` // "error", "warning", "info"
	Message       string       `json:"message"`
	Location      nir.Location `json:"location"`
	EvidenceClass string       `json:"evidence_class"` // "deterministic"
	Details       string       `json:"details,omitempty"`
}

func (f Finding) String() string {
	return fmt.Sprintf("[%s] %s (at %s)", f.RuleID, f.Message, f.Location)
}

// Inventory indexes all extracted Kubernetes resources across the scanned documents.
type Inventory struct {
	Resources   []*Resource
	byScopeKey  map[string]*Resource
	byKind      map[string][]*Resource
	byGroupKind map[string][]*Resource
	byNamespace map[string][]*Resource
}

// NewInventory initializes an empty Resource Inventory.
func NewInventory() *Inventory {
	return &Inventory{
		Resources:   make([]*Resource, 0),
		byScopeKey:  make(map[string]*Resource),
		byKind:      make(map[string][]*Resource),
		byGroupKind: make(map[string][]*Resource),
		byNamespace: make(map[string][]*Resource),
	}
}

// Add indexes a resource into the inventory.
func (inv *Inventory) Add(r *Resource) {
	inv.Resources = append(inv.Resources, r)
	inv.byScopeKey[r.ID.ScopeKey()] = r
	inv.byKind[r.ID.Kind] = append(inv.byKind[r.ID.Kind], r)

	gk := r.ID.Kind
	if r.ID.Group != "" {
		gk = r.ID.Group + "/" + r.ID.Kind
	}
	inv.byGroupKind[gk] = append(inv.byGroupKind[gk], r)

	ns := r.ID.Namespace
	if ns == "" {
		ns = "_cluster_"
	}
	inv.byNamespace[ns] = append(inv.byNamespace[ns], r)
}

// GetByScopeKey retrieves a resource by (Group, Kind, Namespace, Name).
func (inv *Inventory) GetByScopeKey(scopeKey string) *Resource {
	return inv.byScopeKey[scopeKey]
}

// GetByKind returns all resources matching the specified Kind.
func (inv *Inventory) GetByKind(kind string) []*Resource {
	return inv.byKind[kind]
}

// ParseAPIVersion splits an apiVersion string into Group and Version per K8S-API-001.
// e.g. "v1" -> ("", "v1"), "apps/v1" -> ("apps", "v1").
func ParseAPIVersion(apiVersion string) (group string, version string, valid bool) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return "", "", false
	}
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 1 {
		// Core API group (e.g. "v1")
		return "", parts[0], true
	}
	if len(parts) == 2 {
		if parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	}
	return "", "", false
}

// Extract extracts and indexes resources from a list of NIR documents, returning findings.
func Extract(docs []*nir.Document) (*Inventory, []Finding) {
	inv := NewInventory()
	var findings []Finding

	for _, doc := range docs {
		if doc.Root == nil || doc.Root.Kind == nir.KindNull {
			continue // empty document, handled by parser/edge rules
		}

		docFindings := extractFromNode(doc, doc.Root, inv)
		findings = append(findings, docFindings...)
	}

	return inv, findings
}

func extractFromNode(doc *nir.Document, root *nir.Node, inv *Inventory) []Finding {
	var findings []Finding

	// K8S-GVK-001: Root must be a mapping
	if root.Kind != nir.KindMapping {
		findings = append(findings, Finding{
			RuleID:        RuleRootNotMapping,
			Severity:      "error",
			Message:       fmt.Sprintf("Document root must be a YAML mapping, got %s", root.Kind),
			Location:      root.Location,
			EvidenceClass: "deterministic",
		})
		return findings
	}

	// Check for List kinds (e.g., kind: List)
	kindNode := root.Get("kind")
	if kindNode != nil && kindNode.Kind == nir.KindScalar && kindNode.Value == "List" {
		itemsNode := root.Get("items")
		if itemsNode != nil && itemsNode.Kind == nir.KindSequence {
			for _, item := range itemsNode.Items {
				itemFindings := extractFromNode(doc, item, inv)
				findings = append(findings, itemFindings...)
			}
			return findings
		}
	}

	// K8S-GVK-002 / 004: apiVersion
	apiVerNode := root.Get("apiVersion")
	if apiVerNode == nil {
		findings = append(findings, Finding{
			RuleID:        RuleMissingAPIVersion,
			Severity:      "error",
			Message:       "Missing required root field \"apiVersion\"",
			Location:      root.Location,
			EvidenceClass: "deterministic",
		})
	} else if apiVerNode.Kind != nir.KindScalar || apiVerNode.ScalarType != nir.TypeString {
		findings = append(findings, Finding{
			RuleID:        RuleAPIVersionNotStr,
			Severity:      "error",
			Message:       "Field \"apiVersion\" must be a string",
			Location:      apiVerNode.Location,
			EvidenceClass: "deterministic",
		})
	}

	// K8S-GVK-003 / 005: kind
	if kindNode == nil {
		findings = append(findings, Finding{
			RuleID:        RuleMissingKind,
			Severity:      "error",
			Message:       "Missing required root field \"kind\"",
			Location:      root.Location,
			EvidenceClass: "deterministic",
		})
	} else if kindNode.Kind != nir.KindScalar || kindNode.ScalarType != nir.TypeString {
		findings = append(findings, Finding{
			RuleID:        RuleKindNotStr,
			Severity:      "error",
			Message:       "Field \"kind\" must be a string",
			Location:      kindNode.Location,
			EvidenceClass: "deterministic",
		})
	}

	if apiVerNode == nil || kindNode == nil {
		return findings
	}

	apiVersion := apiVerNode.Value
	kind := kindNode.Value

	group, version, validVer := ParseAPIVersion(apiVersion)
	if !validVer {
		findings = append(findings, Finding{
			RuleID:        RuleInvalidAPIVersion,
			Severity:      "error",
			Message:       fmt.Sprintf("Invalid apiVersion format %q; expected \"VERSION\" or \"GROUP/VERSION\"", apiVersion),
			Location:      apiVerNode.Location,
			EvidenceClass: "deterministic",
		})
	}

	// K8S-GVK-007 / 008: metadata
	metaNode := root.Get("metadata")
	var name string
	var namespace string
	isClusterScoped := ClusterScopedKinds[kind]
	if !isClusterScoped {
		namespace = "default"
	}
	var labels, annotations map[string]string

	if metaNode == nil {
		findings = append(findings, Finding{
			RuleID:        RuleMissingMetadata,
			Severity:      "warning",
			Message:       "Missing \"metadata\" section in Kubernetes object",
			Location:      root.Location,
			EvidenceClass: "deterministic",
		})
		findings = append(findings, Finding{
			RuleID:        RuleMissingName,
			Severity:      "error",
			Message:       fmt.Sprintf("Resource %s is missing required \"metadata.name\"", kind),
			Location:      root.Location,
			EvidenceClass: "deterministic",
		})
	} else if metaNode.Kind != nir.KindMapping {
		findings = append(findings, Finding{
			RuleID:        RuleMetadataNotMapping,
			Severity:      "error",
			Message:       "Field \"metadata\" must be a mapping",
			Location:      metaNode.Location,
			EvidenceClass: "deterministic",
		})
		findings = append(findings, Finding{
			RuleID:        RuleMissingName,
			Severity:      "error",
			Message:       fmt.Sprintf("Resource %s is missing required \"metadata.name\"", kind),
			Location:      metaNode.Location,
			EvidenceClass: "deterministic",
		})
	} else {
		// K8S-GVK-010: metadata.name
		nameNode := metaNode.Get("name")
		if nameNode == nil || nameNode.Value == "" {
			findings = append(findings, Finding{
				RuleID:        RuleMissingName,
				Severity:      "error",
				Message:       fmt.Sprintf("Resource %s is missing required \"metadata.name\"", kind),
				Location:      metaNode.Location,
				EvidenceClass: "deterministic",
			})
		} else {
			name = nameNode.Value
		}

		// Namespace
		nsNode := metaNode.Get("namespace")
		if nsNode != nil && nsNode.Value != "" {
			namespace = nsNode.Value
		}

		// Labels & Annotations
		labels = extractStringMap(metaNode.Get("labels"))
		annotations = extractStringMap(metaNode.Get("annotations"))
	}

	resID := ResourceID{
		Group:     group,
		Version:   version,
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}

	res := &Resource{
		ID:              resID,
		APIVersion:      apiVersion,
		IsClusterScoped: isClusterScoped,
		Labels:          labels,
		Annotations:     annotations,
		Doc:             doc,
		RootNode:        root,
		Location:        root.Location,
	}

	// Collision / duplicate detection: MAINT-DUP-001
	if name != "" {
		scopeKey := resID.ScopeKey()
		if existing, exists := inv.byScopeKey[scopeKey]; exists {
			findings = append(findings, Finding{
				RuleID:        RuleDuplicateResource,
				Severity:      "error",
				Message:       fmt.Sprintf("Duplicate resource declaration: %s already declared at %s", resID, existing.Location),
				Location:      root.Location,
				EvidenceClass: "deterministic",
				Details:       fmt.Sprintf("Original declaration: %s", existing.Location),
			})
		} else {
			inv.Add(res)
		}
	} else {
		inv.Add(res)
	}

	return findings
}

func extractStringMap(n *nir.Node) map[string]string {
	if n == nil || n.Kind != nir.KindMapping {
		return nil
	}
	m := make(map[string]string)
	for _, entry := range n.Fields {
		if entry.Key != nil && entry.Value != nil && entry.Value.Kind == nir.KindScalar {
			m[entry.Key.Value] = entry.Value.Value
		}
	}
	return m
}
