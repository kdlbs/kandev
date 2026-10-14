package persistence

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/db"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-PLATFORM-SQLITE-SNAPSHOT-001.1
func TestPreMigrationBackupCopiesWithoutRebuilding(t *testing.T) {
	dir := t.TempDir()
	src := fileSQLiteDB(t, filepath.Join(dir, "source.db"))
	src.MustExec(`PRAGMA journal_mode=WAL`)
	src.MustExec(`CREATE TABLE items (id INTEGER PRIMARY KEY, value BLOB)`)
	src.MustExec(`INSERT INTO items VALUES (1, zeroblob(1048576)), (2, 'keep')`)
	src.MustExec(`DELETE FROM items WHERE id=1`)
	var free int
	if err := src.Get(&free, `PRAGMA freelist_count`); err != nil {
		t.Fatal(err)
	}
	if free == 0 {
		t.Fatal("fixture has no free pages")
	}
	path, _, err := createPreMigrationBackup(context.Background(), src, dir, "old")
	if err != nil {
		t.Fatal(err)
	}
	copyDB := fileSQLiteDB(t, "file:"+db.EscapeSQLiteURIPath(filepath.ToSlash(path)))
	var copiedFree int
	if err := copyDB.Get(&copiedFree, `PRAGMA freelist_count`); err != nil {
		t.Fatal(err)
	}
	if copiedFree != free {
		t.Fatalf("backup rebuilt free pages: got %d, want %d", copiedFree, free)
	}
	var value, integrity string
	if err := copyDB.Get(&value, `SELECT value FROM items WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err := copyDB.Get(&integrity, `PRAGMA integrity_check`); err != nil {
		t.Fatal(err)
	}
	if value != "keep" || integrity != "ok" {
		t.Fatalf("invalid backup: value=%q integrity=%q", value, integrity)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatal("backup is not private")
	}
}

func TestCopySQLiteSnapshotMemoryAndFailureCleanup(t *testing.T) {
	src := fileSQLiteDB(t, ":memory:")
	src.SetMaxOpenConns(1)
	src.MustExec(`CREATE TABLE items (value TEXT)`)
	src.MustExec(`INSERT INTO items VALUES ('retained')`)
	path := filepath.Join(t.TempDir(), "copy ?#% ü.db")
	if _, err := CopySQLiteSnapshotContext(t.Context(), src, path); err != nil {
		t.Fatal(err)
	}
	copyDB := fileSQLiteDB(t, "file:"+db.EscapeSQLiteURIPath(filepath.ToSlash(path)))
	var value string
	if err := copyDB.Get(&value, `SELECT value FROM items`); err != nil {
		t.Fatal(err)
	}
	if value != "retained" {
		t.Fatalf("value=%q", value)
	}
	if _, err := CopySQLiteSnapshot(src, path); err == nil {
		t.Fatal("overwrote existing destination")
	}
	if err := copyDB.Get(&value, `SELECT value FROM items`); err != nil {
		t.Fatal(err)
	}
	if value != "retained" {
		t.Fatal("existing destination changed")
	}

	canceledPath := filepath.Join(t.TempDir(), "canceled.db")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CopySQLiteSnapshotContext(ctx, src, canceledPath); err == nil {
		t.Fatal("canceled backup succeeded")
	}
	if _, err := os.Stat(canceledPath); !os.IsNotExist(err) {
		t.Fatalf("canceled output remains: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := CopySQLiteSnapshot(src, canceledPath); err == nil {
		t.Fatal("closed source backup succeeded")
	}
	if _, err := os.Stat(canceledPath); !os.IsNotExist(err) {
		t.Fatalf("failed output remains: %v", err)
	}
}

// @covers AC-PLATFORM-SQLITE-SNAPSHOT-001.2
func TestCopySQLiteSnapshotCancelsLockedCopy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := fileSQLiteDB(t, ":memory:")
		src.SetMaxOpenConns(1)
		src.MustExec(`CREATE TABLE items (value TEXT)`)
		src.MustExec(`INSERT INTO items VALUES ('committed')`)
		// The source connection's open write transaction makes backup_step return
		// LOCKED. The copy has acquired both connections and created its output.
		src.MustExec(`BEGIN IMMEDIATE`)
		src.MustExec(`INSERT INTO items VALUES ('uncommitted')`)
		path := filepath.Join(t.TempDir(), "interrupted.db")
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err := CopySQLiteSnapshotContext(ctx, src, path)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("locked copy error: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("partial backup remains: %v", err)
		}
		if src.Stats().InUse != 0 {
			t.Fatal("copy leaked source connection")
		}
		src.MustExec(`ROLLBACK`)
		if _, err := CopySQLiteSnapshotContext(t.Context(), src, path); err != nil {
			t.Fatal(err)
		}
		copied := fileSQLiteDB(t, path)
		var count int
		var integrity string
		if err := copied.Get(&count, `SELECT COUNT(*) FROM items`); err != nil {
			t.Fatal(err)
		}
		if err := copied.Get(&integrity, `PRAGMA integrity_check`); err != nil {
			t.Fatal(err)
		}
		if count != 1 || integrity != "ok" {
			t.Fatalf("retry produced invalid copy: count=%d integrity=%s", count, integrity)
		}
	})
}

func TestCopySQLiteSnapshotAllowsActiveWALWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	writer := fileSQLiteDB(t, path)
	writer.MustExec(`PRAGMA journal_mode=WAL`)
	writer.MustExec(`CREATE TABLE items (value TEXT)`)
	writer.MustExec(`INSERT INTO items VALUES ('committed')`)
	reader := fileSQLiteDB(t, path)
	tx, err := writer.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	tx.MustExec(`INSERT INTO items VALUES ('uncommitted')`)
	destination := filepath.Join(t.TempDir(), "copy.db")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := CopySQLiteSnapshotContext(ctx, reader, destination); err != nil {
		t.Fatal(err)
	}
	copied := fileSQLiteDB(t, destination)
	var count int
	if err := copied.Get(&count, `SELECT COUNT(*) FROM items`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("copy included uncommitted write: %d rows", count)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestCopySQLiteSnapshotStopsStalledCopy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src := fileSQLiteDB(t, ":memory:")
		src.SetMaxOpenConns(1)
		src.MustExec(`CREATE TABLE items (value TEXT)`)
		src.MustExec(`BEGIN IMMEDIATE`)
		src.MustExec(`INSERT INTO items VALUES ('locked')`)
		path := filepath.Join(t.TempDir(), "stalled.db")
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		_, err := CopySQLiteSnapshotContext(ctx, src, path)
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("copy must stop stalled retries before caller deadline: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stalled output remains: %v", err)
		}
		src.MustExec(`ROLLBACK`)
	})
}
