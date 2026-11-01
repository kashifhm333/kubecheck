package diagnostic_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/schema"
)

func TestFindingConversions(t *testing.T) {
	loc := nir.Location{FilePath: "test.yaml", Line: 10, Column: 2}

	// FromParser
	pf := parser.Finding{
		RuleID:        "KCHK-SYN-001",
		Severity:      "error",
		Message:       "duplicate key",
		Location:      loc,
		EvidenceClass: "deterministic",
	}
	f1 := diagnostic.FromParser(pf)
	if f1.RuleID != "KCHK-SYN-001" || f1.Severity != diagnostic.SeverityError {
		t.Errorf("unexpected conversion FromParser: %+v", f1)
	}
	if !strings.Contains(f1.String(), "KCHK-SYN-001") {
		t.Errorf("unexpected f1.String(): %s", f1.String())
	}

	// FromInventory
	inf := inventory.Finding{
		RuleID:        "K8S-GVK-002",
		Severity:      "error",
		Message:       "missing apiVersion",
		Location:      loc,
		EvidenceClass: "deterministic",
	}
	f2 := diagnostic.FromInventory(inf)
	if f2.RuleID != "K8S-GVK-002" || f2.Severity != diagnostic.SeverityError {
		t.Errorf("unexpected conversion FromInventory: %+v", f2)
	}

	// FromSchema
	sf := schema.Finding{
		RuleID:        "K8S-SCHEMA-001",
		Severity:      "warning",
		Message:       "unknown field",
		Location:      loc,
		EvidenceClass: "deterministic",
	}
	f3 := diagnostic.FromSchema(sf)
	if f3.RuleID != "K8S-SCHEMA-001" || f3.Severity != diagnostic.SeverityWarning {
		t.Errorf("unexpected conversion FromSchema: %+v", f3)
	}
}

func TestDeduplicateAndSort(t *testing.T) {
	// Empty input
	if res := diagnostic.DeduplicateAndSort(nil); res != nil {
		t.Errorf("expected nil for empty input, got %+v", res)
	}

	findings := []diagnostic.Finding{
		{
			RuleID:   "RULE-B",
			Message:  "msg B",
			Location: nir.Location{FilePath: "b.yaml", Line: 5, Column: 1},
		},
		{
			RuleID:   "RULE-A",
			Message:  "msg A2",
			Location: nir.Location{FilePath: "a.yaml", Line: 10, Column: 1},
		},
		{
			RuleID:   "RULE-A",
			Message:  "msg A1",
			Location: nir.Location{FilePath: "a.yaml", Line: 2, Column: 5},
		},
		{
			// Duplicate of above
			RuleID:   "RULE-A",
			Message:  "msg A1",
			Location: nir.Location{FilePath: "a.yaml", Line: 2, Column: 5},
		},
		{
			RuleID:   "RULE-A",
			Message:  "msg A0",
			Location: nir.Location{FilePath: "a.yaml", Line: 2, Column: 1},
		},
	}

	sorted := diagnostic.DeduplicateAndSort(findings)
	if len(sorted) != 4 {
		t.Fatalf("expected 4 findings after deduplication, got %d", len(sorted))
	}

	// Verify order: a.yaml:2:1 (RULE-A) -> a.yaml:2:5 (RULE-A) -> a.yaml:10:1 (RULE-A) -> b.yaml:5:1 (RULE-B)
	if sorted[0].Location.Line != 2 || sorted[0].Location.Column != 1 {
		t.Errorf("expected first finding at line 2 col 1, got %v", sorted[0].Location)
	}
	if sorted[1].Location.Line != 2 || sorted[1].Location.Column != 5 {
		t.Errorf("expected second finding at line 2 col 5, got %v", sorted[1].Location)
	}
	if sorted[2].Location.Line != 10 {
		t.Errorf("expected third finding at line 10, got %v", sorted[2].Location)
	}
	if sorted[3].Location.FilePath != "b.yaml" {
		t.Errorf("expected fourth finding in b.yaml, got %s", sorted[3].Location.FilePath)
	}
}
