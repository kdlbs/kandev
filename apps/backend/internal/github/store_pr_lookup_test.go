package github

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// newTestStoreWithWorkspaces builds a store whose `tasks` table carries
// workspace_id, so the workspace-scoped PR-number lookup can be exercised.
func newTestStoreWithWorkspaces(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "github.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestListTaskIDsByPRNumber(t *testing.T) {
	store := newTestStoreWithWorkspaces(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// task-1 (ws-1) has PR #1243, task-2 (ws-1) has PR #99,
	// task-3 (ws-2) also has PR #1243 — must not leak into ws-1.
	exec := store.db
	for _, q := range []string{
		`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`,
		`INSERT INTO tasks (id, workspace_id) VALUES ('task-2', 'ws-1')`,
		`INSERT INTO tasks (id, workspace_id) VALUES ('task-3', 'ws-2')`,
	} {
		if _, err := exec.Exec(q); err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}
	mkPR := func(id, taskID string, num int) *TaskPR {
		return &TaskPR{
			ID: id, TaskID: taskID, RepositoryID: "repo-" + id,
			Owner: "kdlbs", Repo: "kandev", PRNumber: num,
			PRURL: "https://github.com/kdlbs/kandev/pull/1", PRTitle: "x",
			HeadBranch: "feat/x", BaseBranch: "main", State: "open", CreatedAt: now,
		}
	}
	for _, pr := range []*TaskPR{
		mkPR("p1", "task-1", 1243),
		mkPR("p2", "task-2", 99),
		mkPR("p3", "task-3", 1243),
	} {
		if err := store.CreateTaskPR(ctx, pr); err != nil {
			t.Fatalf("create PR: %v", err)
		}
	}

	got, err := store.ListTaskIDsByPRNumber(ctx, "ws-1", 1243)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 || got[0] != "task-1" {
		t.Errorf("expected [task-1] for ws-1 PR #1243, got %v", got)
	}

	none, err := store.ListTaskIDsByPRNumber(ctx, "ws-1", 7777)
	if err != nil {
		t.Fatalf("lookup unknown: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected no tasks for unknown PR number, got %v", none)
	}
}
