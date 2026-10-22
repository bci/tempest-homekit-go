// Package logger provides centralized logging with level filtering for the Tempest HomeKit application.
//
// It wraps the standard library log package with a process-wide level
// ("debug", "info", "warn", "error") and an optional case-insensitive
// substring filter. Level and filter are package globals set once at startup
// by SetLogLevel and SetLogFilter; they are not synchronised, so they should
// not be changed while other goroutines are logging.
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// currentLogLevel is the global log level used for filtering. It defaults to
// "error" until SetLogLevel is called. The logFilter variable that follows
// holds the lower-cased substring a message must contain to be logged; empty
// means no filtering. It is set via SetLogFilter.
var currentLogLevel string = "error"
var logFilter string = "" // Filter string for log messages

// Log level constants accepted by SetLogLevel.
const (
	LogLevelDebug = "debug" // All messages, including Debug; adds file:line to output
	LogLevelInfo  = "info"  // Info, Warn, Error and Alarm messages
	LogLevelWarn  = "warn"  // Warn, Error and Alarm messages
	LogLevelError = "error" // Default; Warn, Error and Alarm are still emitted (see Warn)
)

// SetLogLevel configures the global log level and output settings.
//
// Accepts both 'warn' and 'warning' (normalized to 'warn' internally).
// The level "debug" adds the short file name and line number to every log
// line; "error" and any unrecognised level also direct log output to
// os.Stderr. Unrecognised levels are stored as-is, which disables Debug and
// Info and also Warn, leaving only Error and Alarm.
func SetLogLevel(level string) {
	// Normalize 'warning' to 'warn' for consistency
	if level == "warning" {
		level = "warn"
	}
	currentLogLevel = level
	switch level {
	case "debug":
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	case "info":
		log.SetFlags(log.LstdFlags)
	case "warn":
		log.SetFlags(log.LstdFlags)
	case "error":
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	default:
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	}
}

// SetLogFilter configures the global log filter string.
//
// Only messages containing filter (case-insensitive) will be output. The
// filter applies to every level, including Error and Alarm. An empty filter
// disables filtering.
func SetLogFilter(filter string) {
	logFilter = strings.ToLower(filter)
}

// shouldLog reports whether message passes the substring filter set by
// SetLogFilter. It returns true when no filter is set. It does not check the
// log level; callers do that themselves.
func shouldLog(message string) bool {
	if logFilter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(message), logFilter)
}

// Debug prints debug messages only if log level is debug.
//
// format and v are passed to fmt.Sprintf; the result is prefixed with
// "DEBUG: " and subject to the log filter.
func Debug(format string, v ...interface{}) {
	if currentLogLevel == LogLevelDebug {
		message := fmt.Sprintf(format, v...)
		if shouldLog(message) {
			log.Printf("DEBUG: %s", message)
		}
	}
}

// Info prints info messages only if log level is debug or info.
//
// format and v are passed to fmt.Sprintf; the result is prefixed with
// "INFO: " and subject to the log filter.
func Info(format string, v ...interface{}) {
	if currentLogLevel == LogLevelDebug || currentLogLevel == LogLevelInfo {
		message := fmt.Sprintf(format, v...)
		if shouldLog(message) {
			log.Printf("INFO: %s", message)
		}
	}
}

// Warn prints warning messages if log level is debug, info, warn, or error.
//
// Because the default "error" level is included, warnings are effectively
// always shown unless SetLogLevel was given an unrecognised level. format
// and v are passed to fmt.Sprintf; the result is prefixed with "WARN: " and
// subject to the log filter.
func Warn(format string, v ...interface{}) {
	if currentLogLevel == LogLevelDebug || currentLogLevel == LogLevelInfo || currentLogLevel == LogLevelWarn || currentLogLevel == LogLevelError {
		message := fmt.Sprintf(format, v...)
		if shouldLog(message) {
			log.Printf("WARN: %s", message)
		}
	}
}

// Error always prints error messages, regardless of log level.
//
// format and v are passed to fmt.Sprintf; the result is prefixed with
// "ERROR: ". The log filter still applies.
func Error(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	if shouldLog(message) {
		log.Printf("ERROR: %s", message)
	}
}

// Alarm always prints alarm notifications, bypassing log level filtering.
//
// Alarms are critical events that should always be visible. format and v
// are passed to fmt.Sprintf; the result is prefixed with "🚨 ALARM: ". The
// substring filter set by SetLogFilter still applies.
func Alarm(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	if shouldLog(message) {
		log.Printf("🚨 ALARM: %s", message)
	}
}

// IsDebugEnabled returns true if debug logging is currently enabled, letting
// callers skip building expensive debug output.
func IsDebugEnabled() bool {
	return currentLogLevel == LogLevelDebug
}
