// Package logger builds a slog.Logger from the application config.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

// Options configure how the logger is built.
type Options struct {
	AppEnv   string
	LogLevel string
}

// New returns a slog.Logger writing to w. Production uses JSON; otherwise text.
func New(w io.Writer, opts Options) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(opts.LogLevel))); err != nil {
		level = slog.LevelInfo
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if opts.AppEnv == "production" {
		handler = slog.NewJSONHandler(w, handlerOpts)
	} else {
		handler = slog.NewTextHandler(w, handlerOpts)
	}
	return slog.New(handler)
}
