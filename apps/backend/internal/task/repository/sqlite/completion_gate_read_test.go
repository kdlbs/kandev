package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newCompletionGateReadTestRepo(t *testing.T) (*Repository, *sqlx.DB, *sqlx.DB, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "completion-gate-read.db")
	writerRaw, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open SQLite writer: %v", err)
	}
	readerRaw, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		_ = writerRaw.Close()
		t.Fatalf("open SQLite reader: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatalf("create SQLite task repository: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return repo, writer, reader, databasePath
}

func TestCompletionGateReadDuringWriter(t *testing.T) {
	repo, _, _, databasePath := newCompletionGateReadTestRepo(t)
	ctx := context.Background()
	const taskID = "task-completion-gate-reader"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Reader snapshot"}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	blockerRaw, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open writer blocker: %v", err)
	}
	blocker := sqlx.NewDb(blockerRaw, "sqlite3")
	t.Cleanup(func() { _ = blocker.Close() })
	blockerTx, err := blocker.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("hold SQLite writer: %v", err)
	}
	defer func() { _ = blockerTx.Rollback() }()

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := repo.GetTaskCompletionGate(readCtx, taskID)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("completion-gate read while writer was held: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		_ = blockerTx.Rollback()
		<-done
		t.Fatal("completion-gate reader did not finish while writer was held")
	}
}

func TestCompletionGateReaderReleaseAfterErrorAndCancellation(t *testing.T) {
	repo, _, _, _ := newCompletionGateReadTestRepo(t)
	ctx := context.Background()
	const taskID = "task-completion-gate-reader-release"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Reader lifecycle"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.GetTaskCompletionGate(ctx, "missing-completion-gate-task"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing task error = %v, want ErrTaskNotFound", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.GetTaskCompletionGate(canceled, taskID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read error = %v, want context.Canceled", err)
	}
	if _, err := repo.GetTaskCompletionGate(ctx, taskID); err != nil {
		t.Fatalf("read after error and cancellation: %v", err)
	}
}

func TestCompletionGateReadPostgres(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	const taskID = "task-completion-gate-reader-pg"
	seedPostgresTask(t, repo, taskID)
	tx, err := repo.beginTaskCompletionGateRead(ctx)
	if err != nil {
		t.Fatalf("begin read-only completion-gate snapshot: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	var isolation, readOnly string
	if err := tx.QueryRowContext(ctx, `SELECT current_setting('transaction_isolation'), current_setting('transaction_read_only')`).Scan(&isolation, &readOnly); err != nil {
		t.Fatalf("read completion-gate transaction settings: %v", err)
	}
	if isolation != "repeatable read" || readOnly != "on" {
		t.Fatalf("completion-gate transaction = %q/read_only=%q, want repeatable read/read-only", isolation, readOnly)
	}
	snapshot, err := repo.readTaskCompletionGateTx(ctx, tx, taskID, false)
	if err != nil {
		t.Fatalf("read completion-gate snapshot: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit completion-gate read: %v", err)
	}
	if snapshot.TaskID != taskID {
		t.Fatalf("snapshot task ID = %q, want %q", snapshot.TaskID, taskID)
	}
	publicSnapshot, err := repo.GetTaskCompletionGate(ctx, taskID)
	if err != nil || publicSnapshot == nil || publicSnapshot.TaskID != taskID {
		t.Fatalf("GetTaskCompletionGate = %+v, err=%v", publicSnapshot, err)
	}
}
