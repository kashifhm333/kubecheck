package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/bundle"
	"github.com/kubecheck/kubecheck/pkg/exitcode"
)

func TestRootHelp(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"--help"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v", code)
	}

	helpOutput := out.String()
	if !strings.Contains(helpOutput, "KubeCheck is an offline-first diagnostic engine") {
		t.Errorf("help output missing product intro: %s", helpOutput)
	}
	if !strings.Contains(helpOutput, "Available Commands:") {
		t.Errorf("help output missing commands section: %s", helpOutput)
	}

	// Test no args
	out.Reset()
	code = cli.Execute([]string{})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0) for empty args, got %v", code)
	}
}

func TestVersionCommand(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"version"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v", code)
	}

	versionOutput := out.String()
	if !strings.Contains(versionOutput, "KubeCheck Engine:") {
		t.Errorf("version output missing engine version: %s", versionOutput)
	}
	if !strings.Contains(versionOutput, "Default Knowledge Bundle:") {
		t.Errorf("version output missing bundle version: %s", versionOutput)
	}

	// Test -v flag to root
	out.Reset()
	code = cli.Execute([]string{"-v"})
	if code != exitcode.Success {
		t.Errorf("expected -v to succeed, got %v", code)
	}
}

func TestRulesCommand(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"rules"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v", code)
	}
	if !strings.Contains(out.String(), "Active Diagnostic Rules:") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestBundleCommand(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"bundle"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v", code)
	}
	if !strings.Contains(out.String(), "Knowledge Bundle Status:") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestCompletionCommand(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"completion", "bash"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v", code)
	}
	if !strings.Contains(out.String(), "bash completion") {
		t.Errorf("unexpected output: %s", out.String())
	}

	// Missing shell
	errOut.Reset()
	code = cli.Execute([]string{"completion"})
	if code != exitcode.UsageError {
		t.Errorf("expected UsageError for missing completion arg, got %v", code)
	}
}

func TestAnalyzeMissingInputs(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze"})
	if code != exitcode.UsageError {
		t.Errorf("expected exit code UsageError (2) when no inputs provided, got %v", code)
	}

	errStr := errOut.String()
	if !strings.Contains(errStr, "error: no input manifest files or directories specified") {
		t.Errorf("expected missing input error message, got: %s", errStr)
	}
}

func TestUnknownCommand(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"nonexistent-command"})
	if code != exitcode.UsageError {
		t.Errorf("expected exit code UsageError (2) for unknown command, got %v", code)
	}

	// Flag to root that is not help/version
	code = cli.Execute([]string{"--invalid-flag"})
	if code != exitcode.UsageError {
		t.Errorf("expected exit code UsageError (2) for invalid flag, got %v", code)
	}
}

func TestAnalyzeValidArguments(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "pod.yaml")
	manifest := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: nginx
    image: nginx:1.25.3
    securityContext:
      readOnlyRootFilesystem: true
      capabilities:
        drop:
        - ALL
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 200m
        memory: 256Mi
    livenessProbe:
      httpGet:
        path: /healthz
        port: 80
    readinessProbe:
      httpGet:
        path: /ready
        port: 80
`)
	if err := os.WriteFile(validFile, manifest, 0o600); err != nil {
		t.Fatalf("failed to create temp manifest: %v", err)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", validFile, "--target-version", "1.29"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0), got %v (err: %s)", code, errOut.String())
	}

	output := out.String()
	if !strings.Contains(output, "Target: Kubernetes v1.29") {
		t.Errorf("expected target version v1.29 in output, got: %s", output)
	}
	if !strings.Contains(output, "Analyzed 1 file(s), 1 document(s)") {
		t.Errorf("expected 1 file analyzed in output, got: %s", output)
	}
	if !strings.Contains(output, "Result: Clean") {
		t.Errorf("expected Clean result, got: %s", output)
	}
}

func TestAnalyzeNonexistentFile(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", "nonexistent-manifest.yaml"})
	if code != exitcode.InputError {
		t.Errorf("expected exit code InputError (4) for nonexistent file, got %v", code)
	}
}

func TestAnalyzeDuplicateKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dupFile := filepath.Join(tmpDir, "dup.yaml")
	manifest := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\ndata:\n  key: val1\n  key: val2\n")
	if err := os.WriteFile(dupFile, manifest, 0o600); err != nil {
		t.Fatalf("failed to create temp manifest: %v", err)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", dupFile})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1) for duplicate keys, got %v", code)
	}
	if !strings.Contains(out.String(), "KCHK-SYN-001") {
		t.Errorf("expected KCHK-SYN-001 finding in output, got: %s", out.String())
	}
}

func TestAnalyzeSchemaViolations(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad-pod.yaml")
	// Pod with invalid restartPolicy enum and missing containers
	manifest := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: bad-pod\nspec:\n  restartPolicy: BadPolicy\n")
	if err := os.WriteFile(badFile, manifest, 0o600); err != nil {
		t.Fatalf("failed to create temp manifest: %v", err)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", badFile})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1), got %v", code)
	}
	output := out.String()
	if !strings.Contains(output, "K8S-SCHEMA-003") {
		t.Errorf("expected K8S-SCHEMA-003 (missing containers), got: %s", output)
	}
	if !strings.Contains(output, "K8S-SCHEMA-004") {
		t.Errorf("expected K8S-SCHEMA-004 (invalid enum), got: %s", output)
	}
}

func TestAnalyzeDuplicateResources(t *testing.T) {
	tmpDir := t.TempDir()
	dupResFile := filepath.Join(tmpDir, "dup-res.yaml")
	manifest := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n")
	if err := os.WriteFile(dupResFile, manifest, 0o600); err != nil {
		t.Fatalf("failed to create temp manifest: %v", err)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", dupResFile})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1), got %v", code)
	}
	if !strings.Contains(out.String(), "MAINT-DUP-001") {
		t.Errorf("expected MAINT-DUP-001 finding, got: %s", out.String())
	}
}

func TestAnalyzeCrossResourceFindings(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Workload selector self-mismatch: should trigger CROSS-WORKLOAD-SEL-001 (SeverityError)
	mismatchFile := filepath.Join(tmpDir, "selector-mismatch.yaml")
	manifestMismatch := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken-deploy
spec:
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: worker
    spec:
      containers:
      - name: worker
        image: worker:1.0.0
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
`)
	if err := os.WriteFile(mismatchFile, manifestMismatch, 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	code := cli.Execute([]string{"analyze", mismatchFile})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1) for selector mismatch, got %v", code)
	}
	if !strings.Contains(out.String(), "CROSS-WORKLOAD-SEL-001") {
		t.Errorf("expected CROSS-WORKLOAD-SEL-001 in output, got: %s", out.String())
	}

	// 2. Valid Service + Deployment pair with matching ports
	validPairFile := filepath.Join(tmpDir, "valid-pair.yaml")
	manifestValidPair := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-deploy
spec:
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginx:1.25.3
        ports:
        - containerPort: 80
          name: http
        securityContext:
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 80
        readinessProbe:
          httpGet:
            path: /ready
            port: 80
---
apiVersion: v1
kind: Service
metadata:
  name: web-svc
spec:
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 80
`)
	if err := os.WriteFile(validPairFile, manifestValidPair, 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", validPairFile})
	if code != exitcode.Success {
		t.Errorf("expected clean exit code 0 for valid pair, got %v (out: %s)", code, out.String())
	}
	if !strings.Contains(out.String(), "Result: Clean") {
		t.Errorf("expected Result: Clean, got: %s", out.String())
	}
}

func findFixturePath(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	alt := filepath.Join("..", "..", rel)
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	return rel
}

func TestAnalyzeSecurityRulesAndPSS(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	// 1. Verify rules command lists security rules
	code := cli.Execute([]string{"rules"})
	if code != exitcode.Success {
		t.Errorf("expected rules command to succeed, got %v", code)
	}
	rulesOutput := out.String()
	if !strings.Contains(rulesOutput, "SEC-CTX-003") || !strings.Contains(rulesOutput, "SEC-RBAC-012") {
		t.Errorf("expected security rules in rules catalog output, got: %s", rulesOutput)
	}

	// 2. Dangerous host access fixture
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", findFixturePath("tests/fixtures/security/invalid/privileged_host_access.yaml")})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1) for privileged host access, got %v (err: %s)", code, errOut.String())
	}
	scanOutput := out.String()
	if !strings.Contains(scanOutput, "SEC-CTX-009") {
		t.Errorf("expected SEC-CTX-009 (runtime socket), got: %s", scanOutput)
	}
	if !strings.Contains(scanOutput, "SEC-CTX-005") {
		t.Errorf("expected SEC-CTX-005 (hostNetwork), got: %s", scanOutput)
	}

	// 3. Dangerous RBAC fixture
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", findFixturePath("tests/fixtures/security/invalid/wildcard_rbac.yaml")})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected exit code FindingsDetected (1) for wildcard RBAC, got %v (err: %s)", code, errOut.String())
	}
	rbacOutput := out.String()
	if !strings.Contains(rbacOutput, "SEC-RBAC-012") {
		t.Errorf("expected SEC-RBAC-012 (system:masters), got: %s", rbacOutput)
	}
	if !strings.Contains(rbacOutput, "SEC-RBAC-004") {
		t.Errorf("expected SEC-RBAC-004 (wildcard verbs), got: %s", rbacOutput)
	}

	// 4. Hardened clean pod evaluated against restricted PSS profile
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", findFixturePath("tests/fixtures/security/valid/secure_pod.yaml"), "--pss", "restricted"})
	if code != exitcode.Success {
		t.Errorf("expected exit code Success (0) for hardened pod under restricted PSS, got %v (out: %s)", code, out.String())
	}
	if !strings.Contains(out.String(), "Result: Clean") {
		t.Errorf("expected Result: Clean, got: %s", out.String())
	}
}

func createTestBundleForCLI(t *testing.T, dir string, version int) {
	manifest := &bundle.BundleManifest{
		FormatVersion: bundle.CurrentFormatVersion,
		BundleVersion: version,
		BundleID:      fmt.Sprintf("bundle-test-v%d", version),
		EngineCompatibility: bundle.EngineCompatibility{
			MinVersion: "1.0.0",
			MaxVersion: "1.99.99",
		},
		KubernetesVersions: []bundle.KubernetesVersion{
			{Minor: "1.31", PatchPin: "1.31.2", SchemaVersion: "1.0.0", Status: "supported"},
		},
		Generation: bundle.GenerationMetadata{
			Timestamp:        "2026-10-31T00:00:00Z",
			GeneratorVersion: "1.0.0-dev",
			FreshUntil:       "2027-10-31T00:00:00Z",
		},
		Files: []bundle.BundleFile{
			{
				Path:   "rules.json",
				SHA256: bundle.ComputeBytesSHA256([]byte("{}")),
				Size:   2,
				Kind:   "rule_pack",
			},
		},
		Licenses: []string{"Apache-2.0"},
	}

	_ = os.WriteFile(filepath.Join(dir, "rules.json"), []byte("{}"), 0600)
	_, priv := bundle.DefaultKeyPair()
	_ = bundle.SignManifest(manifest, priv, bundle.DefaultReleaseKeyID)
	data, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
}

func TestBundleCLIIntegration(t *testing.T) {
	// Isolate bundle store to temp dir
	tmpHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpHome)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	// 1. Status command
	code := cli.Execute([]string{"bundle", "status"})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for bundle status, got %v", code)
	}
	if !strings.Contains(out.String(), "Knowledge Bundle Status:") {
		t.Errorf("expected status output, got: %s", out.String())
	}

	// 2. List command (initially embedded fallback)
	out.Reset()
	code = cli.Execute([]string{"bundle", "list"})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for bundle list, got %v", code)
	}
	if !strings.Contains(out.String(), "Embedded Fallback") {
		t.Errorf("expected Embedded Fallback in list, got: %s", out.String())
	}

	// 3. Verify usage errors
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "verify"})
	if code != exitcode.UsageError {
		t.Errorf("expected UsageError (2) for bundle verify without dir, got %v", code)
	}

	// 4. Verify non-existent directory
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "verify", "nonexistent-bundle-path"})
	if code != exitcode.BundleError {
		t.Errorf("expected BundleError (6) for nonexistent bundle, got %v", code)
	}

	// 5. Create valid bundle v1 and verify
	bundleV1Dir := filepath.Join(tmpHome, "bundle-v1")
	_ = os.MkdirAll(bundleV1Dir, 0700)
	createTestBundleForCLI(t, bundleV1Dir, 100)

	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "verify", bundleV1Dir})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for valid bundle verify, got %v (err: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Cryptographic Ed25519 signature: VERIFIED") {
		t.Errorf("expected VERIFIED in verify output, got: %s", out.String())
	}

	// 6. Activate bundle v1
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "activate", bundleV1Dir})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for bundle activate, got %v (err: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "activated successfully") {
		t.Errorf("expected activation message, got: %s", out.String())
	}

	// 7. Verify list shows active bundle v1
	out.Reset()
	code = cli.Execute([]string{"bundle", "list"})
	if code != exitcode.Success || !strings.Contains(out.String(), "bundle-test-v100") {
		t.Errorf("expected bundle-test-v100 in list, got: %s", out.String())
	}

	// 8. Analyze with explicit --bundle flag
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", findFixturePath("tests/fixtures/security/valid/secure_pod.yaml"), "--bundle", bundleV1Dir})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for analyze with --bundle, got %v (err: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "bundle-test-v100") {
		t.Errorf("expected banner to reflect active bundle ID, got: %s", out.String())
	}

	// 9. Create valid bundle v2 and activate
	bundleV2Dir := filepath.Join(tmpHome, "bundle-v2")
	_ = os.MkdirAll(bundleV2Dir, 0700)
	createTestBundleForCLI(t, bundleV2Dir, 200)

	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "activate", "--from-dir", bundleV2Dir})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for bundle activate v2, got %v (err: %s)", code, errOut.String())
	}

	// 10. Rollback to v1
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "rollback"})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) for bundle rollback, got %v (err: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "bundle-test-v100") {
		t.Errorf("expected rolled back to bundle-test-v100, got: %s", out.String())
	}

	// 11. Rollback again when no prev exists -> error
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "rollback"})
	if code != exitcode.BundleError {
		t.Errorf("expected BundleError (6) for second rollback, got %v", code)
	}

	// 12. Unknown bundle subcommand
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "unknown-sub"})
	if code != exitcode.UsageError {
		t.Errorf("expected UsageError (2) for unknown subcommand, got %v", code)
	}

	// 13. Activate without arguments
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"bundle", "activate"})
	if code != exitcode.UsageError {
		t.Errorf("expected UsageError (2) for activate without args, got %v", code)
	}
}

func TestAnalyzeOutputFormatsAndFlags(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(out, errOut)

	fixture := findFixturePath("tests/fixtures/yaml/invalid/duplicate_key.yaml")

	// 1. JSON output
	code := cli.Execute([]string{"analyze", fixture, "--output", "json"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1), got %v", code)
	}
	var jsonParsed map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonParsed); err != nil {
		t.Fatalf("expected valid JSON output, got err: %v (raw: %s)", err, out.String())
	}
	if jsonParsed["$schema"] != "https://kubecheck.io/schemas/v1/output.json" {
		t.Errorf("unexpected json schema: %v", jsonParsed["$schema"])
	}

	// 2. SARIF output
	out.Reset()
	code = cli.Execute([]string{"analyze", fixture, "-o", "sarif"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1), got %v", code)
	}
	var sarifParsed map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &sarifParsed); err != nil {
		t.Fatalf("expected valid SARIF JSON output, got err: %v", err)
	}
	if sarifParsed["version"] != "2.1.0" {
		t.Errorf("expected sarif version 2.1.0, got: %v", sarifParsed["version"])
	}

	// 3. Compact output
	out.Reset()
	code = cli.Execute([]string{"analyze", fixture, "--output", "compact"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1), got %v", code)
	}
	if !strings.Contains(out.String(), ": [ERROR] [KCHK-SYN-001]") {
		t.Errorf("expected compact finding format, got: %s", out.String())
	}

	// 4. JUnit output
	out.Reset()
	code = cli.Execute([]string{"analyze", fixture, "--output", "junit"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1), got %v", code)
	}
	if !strings.Contains(out.String(), "<testsuites") || !strings.Contains(out.String(), "<failure") {
		t.Errorf("expected junit xml output, got: %s", out.String())
	}

	// 5. Min severity filtering (filter out info on pod.yaml)
	out.Reset()
	validPod := findFixturePath("tests/fixtures/yaml/valid/pod.yaml")
	code = cli.Execute([]string{"analyze", validPod, "--min-severity", "error"})
	if code != exitcode.Success {
		t.Errorf("expected Success (0) with min-severity error on info-only pod, got %v", code)
	}

	// 6. Fail-on threshold warning
	out.Reset()
	code = cli.Execute([]string{"analyze", findFixturePath("tests/fixtures/rules/invalid/unpinned_image.yaml"), "--fail-on", "warning"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1) with fail-on warning, got %v", code)
	}

	// 7. Max findings truncation
	out.Reset()
	code = cli.Execute([]string{"analyze", fixture, "--max-findings", "1"})
	if code != exitcode.FindingsDetected {
		t.Errorf("expected FindingsDetected (1), got %v", code)
	}

	// 8. Unsupported output format
	out.Reset()
	errOut.Reset()
	code = cli.Execute([]string{"analyze", fixture, "--output", "invalid_fmt_xyz"})
	if code != exitcode.UsageError {
		t.Errorf("expected UsageError (2) for invalid format, got %v", code)
	}
}

func TestCLIConstructorsAndHelpers(t *testing.T) {
	cli := NewCLI(nil, nil)
	if cli.Stdout == nil || cli.Stderr == nil || cli.Logger == nil || cli.Config == nil {
		t.Fatal("expected NewCLI with nil streams to default properly")
	}

	if cli.isTerminal(&bytes.Buffer{}) {
		t.Error("expected bytes.Buffer not to be a terminal")
	}
	_ = cli.isTerminal(os.Stdout)

	// Test CommandName
	oldArg0 := os.Args[0]
	defer func() { os.Args[0] = oldArg0 }()

	os.Args[0] = "/usr/local/bin/kubectl-check"
	if name := CommandName(); name != "kubectl check" {
		t.Errorf("expected 'kubectl check', got %q", name)
	}

	os.Args[0] = "/usr/local/bin/kubecheck"
	if name := CommandName(); name != "kubecheck" {
		t.Errorf("expected 'kubecheck', got %q", name)
	}
}
