package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

func New(cfg config.LoggerConfig) *slog.Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if strings.ToLower(cfg.Format) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
