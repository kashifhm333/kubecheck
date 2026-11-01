package parser

import (
	"fmt"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Normative Diagnostic Rule IDs for YAML and Governor defects.
const (
	RuleDuplicateKey   = "KCHK-SYN-001"
	RuleSyntaxError    = "KCHK-SYN-002"
	RuleInvalidUTF8    = "KCHK-SYN-003"
	RuleUnsafeTag      = "KCHK-SEC-001"
	RuleLimitDepth     = "KCHK-GOV-001"
	RuleLimitAliases   = "KCHK-GOV-002"
	RuleLimitScalar    = "KCHK-GOV-003"
	RuleLimitDocuments = "KCHK-GOV-004"
	RuleLimitInputSize = "KCHK-GOV-005"
)

// Finding represents a diagnostic issue detected during input resolution, decoding, or parsing.
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
