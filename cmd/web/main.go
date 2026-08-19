package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"audiax/internal/app"
	"audiax/internal/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := config.NewLogger(cfg)

	db, err := config.NewDatabase(cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	redisClient, err := config.NewRedis(cfg, log)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	server := app.New(app.Dependencies{Config: cfg, Log: log, DB: db, Redis: redisClient})

	// SIGKILL is deliberately absent: it cannot be caught.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "port", cfg.Port, "env", cfg.Env)
		serverErr <- server.Listen(fmt.Sprintf(":%d", cfg.Port))
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received, draining connections", "timeout", cfg.ShutdownTimeout)
		// Lets in-flight requests finish instead of cutting them mid-response.
		if err := server.ShutdownWithTimeout(cfg.ShutdownTimeout); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		log.Info("shutdown complete")
		return nil
	}
}
