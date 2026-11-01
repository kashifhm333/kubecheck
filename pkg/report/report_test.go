package report_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/report"
)

func sampleFindings(tmpFile string) []diagnostic.Finding {
	return []diagnostic.Finding{
		{
			RuleID:   "KCHK-SCH-002",
			Severity: diagnostic.SeverityError,
			Message:  "Invalid value type for field 'spec.replicas'. Expected integer, found string.",
			Location: nir.Location{
				FilePath:      tmpFile,
				Line:          2,
				Column:        11,
				DocumentIndex: 0,
			},
			EvidenceClass: diagnostic.EvidenceDeterministic,
			Remediation:   "Change string value '\"three\"' to integer 3.",
			ResourceKind:  "Deployment",
			ResourceName:  "web",
		},
		{
			RuleID:   "SEC-CTX-004",
			Severity: diagnostic.SeverityWarning,
			Message:  "Container 'nginx' does not drop ALL capabilities",
			Location: nir.Location{
				FilePath:      tmpFile,
				Line:          3,
				Column:        5,
				DocumentIndex: 0,
			},
			EvidenceClass: diagnostic.EvidenceProbable,
			Remediation:   "Add capabilities.drop: ['ALL'] to securityContext.",
			ResourceKind:  "Deployment",
			ResourceName:  "web",
		},
		{
			RuleID:   "SEC-CTX-017",
			Severity: diagnostic.SeverityInfo,
			Message:  "Container 'nginx' does not set readOnlyRootFilesystem: true",
			Location: nir.Location{
				FilePath:      tmpFile,
				Line:          4,
				Column:        5,
				DocumentIndex: 0,
			},
			EvidenceClass: diagnostic.EvidenceContextDependent,
			Remediation:   "Set readOnlyRootFilesystem: true.",
			ResourceKind:  "Deployment",
			ResourceName:  "web",
		},
	}
}

func sampleSummary(passed bool) report.Summary {
	return report.Summary{
		TargetVersion:   "1.31",
		EngineVersion:   "1.0.0-dev",
		BundleID:        "kubecheck-standard-2026.10.1",
		BundleVersion:   20261001,
		Offline:         true,
		Timestamp:       time.Now().UTC(),
		Duration:        15 * time.Millisecond,
		TotalFiles:      1,
		TotalDocuments:  1,
		TotalResources:  1,
		ErrorCount:      1,
		WarningCount:    1,
		InfoCount:       1,
		Passed:          passed,
		FailOnThreshold: "error",
	}
}

func createDummyYAMLFile(t *testing.T) string {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "deploy.yaml")
	content := `apiVersion: apps/v1
spec:
  replicas: "three"
  containers:
  - name: nginx
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write dummy yaml: %v", err)
	}
	return path
}

func TestFilterFindingsAndSeverityLevel(t *testing.T) {
	if report.SeverityLevel(diagnostic.SeverityError) != 3 {
		t.Errorf("expected error level 3")
	}
	if report.SeverityLevel(diagnostic.SeverityWarning) != 2 {
		t.Errorf("expected warning level 2")
	}
	if report.SeverityLevel(diagnostic.SeverityInfo) != 1 {
		t.Errorf("expected info level 1")
	}
	if report.SeverityLevel(diagnostic.Severity("other")) != 0 {
		t.Errorf("expected other level 0")
	}

	findings := sampleFindings("test.yaml")

	// 1. Min severity info -> all 3
	res, tr := report.FilterFindings(findings, "info", 0)
	if len(res) != 3 || tr != 0 {
		t.Errorf("expected 3 findings, got %d (tr: %d)", len(res), tr)
	}

	// 2. Min severity warning -> 2
	res, tr = report.FilterFindings(findings, "warning", 0)
	if len(res) != 2 || tr != 0 {
		t.Errorf("expected 2 findings, got %d", len(res))
	}

	// 3. Min severity error -> 1
	res, tr = report.FilterFindings(findings, "error", 0)
	if len(res) != 1 || tr != 0 {
		t.Errorf("expected 1 finding, got %d", len(res))
	}

	// 4. Truncation limit
	res, tr = report.FilterFindings(findings, "info", 2)
	if len(res) != 2 || tr != 1 {
		t.Errorf("expected 2 findings with 1 truncated, got %d (tr: %d)", len(res), tr)
	}

	// 5. Unknown severity fallback to info
	res, tr = report.FilterFindings(findings, "unknown", 0)
	if len(res) != 3 {
		t.Errorf("expected fallback to 3 findings, got %d", len(res))
	}
}

func TestRenderTerminal(t *testing.T) {
	file := createDummyYAMLFile(t)
	findings := sampleFindings(file)
	summary := sampleSummary(false)

	opts := report.DefaultOptions()
	opts.ColorMode = "always"
	opts.IsTerminal = true

	var buf bytes.Buffer
	err := report.Render(&buf, summary, findings, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "KubeCheck 1.0.0-dev") {
		t.Errorf("missing header in terminal output: %s", out)
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "KCHK-SCH-002") {
		t.Errorf("missing error finding in terminal output: %s", out)
	}
	if !strings.Contains(out, "replicas: \"three\"") {
		t.Errorf("missing code snippet in terminal output: %s", out)
	}
	if !strings.Contains(out, "^^^") {
		t.Errorf("missing carets in snippet: %s", out)
	}
	if !strings.Contains(out, "Remediation:") {
		t.Errorf("missing remediation in terminal output: %s", out)
	}
	if !strings.Contains(out, "FAILED") {
		t.Errorf("missing failed result in summary: %s", out)
	}

	// Test NO_COLOR environment behavior
	t.Setenv("NO_COLOR", "1")
	buf.Reset()
	opts.ColorMode = "auto"
	_ = report.Render(&buf, summary, findings, opts)
	if strings.Contains(buf.String(), "\033[") {
		t.Errorf("expected NO_COLOR to suppress ANSI codes")
	}

	// Test clean passing run
	buf.Reset()
	cleanSummary := sampleSummary(true)
	cleanSummary.ErrorCount = 0
	cleanSummary.WarningCount = 0
	cleanSummary.InfoCount = 0
	_ = report.Render(&buf, cleanSummary, nil, opts)
	if !strings.Contains(buf.String(), "Clean (0 findings)") {
		t.Errorf("expected clean findings output, got: %s", buf.String())
	}

	// Test passed with warnings
	buf.Reset()
	cleanSummary.WarningCount = 2
	_ = report.Render(&buf, cleanSummary, nil, opts)
	if !strings.Contains(buf.String(), "PASSED (2 warning(s)") {
		t.Errorf("expected passed with warnings, got: %s", buf.String())
	}
}

func TestRenderJSON(t *testing.T) {
	file := createDummyYAMLFile(t)
	findings := sampleFindings(file)
	summary := sampleSummary(false)

	opts := report.DefaultOptions()
	opts.Format = "json"

	var buf bytes.Buffer
	err := report.Render(&buf, summary, findings, opts)
	if err != nil {
		t.Fatalf("unexpected error rendering json: %v", err)
	}

	var parsed report.JSONReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	if parsed.Schema != "https://kubecheck.io/schemas/v1/output.json" {
		t.Errorf("unexpected schema: %s", parsed.Schema)
	}
	if parsed.EngineVersion != "1.0.0-dev" {
		t.Errorf("unexpected engine version: %s", parsed.EngineVersion)
	}
	if parsed.Status != "failed" {
		t.Errorf("expected failed status, got: %s", parsed.Status)
	}
	if parsed.Summary.FindingsCount.Error != 1 {
		t.Errorf("expected 1 error count, got: %d", parsed.Summary.FindingsCount.Error)
	}
	if len(parsed.Findings) != 3 {
		t.Fatalf("expected 3 findings, got: %d", len(parsed.Findings))
	}
	if !strings.HasPrefix(parsed.Findings[0].ID, "fnd_") {
		t.Errorf("expected fnd_ prefix for finding ID, got: %s", parsed.Findings[0].ID)
	}
	if parsed.Findings[0].Category != "schema" {
		t.Errorf("expected schema category, got: %s", parsed.Findings[0].Category)
	}
	if parsed.Findings[1].Category != "security" {
		t.Errorf("expected security category, got: %s", parsed.Findings[1].Category)
	}
}

func TestRenderSARIF(t *testing.T) {
	file := createDummyYAMLFile(t)
	findings := sampleFindings(file)
	summary := sampleSummary(false)

	opts := report.DefaultOptions()
	opts.Format = "sarif"

	var buf bytes.Buffer
	err := report.Render(&buf, summary, findings, opts)
	if err != nil {
		t.Fatalf("unexpected error rendering sarif: %v", err)
	}

	var parsed report.SARIFReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid sarif json: %v", err)
	}

	if parsed.Version != "2.1.0" {
		t.Errorf("expected sarif version 2.1.0, got: %s", parsed.Version)
	}
	if len(parsed.Runs) != 1 {
		t.Fatalf("expected 1 run, got: %d", len(parsed.Runs))
	}
	driver := parsed.Runs[0].Tool.Driver
	if driver.Name != "KubeCheck" {
		t.Errorf("expected driver name KubeCheck, got: %s", driver.Name)
	}
	if len(driver.Rules) != 3 {
		t.Errorf("expected 3 registered rules in sarif, got: %d", len(driver.Rules))
	}
	results := parsed.Runs[0].Results
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got: %d", len(results))
	}
	if results[0].Level != "error" {
		t.Errorf("expected level error for first finding, got: %s", results[0].Level)
	}
	if results[1].Level != "warning" {
		t.Errorf("expected level warning for second finding, got: %s", results[1].Level)
	}
	if results[2].Level != "note" {
		t.Errorf("expected level note for info finding, got: %s", results[2].Level)
	}
}

func TestRenderCompact(t *testing.T) {
	file := createDummyYAMLFile(t)
	findings := sampleFindings(file)
	summary := sampleSummary(false)

	opts := report.DefaultOptions()
	opts.Format = "compact"

	var buf bytes.Buffer
	err := report.Render(&buf, summary, findings, opts)
	if err != nil {
		t.Fatalf("unexpected error rendering compact: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in compact output, got: %d", len(lines))
	}
	if !strings.Contains(lines[0], ":0:2:11: [ERROR] [KCHK-SCH-002]") {
		t.Errorf("unexpected compact line format: %s", lines[0])
	}
}

func TestRenderJUnit(t *testing.T) {
	file := createDummyYAMLFile(t)
	findings := sampleFindings(file)
	summary := sampleSummary(false)

	opts := report.DefaultOptions()
	opts.Format = "junit"

	// 1. With findings
	var buf bytes.Buffer
	err := report.Render(&buf, summary, findings, opts)
	if err != nil {
		t.Fatalf("unexpected error rendering junit: %v", err)
	}

	var parsed report.JUnitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid junit xml: %v", err)
	}

	if parsed.Name != "KubeCheck" {
		t.Errorf("expected testsuites name KubeCheck, got: %s", parsed.Name)
	}
	if parsed.Failures != 1 {
		t.Errorf("expected 1 failure in junit, got: %d", parsed.Failures)
	}

	// 2. Clean run
	buf.Reset()
	cleanSummary := sampleSummary(true)
	_ = report.Render(&buf, cleanSummary, nil, opts)
	parsed = report.JUnitTestSuites{}
	if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid clean junit xml: %v", err)
	}
	if parsed.Failures != 0 {
		t.Errorf("expected 0 failures in clean junit run, got: %d", parsed.Failures)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	opts := report.DefaultOptions()
	opts.Format = "unsupported_xyz"
	var buf bytes.Buffer
	err := report.Render(&buf, sampleSummary(true), nil, opts)
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Errorf("expected unsupported format error, got: %v", err)
	}
}
