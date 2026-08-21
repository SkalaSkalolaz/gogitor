package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gogitor/internal/config"
)

func Init(cfg *config.Config) (*slog.Logger, string, error) {
	logDir := filepath.Join(config.Dir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return slog.Default(), "", err
	}

	logPath := filepath.Join(logDir, "gogitor_"+time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.Default(), logPath, err
	}

	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	if cfg.Debug {
		level = slog.LevelDebug
	}

	writers := []io.Writer{f}
	if cfg.Debug || strings.ToLower(cfg.LogLevel) == "debug" {
		writers = append(writers, os.Stdout)
	}

	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler), logPath, nil
}