package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/config"
)

func TestConfigDefaults(t *testing.T) {
	cfg := config.Defaults()

	if cfg.TargetVersion != "1.31" {
		t.Errorf("expected default TargetVersion 1.31, got %s", cfg.TargetVersion)
	}
	if !cfg.Offline {
		t.Errorf("expected Offline default to be true, got %v", cfg.Offline)
	}
	if cfg.MinSeverity != "info" {
		t.Errorf("expected MinSeverity info, got %s", cfg.MinSeverity)
	}
	if cfg.FailOn != "error" {
		t.Errorf("expected FailOn error, got %s", cfg.FailOn)
	}
	if cfg.MaxFindings != 500 {
		t.Errorf("expected MaxFindings 500, got %d", cfg.MaxFindings)
	}
	if cfg.OutputFormat != "terminal" {
		t.Errorf("expected OutputFormat terminal, got %s", cfg.OutputFormat)
	}
	if !cfg.RedactSecrets {
		t.Errorf("expected RedactSecrets to be true, got %v", cfg.RedactSecrets)
	}
}

func TestApplyEnvironment(t *testing.T) {
	cfg := config.Defaults()

	os.Setenv("KUBECHECK_TARGET_VERSION", "1.28")
	os.Setenv("KUBECHECK_OUTPUT", "json")
	os.Setenv("KUBECHECK_FAIL_ON", "warning")
	os.Setenv("KUBECHECK_MIN_SEVERITY", "warn")
	os.Setenv("KUBECHECK_COLOR", "never")
	os.Setenv("KUBECHECK_OFFLINE", "false")
	os.Setenv("KUBECHECK_MAX_FINDINGS", "1000")
	os.Setenv("KUBECHECK_BUNDLE", "/tmp/bundle.tar.gz")
	defer func() {
		os.Unsetenv("KUBECHECK_TARGET_VERSION")
		os.Unsetenv("KUBECHECK_OUTPUT")
		os.Unsetenv("KUBECHECK_FAIL_ON")
		os.Unsetenv("KUBECHECK_MIN_SEVERITY")
		os.Unsetenv("KUBECHECK_COLOR")
		os.Unsetenv("KUBECHECK_OFFLINE")
		os.Unsetenv("KUBECHECK_MAX_FINDINGS")
		os.Unsetenv("KUBECHECK_BUNDLE")
	}()

	cfg.ApplyEnvironment()

	if cfg.TargetVersion != "1.28" {
		t.Errorf("expected TargetVersion 1.28 from env, got %s", cfg.TargetVersion)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("expected OutputFormat json from env, got %s", cfg.OutputFormat)
	}
	if cfg.FailOn != "warning" {
		t.Errorf("expected FailOn warning from env, got %s", cfg.FailOn)
	}
	if cfg.MinSeverity != "warn" {
		t.Errorf("expected MinSeverity warn from env, got %s", cfg.MinSeverity)
	}
	if cfg.Color != "never" {
		t.Errorf("expected Color never from env, got %s", cfg.Color)
	}
	if cfg.Offline != false {
		t.Errorf("expected Offline false from env, got %v", cfg.Offline)
	}
	if cfg.MaxFindings != 1000 {
		t.Errorf("expected MaxFindings 1000 from env, got %d", cfg.MaxFindings)
	}
	if cfg.BundlePath != "/tmp/bundle.tar.gz" {
		t.Errorf("expected BundlePath from env, got %s", cfg.BundlePath)
	}
}

func TestFindConfigFile(t *testing.T) {
	// Explicit path returns itself
	if p := config.FindConfigFile("/custom/path.yaml"); p != "/custom/path.yaml" {
		t.Errorf("expected /custom/path.yaml, got %s", p)
	}

	// Non-existent path returns empty string
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	if p := config.FindConfigFile(""); p != "" {
		t.Errorf("expected empty string in clean dir, got %s", p)
	}

	// Create local config file
	testConfig := filepath.Join(tmpDir, ".kubecheck.yaml")
	os.WriteFile(testConfig, []byte("target_version: 1.29\n"), 0644)

	if p := config.FindConfigFile(""); p != ".kubecheck.yaml" {
		t.Errorf("expected .kubecheck.yaml to be found, got %s", p)
	}
}
