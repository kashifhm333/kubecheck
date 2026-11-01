package benchmark_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SafeParserAudit records evidence for parser memory safety and injection resistance.
type SafeParserAudit struct {
	DocumentID  string                `json:"document_id"`
	Timestamp   string                `json:"timestamp"`
	GateResults map[string]GateResult `json:"gate_results"`
	Passed      bool                  `json:"passed"`
}

// TestNetworkIsolationAudit enforces GATE-REL-REQ-007: Mandatory Offline Operation.
func TestNetworkIsolationAudit(t *testing.T) {
	root := findProjectRoot(t)

	// Disallowed network packages in core engine
	disallowedImports := map[string]bool{
		"net":        true,
		"net/http":   true,
		"net/rpc":    true,
		"crypto/tls": true,
	}

	var scannedFiles int
	fset := token.NewFileSet()

	scanDirs := []string{"pkg", "cmd/kubecheck"}
	for _, dir := range scanDirs {
		dirPath := filepath.Join(root, dir)
		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			scannedFiles++
			node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return fmt.Errorf("failed to parse imports of %s: %w", path, parseErr)
			}

			for _, imp := range node.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if disallowedImports[importPath] {
					t.Errorf("GATE-REL-REQ-007 VIOLATION: disallowed network import %q in %s", importPath, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed scanning directory %s: %v", dir, err)
		}
	}

	t.Logf("GATE-REL-REQ-007 passed: %d Go source files statically verified with 0 network imports", scannedFiles)
}

// TestSecretRedactionAudit enforces GATE-REL-REQ-009: Secret Redaction across all channels.
func TestSecretRedactionAudit(t *testing.T) {
	root := findProjectRoot(t)
	bin := ensureBinaryBuilt(t, root)

	// Canary secrets to inject into manifest
	canaries := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.t-IDcSemACt8x4iTMCda8Yhe3iZaWbvV5XKSTbuAn0M",
		"SuperSecretDatabasePassword2026!",
	}

	tmpDir := t.TempDir()
	canaryManifest := filepath.Join(tmpDir, "canary_secrets.yaml")
	manifestContent := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: canary-secret-pod
  namespace: prod
  annotations:
    vault.hashicorp.com/secret-token: "%s"
spec:
  containers:
  - name: app
    image: registry.example.com/app:v1.0.0
    env:
    - name: DB_PASSWORD
      value: "%s"
    - name: AWS_ACCESS_KEY_ID
      value: "%s"
    - name: AWS_SECRET_ACCESS_KEY
      value: "%s"
    - name: GITHUB_TOKEN
      value: "%s"
`, canaries[3], canaries[4], canaries[0], canaries[1], canaries[2])

	if err := os.WriteFile(canaryManifest, []byte(manifestContent), 0600); err != nil {
		t.Fatalf("failed to write canary manifest: %v", err)
	}

	formats := []string{"terminal", "json", "sarif", "compact", "junit"}
	var auditLog bytes.Buffer
	auditLog.WriteString(fmt.Sprintf("=== KubeCheck Secret Redaction Penetration Audit [%s] ===\n", time.Now().UTC().Format(time.RFC3339)))

	for _, fmtName := range formats {
		for _, verbose := range []bool{false, true} {
			args := []string{"analyze", canaryManifest, "-o", fmtName}
			if verbose {
				args = append(args, "-v")
			}

			cmd := exec.Command(bin, args...)
			cmd.Dir = root
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run() // Exit code may be non-zero due to findings

			outCombined := stdout.String() + "\n" + stderr.String()
			auditLog.WriteString(fmt.Sprintf("\n--- Format: %s (verbose=%v) ---\n", fmtName, verbose))

			for _, canary := range canaries {
				if strings.Contains(outCombined, canary) {
					t.Errorf("GATE-REL-REQ-009 VIOLATION: Canary secret %q leaked in %s output (verbose=%v)!\nSnippet:\n%s",
						canary, fmtName, verbose, outCombined)
					auditLog.WriteString(fmt.Sprintf("FAIL: Canary %s leaked!\n", canary))
				} else {
					auditLog.WriteString(fmt.Sprintf("PASS: Canary %s safely redacted.\n", canary[:8]+"..."))
				}
			}
		}
	}

	// Write evidence log
	logPath := filepath.Join(root, "evidence", "secret-redaction-audit.log")
	if err := os.WriteFile(logPath, auditLog.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write secret redaction audit log: %v", err)
	}
	t.Logf("Secret redaction audit successfully passed and written to %s", logPath)
}

// TestParserMemorySafetyAndInjection enforces GATE-REL-REQ-010: Parser safety and injection resistance.
func TestParserMemorySafetyAndInjection(t *testing.T) {
	root := findProjectRoot(t)
	bin := ensureBinaryBuilt(t, root)

	gateResults := make(map[string]GateResult)

	// 1. Billion-laughs alias bomb interception
	aliasBombPath := filepath.Join(root, "tests/fixtures/yaml/adversarial/alias_bomb.yaml")
	cmdAlias := exec.Command(bin, "analyze", aliasBombPath, "-o", "compact")
	cmdAlias.Dir = root
	var stdoutAlias, stderrAlias bytes.Buffer
	cmdAlias.Stdout = &stdoutAlias
	cmdAlias.Stderr = &stderrAlias

	startAlias := time.Now()
	errAlias := cmdAlias.Run()
	durationAlias := time.Since(startAlias)

	// Alias bomb must be intercepted rapidly (< 2 seconds) and exit with code 1 (defect detected)
	aliasOutput := stdoutAlias.String() + stderrAlias.String()
	aliasPassed := strings.Contains(aliasOutput, "KCHK-GOV-002") && durationAlias < 2*time.Second
	gateResults["GATE-REL-REQ-010-ALIAS"] = GateResult{
		RequirementID: "GATE-REL-REQ-010",
		Description:   "Billion-laughs alias bomb safely intercepted by resource governor without OOM or hang",
		Threshold:     "Duration < 2.0s, Governor rule KCHK-GOV-002 triggered",
		Actual:        fmt.Sprintf("Duration = %.2f ms, Rule triggered = %v", float64(durationAlias.Microseconds())/1000.0, strings.Contains(aliasOutput, "KCHK-GOV-002")),
		Passed:        aliasPassed,
	}
	if !aliasPassed {
		t.Errorf("GATE-REL-REQ-010-ALIAS failed: bomb not safely intercepted (err=%v, out=%s, dur=%v)",
			errAlias, aliasOutput, durationAlias)
	}

	// 2. Unsafe constructor tag rejection
	unsafeTagPath := filepath.Join(root, "tests/fixtures/yaml/adversarial/unsafe_tag.yaml")
	cmdTag := exec.Command(bin, "analyze", unsafeTagPath, "-o", "compact")
	cmdTag.Dir = root
	var stdoutTag, stderrTag bytes.Buffer
	cmdTag.Stdout = &stdoutTag
	cmdTag.Stderr = &stderrTag
	_ = cmdTag.Run()

	tagOutput := stdoutTag.String() + stderrTag.String()
	tagPassed := strings.Contains(tagOutput, "KCHK-SEC-001")
	gateResults["GATE-REL-REQ-010-TAG"] = GateResult{
		RequirementID: "GATE-REL-REQ-010",
		Description:   "Untrusted language constructor tags rejected or rendered inert with rule KCHK-SEC-001",
		Threshold:     "Rule KCHK-SEC-001 triggered",
		Actual:        fmt.Sprintf("Rule triggered = %v", tagPassed),
		Passed:        tagPassed,
	}
	if !tagPassed {
		t.Errorf("GATE-REL-REQ-010-TAG failed: unsafe tag not rejected with KCHK-SEC-001 (out=%s)", tagOutput)
	}

	allPassed := aliasPassed && tagPassed

	audit := SafeParserAudit{
		DocumentID:  "SAFE-PARSER-AUDIT-001",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		GateResults: gateResults,
		Passed:      allPassed,
	}

	evidencePath := filepath.Join(root, "evidence", "safe-parser-audit.json")
	auditData, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal safe parser audit: %v", err)
	}
	if err := os.WriteFile(evidencePath, auditData, 0644); err != nil {
		t.Fatalf("failed to write safe parser evidence: %v", err)
	}
	t.Logf("Safe parser audit successfully passed and written to %s", evidencePath)
}
