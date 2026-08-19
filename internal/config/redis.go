package config

import (
	"context"
	"fmt"
	"log/slog"

	"audiax/internal/constants"

	"github.com/redis/go-redis/v9"
)

// NewRedis expects a rediss:// URL for TLS-only providers such as Upstash.
// ParseURL turns the scheme into the right TLS config, so no manual setup here.
func NewRedis(cfg *Config, log *slog.Logger) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), constants.RedisPingTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	log.Info("redis connected", "addr", opt.Addr)
	return client, nil
}
