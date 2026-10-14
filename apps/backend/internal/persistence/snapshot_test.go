package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/config"
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

func TestSnapshotSQLite_CanceledRemovesDestination(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "snap.db")

	src := fileSQLiteDB(t, srcPath)
	if _, err := src.Exec(`CREATE TABLE things (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshotSQLiteContext(ctx, src, dstPath); err == nil {
		t.Fatal("canceled snapshot succeeded")
	}
	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		t.Fatalf("canceled snapshot destination stat error = %v, want not exist", err)
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

// seedBootStaging creates a boot backup staging directory holding one staged
// file and backdates both by age.
func seedBootStaging(t *testing.T, backupDir, name string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(backupDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	staged := filepath.Join(dir, "kandev-v1.0.0-20260101T000000Z.db")
	if err := os.WriteFile(staged, []byte("staged copy"), 0o600); err != nil {
		t.Fatalf("seed staged file: %v", err)
	}
	when := time.Now().Add(-age)
	for _, p := range []string{staged, dir} {
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatalf("backdate %s: %v", p, err)
		}
	}
	return dir
}

func requireExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if want && err != nil {
		t.Errorf("%s was removed: %v", filepath.Base(path), err)
	}
	if !want && !os.IsNotExist(err) {
		t.Errorf("%s survived: stat err = %v", filepath.Base(path), err)
	}
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.1, AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3
func TestProvideSQLiteReclaimsBootStagingWithoutBackup(t *testing.T) {
	homeDir := t.TempDir()
	backupDir := filepath.Join(homeDir, "data", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("create backups dir: %v", err)
	}
	abandoned := seedBootStaging(t, backupDir, ".kandev-backup-111", 7*time.Hour)
	recent := seedBootStaging(t, backupDir, ".kandev-backup-222", time.Hour)
	snapshotStage := seedBootStaging(t, backupDir, ".snapshot-333", 7*time.Hour)
	published := filepath.Join(backupDir, "kandev-v1.0.0-20260101T000000Z.db")
	if err := os.WriteFile(published, []byte("published"), 0o600); err != nil {
		t.Fatalf("seed published backup: %v", err)
	}
	old := time.Now().Add(-7 * time.Hour)
	if err := os.Chtimes(published, old, old); err != nil {
		t.Fatalf("backdate published backup: %v", err)
	}

	// A fresh database with no version takes no pre-migration backup.
	_, cleanup, err := Provide(&config.Config{HomeDir: homeDir}, nil, "")
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	requireExists(t, abandoned, false)
	requireExists(t, recent, true)
	requireExists(t, snapshotStage, true)
	requireExists(t, published, true)
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3
func TestSweepStaleBootStagingUsesNewestEntry(t *testing.T) {
	backupDir := t.TempDir()
	stale := seedBootStaging(t, backupDir, bootStagingDirPrefix+"stale", 2*staleBootStagingAge)
	// The directory itself is old, but the staged file inside it is still
	// being written, so the staging directory is live.
	live := seedBootStaging(t, backupDir, bootStagingDirPrefix+"live", 2*staleBootStagingAge)
	now := time.Now()
	if err := os.Chtimes(filepath.Join(live, "kandev-v1.0.0-20260101T000000Z.db"), now, now); err != nil {
		t.Fatalf("touch staged file: %v", err)
	}

	SweepStaleBootStaging(backupDir, nil)

	requireExists(t, stale, false)
	requireExists(t, live, true)
}

func TestSweepStaleBootStagingToleratesMissingDirectory(t *testing.T) {
	SweepStaleBootStaging(filepath.Join(t.TempDir(), "absent"), nil)
}
