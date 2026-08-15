// Package storage opens the connections repositories run their queries on. It
// knows how to reach a backing store and nothing about what is kept there.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresConfig struct {
	DSN             string
	MaxConns        int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// PostgresConnect opens the pool and proves it works before handing it back, so
// an unreachable database fails at boot rather than on the first request.
func PostgresConnect(ctx context.Context, settings PostgresConfig) (*pgxpool.Pool, error) {
	if settings.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}
	config, err := pgxpool.ParseConfig(settings.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}
	if settings.MaxConns > 0 {
		config.MaxConns = int32(settings.MaxConns)
	}
	config.MaxConnLifetime = settings.ConnMaxLifetime
	config.MaxConnIdleTime = settings.ConnMaxIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return pool, nil
}
