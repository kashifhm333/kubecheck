// Package report implements multi-format diagnostic serialization and terminal rendering
// per docs/distribution/CLI_UX_AND_EXIT_CODES.md §6 and SYSTEM_ARCHITECTURE.md §9.
package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// Summary holds execution statistics and environment metadata for a diagnostic run.
type Summary struct {
	TargetVersion   string        `json:"target_version"`
	EngineVersion   string        `json:"engine_version"`
	BundleID        string        `json:"bundle_id"`
	BundleVersion   int           `json:"bundle_version"`
	Offline         bool          `json:"offline"`
	Timestamp       time.Time     `json:"timestamp"`
	Duration        time.Duration `json:"duration"`
	TotalFiles      int           `json:"total_files"`
	TotalDocuments  int           `json:"total_documents"`
	TotalResources  int           `json:"total_resources"`
	ErrorCount      int           `json:"error_count"`
	WarningCount    int           `json:"warning_count"`
	InfoCount       int           `json:"info_count"`
	Passed          bool          `json:"passed"`
	FailOnThreshold string        `json:"fail_on_threshold"`
}

// Options configures the report formatting behavior and output destinations.
type Options struct {
	Format          string
	ColorMode       string
	MinSeverity     string
	MaxFindings     int
	ShowRemediation bool
	RedactSecrets   bool
	IsTerminal      bool
}

// DefaultOptions returns standard defaults conforming to CLI_UX_AND_EXIT_CODES.md.
func DefaultOptions() Options {
	return Options{
		Format:          "terminal",
		ColorMode:       "auto",
		MinSeverity:     "info",
		MaxFindings:     500,
		ShowRemediation: true,
		RedactSecrets:   true,
		IsTerminal:      false,
	}
}

// SeverityLevel maps a diagnostic severity string to a comparable integer priority.
func SeverityLevel(sev diagnostic.Severity) int {
	switch strings.ToLower(string(sev)) {
	case "error":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// FilterFindings filters findings by minimum severity threshold and applies the max findings limit.
// Returns the filtered finding slice and the count of truncated findings (if any).
func FilterFindings(findings []diagnostic.Finding, minSeverity string, maxFindings int) ([]diagnostic.Finding, int) {
	minLevel := SeverityLevel(diagnostic.Severity(minSeverity))
	if minLevel == 0 {
		minLevel = 1 // default to "info"
	}

	var filtered []diagnostic.Finding
	for _, f := range findings {
		if SeverityLevel(f.Severity) >= minLevel {
			filtered = append(filtered, f)
		}
	}

	truncated := 0
	if maxFindings > 0 && len(filtered) > maxFindings {
		truncated = len(filtered) - maxFindings
		filtered = filtered[:maxFindings]
	}

	return filtered, truncated
}

// Render serializes diagnostic findings and scan summary into the requested format.
func Render(w io.Writer, summary Summary, findings []diagnostic.Finding, opts Options) error {
	filteredFindings, truncated := FilterFindings(findings, opts.MinSeverity, opts.MaxFindings)

	switch strings.ToLower(opts.Format) {
	case "json":
		return RenderJSON(w, summary, filteredFindings)
	case "sarif":
		return RenderSARIF(w, summary, filteredFindings)
	case "compact":
		return RenderCompact(w, filteredFindings)
	case "junit":
		return RenderJUnit(w, summary, filteredFindings)
	case "terminal", "":
		return RenderTerminal(w, summary, filteredFindings, truncated, opts)
	default:
		return fmt.Errorf("unsupported output format %q: expected terminal, json, sarif, compact, or junit", opts.Format)
	}
}
