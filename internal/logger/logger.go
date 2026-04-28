package logger

import (
	"io"
	"log/slog"
	"strings"
)

// New creates a configured slog.Logger.
// format: "json" or "text" (default: "text").
// level: "debug", "info", "warn", "error" (default: "info").
func New(w io.Writer, format, level string) *slog.Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
