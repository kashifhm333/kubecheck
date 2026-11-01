package rule

import (
	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
)

// Category groups diagnostic rules by domain per STATIC_ANALYSIS_RULES.md.
type Category string

const (
	CategoryYAML            Category = "yaml"
	CategorySchema          Category = "schema"
	CategoryWorkload        Category = "workload"
	CategorySecurity        Category = "security"
	CategoryNetwork         Category = "network"
	CategoryStorage         Category = "storage"
	CategoryReliability     Category = "reliability"
	CategoryMaintainability Category = "maintainability"
)

// Rule defines the interface and metadata for an executable diagnostic rule.
type Rule struct {
	ID            string                                                                       `json:"id"`
	Title         string                                                                       `json:"title"`
	Description   string                                                                       `json:"description"`
	Category      Category                                                                     `json:"category"`
	Severity      diagnostic.Severity                                                          `json:"severity"`
	EvidenceClass diagnostic.EvidenceClass                                                     `json:"evidence_class"`
	ApplicableTo  []string                                                                     `json:"applicable_to,omitempty"` // empty means all kinds
	Evaluate      func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding `json:"-"`
}

// AppliesTo returns true if the rule applies to the specified resource kind.
func (r *Rule) AppliesTo(kind string) bool {
	if len(r.ApplicableTo) == 0 {
		return true
	}
	for _, k := range r.ApplicableTo {
		if k == kind {
			return true
		}
	}
	return false
}
