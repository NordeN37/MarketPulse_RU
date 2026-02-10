package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NordeN37/MarketPulse_RU/internal/config"
)

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new connection pool to PostgreSQL.
func New(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing pg config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to pg: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging pg: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// RunMigrations executes all embedded SQL migration files in order.
// Uses a migrations_log table to track which files have already been applied.
// Safe to call on every startup — already-applied migrations are skipped.
func (db *DB) RunMigrations(ctx context.Context, log *slog.Logger) error {
	// Create tracking table if not exists.
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS migrations_log (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("creating migrations_log: %w", err)
	}

	// Read embedded migration files.
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	// Sort by filename to ensure execution order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Check if already applied.
		var exists bool
		err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM migrations_log WHERE filename = $1)`, name,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		// Read and execute.
		sql, err := MigrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		log.Info("applying migration", "file", name)
		if _, err := db.Pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}

		// Record as applied.
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO migrations_log (filename) VALUES ($1) ON CONFLICT DO NOTHING`, name,
		); err != nil {
			return fmt.Errorf("recording migration %s: %w", name, err)
		}

		log.Info("migration applied", "file", name)
	}

	return nil
}

// Close shuts down the connection pool.
func (db *DB) Close() {
	db.Pool.Close()
}
