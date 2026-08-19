package config

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"audiax/internal/constants"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func NewDatabase(cfg *Config, log *slog.Logger) (*gorm.DB, error) {
	pgConfig := postgres.Config{DSN: cfg.DatabaseURL}

	// Supabase's transaction pooler (port 6543) multiplexes connections and
	// cannot hold server-side prepared statements. pgx uses them by default,
	// which surfaces as sporadic "prepared statement already exists" errors.
	if strings.Contains(cfg.DatabaseURL, constants.SupabaseTransactionPoolerPort) {
		pgConfig.PreferSimpleProtocol = true
		log.Info("supabase transaction pooler detected, disabling prepared statements")
	}

	db, err := gorm.Open(postgres.New(pgConfig), &gorm.Config{
		Logger: gormlogger.New(&slogWriter{log: log}, gormlogger.Config{
			SlowThreshold:             cfg.DBSlowThreshold,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true, // keep credentials and PII out of logs
			Colorful:                  false,
		}),
		NowFunc:                func() time.Time { return time.Now().UTC() },
		SkipDefaultTransaction: true, // we open transactions explicitly where needed
		TranslateError:         true, // surfaces unique violations as gorm.ErrDuplicatedKey
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLife)

	ctx, cancel := context.WithTimeout(context.Background(), constants.DatabasePingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("database connected", "max_open", cfg.DBMaxOpenConns)
	return db, nil
}

type slogWriter struct{ log *slog.Logger }

func (w *slogWriter) Printf(format string, args ...any) {
	w.log.Warn(fmt.Sprintf(format, args...))
}
