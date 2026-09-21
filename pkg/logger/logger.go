// Package logger provides a shared structured logger setup based on log/slog.
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Options configures the logger. Empty values fall back to sensible defaults.
type Options struct {
	// Level is one of debug, info, warn, error. Defaults to info.
	Level string
	// Format is one of json or text. Defaults to json.
	Format string
	// AddSource enables reporting of the source code location in records.
	AddSource bool
}

// New builds a *slog.Logger writing to stderr.
func New(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	}

	var handler slog.Handler
	switch strings.ToLower(orDefault(opts.Format, "json")) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	case "text":
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	default:
		return nil, fmt.Errorf("logger: unknown format %q, want json or text", opts.Format)
	}

	return slog.New(handler), nil
}

// MustNew is a convenience wrapper for process startup.
func MustNew(opts Options) *slog.Logger {
	log, err := New(opts)
	if err != nil {
		panic(err)
	}
	return log
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(orDefault(raw, "info")) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logger: unknown level %q, want debug, info, warn or error", raw)
	}
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
