package backups

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	snapshotBytes := writeValidRestoreTestSnapshot(t, filepath.Join(backupDir, "manual-1.db"))

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
	if !bytes.Equal(got, snapshotBytes) {
		t.Fatal("configured database does not contain the valid snapshot")
	}
	assertRestoredTestFileValue(t, databasePath, "hello")
	if _, err := os.Stat(databasePath + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staged restore remains, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "kandev.db")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("default database unexpectedly exists, stat error = %v", err)
	}
}

func TestConfiguredDatabasePath_RestoreQuiescesPoolAndReplacesWALSidecars(t *testing.T) {
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
	writeValidRestoreTestSnapshot(t, filepath.Join(backupDir, "manual-1.db"))

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
	assertRestoredTestFileValue(t, databasePath, "hello")
}

func TestRestoreRejectsPostgresWithoutClosingPool(t *testing.T) {
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

	pgRaw, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open fake postgres pool: %v", err)
	}
	pg := sqlx.NewDb(pgRaw.DB, "pgx")
	pool := db.NewPool(pg, pg)
	t.Cleanup(func() { _ = pool.Close() })
	if err := pool.Writer().Ping(); err != nil {
		t.Fatalf("ping fake postgres pool before restore: %v", err)
	}

	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(databasePath, pool, tracker, newTestLogger(t))
	if _, err := svc.Restore(context.Background(), "manual-1.db", RestoreConfirmToken); err == nil {
		t.Fatal("Restore accepted a PostgreSQL pool")
	}
	if err := pool.Writer().Ping(); err != nil {
		t.Fatalf("PostgreSQL pool was closed after rejected restore: %v", err)
	}
}

func TestRestoreCheckpointBusyLeavesDatabaseAndSidecarsUntouched(t *testing.T) {
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
	readerTx, err := pool.Reader().Beginx()
	if err != nil {
		t.Fatalf("begin reader transaction: %v", err)
	}
	rows, err := readerTx.Queryx(`SELECT value FROM things`)
	if err != nil {
		_ = readerTx.Rollback()
		t.Fatalf("hold reader snapshot: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		_ = readerTx.Rollback()
		t.Fatal("reader snapshot returned no row")
	}
	if _, err := pool.Writer().Exec(`PRAGMA busy_timeout = 0`); err != nil {
		t.Fatalf("disable checkpoint busy timeout: %v", err)
	}
	t.Cleanup(func() {
		_ = rows.Close()
		_ = readerTx.Rollback()
	})

	walPath := databasePath + "-wal"
	shmPath := databasePath + "-shm"
	walBefore, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatalf("read WAL before restore: %v", err)
	}
	if _, err := os.Stat(shmPath); err != nil {
		t.Fatalf("stat SHM before restore: %v", err)
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	writeValidRestoreTestSnapshot(t, filepath.Join(backupDir, "manual-1.db"))

	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(databasePath, pool, tracker, newTestLogger(t))
	jobID, err := svc.Restore(context.Background(), "manual-1.db", RestoreConfirmToken)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	job := waitForJob(t, tracker, jobID, jobs.StateFailed)
	if job.State != jobs.StateFailed {
		t.Fatalf("state = %s, want failed; message = %s", job.State, job.Message)
	}

	if !strings.Contains(job.Message, "checkpoint sqlite WAL: busy") {
		t.Fatalf("expected checkpoint-busy failure, got %q", job.Message)
	}

	if got, err := os.ReadFile(walPath); err != nil {
		t.Fatalf("read WAL after rejected restore: %v", err)
	} else if string(got) != string(walBefore) {
		t.Fatal("WAL changed after a busy checkpoint")
	}
	if _, err := os.Stat(shmPath); err != nil {
		t.Fatalf("SHM sidecar was removed after a busy checkpoint: %v", err)
	}
	if got, err := os.ReadFile(databasePath); err != nil {
		t.Fatalf("read database after rejected restore: %v", err)
	} else if len(got) == 0 || string(got) == "restored" {
		t.Fatal("database was replaced after a busy checkpoint")
	}
	if err := pool.Writer().Ping(); err != nil {
		t.Fatalf("database pool closed after a busy checkpoint: %v", err)
	}
}

func TestRestoreReplacementFailureRestoresOriginalFiles(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "custom", "named.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("original"), 0o644); err != nil {
		t.Fatalf("write original database: %v", err)
	}
	walPath := databasePath + "-wal"
	shmPath := databasePath + "-shm"
	if err := os.WriteFile(walPath, []byte("wal-before"), 0o644); err != nil {
		t.Fatalf("write WAL sidecar: %v", err)
	}
	if err := os.WriteFile(shmPath, []byte("shm-before"), 0o644); err != nil {
		t.Fatalf("write SHM sidecar: %v", err)
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
	stagedPath := databasePath + ".new"
	if err := os.WriteFile(stagedPath, []byte("restored"), 0o644); err != nil {
		t.Fatalf("write staged database: %v", err)
	}
	rename := func(oldPath, newPath string) error {
		if oldPath == stagedPath {
			return errors.New("injected replacement failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := svc.replaceDatabaseWith(stagedPath, rename); err == nil {
		t.Fatal("replaceDatabaseWith succeeded after injected replacement failure")
	}

	if got, err := os.ReadFile(databasePath); err != nil {
		t.Fatalf("read original database after failed restore: %v", err)
	} else if string(got) != "original" {
		t.Fatalf("database bytes = %q, want original", got)
	}
	if got, err := os.ReadFile(walPath); err != nil {
		t.Fatalf("read WAL after failed restore: %v", err)
	} else if string(got) != "wal-before" {
		t.Fatalf("WAL bytes = %q, want wal-before", got)
	}
	if got, err := os.ReadFile(shmPath); err != nil {
		t.Fatalf("read SHM after failed restore: %v", err)
	} else if string(got) != "shm-before" {
		t.Fatalf("SHM bytes = %q, want shm-before", got)
	}
}
