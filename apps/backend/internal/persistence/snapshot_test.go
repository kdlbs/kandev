package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func fileSQLiteDB(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite at %s: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestSnapshotSQLite verifies that VACUUM INTO creates a readable copy
// that contains the seeded row from the source DB.
func TestSnapshotSQLite_CreatesReadableCopy(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "snap.db")

	src := fileSQLiteDB(t, srcPath)
	if _, err := src.Exec(`CREATE TABLE things (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := src.Exec(`INSERT INTO things VALUES ('1', 'hello')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	size, err := snapshotSQLite(src, dstPath)
	if err != nil {
		t.Fatalf("snapshotSQLite: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}

	// Verify the snapshot is readable and contains the seeded row.
	snap, err := sqlx.Open("sqlite3", dstPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()

	var name string
	if err := snap.QueryRow(`SELECT name FROM things WHERE id = '1'`).Scan(&name); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if name != "hello" {
		t.Errorf("snapshot row name = %q, want %q", name, "hello")
	}
}

// TestPruneBackups verifies that pruneBackups keeps exactly the N newest files.
func TestPruneBackups_KeepsNewest(t *testing.T) {
	dir := t.TempDir()

	// Seed three files with distinct mtimes (oldest to newest).
	names := []string{
		"kandev-v0.1.0-20260101T000000Z.db",
		"kandev-v0.2.0-20260102T000000Z.db",
		"kandev-v0.3.0-20260103T000000Z.db",
	}
	for i, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		// Stagger mtimes by 1 second each so sort order is deterministic.
		mtime := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	if err := pruneBackups(dir, 2); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	remaining, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 files after prune, got %d", len(remaining))
	}
	// The oldest (index 0) should have been deleted.
	for _, e := range remaining {
		if e.Name() == names[0] {
			t.Errorf("oldest file %q was not pruned", names[0])
		}
	}
}

// TestPruneBackups_BelowThreshold verifies that pruneBackups is a no-op
// when fewer than keep files are present.
func TestPruneBackups_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev-v0.1.0-20260101T000000Z.db")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := pruneBackups(dir, 2); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file was unexpectedly removed: %v", err)
	}
}
