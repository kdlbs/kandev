package backups

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
)

func TestConfiguredDatabasePath_RestoreReplacesConfiguredFilename(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("original"), 0o644); err != nil {
		t.Fatalf("write original database: %v", err)
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manual-1.db"), []byte("restored"), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(databasePath, nil, tracker, newTestLogger(t))
	jobID, err := svc.Restore(context.Background(), "manual-1.db", RestoreConfirmToken)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	job := waitForJob(t, tracker, jobID, jobs.StateSucceeded)
	if job.State != jobs.StateSucceeded {
		t.Fatalf("state = %s, want succeeded; message = %s", job.State, job.Message)
	}

	got, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read restored database: %v", err)
	}
	if string(got) != "restored" {
		t.Fatalf("database bytes = %q, want restored", got)
	}
	if _, err := os.Stat(databasePath + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staged restore remains, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "kandev.db")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("default database unexpectedly exists, stat error = %v", err)
	}
}

func TestConfiguredDatabasePath_RestoreQuiescesPoolAndRemovesWALSidecars(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
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
	if _, err := pool.Writer().Exec(`CREATE TABLE things (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := pool.Writer().Exec(`INSERT INTO things (value) VALUES ('before')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if _, err := os.Stat(databasePath + "-wal"); err != nil {
		t.Fatalf("expected WAL sidecar before restore: %v", err)
	}

	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manual-1.db"), []byte("restored"), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	shutdownCalls := 0
	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(databasePath, pool, tracker, newTestLogger(t))
	svc.OrchestratorShutdown = func() { shutdownCalls++ }
	jobID, err := svc.Restore(context.Background(), "manual-1.db", RestoreConfirmToken)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	job := waitForJob(t, tracker, jobID, jobs.StateSucceeded)
	if job.State != jobs.StateSucceeded {
		t.Fatalf("state = %s, want succeeded; message = %s", job.State, job.Message)
	}
	if shutdownCalls != 1 {
		t.Errorf("OrchestratorShutdown calls = %d, want 1", shutdownCalls)
	}
	if _, err := os.Stat(databasePath + "-wal"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("WAL sidecar remains, stat error = %v", err)
	}
	if _, err := os.Stat(databasePath + "-shm"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("SHM sidecar remains, stat error = %v", err)
	}
	if err := pool.Writer().Ping(); err == nil {
		t.Error("database writer remains open after restore")
	}
}
