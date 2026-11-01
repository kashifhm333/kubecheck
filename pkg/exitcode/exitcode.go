// Package exitcode defines the normative, frozen process exit codes for KubeCheck.
// These codes implement the authoritative exit-code contract specified in
// docs/distribution/CLI_UX_AND_EXIT_CODES.md §8 and SYSTEM_ARCHITECTURE.md §9.2.
package exitcode

import "fmt"

// ExitCode represents a numeric process exit code returned by KubeCheck.
type ExitCode int

const (
	// Success indicates that analysis completed cleanly with zero findings at or
	// above the configured failure threshold (--fail-on).
	Success ExitCode = 0

	// FindingsDetected indicates that one or more diagnostic findings were detected
	// at or above the failure threshold (by default, 1 or more Error findings).
	FindingsDetected ExitCode = 1

	// UsageError indicates a user invocation error: invalid CLI arguments, unknown flags,
	// mutually exclusive flags, or a syntax error in the configuration file.
	UsageError ExitCode = 2

	// InputError indicates an input or file system resolution failure: file not found,
	// unreadable directory, permission denied, or invalid UTF-8 byte stream.
	InputError ExitCode = 3

	// OperationalError indicates an internal engine fault: memory governor tripped,
	// alias bomb depth limit exceeded, or unhandled runtime panic.
	OperationalError ExitCode = 4

	// PartialScan indicates an incomplete analysis: execution interrupted by SIGINT/SIGTERM,
	// or unrecoverable parse failure in a multi-document stream while other documents succeeded.
	PartialScan ExitCode = 5

	// BundleError indicates a knowledge bundle or cryptographic failure: missing bundle,
	// invalid signature, corrupted archive, or anti-rollback sequence violation.
	BundleError ExitCode = 6
)

// String returns the canonical human-readable label for the exit code.
func (c ExitCode) String() string {
	switch c {
	case Success:
		return "SUCCESS"
	case FindingsDetected:
		return "FINDINGS_DETECTED"
	case UsageError:
		return "USAGE_ERROR"
	case InputError:
		return "INPUT_ERROR"
	case OperationalError:
		return "OPERATIONAL_ERROR"
	case PartialScan:
		return "PARTIAL_SCAN"
	case BundleError:
		return "BUNDLE_ERROR"
	default:
		return fmt.Sprintf("UNKNOWN_EXIT_CODE(%d)", int(c))
	}
}

// Int returns the raw integer exit code for os.Exit().
func (c ExitCode) Int() int {
	return int(c)
}
