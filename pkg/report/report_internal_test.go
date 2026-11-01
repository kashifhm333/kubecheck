package report

import (
	"strings"
	"testing"
)

func TestDeduceCategoryAllBranches(t *testing.T) {
	cases := []struct {
		ruleID   string
		expected string
	}{
		{"KCHK-SYN-001", "syntax"},
		{"K8S-SCHEMA-001", "schema"},
		{"K8S-SCH-002", "schema"},
		{"SEC-CTX-001", "security"},
		{"SEC-RBAC-001", "security"},
		{"PSS-BASELINE-001", "security"},
		{"NET-SVC-001", "network"},
		{"SVC-PORT-001", "network"},
		{"ING-BACKEND-001", "network"},
		{"WL-CONTAINER-001", "workload"},
		{"CONTAINER-IMAGE-001", "workload"},
		{"CROSS-SVC-001", "cross-resource"},
		{"DEP-API-001", "compatibility"},
		{"UNKNOWN-CUSTOM-001", "reliability"},
	}

	for _, c := range cases {
		got := deduceCategory(c.ruleID)
		if got != c.expected {
			t.Errorf("deduceCategory(%q) = %q, expected %q", c.ruleID, got, c.expected)
		}
	}
}

func TestTerminalColorStyles(t *testing.T) {
	colAlways := newTerminalColor("always", false)
	if !colAlways.enabled {
		t.Error("expected color to be enabled for mode 'always'")
	}
	if !strings.Contains(colAlways.success("OK"), "\033[") {
		t.Error("expected ANSI escape code in success() when enabled")
	}

	colNever := newTerminalColor("never", true)
	if colNever.enabled {
		t.Error("expected color to be disabled for mode 'never'")
	}
	if colNever.success("OK") != "OK" {
		t.Errorf("expected plain string when color disabled, got %q", colNever.success("OK"))
	}

	colAutoNoTerm := newTerminalColor("auto", false)
	if colAutoNoTerm.enabled {
		t.Error("expected color to be disabled for non-terminal auto mode")
	}

	colAutoTerm := newTerminalColor("auto", true)
	if !colAutoTerm.enabled {
		t.Error("expected color to be enabled for terminal auto mode without NO_COLOR")
	}
}
