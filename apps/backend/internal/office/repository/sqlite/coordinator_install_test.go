package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
)

// newTestRepositoryForCoordinatorInstall creates an in-memory SQLite repo,
// mirroring base_test.go's newTestRepo. That helper lives in package
// sqlite_test and cannot reach this package's unexported
// classifyCoordinatorInstallWaitErr, so this file stays in package sqlite
// with its own copy of the same setup.
func newTestRepositoryForCoordinatorInstall(t *testing.T) *Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func TestClassifyCoordinatorInstallWaitErr_CallerCancellationWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := classifyCoordinatorInstallWaitErr(ctx, context.DeadlineExceeded)
	if !errors.Is(err, ctx.Err()) {
		t.Fatalf("got %v, want the caller's own ctx.Err() (%v)", err, ctx.Err())
	}
	if errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatal("caller cancellation must not be reported as contention")
	}
}

func TestClassifyCoordinatorInstallWaitErr_DeadlineExceededIsContention(t *testing.T) {
	err := classifyCoordinatorInstallWaitErr(context.Background(), context.DeadlineExceeded)
	if !errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatalf("got %v, want ErrCoordinatorInstallContention", err)
	}
}

func TestClassifyCoordinatorInstallWaitErr_SQLiteBusyTextIsContention(t *testing.T) {
	err := classifyCoordinatorInstallWaitErr(context.Background(), errors.New("database is locked"))
	if !errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatalf("got %v, want ErrCoordinatorInstallContention", err)
	}
}

func TestClassifyCoordinatorInstallWaitErr_OtherErrorsPassThrough(t *testing.T) {
	plain := errors.New("routine_create_failed")
	err := classifyCoordinatorInstallWaitErr(context.Background(), plain)
	if !errors.Is(err, plain) {
		t.Fatalf("got %v, want the original error unclassified", err)
	}
	if errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatal("a plain read/write failure must not be reported as contention")
	}
}

// TestWithCoordinatorInstallLock_FnContentionIsClassified guards
// AC-OFFICE-COORDINATOR-INSTALL-001.13's "the serialized section fails
// because a concurrent install held it" clause: fn's own error (a write
// racing another process's held lock) must be reported as contention, not
// as a plain create/lookup failure.
func TestWithCoordinatorInstallLock_FnContentionIsClassified(t *testing.T) {
	r := newTestRepositoryForCoordinatorInstall(t)

	err := r.WithCoordinatorInstallLock(context.Background(), "test-lock-key",
		func(ctx context.Context, tx *sqlx.Tx) error {
			return errors.New("database is locked")
		})

	if !errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatalf("got %v, want ErrCoordinatorInstallContention", err)
	}
}
