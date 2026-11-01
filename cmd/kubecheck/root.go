package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kubecheck/kubecheck/pkg/bundle"
	"github.com/kubecheck/kubecheck/pkg/config"
	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/exitcode"
	"github.com/kubecheck/kubecheck/pkg/graph"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/log"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/report"
	"github.com/kubecheck/kubecheck/pkg/rule"
	"github.com/kubecheck/kubecheck/pkg/schema"
	"github.com/kubecheck/kubecheck/pkg/security"
)

// CLI orchestrates command routing and flag execution for KubeCheck.
type CLI struct {
	Stdout io.Writer
	Stderr io.Writer
	Logger *log.Logger
	Config *config.Config
}

// NewCLI creates a new CLI instance with default streams and configuration.
func NewCLI(stdout, stderr io.Writer) *CLI {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	cfg := config.Defaults()
	cfg.ApplyEnvironment()

	logger := log.New(stderr, log.LevelInfo)
	return &CLI{
		Stdout: stdout,
		Stderr: stderr,
		Logger: logger,
		Config: cfg,
	}
}

// isTerminal checks if an output writer is an interactive character terminal (TTY).
func (c *CLI) isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// CommandName returns "kubectl check" if invoked as a kubectl plugin, or "kubecheck".
func CommandName() string {
	bin := filepath.Base(os.Args[0])
	if strings.Contains(bin, "kubectl-check") {
		return "kubectl check"
	}
	return "kubecheck"
}

// PrintHelp outputs the top-level help text adhering to docs/distribution/CLI_UX_AND_EXIT_CODES.md.
func (c *CLI) PrintHelp(w io.Writer) {
	cmd := CommandName()
	helpText := fmt.Sprintf(`KubeCheck is an offline-first diagnostic engine for Kubernetes manifests.

Usage:
  %s <command> [flags] [arguments]

Available Commands:
  analyze      Analyze Kubernetes manifests for syntax, schema, semantic, and security defects
  version      Display engine version, embedded bundle metadata, and supported Kubernetes APIs
  rules        Inspect and explain diagnostic rules in the active catalog
  bundle       Manage, verify, export, import, or update knowledge bundles
  completion   Generate shell completion scripts (bash, zsh, fish, powershell)

Global Flags:
      --config string           Path to configuration file (default search: ./.kubecheck.yaml)
      --offline                 Enforce strict network isolation (default: true)
  -v, --verbose                 Enable verbose trace logging
  -q, --quiet                   Suppress informative banners; emit only findings
  -h, --help                    Help for %s

Examples:
  # Analyze a single manifest
  %s analyze manifest.yaml

  # Recursively analyze a directory against Kubernetes v1.29
  %s analyze -R ./deployments/ --target-version 1.29

  # Validate manifests from stdin
  cat manifest.yaml | %s analyze -

Use "%s <command> --help" for more information about a command.
`, cmd, cmd, cmd, cmd, cmd, cmd)

	fmt.Fprint(w, helpText)
}

// Execute parses command-line arguments and dispatches to the appropriate subcommand handler.
func (c *CLI) Execute(args []string) exitcode.ExitCode {
	if len(args) == 0 {
		c.PrintHelp(c.Stdout)
		return exitcode.Success
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "-h", "--help", "help":
		c.PrintHelp(c.Stdout)
		return exitcode.Success

	case "-v", "--version", "version":
		PrintVersion(c.Stdout)
		return exitcode.Success

	case "analyze":
		return c.runAnalyze(subArgs)

	case "rules":
		return c.runRules(subArgs)

	case "bundle":
		return c.runBundle(subArgs)

	case "completion":
		return c.runCompletion(subArgs)

	default:
		if strings.HasPrefix(subcommand, "-") {
			// Check if flag was passed to root (e.g. `kubecheck --help`)
			c.PrintHelp(c.Stdout)
			return exitcode.UsageError
		}
		fmt.Fprintf(c.Stderr, "error: unknown command %q for %q\nRun '%s --help' for usage.\n",
			subcommand, CommandName(), CommandName())
		return exitcode.UsageError
	}
}

func (c *CLI) runAnalyze(args []string) exitcode.ExitCode {
	startTime := time.Now()
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)

	var (
		files       string
		recursive   bool
		targetVer   string
		targetPSS   string
		outputFmt   string
		failOn      string
		minSeverity string
		offline     bool
		colorMode   string
		maxFindings int
		bundlePath  string
	)

	fs.StringVar(&files, "f", "", "Manifest file or '-' for stdin")
	fs.StringVar(&files, "file", "", "Manifest file or '-' for stdin")
	fs.BoolVar(&recursive, "R", c.Config.Recursive, "Recursively scan directories")
	fs.BoolVar(&recursive, "recursive", c.Config.Recursive, "Recursively scan directories")
	fs.StringVar(&targetVer, "target-version", c.Config.TargetVersion, "Target Kubernetes version")
	fs.StringVar(&targetPSS, "target-pss", c.Config.TargetPSS, "Target Pod Security Standards profile: privileged, baseline, restricted")
	fs.StringVar(&targetPSS, "pss", c.Config.TargetPSS, "Target Pod Security Standards profile: privileged, baseline, restricted")
	fs.StringVar(&outputFmt, "o", c.Config.OutputFormat, "Output format: terminal, json, sarif, junit, compact")
	fs.StringVar(&outputFmt, "output", c.Config.OutputFormat, "Output format: terminal, json, sarif, junit, compact")
	fs.StringVar(&failOn, "fail-on", c.Config.FailOn, "Failure threshold triggering exit code 1: error, warning, info")
	fs.StringVar(&minSeverity, "min-severity", c.Config.MinSeverity, "Minimum severity to display: error, warning, info")
	fs.BoolVar(&offline, "offline", c.Config.Offline, "Enforce strict offline isolation")
	fs.StringVar(&colorMode, "color", c.Config.Color, "ANSI color: auto, always, never")
	fs.IntVar(&maxFindings, "max-findings", c.Config.MaxFindings, "Maximum findings to output before truncation")
	fs.StringVar(&bundlePath, "bundle", c.Config.BundlePath, "Path to local knowledge bundle directory")

	var flagArgs, inputPaths []string
	isFlagWithValue := map[string]bool{
		"-f": true, "--file": true,
		"-o": true, "--output": true,
		"--target-version": true,
		"--target-pss":     true,
		"--pss":            true,
		"--fail-on":        true,
		"--min-severity":   true,
		"--color":          true,
		"--max-findings":   true,
		"--config":         true,
		"--bundle":         true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.SplitN(arg, "=", 2)[0]
			if isFlagWithValue[flagName] && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			inputPaths = append(inputPaths, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return exitcode.UsageError
	}
	inputPaths = append(inputPaths, fs.Args()...)

	if files != "" {
		inputPaths = append(inputPaths, files)
	}

	if len(inputPaths) == 0 {
		fmt.Fprintf(c.Stderr, "error: no input manifest files or directories specified.\nRun '%s analyze --help' for usage.\n", CommandName())
		return exitcode.UsageError
	}

	// Update active configuration with parsed CLI flags
	c.Config.TargetVersion = targetVer
	c.Config.TargetPSS = targetPSS
	c.Config.OutputFormat = outputFmt
	c.Config.FailOn = failOn
	c.Config.MinSeverity = minSeverity
	c.Config.Offline = offline
	c.Config.Color = colorMode
	c.Config.MaxFindings = maxFindings
	c.Config.Recursive = recursive

	// Resolve active knowledge bundle
	bundleValidator := bundle.NewValidator(EngineVersion, nil)
	bundleStore := bundle.NewStore("", bundleValidator)
	bundleResolver := bundle.NewResolver(bundleStore, bundleValidator)
	activeBundle, _, bErr := bundleResolver.Resolve(bundlePath)
	if bErr != nil {
		fmt.Fprintf(c.Stderr, "bundle error: %v\n", bErr)
		return exitcode.BundleError
	}

	resolvedFiles, err := parser.DiscoverFiles(inputPaths, recursive)
	if err != nil {
		fmt.Fprintf(c.Stderr, "error: %v\n", err)
		return exitcode.InputError
	}
	if len(resolvedFiles) == 0 {
		fmt.Fprintf(c.Stderr, "error: no YAML manifest files found in specified path(s)\n")
		return exitcode.InputError
	}

	p := parser.Default()
	var allDocs []*nir.Document
	var canonicalFindings []diagnostic.Finding
	hasInputError := false

	for _, file := range resolvedFiles {
		var docs []*nir.Document
		var findings []parser.Finding
		var pErr error

		if file == "-" {
			docs, findings, pErr = p.ParseReader("<stdin>", os.Stdin)
		} else {
			docs, findings, pErr = p.ParseFile(file)
		}

		if pErr != nil {
			hasInputError = true
		}
		allDocs = append(allDocs, docs...)
		for _, f := range findings {
			canonicalFindings = append(canonicalFindings, diagnostic.FromParser(f))
		}
	}

	// Extract Resource Inventory (GVKNN & root object rules)
	inv, invFindings := inventory.Extract(allDocs)
	for _, f := range invFindings {
		canonicalFindings = append(canonicalFindings, diagnostic.FromInventory(f))
	}

	// Validate resources against local OpenAPI schemas & API lifecycle rules
	schemaValidator := schema.NewValidator(nil)
	for _, res := range inv.Resources {
		schemaFindings := schemaValidator.Validate(res, targetVer)
		for _, f := range schemaFindings {
			canonicalFindings = append(canonicalFindings, diagnostic.FromSchema(f))
		}
	}

	// Evaluate declarative diagnostic rules (workload, security, network)
	ruleEngine := rule.DefaultEngine()
	for _, r := range security.Rules() {
		ruleEngine.Register(r)
	}
	ruleFindings := ruleEngine.Evaluate(inv)
	canonicalFindings = append(canonicalFindings, ruleFindings...)

	// Evaluate Pod Security Standards (PSS)
	pssEvaluator := security.NewPSSEvaluator(security.PSSProfile(targetPSS))
	nsProfiles := make(map[string]security.PSSProfile)
	for _, res := range inv.Resources {
		if res.ID.Kind == "Namespace" && res.Labels != nil {
			if enforce, ok := res.Labels["pod-security.kubernetes.io/enforce"]; ok {
				nsProfiles[res.ID.Name] = security.PSSProfile(enforce)
			}
		}
	}
	for _, res := range inv.Resources {
		profile := security.PSSProfile(targetPSS)
		if p, ok := nsProfiles[res.ID.Namespace]; ok && p != "" {
			profile = p
		}
		if profile != "" {
			pssFindings := pssEvaluator.Evaluate(res, profile)
			canonicalFindings = append(canonicalFindings, pssFindings...)
		}
	}

	// Build relationship graph and evaluate cross-resource rules
	relGraph := graph.NewBuilder(inv).Build()
	graphFindings := graph.Analyze(relGraph)
	canonicalFindings = append(canonicalFindings, graphFindings...)

	// Deterministically deduplicate and sort all canonical findings
	canonicalFindings = diagnostic.DeduplicateAndSort(canonicalFindings)

	duration := time.Since(startTime)
	errorCount := 0
	warningCount := 0
	infoCount := 0

	for _, f := range canonicalFindings {
		switch f.Severity {
		case diagnostic.SeverityError:
			errorCount++
		case diagnostic.SeverityWarning:
			warningCount++
		default:
			infoCount++
		}
	}

	passed := true
	if errorCount > 0 {
		passed = false
	} else if strings.EqualFold(failOn, "warning") && warningCount > 0 {
		passed = false
	} else if strings.EqualFold(failOn, "info") && (warningCount > 0 || infoCount > 0) {
		passed = false
	}

	sum := report.Summary{
		TargetVersion:   targetVer,
		EngineVersion:   EngineVersion,
		BundleID:        activeBundle.BundleID,
		BundleVersion:   activeBundle.BundleVersion,
		Offline:         offline,
		Timestamp:       startTime.UTC(),
		Duration:        duration,
		TotalFiles:      len(resolvedFiles),
		TotalDocuments:  len(allDocs),
		TotalResources:  len(inv.Resources),
		ErrorCount:      errorCount,
		WarningCount:    warningCount,
		InfoCount:       infoCount,
		Passed:          passed,
		FailOnThreshold: failOn,
	}

	reportOpts := report.Options{
		Format:          outputFmt,
		ColorMode:       colorMode,
		MinSeverity:     minSeverity,
		MaxFindings:     maxFindings,
		ShowRemediation: c.Config.ShowRemediation,
		RedactSecrets:   c.Config.RedactSecrets,
		IsTerminal:      c.isTerminal(c.Stdout),
	}

	if err := report.Render(c.Stdout, sum, canonicalFindings, reportOpts); err != nil {
		fmt.Fprintf(c.Stderr, "report error: %v\n", err)
		return exitcode.UsageError
	}

	if hasInputError {
		return exitcode.InputError
	}

	if !passed {
		return exitcode.FindingsDetected
	}
	return exitcode.Success
}

func (c *CLI) runRules(args []string) exitcode.ExitCode {
	fmt.Fprintf(c.Stdout, "Active Diagnostic Rules: catalog loaded from default knowledge bundle v%s\n", BundleVersion)
	ruleEngine := rule.DefaultEngine()
	for _, r := range security.Rules() {
		ruleEngine.Register(r)
	}
	for _, r := range ruleEngine.Rules() {
		fmt.Fprintf(c.Stdout, "  %-18s [%s] %s (%s)\n", r.ID, r.Severity, r.Title, r.Category)
	}
	return exitcode.Success
}

func (c *CLI) runBundle(args []string) exitcode.ExitCode {
	bundleValidator := bundle.NewValidator(EngineVersion, nil)
	bundleStore := bundle.NewStore("", bundleValidator)
	bundleResolver := bundle.NewResolver(bundleStore, bundleValidator)

	if len(args) == 0 || args[0] == "status" || args[0] == "info" {
		m, source, err := bundleResolver.Resolve("")
		if err != nil {
			fmt.Fprintf(c.Stderr, "error: %v\n", err)
			return exitcode.BundleError
		}
		fmt.Fprintf(c.Stdout, "Knowledge Bundle Status:\n")
		fmt.Fprintf(c.Stdout, "  ID:           %s\n", m.BundleID)
		fmt.Fprintf(c.Stdout, "  Version:      %d\n", m.BundleVersion)
		fmt.Fprintf(c.Stdout, "  Format:       %s\n", m.FormatVersion)
		fmt.Fprintf(c.Stdout, "  Source:       %s\n", source)
		fmt.Fprintf(c.Stdout, "  Fresh Until:  %s\n", m.Generation.FreshUntil)
		fmt.Fprintf(c.Stdout, "  Licenses:     %s\n", strings.Join(m.Licenses, ", "))
		return exitcode.Success
	}

	sub := args[0]
	switch sub {
	case "verify":
		if len(args) < 2 {
			fmt.Fprintf(c.Stderr, "error: bundle directory required for verify\nUsage: %s bundle verify <dir>\nNote: performs both SHA-256 checksum comparison and Ed25519 signature authentication.\n", CommandName())
			return exitcode.UsageError
		}
		targetDir := args[1]
		manifest, report, err := bundleValidator.ValidateDir(targetDir, 0)
		if report != nil {
			fmt.Fprint(c.Stdout, report.String())
		}
		if err != nil || manifest == nil {
			fmt.Fprintf(c.Stderr, "verification failed: %v\n", err)
			return exitcode.BundleError
		}
		fmt.Fprintf(c.Stdout, "Bundle verification passed successfully.\n")
		return exitcode.Success

	case "activate", "import":
		var targetDir string
		if len(args) >= 3 && args[1] == "--from-dir" {
			targetDir = args[2]
		} else if len(args) >= 2 {
			targetDir = args[1]
		} else {
			fmt.Fprintf(c.Stderr, "error: bundle directory required for activate\nUsage: %s bundle activate <dir>\n", CommandName())
			return exitcode.UsageError
		}

		manifest, report, err := bundleStore.Activate(targetDir)
		if report != nil {
			fmt.Fprint(c.Stdout, report.String())
		}
		if err != nil {
			fmt.Fprintf(c.Stderr, "activation failed: %v\n", err)
			return exitcode.BundleError
		}
		fmt.Fprintf(c.Stdout, "Bundle %s (v%d) activated successfully in local store.\n", manifest.BundleID, manifest.BundleVersion)
		return exitcode.Success

	case "rollback":
		manifest, err := bundleStore.Rollback()
		if err != nil {
			fmt.Fprintf(c.Stderr, "rollback failed: %v\n", err)
			return exitcode.BundleError
		}
		fmt.Fprintf(c.Stdout, "Successfully rolled back to bundle %s (v%d)\n", manifest.BundleID, manifest.BundleVersion)
		return exitcode.Success

	case "list":
		fmt.Fprintf(c.Stdout, "Installed Knowledge Bundles:\n")
		if active, err := bundleStore.GetActiveManifest(); err == nil && active != nil {
			fmt.Fprintf(c.Stdout, "  * [ACTIVE]   %s (v%d)\n", active.BundleID, active.BundleVersion)
		} else {
			fmt.Fprintf(c.Stdout, "  * [ACTIVE]   %s (v%d) [Embedded Fallback]\n", bundle.DefaultEmbeddedBundleID, bundle.DefaultEmbeddedBundleVersion)
		}
		if prevDir := bundleStore.PreviousBundleDir(); prevDir != "" {
			if prevM, _, err := bundleValidator.ValidateDir(prevDir, 0); err == nil && prevM != nil {
				fmt.Fprintf(c.Stdout, "    [PREVIOUS] %s (v%d)\n", prevM.BundleID, prevM.BundleVersion)
			}
		}
		return exitcode.Success

	default:
		fmt.Fprintf(c.Stderr, "error: unknown bundle subcommand %q. Valid subcommands: status, verify, activate, rollback, list\n", sub)
		return exitcode.UsageError
	}
}

func (c *CLI) runCompletion(args []string) exitcode.ExitCode {
	if len(args) == 0 {
		fmt.Fprintf(c.Stderr, "error: shell type required (bash, zsh, fish, powershell)\n")
		return exitcode.UsageError
	}
	fmt.Fprintf(c.Stdout, "# %s completion for %s (stub)\n", args[0], CommandName())
	return exitcode.Success
}
