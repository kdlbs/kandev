package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func testRepository(t *testing.T) *Repository {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return repo
}

func TestCreateIsIdempotentAndSequencesNeverReuse(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, "user-1", "workspace-1", "tab-1")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	repeated, err := repo.Create(ctx, "user-1", "workspace-1", "tab-1")
	if err != nil {
		t.Fatalf("create repeated: %v", err)
	}
	if repeated.TabID != first.TabID || repeated.Sequence != first.Sequence {
		t.Fatalf("repeated create = %#v, want %#v", repeated, first)
	}

	second, err := repo.Create(ctx, "user-1", "workspace-1", "tab-2")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.Sequence != 2 {
		t.Fatalf("second sequence = %d, want 2", second.Sequence)
	}
	if err := repo.Delete(ctx, "user-1", "tab-2"); err != nil {
		t.Fatalf("delete second: %v", err)
	}

	third, err := repo.Create(ctx, "user-1", "workspace-1", "tab-3")
	if err != nil {
		t.Fatalf("create third: %v", err)
	}
	if third.Sequence != 3 {
		t.Fatalf("third sequence = %d, want 3 after deleting sequence 2", third.Sequence)
	}

	otherWorkspace, err := repo.Create(ctx, "user-1", "workspace-2", "tab-4")
	if err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	if otherWorkspace.Sequence != 1 {
		t.Fatalf("other workspace sequence = %d, want 1", otherWorkspace.Sequence)
	}
}

func TestUpdateLifecyclePersistsSessionAndExit(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, "user-1", "workspace-1", "tab-1"); err != nil {
		t.Fatalf("create: %v", err)
	}

	exitCode := 7
	if err := repo.UpdateLifecycle(ctx, "user-1", "tab-1", "session-1", "running", nil, ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := repo.UpdateLifecycle(ctx, "user-1", "tab-1", "", "exited", &exitCode, ""); err != nil {
		t.Fatalf("set exited: %v", err)
	}

	tab, err := repo.Get(ctx, "user-1", "tab-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tab.SessionID != nil || tab.Status != "exited" || tab.ExitCode == nil || *tab.ExitCode != exitCode {
		t.Fatalf("lifecycle = %#v, want cleared exited session", tab)
	}
}
