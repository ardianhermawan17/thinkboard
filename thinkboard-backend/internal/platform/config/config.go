// Package config holds every bound and timeout from 03-backend-folder-architecture.md §7, read once at startup instead of scattered literals.
package config

import (
	"os"
	"time"
)

// Config is the gateway's runtime configuration.
type Config struct {
	Port            string
	GinMode         string
	ShutdownTimeout time.Duration

	// SupabaseURL is the project's base URL; JWKS is fetched from
	// {SupabaseURL}/auth/v1/.well-known/jwks.json (§5.6 jwt.go).
	SupabaseURL string

	// DatabaseURL is a pgx-compatible connection string for the service-role gateway path (§7.9).
	DatabaseURL string

	// MembershipCacheTTL bounds how stale a cached team-membership check may be (§5.6 cache.go).
	MembershipCacheTTL time.Duration

	// DBMaxConns/DBMinConns/DBMaxConnLifetime/DBMaxConnIdleTime are the §7.9 pool bounds.
	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration

	// MaxRunDuration bounds a detached pipeline run's own execution budget -- the run's context is
	// context.WithoutCancel(reqCtx) plus this timeout, never the bare request context (§7.1 rule 4,
	// §7.5). The actual disposition for a run that hits this budget is a pilot-team product
	// decision, not decided here (see 011-task-detached-pipeline-execution/analyze.json).
	MaxRunDuration time.Duration

	// MaxConcurrentRuns caps in-flight pipeline runs admitted per gateway instance before StartRun
	// queues instead of starting a goroutine (§7.7 admission control).
	MaxConcurrentRuns int64
}

// Load reads Config from the environment, applying defaults for local dev (`supabase start`).
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		GinMode:         getEnv("GIN_MODE", "release"),
		ShutdownTimeout: 30 * time.Second, // §7.10 drain window

		SupabaseURL: getEnv("SUPABASE_URL", "http://127.0.0.1:54321"),

		// Local `supabase start` default direct-connect credentials, per
		// thinkboard-supabase/supabase/config.toml [db] port 54322.
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:54322/postgres"),

		MembershipCacheTTL: 60 * time.Second, // §5.6 "TTL <= 60s"

		DBMaxConns:        40, // §7.9 worked budget, per gateway instance
		DBMinConns:        8,
		DBMaxConnLifetime: 30 * time.Minute,
		DBMaxConnIdleTime: 5 * time.Minute,

		MaxRunDuration:    15 * time.Minute, // §7.5's own worked example figure
		MaxConcurrentRuns: 24,               // §7.9 worked budget, per gateway instance
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
