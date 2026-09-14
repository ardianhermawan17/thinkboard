// Package postgres constructs the pgx pool against Supabase Postgres (03-backend-folder-architecture.md §7.9).
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"thinkboard-backend/internal/platform/config"
)

// NewPool builds a pool sized per cfg, with the settings §7.9 calls REQUIRED under transaction
// pooling because server-side prepared statements break: DefaultQueryExecMode = Exec and
// StatementCacheCapacity = 0. Applied unconditionally so local direct-connect and pooled
// production share one code path.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	poolCfg.ConnConfig.StatementCacheCapacity = 0

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return pool, nil
}
