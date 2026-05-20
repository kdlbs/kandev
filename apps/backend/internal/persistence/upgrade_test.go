package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// seedV0DB creates a file-backed SQLite DB with a minimal v0-style schema
// (tasks table with INTEGER priority) and one seeded row.
func seedV0DB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open seed DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		priority INTEGER DEFAULT 0
	)`); err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks VALUES ('t1', 'hello', 1)`); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// provideForTest constructs a minimal config pointing at dir as the home dir
// and calls persistence.Provide, returning the pool and cleanup.
func provideForTest(t *testing.T, dir, version string) error {
	t.Helper()
	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = filepath.Join(dir, "data", "kandev.db")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "warn", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	pool, cleanup, err := Provide(cfg, log, version)
	if err != nil {
		return err
	}
	t.Cleanup(func() { _ = cleanup() })
	_ = pool
	return nil
}

// TestProvide_TakesSnapshotOnUpgrade verifies that Provide creates a backup
// file when it detects a version change (pre-meta DB with user tables).
func TestProvide_TakesSnapshotOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dataDir, "kandev.db")

	// Seed a v0-style DB (has user tables, no kandev_meta row).
	seedV0DB(t, dbPath)

	if err := provideForTest(t, dir, "vTEST"); err != nil {
		t.Fatalf("Provide: %v", err)
	}

	// A backup should exist.
	backupDir := filepath.Join(dataDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one backup file, got none")
	}

	// The backup should be a readable SQLite DB containing the seeded row.
	snapPath := filepath.Join(backupDir, entries[0].Name())
	snap, err := sqlx.Open("sqlite3", snapPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()

	var title string
	if err := snap.QueryRow(`SELECT title FROM tasks WHERE id = 't1'`).Scan(&title); err != nil {
		t.Fatalf("query snapshot tasks: %v", err)
	}
	if title != "hello" {
		t.Errorf("snapshot task title = %q, want %q", title, "hello")
	}
}

// TestProvide_NoSecondSnapshotSameVersion verifies that a second Provide call
// with the same version does not create an additional snapshot.
func TestProvide_NoSecondSnapshotSameVersion(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dataDir, "kandev.db")
	seedV0DB(t, dbPath)

	// First boot — takes a snapshot.
	if err := provideForTest(t, dir, "vTEST"); err != nil {
		t.Fatalf("first Provide: %v", err)
	}

	// Record the version manually (simulating what storage.go does after all repos init).
	func() {
		db, err := sqlx.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatalf("open for version write: %v", err)
		}
		defer func() { _ = db.Close() }()
		if err := WriteVersion(db, "vTEST"); err != nil {
			t.Fatalf("WriteVersion: %v", err)
		}
	}()

	backupDir := filepath.Join(dataDir, "backups")
	countBefore := backupCount(t, backupDir)

	// Second boot — same version, should not snapshot.
	if err := provideForTest(t, dir, "vTEST"); err != nil {
		t.Fatalf("second Provide: %v", err)
	}

	countAfter := backupCount(t, backupDir)
	if countAfter != countBefore {
		t.Errorf("second boot created extra snapshots: before=%d after=%d", countBefore, countAfter)
	}
}

// TestProvide_BackupFailureReturnsError verifies that Provide returns an error
// and closes the pool when the backup cannot be written (read-only backup dir).
func TestProvide_BackupFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dataDir, "kandev.db")
	seedV0DB(t, dbPath)

	// Create a read-only backups dir so VACUUM INTO will fail.
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o555); err != nil {
		t.Fatalf("mkdir ro backupDir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0o755) })

	// This will fail because pruning/snapshot needs write access.
	// On some CI environments root ignores permission — skip gracefully.
	if os.Getuid() == 0 {
		t.Skip("running as root; read-only dir check not reliable")
	}

	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = dbPath
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "warn", Format: "json", OutputPath: "stderr"})
	_, cleanup, err := Provide(cfg, log, "vTEST")
	if err == nil {
		_ = cleanup()
		t.Fatal("expected Provide to return error for read-only backup dir, got nil")
	}
}

// TestPruneBackups_KeepsExactlyN is an integration-level test that seeds
// 3 backup files and calls pruneBackups(dir, 2) via the persistence package.
func TestPruneBackups_KeepsExactlyN(t *testing.T) {
	dir := t.TempDir()
	// Seed 3 files.
	for i, name := range []string{
		"kandev-pre-meta-20260101T000000Z.db",
		"kandev-pre-meta-20260102T000000Z.db",
		"kandev-pre-meta-20260103T000000Z.db",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	if err := pruneBackups(dir, 2); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 files after prune, got %d", len(entries))
	}
}

func backupCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("readdir %s: %v", dir, err)
	}
	return len(entries)
}
