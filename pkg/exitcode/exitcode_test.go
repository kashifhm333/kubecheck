package exitcode_test

import (
	"testing"

	"github.com/kubecheck/kubecheck/pkg/exitcode"
)

func TestExitCodeValues(t *testing.T) {
	tests := []struct {
		code     exitcode.ExitCode
		expected int
		label    string
	}{
		{exitcode.Success, 0, "SUCCESS"},
		{exitcode.FindingsDetected, 1, "FINDINGS_DETECTED"},
		{exitcode.UsageError, 2, "USAGE_ERROR"},
		{exitcode.InputError, 3, "INPUT_ERROR"},
		{exitcode.OperationalError, 4, "OPERATIONAL_ERROR"},
		{exitcode.PartialScan, 5, "PARTIAL_SCAN"},
		{exitcode.BundleError, 6, "BUNDLE_ERROR"},
	}

	for _, tc := range tests {
		if tc.code.Int() != tc.expected {
			t.Errorf("expected exit code %d, got %d", tc.expected, tc.code.Int())
		}
		if tc.code.String() != tc.label {
			t.Errorf("expected label %s, got %s", tc.label, tc.code.String())
		}
	}
}

func TestUnknownExitCode(t *testing.T) {
	unknown := exitcode.ExitCode(99)
	if unknown.Int() != 99 {
		t.Errorf("expected 99, got %d", unknown.Int())
	}
	expectedStr := "UNKNOWN_EXIT_CODE(99)"
	if unknown.String() != expectedStr {
		t.Errorf("expected %s, got %s", expectedStr, unknown.String())
	}
}
