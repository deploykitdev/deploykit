package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

// DB wraps an *sql.DB connection to a SQLite database.
type DB struct {
	db     *sql.DB
	logger *slog.Logger

	// DSN is the data source name for the database.
	DSN string
}

// NewDB creates a new DB instance.
func NewDB(dsn string, logger *slog.Logger) *DB {
	return &DB{
		DSN:    dsn,
		logger: logger,
	}
}

// Open opens the database connection, enables WAL mode and foreign keys,
// and runs any pending migrations.
func (db *DB) Open() error {
	var err error
	db.db, err = sql.Open("sqlite", db.DSN)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("enabling wal mode: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enabling foreign keys: %w", err)
	}

	// Set busy timeout to 5s so concurrent writers wait instead of returning SQLITE_BUSY immediately.
	if _, err := db.db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("setting busy timeout: %w", err)
	}

	// Synchronous NORMAL is safe with WAL mode and significantly reduces fsync overhead.
	if _, err := db.db.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		return fmt.Errorf("setting synchronous mode: %w", err)
	}

	// Store temp tables and indices in memory instead of disk.
	if _, err := db.db.Exec(`PRAGMA temp_store = MEMORY`); err != nil {
		return fmt.Errorf("setting temp store: %w", err)
	}

	// Increase cache size to ~64MB for better read performance.
	if _, err := db.db.Exec(`PRAGMA cache_size = -64000`); err != nil {
		return fmt.Errorf("setting cache size: %w", err)
	}

	// Set mmap size to 128MB for memory-mapped I/O.
	if _, err := db.db.Exec(`PRAGMA mmap_size = 134217728`); err != nil {
		return fmt.Errorf("setting mmap size: %w", err)
	}

	// Run pending migrations.
	if err := db.migrate(); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}
