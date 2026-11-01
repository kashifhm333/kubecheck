package schema

import (
	"fmt"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Diagnostic rule IDs for schema and API lifecycle defects per STATIC_ANALYSIS_RULES.md.
const (
	RuleUnknownGVK      = "K8S-GVK-009"
	RuleUnknownField    = "K8S-SCHEMA-001"
	RuleWrongType       = "K8S-SCHEMA-002"
	RuleMissingRequired = "K8S-SCHEMA-003"
	RuleInvalidEnum     = "K8S-SCHEMA-004"
	RuleDeprecatedAPI   = "DEP-API-001"
	RuleRemovedAPI      = "DEP-API-002"
)

// FieldType identifies allowed property types in a structural schema.
type FieldType string

const (
	TypeString  FieldType = "string"
	TypeInteger FieldType = "integer"
	TypeNumber  FieldType = "number"
	TypeBoolean FieldType = "boolean"
	TypeObject  FieldType = "object"
	TypeArray   FieldType = "array"
	TypeAny     FieldType = "any"
)

// PropertySchema defines validation rules for an individual field or nested object.
type PropertySchema struct {
	Type                  FieldType                  `json:"type"`
	Description           string                     `json:"description,omitempty"`
	Required              []string                   `json:"required,omitempty"`
	Properties            map[string]*PropertySchema `json:"properties,omitempty"`
	Items                 *PropertySchema            `json:"items,omitempty"`
	Enum                  []string                   `json:"enum,omitempty"`
	PreserveUnknownFields bool                       `json:"preserve_unknown_fields,omitempty"`
	AdditionalProperties  bool                       `json:"additional_properties,omitempty"`
}

// ResourceSchema represents the complete structural schema for a Kubernetes GVK.
type ResourceSchema struct {
	Group       string          `json:"group"`
	Version     string          `json:"version"`
	Kind        string          `json:"kind"`
	ClusterWide bool            `json:"cluster_wide"`
	Root        *PropertySchema `json:"root"`
}

// LifecycleInfo tracks the deprecation and removal lifecycle of an API version.
type LifecycleInfo struct {
	Group          string
	Version        string
	Kind           string
	DeprecatedIn   string // Kubernetes minor version when deprecated (e.g. "1.16")
	RemovedIn      string // Kubernetes minor version when removed (e.g. "1.22")
	ReplacementAPI string // e.g. "apps/v1", "batch/v1"
}

// Finding represents a diagnostic issue detected during schema validation.
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
