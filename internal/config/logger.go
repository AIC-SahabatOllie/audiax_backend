package config

import (
	"log/slog"
	"os"
)

func NewLogger(cfg *Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if !cfg.IsProduction() {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	log := slog.New(handler).With("app", cfg.AppName)
	slog.SetDefault(log)
	return log
}
