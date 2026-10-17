package logger

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// captureLogOutput is a helper to capture log output while running f().
// It redirects the standard logger to a buffer for the duration of f,
// restores os.Stderr afterwards, and returns everything written.
func captureLogOutput(f func()) string {
	var buf bytes.Buffer
	// set log output to buffer
	log.SetOutput(&buf)
	f()
	// restore to stderr to be safe for other tests
	log.SetOutput(os.Stderr)
	return buf.String()
}

// TestSetLogFilterAndShouldLogBehavior checks that a filter suppresses messages not containing the filter string.
func TestSetLogFilterAndShouldLogBehavior(t *testing.T) {
	// Ensure clean state
	SetLogFilter("")
	SetLogLevel(LogLevelDebug)

	SetLogFilter("needle")

	out := captureLogOutput(func() {
		Debug("this has needle in text")
		Debug("this does not")
	})

	if !strings.Contains(out, "DEBUG: this has needle in text") {
		t.Fatalf("expected debug message with filter to be logged, got: %q", out)
	}
	if strings.Contains(out, "DEBUG: this does not") {
		t.Fatalf("did not expect non-matching debug message to be logged, got: %q", out)
	}

	// cleanup
	SetLogFilter("")
}

// TestInfoAndDebugLevelFiltering checks that Debug is suppressed at info level and both Debug and Info log at debug level.
func TestInfoAndDebugLevelFiltering(t *testing.T) {
	SetLogFilter("")

	// At info level, Debug should not log, Info should
	SetLogLevel(LogLevelInfo)
	out := captureLogOutput(func() {
		Debug("debug-msg")
		Info("info-msg")
	})
	if strings.Contains(out, "DEBUG: debug-msg") {
		t.Fatalf("did not expect DEBUG to log at info level, got: %q", out)
	}
	if !strings.Contains(out, "INFO: info-msg") {
		t.Fatalf("expected INFO to log at info level, got: %q", out)
	}

	// At debug level, both should log
	SetLogLevel(LogLevelDebug)
	out2 := captureLogOutput(func() {
		Debug("debug-msg")
		Info("info-msg")
	})
	if !strings.Contains(out2, "DEBUG: debug-msg") || !strings.Contains(out2, "INFO: info-msg") {
		t.Fatalf("expected both DEBUG and INFO to log at debug level, got: %q", out2)
	}

	// restore
	SetLogLevel(LogLevelError)
}

// TestWarningAliasNormalization checks that level "warning" behaves as "warn": Warn logs, Info does not.
func TestWarningAliasNormalization(t *testing.T) {
	SetLogFilter("")

	// Test that 'warning' is accepted and normalized to 'warn'
	SetLogLevel("warning")
	out := captureLogOutput(func() {
		Warn("test-warning")
		Info("test-info")
	})

	// Should show warn messages at 'warning' level (normalized to 'warn')
	if !strings.Contains(out, "WARN: test-warning") {
		t.Fatalf("expected WARN to log when level set to 'warning', got: %q", out)
	}
	// Should not show info at warn level
	if strings.Contains(out, "INFO: test-info") {
		t.Fatalf("did not expect INFO to log when level set to 'warning', got: %q", out)
	}

	// restore
	SetLogLevel(LogLevelError)
}

// TestErrorAlwaysLogs checks that Error output appears at the default error level.
func TestErrorAlwaysLogs(t *testing.T) {
	SetLogFilter("")
	SetLogLevel(LogLevelError)

	out := captureLogOutput(func() {
		Error("fatal-error")
	})
	if !strings.Contains(out, "ERROR: fatal-error") {
		t.Fatalf("expected Error to always log, got: %q", out)
	}
}

// TestWarnLogLevel checks which of Error, Warn and Info are emitted at the error, warn and info levels.
func TestWarnLogLevel(t *testing.T) {
	SetLogFilter("")

	// At error level, Warn should log
	SetLogLevel(LogLevelError)
	out := captureLogOutput(func() {
		Warn("warning-message")
		Info("info-message")
	})
	if !strings.Contains(out, "WARN: warning-message") {
		t.Fatalf("expected WARN to log at error level, got: %q", out)
	}
	if strings.Contains(out, "INFO: info-message") {
		t.Fatalf("did not expect INFO to log at error level, got: %q", out)
	}

	// At warn level, both Warn and Error should log, but not Info
	SetLogLevel(LogLevelWarn)
	out2 := captureLogOutput(func() {
		Error("error-message")
		Warn("warning-message")
		Info("info-message")
	})
	if !strings.Contains(out2, "ERROR: error-message") {
		t.Fatalf("expected ERROR to log at warn level, got: %q", out2)
	}
	if !strings.Contains(out2, "WARN: warning-message") {
		t.Fatalf("expected WARN to log at warn level, got: %q", out2)
	}
	if strings.Contains(out2, "INFO: info-message") {
		t.Fatalf("did not expect INFO to log at warn level, got: %q", out2)
	}

	// At info level, Warn, Info, and Error should log
	SetLogLevel(LogLevelInfo)
	out3 := captureLogOutput(func() {
		Error("error-message")
		Warn("warning-message")
		Info("info-message")
	})
	if !strings.Contains(out3, "ERROR: error-message") {
		t.Fatalf("expected ERROR to log at info level, got: %q", out3)
	}
	if !strings.Contains(out3, "WARN: warning-message") {
		t.Fatalf("expected WARN to log at info level, got: %q", out3)
	}
	if !strings.Contains(out3, "INFO: info-message") {
		t.Fatalf("expected INFO to log at info level, got: %q", out3)
	}

	// restore
	SetLogLevel(LogLevelError)
}
