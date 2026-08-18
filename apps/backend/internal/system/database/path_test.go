package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
)

func TestConfiguredDatabasePath_StatsUseSiblingFiles(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}
	if err := os.WriteFile(databasePath+"-wal", []byte("wal-bytes"), 0o644); err != nil {
		t.Fatalf("write wal: %v", err)
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	backupPath := filepath.Join(backupDir, "manual-1.db")
	if err := os.WriteFile(backupPath, []byte("snapshot"), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	backupTime := time.Now().Add(-time.Minute)
	if err := os.Chtimes(backupPath, backupTime, backupTime); err != nil {
		t.Fatalf("set snapshot time: %v", err)
	}

	svc := NewService(nil, databasePath, ResetDirs{}, nil, nil)
	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Path != databasePath {
		t.Errorf("Path = %q, want %q", stats.Path, databasePath)
	}
	if stats.WALSizeBytes != int64(len("wal-bytes")) {
		t.Errorf("WALSizeBytes = %d, want %d", stats.WALSizeBytes, len("wal-bytes"))
	}
	if stats.LastBackupAt == nil {
		t.Fatal("LastBackupAt is nil, want the custom backup time")
	}
	if stats.LastBackupAt.Before(backupTime.Add(-time.Second)) {
		t.Errorf("LastBackupAt = %v, want at least %v", *stats.LastBackupAt, backupTime.Add(-time.Second))
	}
}

func TestConfiguredDatabasePath_FactoryResetUsesSiblingBackupDir(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}
	writerRaw, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	readerRaw, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		_ = writerRaw.Close()
		t.Fatalf("open sqlite reader: %v", err)
	}
	pool := db.NewPool(sqlx.NewDb(writerRaw, "sqlite3"), sqlx.NewDb(readerRaw, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	for _, statement := range []string{
		`CREATE TABLE kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
	} {
		if _, err := pool.Writer().Exec(statement); err != nil {
			t.Fatalf("seed schema: %v", err)
		}
	}

	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(pool, databasePath, ResetDirs{}, tracker, newTestLogger(t))
	jobID, err := svc.FactoryReset(context.Background(), resetConfirmToken)
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}
	job := waitForState(t, tracker, jobID, jobs.StateSucceeded)
	snapshotPath, ok := job.Result["snapshot_path"].(string)
	if !ok || snapshotPath == "" {
		t.Fatalf("snapshot_path = %#v, want path", job.Result["snapshot_path"])
	}
	wantDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if filepath.Dir(snapshotPath) != wantDir {
		t.Errorf("snapshot dir = %q, want %q", filepath.Dir(snapshotPath), wantDir)
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Errorf("snapshot missing: %v", err)
	}
}
