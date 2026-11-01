// Package config implements configuration loading and the strict precedence hierarchy
// specified in docs/distribution/CLI_UX_AND_EXIT_CODES.md §3.
// Precedence order: CLI Flag > Environment Variable > Local Config > Global Config > Built-in Default.
package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config represents the complete runtime configuration for a KubeCheck invocation.
type Config struct {
	TargetVersion   string   `json:"target_version" yaml:"target_version"`
	TargetPSS       string   `json:"target_pss" yaml:"target_pss"`
	Offline         bool     `json:"offline" yaml:"offline"`
	MinSeverity     string   `json:"min_severity" yaml:"min_severity"`
	FailOn          string   `json:"fail_on" yaml:"fail_on"`
	MaxFindings     int      `json:"max_findings" yaml:"max_findings"`
	Recursive       bool     `json:"recursive" yaml:"recursive"`
	OutputFormat    string   `json:"output_format" yaml:"output_format"`
	Color           string   `json:"color" yaml:"color"`
	ShowRemediation bool     `json:"show_remediation" yaml:"show_remediation"`
	RedactSecrets   bool     `json:"redact_secrets" yaml:"redact_secrets"`
	BundlePath      string   `json:"bundle_path" yaml:"bundle_path"`
	ConfigFile      string   `json:"config_file" yaml:"config_file"`
	IncludeRules    []string `json:"include_rules" yaml:"include_rules"`
	ExcludeRules    []string `json:"exclude_rules" yaml:"exclude_rules"`
	IncludeCats     []string `json:"include_categories" yaml:"include_categories"`
	IgnorePatterns  []string `json:"ignore_patterns" yaml:"ignore_patterns"`
}

// Defaults returns a Config initialized with the normative defaults from CLI_UX_AND_EXIT_CODES.md.
func Defaults() *Config {
	return &Config{
		TargetVersion:   "1.31",
		Offline:         true,
		MinSeverity:     "info",
		FailOn:          "error",
		MaxFindings:     500,
		Recursive:       false,
		OutputFormat:    "terminal",
		Color:           "auto",
		ShowRemediation: true,
		RedactSecrets:   true,
		BundlePath:      "",
		ConfigFile:      "",
		IncludeRules:    nil,
		ExcludeRules:    nil,
		IncludeCats:     nil,
		IgnorePatterns:  nil,
	}
}

// ApplyEnvironment overrides configuration values with defined environment variables.
func (c *Config) ApplyEnvironment() {
	if val := os.Getenv("KUBECHECK_TARGET_VERSION"); val != "" {
		c.TargetVersion = val
	}
	if val := os.Getenv("KUBECHECK_TARGET_PSS"); val != "" {
		c.TargetPSS = val
	}
	if val := os.Getenv("KUBECHECK_OUTPUT"); val != "" {
		c.OutputFormat = val
	}
	if val := os.Getenv("KUBECHECK_MIN_SEVERITY"); val != "" {
		c.MinSeverity = val
	}
	if val := os.Getenv("KUBECHECK_FAIL_ON"); val != "" {
		c.FailOn = val
	}
	if val := os.Getenv("KUBECHECK_COLOR"); val != "" {
		c.Color = val
	}
	if val := os.Getenv("KUBECHECK_OFFLINE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			c.Offline = b
		}
	}
	if val := os.Getenv("KUBECHECK_MAX_FINDINGS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			c.MaxFindings = n
		}
	}
	if val := os.Getenv("KUBECHECK_BUNDLE"); val != "" {
		c.BundlePath = val
	}
}

// FindConfigFile locates the active configuration file, checking local directory first,
// then global user configuration.
func FindConfigFile(explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}
	// 1. Local workspace config
	for _, name := range []string{".kubecheck.yaml", ".kubecheck.yml"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	// 2. Global user config
	if home, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(home, ".config", "kubecheck", "config.yaml")
		if _, err := os.Stat(globalPath); err == nil {
			return globalPath
		}
	}
	return ""
}
