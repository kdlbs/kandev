package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newSearchTestRepo creates a test repo with a minimal tasks table for
// search tests. ADR 0005 Wave F dropped tasks.assignee_agent_profile_id;
// we keep the column on the test schema as a transient holding place so
// existing fixtures still compile, but the runner projection in repo
// queries reads from workflow_step_participants — those stub tables are
// created here too.
func newSearchTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('critical','high','medium','low')),
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create workflows table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			agent_profile_id TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create workflow_steps table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		t.Fatalf("create workflow_step_participants table: %v", err)
	}
	return repo
}

func insertTask(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, wsID, title, desc, identifier string) {
	t.Helper()
	_, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, description, identifier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, desc, identifier)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func TestSearchTasks_MatchesTitle(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Fix login bug", "desc here", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Add payment flow", "payment desc", "KAN-2")
	insertTask(t, repo, ctx, "t3", "ws1", "Refactor auth", "auth refactor", "KAN-3")

	results, err := repo.SearchTasks(ctx, "ws1", "login", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Fix login bug" {
		t.Errorf("expected title 'Fix login bug', got %q", results[0].Title)
	}
}

func TestSearchTasks_MatchesIdentifier(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Some task", "some desc", "KAN-42")
	insertTask(t, repo, ctx, "t2", "ws1", "Another task", "another desc", "KAN-99")

	results, err := repo.SearchTasks(ctx, "ws1", "KAN-42", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Identifier != "KAN-42" {
		t.Errorf("expected identifier 'KAN-42', got %q", results[0].Identifier)
	}
}

func TestSearchTasks_MatchesDescription(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Title A", "fix the authentication module", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Title B", "update the readme", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "authentication", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task t1, got %q", results[0].ID)
	}
}

func TestSearchTasks_NoResults(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Some task", "desc", "KAN-1")

	results, err := repo.SearchTasks(ctx, "ws1", "nonexistent", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchTasks_RespectsLimit(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("t%d", i)
		ident := fmt.Sprintf("KAN-%d", i)
		insertTask(t, repo, ctx, id, "ws1", "Match task", "desc", ident)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "Match", 3)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

func TestSearchTasks_WorkspaceIsolation(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Shared title", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws2", "Shared title", "desc", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "Shared", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ws1, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task from ws1, got %q", results[0].ID)
	}
}

func TestSearchTasks_ExcludesArchived(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Active task", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Archived task", "desc", "KAN-2")
	// Archive t2
	_, err := repo.ExecRaw(ctx, `UPDATE tasks SET archived_at = datetime('now') WHERE id = ?`, "t2")
	if err != nil {
		t.Fatalf("archive task: %v", err)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "task", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (archived excluded), got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected active task, got %q", results[0].ID)
	}
}

func TestSearchTasks_ExcludesEphemeral(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Normal task", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Ephemeral task", "desc", "KAN-2")
	// Mark t2 as ephemeral
	_, err := repo.ExecRaw(ctx, `UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`, "t2")
	if err != nil {
		t.Fatalf("set ephemeral: %v", err)
	}

	results, err := repo.SearchTasks(ctx, "ws1", "task", 50)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (ephemeral excluded), got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected normal task, got %q", results[0].ID)
	}
}

// -- FTS5 tests --

// newFTSTestRepo creates a test repo with a tasks table and FTS5 index.
// Skips the test if the SQLite build does not include FTS5.
func newFTSTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// Create FTS5 virtual table and sync triggers.
	_, err := repo.ExecRaw(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(
			title, description, identifier,
			content='tasks',
			content_rowid='rowid'
		)
	`)
	if err != nil {
		t.Skipf("FTS5 not available in this SQLite build: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS tasks_fts_insert AFTER INSERT ON tasks BEGIN
			INSERT INTO tasks_fts(rowid, title, description, identifier)
			VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
		END`,
		`CREATE TRIGGER IF NOT EXISTS tasks_fts_update AFTER UPDATE ON tasks BEGIN
			INSERT INTO tasks_fts(tasks_fts, rowid, title, description, identifier)
			VALUES('delete', old.rowid, old.title, COALESCE(old.description,''), COALESCE(old.identifier,''));
			INSERT INTO tasks_fts(rowid, title, description, identifier)
			VALUES (new.rowid, new.title, COALESCE(new.description,''), COALESCE(new.identifier,''));
		END`,
		`CREATE TRIGGER IF NOT EXISTS tasks_fts_delete AFTER DELETE ON tasks BEGIN
			INSERT INTO tasks_fts(tasks_fts, rowid, title, description, identifier)
			VALUES('delete', old.rowid, old.title, COALESCE(old.description,''), COALESCE(old.identifier,''));
		END`,
	} {
		if _, err := repo.ExecRaw(ctx, stmt); err != nil {
			t.Fatalf("create FTS trigger: %v", err)
		}
	}

	return repo
}

func TestSearchTasksFTS_MatchesTitlePrefix(t *testing.T) {
	repo := newFTSTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Fix login bug", "desc here", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Add payment flow", "payment desc", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "login", 50)
	if err != nil {
		t.Fatalf("SearchTasks FTS: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Fix login bug" {
		t.Errorf("expected title 'Fix login bug', got %q", results[0].Title)
	}
}

func TestSearchTasksFTS_MatchesIdentifier(t *testing.T) {
	repo := newFTSTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Some task", "some desc", "KAN-42")
	insertTask(t, repo, ctx, "t2", "ws1", "Another task", "another desc", "KAN-99")

	results, err := repo.SearchTasks(ctx, "ws1", "KAN-42", 50)
	if err != nil {
		t.Fatalf("SearchTasks FTS: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Identifier != "KAN-42" {
		t.Errorf("expected identifier 'KAN-42', got %q", results[0].Identifier)
	}
}

func TestSearchTasksFTS_PrefixMatch(t *testing.T) {
	repo := newFTSTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Authentication module", "auth desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws1", "Billing module", "bill desc", "KAN-2")

	// "auth" should prefix-match "Authentication"
	results, err := repo.SearchTasks(ctx, "ws1", "auth", 50)
	if err != nil {
		t.Fatalf("SearchTasks FTS prefix: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for prefix match, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task t1, got %q", results[0].ID)
	}
}

func TestSearchTasksFTS_FallbackToLike(t *testing.T) {
	// Use a repo without FTS5 table -- should fall back to LIKE search.
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Fix login bug", "desc here", "KAN-1")

	results, err := repo.SearchTasks(ctx, "ws1", "login", 50)
	if err != nil {
		t.Fatalf("SearchTasks fallback: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result via LIKE fallback, got %d", len(results))
	}
}

func TestSearchTasksFTS_WorkspaceIsolation(t *testing.T) {
	repo := newFTSTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "t1", "ws1", "Shared title", "desc", "KAN-1")
	insertTask(t, repo, ctx, "t2", "ws2", "Shared title", "desc", "KAN-2")

	results, err := repo.SearchTasks(ctx, "ws1", "Shared", 50)
	if err != nil {
		t.Fatalf("SearchTasks FTS: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ws1, got %d", len(results))
	}
	if results[0].ID != "t1" {
		t.Errorf("expected task from ws1, got %q", results[0].ID)
	}
}

// -- ListTasksByWorkspace and GetTaskByID tests --

func TestListTasksByWorkspace_ReturnsTasksForWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "i1", "ws-a", "Issue Alpha", "alpha desc", "KAN-1")
	insertTask(t, repo, ctx, "i2", "ws-a", "Issue Beta", "beta desc", "KAN-2")
	insertTask(t, repo, ctx, "i3", "ws-b", "Other WS Issue", "other desc", "OTH-1")

	issues, err := repo.ListTasksByWorkspace(ctx, "ws-a", true)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues for ws-a, got %d", len(issues))
	}
	for _, iss := range issues {
		if iss.WorkspaceID != "ws-a" {
			t.Errorf("unexpected workspace_id %q", iss.WorkspaceID)
		}
	}
}

func TestListTasksByWorkspace_ExcludesArchivedAndEphemeral(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "n1", "ws-c", "Normal task", "", "KAN-1")
	now := "2025-01-01 00:00:00"
	_, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks
			(id, workspace_id, title, identifier, is_ephemeral, created_at, updated_at)
		VALUES ('eph', 'ws-c', 'Ephemeral', 'KAN-2', 1, ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert ephemeral task: %v", err)
	}
	_, err = repo.ExecRaw(ctx, `
		INSERT INTO tasks
			(id, workspace_id, title, identifier, archived_at, created_at, updated_at)
		VALUES ('arc', 'ws-c', 'Archived', 'KAN-3', ?, ?, ?)
	`, now, now, now)
	if err != nil {
		t.Fatalf("insert archived task: %v", err)
	}

	issues, err := repo.ListTasksByWorkspace(ctx, "ws-c", true)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (normal only), got %d", len(issues))
	}
	if issues[0].ID != "n1" {
		t.Errorf("expected task n1, got %q", issues[0].ID)
	}
}

// TestListTasksByWorkspace_SystemTaskFilter verifies that tasks living
// in a kandev-managed system workflow (template_id = "routine") are
// excluded by default and re-included when includeSystem=true. IsSystem
// is set on the projection so the UI can mark them.
func TestListTasksByWorkspace_SystemTaskFilter(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflows (id, workspace_id, workflow_template_id, name, created_at, updated_at)
		VALUES ('wf-user', 'ws-x', 'office-default', 'User', datetime('now'), datetime('now')),
		       ('wf-sys',  'ws-x', 'routine',        'System', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed workflows: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_id, title, identifier, created_at, updated_at)
		VALUES ('user-1', 'ws-x', 'wf-user', 'User Task',  'KAN-1', datetime('now'), datetime('now')),
		       ('sys-1',  'ws-x', 'wf-sys',  'Routine fire','KAN-2', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	user, err := repo.ListTasksByWorkspace(ctx, "ws-x", false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace(false): %v", err)
	}
	if len(user) != 1 || user[0].ID != "user-1" {
		t.Errorf("default list should hide system task, got %+v", user)
	}
	if user[0].IsSystem {
		t.Errorf("user task should not be flagged is_system")
	}

	all, err := repo.ListTasksByWorkspace(ctx, "ws-x", true)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace(true): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("includeSystem should return both, got %d", len(all))
	}
	var sys *sqlite.TaskRow
	for _, r := range all {
		if r.ID == "sys-1" {
			sys = r
		}
	}
	if sys == nil {
		t.Fatal("system task missing when included")
	}
	if !sys.IsSystem {
		t.Errorf("system task should be flagged is_system, got %+v", sys)
	}
}

func TestGetTaskByID_ReturnsIssue(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "get1", "ws-d", "Get me", "get desc", "KAN-10")

	iss, err := repo.GetTaskByID(ctx, "get1")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if iss == nil {
		t.Fatal("expected issue, got nil")
	}
	if iss.ID != "get1" {
		t.Errorf("expected id get1, got %q", iss.ID)
	}
	if iss.Title != "Get me" {
		t.Errorf("expected title 'Get me', got %q", iss.Title)
	}
}

func TestGetTaskByID_ReturnsNilForMissingID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	iss, err := repo.GetTaskByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("expected no error for missing id, got: %v", err)
	}
	if iss != nil {
		t.Errorf("expected nil for missing id, got %+v", iss)
	}
}

// -- CountActionableTasksForAgent tests --

// insertAssignedTask inserts a task with a given state and assignee.
// ADR 0005 Wave F: assignee lives in workflow_step_participants.
// We seed a synthetic step id derived from the task id so the test
// keeps the (step, task) participant key well-formed.
func insertAssignedTask(
	t *testing.T, repo *sqlite.Repository, ctx context.Context,
	id, wsID, agentID, state string, archivedAt string,
) {
	t.Helper()
	stepID := "step-" + id
	if archivedAt != "" {
		_, err := repo.ExecRaw(ctx, `
			INSERT INTO tasks (id, workspace_id, workflow_step_id, state, title, archived_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		`, id, wsID, stepID, state, id, archivedAt)
		if err != nil {
			t.Fatalf("insert archived task %s: %v", id, err)
		}
	} else {
		_, err := repo.ExecRaw(ctx, `
			INSERT INTO tasks (id, workspace_id, workflow_step_id, state, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		`, id, wsID, stepID, state, id)
		if err != nil {
			t.Fatalf("insert task %s: %v", id, err)
		}
	}
	if agentID != "" {
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
			VALUES (?, ?, ?, 'runner', ?, 0, 0)
		`, "p-"+id, stepID, id, agentID); err != nil {
			t.Fatalf("insert runner participant for %s: %v", id, err)
		}
	}
}

func TestCountActionableTasksForAgent_CountsTodoAndInProgress(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	agentID := "agent-count-1"

	insertAssignedTask(t, repo, ctx, "task-todo", "ws1", agentID, "TODO", "")
	insertAssignedTask(t, repo, ctx, "task-inprogress", "ws1", agentID, "IN_PROGRESS", "")
	insertAssignedTask(t, repo, ctx, "task-done", "ws1", agentID, "DONE", "")
	insertAssignedTask(t, repo, ctx, "task-cancelled", "ws1", agentID, "CANCELLED", "")
	insertAssignedTask(t, repo, ctx, "task-in-review", "ws1", agentID, "IN_REVIEW", "")

	count, err := repo.CountActionableTasksForAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("CountActionableTasksForAgent: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 actionable tasks (TODO + IN_PROGRESS), got %d", count)
	}
}

func TestCountActionableTasksForAgent_ExcludesArchived(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	agentID := "agent-count-2"

	insertAssignedTask(t, repo, ctx, "task-todo-active", "ws1", agentID, "TODO", "")
	insertAssignedTask(t, repo, ctx, "task-todo-archived", "ws1", agentID, "TODO", "2025-01-01 00:00:00")

	count, err := repo.CountActionableTasksForAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("CountActionableTasksForAgent: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (archived excluded), got %d", count)
	}
}

func TestCountActionableTasksForAgent_ReturnsZeroForNoTasks(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	count, err := repo.CountActionableTasksForAgent(ctx, "agent-no-tasks")
	if err != nil {
		t.Fatalf("CountActionableTasksForAgent: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for agent with no tasks, got %d", count)
	}
}

func TestCountActionableTasksForAgent_AgentIsolation(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertAssignedTask(t, repo, ctx, "task-a", "ws1", "agent-a", "TODO", "")
	insertAssignedTask(t, repo, ctx, "task-b", "ws1", "agent-b", "TODO", "")
	insertAssignedTask(t, repo, ctx, "task-c", "ws1", "agent-b", "IN_PROGRESS", "")

	countA, err := repo.CountActionableTasksForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("CountActionableTasksForAgent agent-a: %v", err)
	}
	if countA != 1 {
		t.Errorf("agent-a: expected 1, got %d", countA)
	}

	countB, err := repo.CountActionableTasksForAgent(ctx, "agent-b")
	if err != nil {
		t.Fatalf("CountActionableTasksForAgent agent-b: %v", err)
	}
	if countB != 2 {
		t.Errorf("agent-b: expected 2, got %d", countB)
	}
}
