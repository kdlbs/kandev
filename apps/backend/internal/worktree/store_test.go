package worktree

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// newTestStore opens an in-memory SQLite DB and constructs a *SQLiteStore on it.
// initSchema runs inside NewSQLiteStore so the table is ready.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store, err := NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestSQLiteStore_ListActiveWorktreePaths(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	mustCreate := func(wt *Worktree) {
		t.Helper()
		if err := store.CreateWorktree(ctx, wt); err != nil {
			t.Fatalf("create %s: %v", wt.ID, err)
		}
	}

	// Active worktree with a real path — should appear.
	mustCreate(&Worktree{
		ID:           "wt-active-1",
		SessionID:    "sess-1",
		RepositoryID: "repo-1",
		Path:         "/tmp/kandev/tasks/task1/repoA",
		Branch:       "feature/task1",
		Status:       StatusActive,
		CreatedAt:    now, UpdatedAt: now,
	})

	// Active worktree with empty path — must be filtered out (the GC
	// can't act on an empty path anyway, and the SQL guard prevents
	// accidental wildcard matches).
	mustCreate(&Worktree{
		ID:           "wt-active-empty",
		SessionID:    "sess-2",
		RepositoryID: "repo-2",
		Path:         "",
		Branch:       "feature/task2",
		Status:       StatusActive,
		CreatedAt:    now, UpdatedAt: now,
	})

	// "Deleted" status worktree — must be filtered out.
	deletedAt := now
	mustCreate(&Worktree{
		ID:           "wt-deleted",
		SessionID:    "sess-3",
		RepositoryID: "repo-3",
		Path:         "/tmp/kandev/tasks/task3/repoA",
		Branch:       "feature/task3",
		Status:       "deleted",
		CreatedAt:    now, UpdatedAt: now,
		DeletedAt: &deletedAt,
	})

	// Second active worktree to confirm ordering doesn't matter.
	mustCreate(&Worktree{
		ID:           "wt-active-2",
		SessionID:    "sess-4",
		RepositoryID: "repo-4",
		Path:         "/tmp/kandev/tasks/task4/repoB",
		Branch:       "feature/task4",
		Status:       StatusActive,
		CreatedAt:    now, UpdatedAt: now,
	})

	got, err := store.ListActiveWorktreePaths(ctx)
	if err != nil {
		t.Fatalf("list paths: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"/tmp/kandev/tasks/task1/repoA",
		"/tmp/kandev/tasks/task4/repoB",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
