package backups

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
