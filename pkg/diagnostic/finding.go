package diagnostic

import (
	"fmt"
	"sort"

	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/schema"
)

// Severity indicates the impact level of a diagnostic finding per CLI-UX-001.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// EvidenceClass categorizes the certainty of the diagnostic verdict per RULE-SPEC-001.
type EvidenceClass string

const (
	EvidenceDeterministic    EvidenceClass = "deterministic"
	EvidenceProbable         EvidenceClass = "probable"
	EvidenceContextDependent EvidenceClass = "context-dependent"
)

// Finding represents a canonical diagnostic finding produced by any engine component.
type Finding struct {
	RuleID        string        `json:"rule_id"`
	Severity      Severity      `json:"severity"`
	Message       string        `json:"message"`
	Location      nir.Location  `json:"location"`
	EvidenceClass EvidenceClass `json:"evidence_class"`
	Details       string        `json:"details,omitempty"`
	Remediation   string        `json:"remediation,omitempty"`
	ResourceKind  string        `json:"resource_kind,omitempty"`
	ResourceName  string        `json:"resource_name,omitempty"`
}

func (f Finding) String() string {
	return fmt.Sprintf("[%s] %s (at %s)", f.RuleID, f.Message, f.Location)
}

// DeduplicateAndSort removes duplicate findings and sorts them deterministically:
// FilePath -> Line -> Column -> RuleID -> Message.
func DeduplicateAndSort(findings []Finding) []Finding {
	if len(findings) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var deduped []Finding

	for _, f := range findings {
		key := fmt.Sprintf("%s|%s|%d|%d|%s", f.RuleID, f.Location.FilePath, f.Location.Line, f.Location.Column, f.Message)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, f)
		}
	}

	sort.Slice(deduped, func(i, j int) bool {
		fi, fj := deduped[i], deduped[j]
		if fi.Location.FilePath != fj.Location.FilePath {
			return fi.Location.FilePath < fj.Location.FilePath
		}
		if fi.Location.Line != fj.Location.Line {
			return fi.Location.Line < fj.Location.Line
		}
		if fi.Location.Column != fj.Location.Column {
			return fi.Location.Column < fj.Location.Column
		}
		if fi.RuleID != fj.RuleID {
			return fi.RuleID < fj.RuleID
		}
		return fi.Message < fj.Message
	})

	return deduped
}

// FromParser converts a parser.Finding into a canonical Finding.
func FromParser(pf parser.Finding) Finding {
	return Finding{
		RuleID:        pf.RuleID,
		Severity:      Severity(pf.Severity),
		Message:       pf.Message,
		Location:      pf.Location,
		EvidenceClass: EvidenceClass(pf.EvidenceClass),
		Details:       pf.Details,
	}
}

// FromInventory converts an inventory.Finding into a canonical Finding.
func FromInventory(inf inventory.Finding) Finding {
	return Finding{
		RuleID:        inf.RuleID,
		Severity:      Severity(inf.Severity),
		Message:       inf.Message,
		Location:      inf.Location,
		EvidenceClass: EvidenceClass(inf.EvidenceClass),
		Details:       inf.Details,
	}
}

// FromSchema converts a schema.Finding into a canonical Finding.
func FromSchema(sf schema.Finding) Finding {
	return Finding{
		RuleID:        sf.RuleID,
		Severity:      Severity(sf.Severity),
		Message:       sf.Message,
		Location:      sf.Location,
		EvidenceClass: EvidenceClass(sf.EvidenceClass),
		Details:       sf.Details,
	}
}
