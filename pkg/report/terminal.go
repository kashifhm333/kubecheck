package report

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// TerminalColor holds ANSI styling codes.
type TerminalColor struct {
	enabled bool
}

func newTerminalColor(mode string, isTerm bool) *TerminalColor {
	enabled := false
	switch strings.ToLower(mode) {
	case "always":
		enabled = true
	case "never":
		enabled = false
	default: // "auto"
		if os.Getenv("NO_COLOR") == "" && isTerm {
			enabled = true
		}
	}
	return &TerminalColor{enabled: enabled}
}

func (c *TerminalColor) bold(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

func (c *TerminalColor) dim(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[2m" + s + "\033[0m"
}

func (c *TerminalColor) error(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1;31m" + s + "\033[0m"
}

func (c *TerminalColor) warning(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1;33m" + s + "\033[0m"
}

func (c *TerminalColor) info(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1;36m" + s + "\033[0m"
}

func (c *TerminalColor) success(s string) string {
	if !c.enabled {
		return s
	}
	return "\033[1;32m" + s + "\033[0m"
}

// RenderTerminal prints the rich human-readable terminal report per CLI_UX_AND_EXIT_CODES.md §6.1.
func RenderTerminal(w io.Writer, summary Summary, findings []diagnostic.Finding, truncated int, opts Options) error {
	col := newTerminalColor(opts.ColorMode, opts.IsTerminal)

	// Header banner
	bundleInfo := ""
	if summary.BundleID != "" {
		bundleInfo = fmt.Sprintf(" · Bundle: %s (v%d)", summary.BundleID, summary.BundleVersion)
	}
	fmt.Fprintf(w, "[KubeCheck %s · Target: Kubernetes v%s%s · Offline: %t]\n",
		summary.EngineVersion, summary.TargetVersion, bundleInfo, summary.Offline)
	fmt.Fprintf(w, "Analyzed %d file(s), %d document(s), %d resource(s)\n\n",
		summary.TotalFiles, summary.TotalDocuments, summary.TotalResources)

	// Findings list
	for _, f := range findings {
		var sevHeader string
		switch f.Severity {
		case diagnostic.SeverityError:
			sevHeader = col.error("ERROR")
		case diagnostic.SeverityWarning:
			sevHeader = col.warning("WARNING")
		default:
			sevHeader = col.info("INFO")
		}

		locStr := f.Location.FilePath
		if f.Location.Line > 0 {
			locStr = fmt.Sprintf("%s:%d:%d", f.Location.FilePath, f.Location.Line, f.Location.Column)
		}
		docStr := fmt.Sprintf("[Doc #%d]", f.Location.DocumentIndex)

		fmt.Fprintf(w, "%s %s in %s %s:\n", sevHeader, col.bold("["+f.RuleID+"]"), locStr, col.dim(docStr))
		fmt.Fprintf(w, "  %s\n", f.Message)

		// Print source code snippet with carets if file is available
		if snippet := extractSnippet(f.Location.FilePath, f.Location.Line, f.Location.Column, f.RuleID, col); snippet != "" {
			fmt.Fprintln(w)
			fmt.Fprint(w, snippet)
		}

		// Remediation hint
		if opts.ShowRemediation && f.Remediation != "" {
			fmt.Fprintf(w, "  %s %s\n", col.bold("Remediation:"), f.Remediation)
		}

		// Evidence and confidence
		if f.EvidenceClass != "" {
			evidence := string(f.EvidenceClass)
			if f.Details != "" {
				evidence = fmt.Sprintf("%s (%s)", f.EvidenceClass, f.Details)
			}
			fmt.Fprintf(w, "  %s %s · %s %s\n", col.dim("Confidence:"), f.EvidenceClass, col.dim("Evidence:"), evidence)
		}

		fmt.Fprintln(w)
	}

	if truncated > 0 {
		fmt.Fprintf(w, "%s\n\n", col.warning(fmt.Sprintf("... [truncated %d finding(s) due to --max-findings limit of %d]", truncated, opts.MaxFindings)))
	}

	// Scan summary footer
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
	fmt.Fprintln(w, col.bold("Scan Summary:"))
	fmt.Fprintf(w, "  Target Version:  v%s\n", summary.TargetVersion)
	fmt.Fprintf(w, "  Documents:       %d parsed\n", summary.TotalDocuments)
	fmt.Fprintf(w, "  Resources:       %d evaluated\n", summary.TotalResources)
	fmt.Fprintf(w, "  Findings:        %d Error, %d Warning, %d Info\n", summary.ErrorCount, summary.WarningCount, summary.InfoCount)
	if summary.Duration > 0 {
		fmt.Fprintf(w, "  Duration:        %.1f ms\n", float64(summary.Duration.Microseconds())/1000.0)
	}

	var resultStr string
	if !summary.Passed {
		failCount := summary.ErrorCount
		if strings.EqualFold(summary.FailOnThreshold, "warning") {
			failCount += summary.WarningCount
		} else if strings.EqualFold(summary.FailOnThreshold, "info") {
			failCount += summary.WarningCount + summary.InfoCount
		}
		resultStr = col.error(fmt.Sprintf("FAILED (Found %d finding(s) at or above fail threshold %q)", failCount, summary.FailOnThreshold))
	} else if summary.WarningCount > 0 || summary.InfoCount > 0 {
		resultStr = col.warning(fmt.Sprintf("PASSED (%d warning(s), %d info)", summary.WarningCount, summary.InfoCount))
	} else {
		resultStr = col.success("Clean (0 findings)")
	}

	fmt.Fprintf(w, "Result: %s\n", resultStr)
	return nil
}

var snippetRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|secret|token|apikey|api_key|private_key|auth|bearer)[\w.-]*\s*[:=]\s*["']?([^"'\s\n]+)["']?`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----`),
}

func maskSensitiveSnippet(line string) string {
	res := line
	for _, re := range snippetRedactionPatterns {
		if strings.Contains(re.String(), "password") {
			res = re.ReplaceAllString(res, `$1: "[REDACTED]"`)
		} else {
			res = re.ReplaceAllString(res, "[REDACTED]")
		}
	}
	return res
}

// extractSnippet reads the target line and immediate context from the file and formats with carets.
func extractSnippet(filePath string, targetLine, targetCol int, ruleID string, col *TerminalColor) string {
	if filePath == "" || filePath == "-" || filePath == "<stdin>" || targetLine <= 0 {
		return ""
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	totalLines := len(lines)
	if targetLine > totalLines {
		return ""
	}

	start := targetLine - 2
	if start < 1 {
		start = 1
	}
	end := targetLine + 1
	if end > totalLines {
		end = totalLines
	}

	var sb strings.Builder
	secretValRe := regexp.MustCompile(`(:\s*["']?)([^"'\s\n]+)(["']?)`)
	for lineNum := start; lineNum <= end; lineNum++ {
		lineContent := maskSensitiveSnippet(lines[lineNum-1])
		if strings.Contains(strings.ToUpper(ruleID), "SECRET") {
			lineContent = secretValRe.ReplaceAllString(lineContent, `${1}[REDACTED]${3}`)
		}
		if lineNum == targetLine {
			sb.WriteString(fmt.Sprintf("%s %4d | %s\n", col.error(">"), lineNum, lineContent))
			// Caret underline
			caretIndent := 0
			if targetCol > 1 {
				caretIndent = targetCol - 1
			} else {
				// Fallback to leading whitespace of line
				caretIndent = len(lineContent) - len(strings.TrimLeft(lineContent, " "))
			}
			caretLen := len(strings.TrimSpace(lineContent))
			if caretLen < 1 {
				caretLen = 1
			}
			if caretLen > 40 {
				caretLen = 40
			}
			caretStr := strings.Repeat("^", caretLen)
			sb.WriteString(fmt.Sprintf("       | %s%s\n", strings.Repeat(" ", caretIndent), col.error(caretStr)))
		} else {
			sb.WriteString(fmt.Sprintf("  %s | %s\n", col.dim(fmt.Sprintf("%4d", lineNum)), lineContent))
		}
	}

	return sb.String()
}
