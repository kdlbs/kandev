package persistence

import (
	"context"
	"github.com/kandev/kandev/internal/db"
	"os"
	"path/filepath"
	"testing"
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
