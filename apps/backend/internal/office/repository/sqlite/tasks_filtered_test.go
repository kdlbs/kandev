package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// ensureTasksTable creates the minimal schema ListTasksFiltered touches.
// Includes the workflow_* tables the RunnerProjection helper joins on,
// so SELECT statements don't error on missing tables. Idempotent.
func ensureTasksTable(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			state TEXT NOT NULL DEFAULT 'TODO',
			priority TEXT NOT NULL DEFAULT 'medium',
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			assignee_agent_profile_id TEXT DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			archived_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			role TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			agent_profile_id TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := repo.ExecRaw(context.Background(), s); err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
}

// insertTaskRow inserts a single task with the given fields.
func insertTaskRow(t *testing.T, repo *sqlite.Repository, id, workspaceID, state, priority, updatedAt string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id, title, state, priority, created_at, updated_at, is_ephemeral)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		id, workspaceID, id+"-title", state, priority, updatedAt, updatedAt,
	)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func TestListTasksFiltered_Pagination(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	// Insert 5 tasks with distinct updated_at so the keyset cursor is unambiguous.
	// Use ISO/RFC3339 strings — the mattn/go-sqlite3 driver normalises DATETIME
	// columns to that format on read, so this matches what SELECT returns.
	for i, ts := range []string{
		"2026-04-01T12:00:00Z", "2026-04-02T12:00:00Z", "2026-04-03T12:00:00Z",
		"2026-04-04T12:00:00Z", "2026-04-05T12:00:00Z",
	} {
		insertTaskRow(t, repo, "t"+string(rune('0'+i)), "ws-1", "TODO", "medium", ts)
	}

	page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		Limit: 2, SortDesc: true,
	})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if got := len(page.Tasks); got != 2 {
		t.Fatalf("page 1 size = %d, want 2", got)
	}
	if page.NextCursor == "" {
		t.Fatal("expected NextCursor on first page")
	}
	// Newest first: t4 (2026-04-05), t3 (2026-04-04)
	if page.Tasks[0].ID != "t4" || page.Tasks[1].ID != "t3" {
		t.Errorf("page 1 ids = %s,%s; want t4,t3", page.Tasks[0].ID, page.Tasks[1].ID)
	}

	page2, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		Limit: 2, SortDesc: true, CursorValue: page.NextCursor, CursorID: page.NextID,
	})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if got := len(page2.Tasks); got != 2 {
		t.Fatalf("page 2 size = %d, want 2", got)
	}
	if page2.Tasks[0].ID != "t2" || page2.Tasks[1].ID != "t1" {
		t.Errorf("page 2 ids = %s,%s; want t2,t1", page2.Tasks[0].ID, page2.Tasks[1].ID)
	}

	page3, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		Limit: 2, SortDesc: true, CursorValue: page2.NextCursor, CursorID: page2.NextID,
	})
	if err != nil {
		t.Fatalf("list page 3: %v", err)
	}
	if got := len(page3.Tasks); got != 1 {
		t.Fatalf("page 3 size = %d, want 1", got)
	}
	if page3.NextCursor != "" {
		t.Errorf("expected empty NextCursor on final page, got %q", page3.NextCursor)
	}
}

func TestListTasksFiltered_StatusAndPriorityFilters(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	insertTaskRow(t, repo, "a", "ws-1", "TODO", "high", "2026-04-01T12:00:00Z")
	insertTaskRow(t, repo, "b", "ws-1", "IN_PROGRESS", "high", "2026-04-02T12:00:00Z")
	insertTaskRow(t, repo, "c", "ws-1", "COMPLETED", "low", "2026-04-03T12:00:00Z")
	insertTaskRow(t, repo, "d", "ws-1", "IN_PROGRESS", "medium", "2026-04-04T12:00:00Z")

	page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		Status: []string{"IN_PROGRESS"},
	})
	if err != nil {
		t.Fatalf("list filtered by status: %v", err)
	}
	if len(page.Tasks) != 2 {
		t.Fatalf("status filter rows = %d, want 2", len(page.Tasks))
	}

	page, err = repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		Status:   []string{"IN_PROGRESS"},
		Priority: []string{"high"},
	})
	if err != nil {
		t.Fatalf("list filtered combined: %v", err)
	}
	if len(page.Tasks) != 1 || page.Tasks[0].ID != "b" {
		t.Fatalf("combined filter rows = %v, want [b]", taskIDs(page.Tasks))
	}
}

// SortDesc=false on a date column must produce ascending order.
// Regression for a bug where the dir flip was gated on TaskSortPriority,
// silently dropping order=asc for updated_at / created_at sorts.
func TestListTasksFiltered_AscendingSort(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	insertTaskRow(t, repo, "t1", "ws-1", "TODO", "medium", "2026-04-01T12:00:00Z")
	insertTaskRow(t, repo, "t2", "ws-1", "TODO", "medium", "2026-04-02T12:00:00Z")
	insertTaskRow(t, repo, "t3", "ws-1", "TODO", "medium", "2026-04-03T12:00:00Z")

	page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskSortUpdatedAt,
		SortDesc:  false,
	})
	if err != nil {
		t.Fatalf("list ascending: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 3 || got[0] != "t1" || got[2] != "t3" {
		t.Fatalf("ascending order = %v, want [t1 t2 t3]", got)
	}
}

func TestListTasksFiltered_RejectsInvalidSort(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.ListTasksFiltered(context.Background(), "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskListSortField("title; DROP TABLE tasks;--"),
	})
	if err == nil {
		t.Fatal("expected error for invalid sort field")
	}
}

func taskIDs(tasks []*sqlite.TaskRow) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return out
}
