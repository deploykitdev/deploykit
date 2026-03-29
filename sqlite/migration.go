package sqlite

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate runs all pending SQL migrations in order.
func (db *DB) migrate() error {
	// Create the migrations tracking table if it doesn't exist.
	if _, err := db.db.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			name TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Read all migration files.
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	// Sort by filename to ensure order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Check if migration has already been applied.
		var count int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE name = ?`, name).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		// Read and execute the migration.
		content, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		db.logger.Info("applying migration", "name", name)

		tx, err := db.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for %s: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", name, err)
		}

		if _, err := tx.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}
	}

	return nil
}
