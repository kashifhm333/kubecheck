package rule

import (
	"fmt"
	"sort"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
)

// Engine orchestrates evaluation of all registered diagnostic rules against an inventory.
type Engine struct {
	rules []*Rule
}

// NewEngine creates an Engine with the provided rules.
func NewEngine(rules []*Rule) *Engine {
	return &Engine{rules: rules}
}

// DefaultEngine creates an Engine preloaded with all standard built-in rules.
func DefaultEngine() *Engine {
	return NewEngine(DefaultRules())
}

// Register adds a new Rule to the engine.
func (e *Engine) Register(r *Rule) {
	e.rules = append(e.rules, r)
}

// Rules returns all registered rules.
func (e *Engine) Rules() []*Rule {
	return e.rules
}

// Evaluate runs all applicable rules against every resource in the inventory.
// Findings are deterministically sorted and deduplicated per RULE-SPEC-005.
func (e *Engine) Evaluate(inv *inventory.Inventory) []diagnostic.Finding {
	if inv == nil || len(inv.Resources) == 0 {
		return nil
	}

	var allFindings []diagnostic.Finding
	seen := make(map[string]bool)

	for _, res := range inv.Resources {
		for _, r := range e.rules {
			if !r.AppliesTo(res.ID.Kind) {
				continue
			}

			findings := r.Evaluate(res, inv)
			for _, f := range findings {
				key := fmt.Sprintf("%s|%s|%d|%d", f.RuleID, f.Location.FilePath, f.Location.Line, f.Location.Column)
				if !seen[key] {
					seen[key] = true
					allFindings = append(allFindings, f)
				}
			}
		}
	}

	// Canonical sorting: FilePath -> Line -> Column -> RuleID
	sort.Slice(allFindings, func(i, j int) bool {
		fi, fj := allFindings[i], allFindings[j]
		if fi.Location.FilePath != fj.Location.FilePath {
			return fi.Location.FilePath < fj.Location.FilePath
		}
		if fi.Location.Line != fj.Location.Line {
			return fi.Location.Line < fj.Location.Line
		}
		if fi.Location.Column != fj.Location.Column {
			return fi.Location.Column < fj.Location.Column
		}
		return fi.RuleID < fj.RuleID
	})

	return allFindings
}
