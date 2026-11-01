package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// JSONReport represents the root JSON schema for KubeCheck diagnostic output per docs/output/JSON_SCHEMA.md.
type JSONReport struct {
	Schema             string        `json:"$schema"`
	EngineVersion      string        `json:"engine_version"`
	BundleVersion      int           `json:"bundle_version"`
	BundleID           string        `json:"bundle_id"`
	TargetVersion      string        `json:"target_version"`
	ExecutionTimestamp string        `json:"execution_timestamp"`
	Status             string        `json:"status"`
	Summary            JSONSummary   `json:"summary"`
	Findings           []JSONFinding `json:"findings"`
}

// JSONSummary holds summary counters in JSON output.
type JSONSummary struct {
	TotalFiles     int               `json:"total_files"`
	TotalDocuments int               `json:"total_documents"`
	TotalResources int               `json:"total_resources"`
	FindingsCount  JSONFindingsCount `json:"findings_count"`
	DurationMs     float64           `json:"duration_ms"`
	Passed         bool              `json:"passed"`
}

// JSONFindingsCount tracks counts by severity.
type JSONFindingsCount struct {
	Error   int `json:"error"`
	Warning int `json:"warning"`
	Info    int `json:"info"`
}

// JSONFinding represents an individual diagnostic finding in JSON format.
type JSONFinding struct {
	ID          string           `json:"id"`
	RuleID      string           `json:"rule_id"`
	Category    string           `json:"category"`
	Severity    string           `json:"severity"`
	Confidence  string           `json:"confidence"`
	Message     string           `json:"message"`
	Location    JSONLocation     `json:"location"`
	Resource    *JSONResource    `json:"resource,omitempty"`
	Remediation *JSONRemediation `json:"remediation,omitempty"`
}

// JSONLocation specifies 1-based source coordinates.
type JSONLocation struct {
	FilePath      string `json:"file_path"`
	DocumentIndex int    `json:"document_index"`
	StartLine     int    `json:"start_line"`
	StartColumn   int    `json:"start_column"`
	EndLine       int    `json:"end_line"`
	EndColumn     int    `json:"end_column"`
	ByteOffset    int    `json:"byte_offset"`
}

// JSONResource identifies the target Kubernetes object.
type JSONResource struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
}

// JSONRemediation holds actionable fix guidance.
type JSONRemediation struct {
	Summary string `json:"summary"`
}

// RenderJSON serializes the analysis result to canonical indented JSON.
func RenderJSON(w io.Writer, summary Summary, findings []diagnostic.Finding) error {
	status := "completed"
	if !summary.Passed {
		status = "failed"
	}

	ts := summary.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	report := JSONReport{
		Schema:             "https://kubecheck.io/schemas/v1/output.json",
		EngineVersion:      summary.EngineVersion,
		BundleVersion:      summary.BundleVersion,
		BundleID:           summary.BundleID,
		TargetVersion:      summary.TargetVersion,
		ExecutionTimestamp: ts.Format(time.RFC3339),
		Status:             status,
		Summary: JSONSummary{
			TotalFiles:     summary.TotalFiles,
			TotalDocuments: summary.TotalDocuments,
			TotalResources: summary.TotalResources,
			FindingsCount: JSONFindingsCount{
				Error:   summary.ErrorCount,
				Warning: summary.WarningCount,
				Info:    summary.InfoCount,
			},
			DurationMs: float64(summary.Duration.Microseconds()) / 1000.0,
			Passed:     summary.Passed,
		},
		Findings: make([]JSONFinding, 0, len(findings)),
	}

	for _, f := range findings {
		cat := deduceCategory(f.RuleID)
		id := generateFindingID(f)

		jf := JSONFinding{
			ID:         id,
			RuleID:     f.RuleID,
			Category:   cat,
			Severity:   string(f.Severity),
			Confidence: string(f.EvidenceClass),
			Message:    f.Message,
			Location: JSONLocation{
				FilePath:      f.Location.FilePath,
				DocumentIndex: f.Location.DocumentIndex,
				StartLine:     f.Location.Line,
				StartColumn:   f.Location.Column,
				EndLine:       f.Location.Line,
				EndColumn:     f.Location.Column,
				ByteOffset:    f.Location.ByteOffset,
			},
		}

		if f.ResourceKind != "" || f.ResourceName != "" {
			jf.Resource = &JSONResource{
				Kind: f.ResourceKind,
				Name: f.ResourceName,
			}
		}

		if f.Remediation != "" {
			jf.Remediation = &JSONRemediation{
				Summary: f.Remediation,
			}
		}

		report.Findings = append(report.Findings, jf)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

func generateFindingID(f diagnostic.Finding) string {
	raw := fmt.Sprintf("%s|%s|%d|%d|%d|%s", f.RuleID, f.Location.FilePath, f.Location.DocumentIndex, f.Location.Line, f.Location.Column, f.Message)
	h := sha256.Sum256([]byte(raw))
	return "fnd_" + hex.EncodeToString(h[:8])
}

func deduceCategory(ruleID string) string {
	upper := strings.ToUpper(ruleID)
	switch {
	case strings.Contains(upper, "SYN"):
		return "syntax"
	case strings.Contains(upper, "SCH") || strings.Contains(upper, "SCHEMA"):
		return "schema"
	case strings.Contains(upper, "CROSS"):
		return "cross-resource"
	case strings.Contains(upper, "SEC") || strings.Contains(upper, "RBAC") || strings.Contains(upper, "PSS"):
		return "security"
	case strings.Contains(upper, "NET") || strings.Contains(upper, "SVC") || strings.Contains(upper, "ING"):
		return "network"
	case strings.Contains(upper, "WL") || strings.Contains(upper, "CONTAINER"):
		return "workload"
	case strings.Contains(upper, "DEP"):
		return "compatibility"
	default:
		return "reliability"
	}
}
