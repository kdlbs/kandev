package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-sqlite3"
)

const (
	defaultBusyTimeout = 5 * time.Second

	// sqliteWriterDriverName is a writer-only driver whose connections apply
	// the writer pragmas that have no DSN parameter. Callers still pass
	// "sqlite3" to sqlx.NewDb, which takes the dialect from that argument.
	sqliteWriterDriverName = "sqlite3_kandev_writer"

	// journalSizeLimitSQL bounds the WAL file that remains after a WAL reset.
	// A checkpoint rewrites the WAL in place without shrinking it, so without
	// this bound the file keeps its high-water mark for the life of the
	// database. It is a reset-time bound, not a per-write budget: 64 MiB leaves
	// room for a burst well over the ~4 MiB default autocheckpoint.
	journalSizeLimitSQL = "PRAGMA journal_size_limit = 67108864"

	// defaultSQLiteReaderConns is the number of concurrent read connections.
	// SQLite WAL mode allows many readers alongside a single writer; 4 is a
	// reasonable default for a desktop/server workload.
	defaultSQLiteReaderConns = 4

	// maxSQLiteReaderConns bounds a configured reader pool so a mistaken
	// setting cannot consume an unbounded number of file descriptors.
	maxSQLiteReaderConns = 64
)

func init() {
	sql.Register(sqliteWriterDriverName, &sqlite3.SQLiteDriver{ConnectHook: applyWriterPragmas})
}

// applyWriterPragmas runs on every new writer connection. journal_size_limit
// is per-connection state and the pool can replace its connection at any time,
// so it cannot be set once per handle. The standard driver.ExecerContext
// interface keeps one code path for cgo and non-cgo builds: the non-cgo
// go-sqlite3 stub does not implement it and cannot open a database anyway.
func applyWriterPragmas(conn *sqlite3.SQLiteConn) error {
	execer, ok := any(conn).(driver.ExecerContext)
	if !ok {
		return nil
	}
	_, err := execer.ExecContext(context.Background(), journalSizeLimitSQL, nil)
	return err
}

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
	// - synchronous=NORMAL: commits survive process termination. Power-loss
	//   durability for journal release comes from db.DurabilityBarrier, which
	//   commits on this connection with FULL before any journal release.
	// - cache=shared: allow multiple connections to share a page cache.
	// - txlock=immediate: acquire the writer before transaction predicate reads.
	// journal_size_limit has no DSN parameter; applyWriterPragmas sets it.
	dsn := fmt.Sprintf(
		"file:%s?_foreign_keys=on&_mode=rwc&_busy_timeout=%d&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared&_txlock=immediate",
		normalizedPath,
		int(defaultBusyTimeout/time.Millisecond),
	)
	db, err := sql.Open(sqliteWriterDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// One connection serializes this pool's writers; independent pools may contend.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db, nil
}

// OpenSQLiteReader opens a read-only SQLite connection pool with the default
// number of concurrent connections.
func OpenSQLiteReader(dbPath string) (*sql.DB, error) {
	return OpenSQLiteReaderWithConns(dbPath, defaultSQLiteReaderConns)
}

// OpenSQLiteReaderWithConns opens a read-only SQLite connection pool with the
// requested number of concurrent connections. Combined with WAL mode, this
// allows readers to proceed without blocking on (or being blocked by) writes.
// A zero value keeps the default; negative values and values above
// maxSQLiteReaderConns are rejected.
func OpenSQLiteReaderWithConns(dbPath string, readerConns int) (*sql.DB, error) {
	if readerConns == 0 {
		readerConns = defaultSQLiteReaderConns
	}
	if readerConns < 1 || readerConns > maxSQLiteReaderConns {
		return nil, fmt.Errorf("sqlite reader connections must be between 1 and %d: %d", maxSQLiteReaderConns, readerConns)
	}
	normalizedPath := normalizeSQLitePath(dbPath)

	// Reader DSN: read-only mode and FK enforcement.
	// Do not enable SQLite shared-cache mode: an active reader in that cache
	// can make a later reader observe its stale snapshot after a writer commits.
	// WAL is persistent; synchronous is connection-local and only the writer commits.
	dsn := fmt.Sprintf(
		"file:%s?_foreign_keys=on&mode=ro&_busy_timeout=%d",
		normalizedPath,
		int(defaultBusyTimeout/time.Millisecond),
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only database: %w", err)
	}

	db.SetMaxOpenConns(readerConns)
	db.SetMaxIdleConns(readerConns)

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
