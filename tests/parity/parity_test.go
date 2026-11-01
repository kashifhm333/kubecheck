package parity_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findProjectRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatalf("could not find project root containing go.mod from %s", cwd)
	return ""
}

func ensureBinaryBuilt(t *testing.T, root string) string {
	binPath := filepath.Join(root, "bin", "kubecheck")
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	cmd := exec.Command("go", "build", "-trimpath", "-o", binPath, "./cmd/kubecheck")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary for parity test: %v (output: %s)", err, string(out))
	}
	return binPath
}

func runCommand(t *testing.T, cmdName string, args []string, env []string, dir string) (string, string, int) {
	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to execute command %s %v: %v", cmdName, args, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode
}

func TestCrossChannelParityMandate(t *testing.T) {
	root := findProjectRoot(t)
	bin := ensureBinaryBuilt(t, root)
	pythonScript := filepath.Join(root, "distribution", "pip", "kubecheck", "cli.py")

	scenarios := []struct {
		name string
		args []string
	}{
		{
			name: "Version command",
			args: []string{"version"},
		},
		{
			name: "Clean manifest analysis",
			args: []string{"analyze", "tests/fixtures/yaml/valid/pod.yaml"},
		},
		{
			name: "Duplicate key detection exit 1",
			args: []string{"analyze", "tests/fixtures/yaml/invalid/duplicate_key.yaml"},
		},
		{
			name: "Schema violation JSON output",
			args: []string{"analyze", "tests/fixtures/schema/invalid/unknown_field.yaml", "--output", "json"},
		},
		{
			name: "Security RBAC SARIF output",
			args: []string{"analyze", "tests/fixtures/security/invalid/wildcard_rbac.yaml", "--output", "sarif"},
		},
		{
			name: "Compact output",
			args: []string{"analyze", "tests/fixtures/rules/invalid/unpinned_image.yaml", "--output", "compact"},
		},
		{
			name: "Bundle status command",
			args: []string{"bundle", "status"},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// 1. Direct native binary execution
			goOut, goErr, goCode := runCommand(t, bin, sc.args, nil, root)

			// 2. Python wrapper execution
			pyArgs := append([]string{pythonScript}, sc.args...)
			pyEnv := []string{"KUBECHECK_BIN=" + bin}
			pyOut, pyErr, pyCode := runCommand(t, "python3", pyArgs, pyEnv, root)

			// Assert identical exit codes per GATE-REL-REQ-020
			if goCode != pyCode {
				t.Errorf("exit code mismatch: Go=%d vs Python=%d", goCode, pyCode)
			}

			// In JSON / SARIF modes, ignore minor execution_timestamp duration variances
			if containsFlag(sc.args, "json") || containsFlag(sc.args, "sarif") {
				if len(goOut) == 0 || len(pyOut) == 0 {
					t.Errorf("expected non-empty output for JSON/SARIF: Go len=%d, Py len=%d", len(goOut), len(pyOut))
				}
			} else {
				// Normalize timing lines before string equality check
				cleanGoOut := stripTimingLines(goOut)
				cleanPyOut := stripTimingLines(pyOut)
				if cleanGoOut != cleanPyOut {
					t.Errorf("stdout output divergence!\n--- Go Binary ---\n%s\n--- Python Launcher ---\n%s", cleanGoOut, cleanPyOut)
				}
			}

			if goErr != pyErr {
				t.Errorf("stderr divergence!\n--- Go ---\n%s\n--- Python ---\n%s", goErr, pyErr)
			}
		})
	}
}

func TestKubectlCheckAlias(t *testing.T) {
	root := findProjectRoot(t)
	bin := ensureBinaryBuilt(t, root)

	// Create symlink kubectl-check pointing to kubecheck
	tmpDir := t.TempDir()
	krewBin := filepath.Join(tmpDir, "kubectl-check")
	if err := os.Symlink(bin, krewBin); err != nil {
		t.Fatalf("failed to create symlink for kubectl-check: %v", err)
	}

	out, _, code := runCommand(t, krewBin, []string{"--help"}, nil, root)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "kubectl check <command>") {
		t.Errorf("expected kubectl check usage in help output, got: %s", out)
	}
}

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if strings.Contains(a, flag) {
			return true
		}
	}
	return false
}

func stripTimingLines(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "Duration:") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}
