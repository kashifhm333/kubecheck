package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
)

// RenderCompact prints diagnostic findings in standard UNIX line-oriented format per CLI_UX_AND_EXIT_CODES.md §6.2.
// Format: <file>:<doc_idx>:<line>:<col>: [<SEVERITY>] [<RULE_ID>] <MESSAGE>
func RenderCompact(w io.Writer, findings []diagnostic.Finding) error {
	for _, f := range findings {
		sev := strings.ToUpper(string(f.Severity))
		filePath := f.Location.FilePath
		if filePath == "" {
			filePath = "<unknown>"
		}

		line := f.Location.Line
		col := f.Location.Column
		docIdx := f.Location.DocumentIndex

		fmt.Fprintf(w, "%s:%d:%d:%d: [%s] [%s] %s\n",
			filePath, docIdx, line, col, sev, f.RuleID, f.Message)
	}
	return nil
}
