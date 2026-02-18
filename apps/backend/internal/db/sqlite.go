package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultBusyTimeout = 5 * time.Second

	// defaultSQLiteReaderConns is the number of concurrent read connections.
	// SQLite WAL mode allows many readers alongside a single writer; 4 is a
	// reasonable default for a desktop/server workload.
	defaultSQLiteReaderConns = 4
)

// OpenSQLite opens a SQLite database configured for writes (single connection).
func OpenSQLite(dbPath string) (*sql.DB, error) {
	normalizedPath := normalizeSQLitePath(dbPath)
	if err := ensureSQLiteDir(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to prepare database path: %w", err)
	}
	if err := ensureSQLiteFile(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to create database file: %w", err)
	}

	// Writer DSN settings:
	// - foreign_keys=on: enforce FK constraints consistently.
	// - busy_timeout: wait briefly on locks to reduce transient "database is locked".
	// - journal_mode=WAL: better read concurrency with a single writer.
	// - synchronous=NORMAL: reasonable durability/perf tradeoff for app workloads.
	// - cache=shared: allow multiple connections to share a page cache.
	dsn := fmt.Sprintf(
		"file:%s?_foreign_keys=on&_mode=rwc&_busy_timeout=%d&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared",
		normalizedPath,
		int(defaultBusyTimeout/time.Millisecond),
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Single writer connection: serializes writes and avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db, nil
}

// OpenSQLiteReader opens a read-only SQLite connection pool with multiple
// concurrent connections. Combined with WAL mode, this allows readers to
// proceed without blocking on (or being blocked by) writes.
func OpenSQLiteReader(dbPath string) (*sql.DB, error) {
	normalizedPath := normalizeSQLitePath(dbPath)

	// Reader DSN: read-only mode, FK enforcement, shared cache.
	// journal_mode and synchronous are database-level (set by the writer).
	dsn := fmt.Sprintf(
		"file:%s?_foreign_keys=on&_mode=ro&_busy_timeout=%d&_cache=shared",
		normalizedPath,
		int(defaultBusyTimeout/time.Millisecond),
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only database: %w", err)
	}

	db.SetMaxOpenConns(defaultSQLiteReaderConns)
	db.SetMaxIdleConns(defaultSQLiteReaderConns)

	return db, nil
}

func ensureSQLiteDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func ensureSQLiteFile(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func normalizeSQLitePath(dbPath string) string {
	if dbPath == "" {
		return dbPath
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return dbPath
	}
	return abs
}
