package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"audiax/internal/constants"

	"github.com/joho/godotenv"
)

// Config holds every tunable the app reads, resolved once at startup.
// Reading env vars in exactly one place means a typo fails loudly here
// instead of silently becoming a zero value somewhere deep in the app.
type Config struct {
	AppName string
	Env     string
	Port    int

	LogLevel slog.Level

	DatabaseURL     string
	DBMaxIdleConns  int
	DBMaxOpenConns  int
	DBConnMaxLife   time.Duration
	DBSlowThreshold time.Duration

	RedisURL string

	SessionTTL time.Duration
	BcryptCost int

	ShutdownTimeout time.Duration
	CORSOrigins     string
}

func Load() (*Config, error) {
	// .env is for local development only; real deployments inject env vars directly.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	cfg := &Config{
		AppName: env("APP_NAME", constants.DefaultAppName),
		Env:     env("APP_ENV", constants.DefaultAppEnv),
		Port:    envInt("PORT", constants.DefaultPort),

		LogLevel: logLevel(env("LOG_LEVEL", constants.DefaultLogLevel)),

		DatabaseURL:     os.Getenv("DATABASE_URL"),
		DBMaxIdleConns:  envInt("DB_MAX_IDLE_CONNS", constants.DefaultDBMaxIdleConns),
		DBMaxOpenConns:  envInt("DB_MAX_OPEN_CONNS", constants.DefaultDBMaxOpenConns),
		DBConnMaxLife:   envDuration("DB_CONN_MAX_LIFETIME", constants.DefaultDBConnMaxLife),
		DBSlowThreshold: envDuration("DB_SLOW_THRESHOLD", constants.DefaultDBSlowThreshold),

		RedisURL: os.Getenv("REDIS_URL"),

		SessionTTL: envDuration("SESSION_TTL", constants.DefaultSessionTTL),
		BcryptCost: envInt("BCRYPT_COST", constants.DefaultBcryptCost),

		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", constants.DefaultShutdownTimeout),
		CORSOrigins:     env("CORS_ORIGINS", constants.DefaultCORSOrigins),
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, errors.New("REDIS_URL is required")
	}
	return cfg, nil
}

func (c *Config) IsProduction() bool { return c.Env == constants.EnvProduction }

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return v
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(env(key, ""))
	if err != nil {
		return fallback
	}
	return v
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}
