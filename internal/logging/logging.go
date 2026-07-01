// Package logging configures the process-wide structured logger (log/slog).
//
// The logger is configured from the environment so operators can adjust
// verbosity and output format without code changes:
//
//	LOG_LEVEL  = debug | info | warn | error   (default: info)
//	LOG_FORMAT = text  | json                  (default: text)
//
// This aligns tempestwx-utilities with the log/slog logging pattern used across
// the sibling services (ytdl-api, nextdns-operator, claude-code-api).
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init builds a logger from the LOG_LEVEL and LOG_FORMAT environment variables,
// installs it as the process default via slog.SetDefault, and returns it.
func Init() *slog.Logger {
	logger := New(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))
	slog.SetDefault(logger)
	return logger
}

// New constructs a *slog.Logger for the given level and format strings. Unknown
// or empty values fall back to info level and text format. Output is written to
// stderr. It is exported so tests (and callers wanting a non-default logger) can
// build a logger without touching the global default.
func New(level, format string) *slog.Logger {
	return newLogger(os.Stderr, level, format)
}

// newLogger is the writer-parameterized core of New, split out so tests can
// capture output without touching os.Stderr.
func newLogger(w io.Writer, level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: ParseLevel(level)}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default: // "text", "", or anything unrecognized
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
}

// ParseLevel maps a case-insensitive level name to a slog.Level, defaulting to
// slog.LevelInfo for empty or unrecognized input.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
