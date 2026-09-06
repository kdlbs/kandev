package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/terminal/models"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-TASK-TERMINALS-001.1, AC-TASKS-TASK-TERMINALS-001.2,
// AC-TASKS-TASK-TERMINALS-001.3
func TestPostgresTerminalRepositoryLifecycle(t *testing.T) {
	t.Run("create get and rename", testPostgresTerminalCreateGetRename)
	t.Run("list and state", testPostgresTerminalListAndState)
	t.Run("delete", testPostgresTerminalDelete)
}

func newPostgresTerminalTestRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	return repo
}

func testPostgresTerminalCreateGetRename(t *testing.T) {
	repo := newPostgresTerminalTestRepo(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, "task-1", "env-1", "terminal-1", "make dev")
	if err != nil {
		t.Fatalf("create first terminal: %v", err)
	}
	second, err := repo.Create(ctx, "task-1", "env-1", "terminal-2", "")
	if err != nil {
		t.Fatalf("create second terminal: %v", err)
	}
	if first.Seq != 1 || first.State != models.StateOpen || first.InitialCommand != "make dev" {
		t.Fatalf("first terminal = %#v, want seq 1, open state, and initial command", first)
	}
	if second.Seq != 2 {
		t.Fatalf("second terminal seq = %d, want 2", second.Seq)
	}

	name := "build watcher"
	if err := repo.Rename(ctx, first.ID, &name); err != nil {
		t.Fatalf("rename terminal: %v", err)
	}
	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("get terminal: %v", err)
	}
	if got.CustomName == nil || *got.CustomName != name {
		t.Fatalf("custom name = %v, want %q", got.CustomName, name)
	}
	if err := repo.Rename(ctx, first.ID, nil); err != nil {
		t.Fatalf("clear terminal name: %v", err)
	}
	got, err = repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("get terminal after clear: %v", err)
	}
	if got.CustomName != nil {
		t.Fatalf("custom name after clear = %v, want nil", got.CustomName)
	}
}

func testPostgresTerminalListAndState(t *testing.T) {
	repo := newPostgresTerminalTestRepo(t)
	ctx := context.Background()
	first, err := repo.Create(ctx, "task-1", "env-1", "terminal-1", "")
	if err != nil {
		t.Fatalf("create first terminal: %v", err)
	}
	second, err := repo.Create(ctx, "task-1", "env-1", "terminal-2", "")
	if err != nil {
		t.Fatalf("create second terminal: %v", err)
	}
	if err := repo.SetState(ctx, second.ID, models.StateParked); err != nil {
		t.Fatalf("park terminal: %v", err)
	}
	open, err := repo.ListByTask(ctx, "task-1", false)
	if err != nil {
		t.Fatalf("list open terminals: %v", err)
	}
	if len(open) != 1 || open[0].ID != first.ID {
		t.Fatalf("open terminals = %#v, want only %q", open, first.ID)
	}
	all, err := repo.ListByTask(ctx, "task-1", true)
	if err != nil {
		t.Fatalf("list all terminals: %v", err)
	}
	if len(all) != 2 || all[0].ID != first.ID || all[1].ID != second.ID {
		t.Fatalf("all terminals = %#v, want %q then %q", all, first.ID, second.ID)
	}

	if err := repo.SetState(ctx, second.ID, models.StateOpen); err != nil {
		t.Fatalf("resume terminal: %v", err)
	}
	open, err = repo.ListByTask(ctx, "task-1", false)
	if err != nil {
		t.Fatalf("list resumed terminals: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("resumed terminal count = %d, want 2", len(open))
	}
}

func testPostgresTerminalDelete(t *testing.T) {
	repo := newPostgresTerminalTestRepo(t)
	ctx := context.Background()
	first, err := repo.Create(ctx, "task-1", "env-1", "terminal-1", "")
	if err != nil {
		t.Fatalf("create first terminal: %v", err)
	}
	if _, err := repo.Create(ctx, "task-1", "env-1", "terminal-2", ""); err != nil {
		t.Fatalf("create second terminal: %v", err)
	}
	if _, err := repo.Create(ctx, "task-2", "env-2", "terminal-3", ""); err != nil {
		t.Fatalf("create other task terminal: %v", err)
	}
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("delete terminal: %v", err)
	}
	if _, err := repo.Get(ctx, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted terminal error = %v, want ErrNotFound", err)
	}
	deleted, err := repo.DeleteByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("delete task terminals: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted task terminal count = %d, want 1", deleted)
	}
	other, err := repo.ListByTask(ctx, "task-2", true)
	if err != nil {
		t.Fatalf("list other task terminals: %v", err)
	}
	if len(other) != 1 || other[0].ID != "terminal-3" {
		t.Fatalf("other task terminals = %#v, want terminal-3", other)
	}
}
