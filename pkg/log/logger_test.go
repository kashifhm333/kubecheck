package log_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/log"
)

func TestLoggerLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, log.LevelWarn)

	logger.Info("this is info and should not appear")
	if buf.Len() > 0 {
		t.Errorf("expected no info log output, got: %s", buf.String())
	}

	logger.Warn("this is a warning")
	if !strings.Contains(buf.String(), "[WARN] this is a warning") {
		t.Errorf("expected warning log, got: %s", buf.String())
	}

	buf.Reset()
	logger.Error("this is an error")
	if !strings.Contains(buf.String(), "[ERROR] this is an error") {
		t.Errorf("expected error log, got: %s", buf.String())
	}

	// Test SetLevel
	logger.SetLevel(log.LevelTrace)
	buf.Reset()
	logger.Trace("this is trace")
	if !strings.Contains(buf.String(), "[TRACE] this is trace") {
		t.Errorf("expected trace log, got: %s", buf.String())
	}

	buf.Reset()
	logger.Info("this is now visible info")
	if !strings.Contains(buf.String(), "[INFO] this is now visible info") {
		t.Errorf("expected info log, got: %s", buf.String())
	}
}

func TestLoggerDefaultsAndConstructors(t *testing.T) {
	def := log.Default()
	if def == nil {
		t.Fatal("expected non-nil default logger")
	}

	nilOut := log.New(nil, log.LevelInfo)
	if nilOut == nil {
		t.Fatal("expected non-nil logger with nil writer fallback")
	}
}

func TestLevelString(t *testing.T) {
	cases := []struct {
		lvl      log.Level
		expected string
	}{
		{log.LevelError, "ERROR"},
		{log.LevelWarn, "WARN"},
		{log.LevelInfo, "INFO"},
		{log.LevelDebug, "DEBUG"},
		{log.LevelTrace, "TRACE"},
		{log.Level(99), "UNKNOWN"},
	}

	for _, c := range cases {
		if c.lvl.String() != c.expected {
			t.Errorf("expected Level(%d).String() == %q, got %q", c.lvl, c.expected, c.lvl.String())
		}
	}
}

func TestLoggerSecretRedaction(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, log.LevelDebug)

	logger.Debug("Connecting with password: mySuperSecretPassword123")
	output := buf.String()

	if strings.Contains(output, "mySuperSecretPassword123") {
		t.Errorf("security violation: raw password leaked into log: %s", output)
	}
	if !strings.Contains(output, "password: [REDACTED]") {
		t.Errorf("expected redacted password, got: %s", output)
	}

	buf.Reset()
	logger.Debug("Using API token=ghp_secretTokenABCXYZ123")
	output = buf.String()
	if strings.Contains(output, "ghp_secretTokenABCXYZ123") {
		t.Errorf("security violation: raw token leaked into log: %s", output)
	}
	if !strings.Contains(output, "token=[REDACTED]") {
		t.Errorf("expected redacted token, got: %s", output)
	}

	// Test private key material redaction
	buf.Reset()
	logger.Debug("Key: -----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0\n-----END RSA PRIVATE KEY-----")
	output = buf.String()
	if strings.Contains(output, "MIIEowIBAAKCAQEA0") {
		t.Errorf("security violation: private key material leaked: %s", output)
	}
	if !strings.Contains(output, "[REDACTED_KEY_MATERIAL]") {
		t.Errorf("expected [REDACTED_KEY_MATERIAL], got: %s", output)
	}
}
