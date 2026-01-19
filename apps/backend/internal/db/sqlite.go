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
)

// OpenSQLite opens a SQLite database with shared best-practice settings.
func OpenSQLite(dbPath string) (*sql.DB, error) {
	normalizedPath := normalizeSQLitePath(dbPath)
	if err := ensureSQLiteDir(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to prepare database path: %w", err)
	}
	if err := ensureSQLiteFile(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to create database file: %w", err)
	}

	// SQLite DSN settings:
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

	// SQLite allows a single writer, keep pool small and deterministic.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

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
