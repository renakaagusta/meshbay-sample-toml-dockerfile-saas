// Package migrate applies the SQL files embedded in the binary.
//
// It is a separate command so Meshbay's pre_deploy hook runs the schema change
// once per deploy, before the new revision starts, instead of every replica
// migrating on boot.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const createVersionTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version text PRIMARY KEY,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// Run applies every migration that is not in schema_migrations yet, in name
// order, each in its own transaction.
func Run(runContext context.Context, database *pgxpool.Pool, logger *log.Logger) error {
	if _, failure := database.Exec(runContext, createVersionTable); failure != nil {
		return fmt.Errorf("create schema_migrations: %w", failure)
	}
	entries, failure := fs.Glob(migrationFiles, "migrations/*.sql")
	if failure != nil {
		return fmt.Errorf("list migrations: %w", failure)
	}
	sort.Strings(entries)
	applied := 0
	for _, entry := range entries {
		version := entry[len("migrations/"):]
		var exists bool
		if failure := database.QueryRow(runContext,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); failure != nil {
			return fmt.Errorf("check migration %s: %w", version, failure)
		}
		if exists {
			continue
		}
		statement, failure := migrationFiles.ReadFile(entry)
		if failure != nil {
			return fmt.Errorf("read migration %s: %w", version, failure)
		}
		transaction, failure := database.Begin(runContext)
		if failure != nil {
			return fmt.Errorf("begin migration %s: %w", version, failure)
		}
		if _, failure := transaction.Exec(runContext, string(statement)); failure != nil {
			_ = transaction.Rollback(runContext)
			return fmt.Errorf("apply migration %s: %w", version, failure)
		}
		if _, failure := transaction.Exec(runContext,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
		); failure != nil {
			_ = transaction.Rollback(runContext)
			return fmt.Errorf("record migration %s: %w", version, failure)
		}
		if failure := transaction.Commit(runContext); failure != nil {
			return fmt.Errorf("commit migration %s: %w", version, failure)
		}
		logger.Printf("applied migration %s", version)
		applied++
	}
	logger.Printf("migrations up to date (%d applied)", applied)
	return nil
}
