package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	argus "github.com/argus-env/argus"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	configuration, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	configuration.MaxConns = maxConnections()
	configuration.MinConns = 0
	configuration.MaxConnIdleTime = 5 * time.Minute
	configuration.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		return nil, fmt.Errorf("connect to Neon: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping Neon: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()

	// Cold serverless instances may initialize concurrently. A session-level
	// advisory lock serializes the migration ledger and all migration work.
	const migrationLock int64 = 0x41726775734d6967
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.Exec(unlockContext, `SELECT pg_advisory_unlock($1)`, migrationLock)
	}()

	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := argus.Migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var applied bool
		if err := connection.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		contents, err := argus.Migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := connection.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, entry.Name()); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func maxConnections() int32 {
	maximum := int32(10)
	if os.Getenv("VERCEL") != "" {
		maximum = 3
	}
	if configured := os.Getenv("ARGUS_DB_MAX_CONNS"); configured != "" {
		parsed, err := strconv.ParseInt(configured, 10, 32)
		if err == nil && parsed >= 1 && parsed <= 50 {
			maximum = int32(parsed)
		}
	}
	return maximum
}
