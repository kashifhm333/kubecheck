// Package log provides a secret-safe, stderr-directed logging sink for KubeCheck.
// It adheres to SARCH-REQ-007, SARCH-REQ-008, and PRIVACY-REQ-018:
// 1. All logs flow exclusively to os.Stderr; os.Stdout is reserved for analysis reports.
// 2. Sensitive keywords and token patterns are masked before emission.
package log

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Level defines log verbosity levels.
type Level int

const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
	LevelTrace
)

func (l Level) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarn:
		return "WARN"
	case LevelInfo:
		return "INFO"
	case LevelDebug:
		return "DEBUG"
	case LevelTrace:
		return "TRACE"
	default:
		return "UNKNOWN"
	}
}

// Logger is a concurrency-safe logger that enforces secret redaction and writes to stderr.
type Logger struct {
	mu            sync.Mutex
	out           io.Writer
	minLevel      Level
	redactionList []*regexp.Regexp
}

// Default sensitive token patterns to redact from log streams.
var defaultSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|secret|token|apikey|private_key|authorization)\s*[:=]\s*["']?([^"'\s]+)["']?`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`),
}

// New creates a new Logger writing to the specified writer at minLevel.
func New(out io.Writer, minLevel Level) *Logger {
	if out == nil {
		out = os.Stderr
	}
	return &Logger{
		out:           out,
		minLevel:      minLevel,
		redactionList: defaultSensitivePatterns,
	}
}

// Default creates a standard production logger writing to os.Stderr at LevelInfo.
func Default() *Logger {
	return New(os.Stderr, LevelInfo)
}

// SetLevel dynamically adjusts the logging verbosity.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// Redact masks sensitive tokens matching the redaction pattern set.
func (l *Logger) Redact(msg string) string {
	redacted := msg
	for _, re := range l.redactionList {
		redacted = re.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.Contains(match, ":") {
				parts := strings.SplitN(match, ":", 2)
				return parts[0] + ": [REDACTED]"
			}
			if strings.Contains(match, "=") {
				parts := strings.SplitN(match, "=", 2)
				return parts[0] + "=[REDACTED]"
			}
			return "[REDACTED_KEY_MATERIAL]"
		})
	}
	return redacted
}

func (l *Logger) log(level Level, format string, args ...any) {
	if level > l.minLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	raw := fmt.Sprintf(format, args...)
	safe := l.Redact(raw)
	fmt.Fprintf(l.out, "[kubecheck] [%s] %s\n", level.String(), safe)
}

func (l *Logger) Error(format string, args ...any) { l.log(LevelError, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, format, args...) }
func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, format, args...) }
func (l *Logger) Trace(format string, args ...any) { l.log(LevelTrace, format, args...) }
