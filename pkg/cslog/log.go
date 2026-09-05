package cslog

import (
	"context"
	"log/slog"
)

const (
	// LogLevelTrace is a very verbose log level for development use
	LogLevelTrace = slog.Level(-8)

	// LogLevelFatal is a log level for fatal errors
	LogLevelFatal = slog.Level(12)
)

// Trace logs a message at the Trace level
// with the given context, message, and optional arguments.
// Example usage:
//
//	cslog.Trace(ctx, "This is a trace message", "key1", "value1")
func Trace(ctx context.Context, msg string, args ...any) {
	slog.Log(ctx, LogLevelTrace, msg, args...)
}

// Fatal logs a message at the Fatal level
// with the given context, message, and optional arguments.
// Example usage:
//
//	cslog.Fatal(ctx, "This is a fatal message", "key1", "value1")
func Fatal(ctx context.Context, msg string, args ...any) {
	slog.Log(ctx, LogLevelFatal, msg, args...)
}
