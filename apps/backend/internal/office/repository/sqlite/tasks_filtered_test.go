package sqlite_test

import (
	"context"
	"fmt"
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
			origin TEXT DEFAULT 'manual',
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
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
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

func TestListTasksFiltered_AssigneeAndProjectFilters(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks
			(id, workspace_id, workflow_step_id, project_id, title, state, priority, created_at, updated_at)
		VALUES
			('af-1', 'ws-1', 'step-1', 'proj-a', 'One',   'TODO', 'medium', '2026-04-01T12:00:00Z', '2026-04-01T12:00:00Z'),
			('af-2', 'ws-1', 'step-2', 'proj-a', 'Two',   'TODO', 'medium', '2026-04-02T12:00:00Z', '2026-04-02T12:00:00Z'),
			('af-3', 'ws-1', 'step-3', 'proj-b', 'Three', 'TODO', 'medium', '2026-04-03T12:00:00Z', '2026-04-03T12:00:00Z')
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, position)
		VALUES
			('p-1', 'step-1', 'af-1', 'runner', 'agent-x', 0),
			('p-2', 'step-2', 'af-2', 'runner', 'agent-y', 0),
			('p-3', 'step-3', 'af-3', 'runner', 'agent-x', 0)
	`); err != nil {
		t.Fatalf("seed runners: %v", err)
	}

	// The sort is stated explicitly rather than relying on the zero-value
	// default, so a change of default surfaces as a sort failure elsewhere
	// instead of a confusing "wrong ids" failure here.
	ascByUpdated := sqlite.ListTasksOptions{SortField: sqlite.TaskSortUpdatedAt, SortDesc: false}

	opts := ascByUpdated
	opts.AssigneeID = "agent-x"
	page, err := repo.ListTasksFiltered(ctx, "ws-1", opts)
	if err != nil {
		t.Fatalf("filter by assignee: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 2 || got[0] != "af-1" || got[1] != "af-3" {
		t.Fatalf("assignee filter = %v, want [af-1 af-3]", got)
	}
	if page.Tasks[0].AssigneeAgentProfileID != "agent-x" {
		t.Errorf("assignee = %q, want agent-x projected onto the row", page.Tasks[0].AssigneeAgentProfileID)
	}

	opts = ascByUpdated
	opts.ProjectID = "proj-a"
	page, err = repo.ListTasksFiltered(ctx, "ws-1", opts)
	if err != nil {
		t.Fatalf("filter by project: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 2 || got[0] != "af-1" || got[1] != "af-2" {
		t.Fatalf("project filter = %v, want [af-1 af-2]", got)
	}

	opts = ascByUpdated
	opts.AssigneeID = "agent-x"
	opts.ProjectID = "proj-b"
	page, err = repo.ListTasksFiltered(ctx, "ws-1", opts)
	if err != nil {
		t.Fatalf("combined filter: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 1 || got[0] != "af-3" {
		t.Fatalf("combined filter = %v, want [af-3]", got)
	}
}

// The cursor value is read off the sort column, so each sort field needs
// its own round-trip: a created_at cursor fed into a priority sort would
// silently return the wrong page.
func TestListTasksFiltered_CursorPerSortField(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, state, priority, created_at, updated_at) VALUES
			('cs-crit', 'ws-1', 'a', 'TODO', 'critical', '2026-04-03T12:00:00Z', '2026-04-01T12:00:00Z'),
			('cs-high', 'ws-1', 'b', 'TODO', 'high',     '2026-04-02T12:00:00Z', '2026-04-02T12:00:00Z'),
			('cs-low',  'ws-1', 'c', 'TODO', 'low',      '2026-04-01T12:00:00Z', '2026-04-03T12:00:00Z')
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	// created_at DESC is the reverse of updated_at DESC for this fixture,
	// so a wrong sort column would be visible.
	page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskSortCreatedAt, SortDesc: true, Limit: 2,
	})
	if err != nil {
		t.Fatalf("created_at page 1: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 2 || got[0] != "cs-crit" || got[1] != "cs-high" {
		t.Fatalf("created_at page 1 = %v, want [cs-crit cs-high]", got)
	}
	if page.NextCursor != page.Tasks[1].CreatedAt {
		t.Errorf("NextCursor = %q, want the tail row's created_at %q", page.NextCursor, page.Tasks[1].CreatedAt)
	}
	if page.NextID != "cs-high" {
		t.Errorf("NextID = %q, want cs-high", page.NextID)
	}
	page2, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskSortCreatedAt, SortDesc: true, Limit: 2,
		CursorValue: page.NextCursor, CursorID: page.NextID,
	})
	if err != nil {
		t.Fatalf("created_at page 2: %v", err)
	}
	if got := taskIDs(page2.Tasks); len(got) != 1 || got[0] != "cs-low" {
		t.Fatalf("created_at page 2 = %v, want [cs-low]", got)
	}

	// priority sorts lexically on the column value: critical < high < low.
	page, err = repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskSortPriority, Limit: 2,
	})
	if err != nil {
		t.Fatalf("priority page 1: %v", err)
	}
	if got := taskIDs(page.Tasks); len(got) != 2 || got[0] != "cs-crit" || got[1] != "cs-high" {
		t.Fatalf("priority page 1 = %v, want [cs-crit cs-high] ascending", got)
	}
	if page.NextCursor != "high" {
		t.Errorf("NextCursor = %q, want the tail row's priority 'high'", page.NextCursor)
	}
	page2, err = repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{
		SortField: sqlite.TaskSortPriority, Limit: 2,
		CursorValue: page.NextCursor, CursorID: page.NextID,
	})
	if err != nil {
		t.Fatalf("priority page 2: %v", err)
	}
	if got := taskIDs(page2.Tasks); len(got) != 1 || got[0] != "cs-low" {
		t.Fatalf("priority page 2 = %v, want [cs-low]", got)
	}
	if page2.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty on the final page", page2.NextCursor)
	}
}

// An out-of-range limit is clamped to 100 rather than rejected. Seeding 101
// rows is what makes the clamp observable: with fewer rows than the clamp,
// every limit ≥ the row count returns the same page and the assertion would
// hold no matter what resolveListTasksOptions did.
func TestListTasksFiltered_ClampsLimit(t *testing.T) {
	repo := newTestRepo(t)
	ensureTasksTable(t, repo)
	ctx := context.Background()

	for i := 0; i < 101; i++ {
		insertTaskRow(t, repo,
			fmt.Sprintf("cl-%03d", i), "ws-1", "TODO", "medium",
			fmt.Sprintf("2026-04-01T12:%02d:00Z", i%60))
	}

	for _, limit := range []int{0, -5, 501} {
		page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{Limit: limit})
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(page.Tasks) != 100 {
			t.Errorf("limit %d returned %d rows, want the clamped 100", limit, len(page.Tasks))
		}
		// 101 rows against a clamp of 100 means there is a next page.
		if page.NextCursor == "" {
			t.Errorf("limit %d NextCursor is empty, want the 101st row to be reachable", limit)
		}
		if page.NextID == "" {
			t.Errorf("limit %d NextID is empty, want the tail row id", limit)
		}
	}

	// An in-range limit is honoured verbatim, so the clamp is not blanket.
	page, err := repo.ListTasksFiltered(ctx, "ws-1", sqlite.ListTasksOptions{Limit: 7})
	if err != nil {
		t.Fatalf("limit 7: %v", err)
	}
	if len(page.Tasks) != 7 {
		t.Errorf("limit 7 returned %d rows, want 7", len(page.Tasks))
	}
}

func taskIDs(tasks []*sqlite.TaskRow) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return out
}
