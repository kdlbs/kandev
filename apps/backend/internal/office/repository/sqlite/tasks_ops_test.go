package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestGetTaskProjectID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, project_id, title, created_at, updated_at) VALUES
			('tp-with', 'ws-1', 'proj-9', 'a', datetime('now'), datetime('now')),
			('tp-none', 'ws-1', '',       'b', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = NULL WHERE id = 'tp-none'`); err != nil {
		t.Fatalf("null out project_id: %v", err)
	}

	got, err := repo.GetTaskProjectID(ctx, "tp-with")
	if err != nil {
		t.Fatalf("GetTaskProjectID: %v", err)
	}
	if got != "proj-9" {
		t.Errorf("got %q, want proj-9", got)
	}

	// A NULL project_id reads back as the empty string, not an error.
	got, err = repo.GetTaskProjectID(ctx, "tp-none")
	if err != nil {
		t.Fatalf("GetTaskProjectID (NULL column): %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	if _, err = repo.GetTaskProjectID(ctx, "missing"); err == nil {
		t.Error("GetTaskProjectID(missing) = nil error, want not-found")
	} else if !strings.Contains(err.Error(), "task not found: missing") {
		t.Errorf("error = %q, want it to name the task", err)
	}
}

func TestGetTaskWorkspaceID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "tw-1", "ws-lookup", "Title", "", "KAN-1")

	cases := []struct {
		name   string
		taskID string
		want   string
	}{
		{"existing task", "tw-1", "ws-lookup"},
		{"missing task", "tw-nope", ""},
		{"empty id short-circuits", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetTaskWorkspaceID(ctx, tc.taskID)
			if err != nil {
				t.Fatalf("GetTaskWorkspaceID: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpdateTaskState(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "us-1", "ws-1", "Title", "", "KAN-1")
	insertTask(t, repo, ctx, "us-2", "ws-1", "Other", "", "KAN-2")

	if err := repo.UpdateTaskState(ctx, "us-1", "IN_PROGRESS"); err != nil {
		t.Fatalf("UpdateTaskState: %v", err)
	}
	row, err := repo.GetTaskByID(ctx, "us-1")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if row.Status != "IN_PROGRESS" {
		t.Errorf("state = %q, want IN_PROGRESS", row.Status)
	}
	// The neighbouring row keeps its state.
	if other, err := repo.GetTaskByID(ctx, "us-2"); err != nil {
		t.Fatalf("GetTaskByID(us-2): %v", err)
	} else if other.Status != "TODO" {
		t.Errorf("us-2 state = %q, want the untouched TODO", other.Status)
	}

	err = repo.UpdateTaskState(ctx, "missing", "COMPLETED")
	if err == nil {
		t.Fatal("UpdateTaskState(missing) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "task not found: missing") {
		t.Errorf("error = %q, want it to name the task", err)
	}
}

func TestUpdateTaskPriorityAndProjectID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "sc-1", "ws-1", "Title", "", "KAN-1")

	if err := repo.UpdateTaskPriority(ctx, "sc-1", "critical"); err != nil {
		t.Fatalf("UpdateTaskPriority: %v", err)
	}
	if err := repo.UpdateTaskProjectID(ctx, "sc-1", "proj-7"); err != nil {
		t.Fatalf("UpdateTaskProjectID: %v", err)
	}
	row, err := repo.GetTaskByID(ctx, "sc-1")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if row.Priority != "critical" {
		t.Errorf("priority = %q, want critical", row.Priority)
	}
	if row.ProjectID != "proj-7" {
		t.Errorf("project_id = %q, want proj-7", row.ProjectID)
	}

	// An empty project id clears the assignment.
	if err := repo.UpdateTaskProjectID(ctx, "sc-1", ""); err != nil {
		t.Fatalf("UpdateTaskProjectID(clear): %v", err)
	}
	if row, err = repo.GetTaskByID(ctx, "sc-1"); err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if row.ProjectID != "" {
		t.Errorf("project_id = %q, want it cleared", row.ProjectID)
	}

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"priority", func() error { return repo.UpdateTaskPriority(ctx, "missing", "low") }},
		{"project id", func() error { return repo.UpdateTaskProjectID(ctx, "missing", "p") }},
	} {
		t.Run(tc.name+" on a missing task", func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("got nil error, want not-found")
			}
			if !strings.Contains(err.Error(), "task not found: missing") {
				t.Errorf("error = %q, want it to name the task", err)
			}
		})
	}
}

func TestUpdateTaskParentID_NormalisesInheritParentWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, parent_id, title, metadata, created_at, updated_at) VALUES
			('pa-inherit', 'ws-1', 'old-parent', 'a', '{"workspace":{"mode":"inherit_parent"}}',
			 datetime('now'), datetime('now')),
			('pa-shared',  'ws-1', 'old-parent', 'b', '{"workspace":{"mode":"new_workspace"}}',
			 datetime('now'), datetime('now')),
			('pa-badjson', 'ws-1', 'old-parent', 'c', 'not json',
			 datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	if err := repo.UpdateTaskParentID(ctx, "pa-inherit", "new-parent"); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}
	row, err := repo.GetTaskByID(ctx, "pa-inherit")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if row.ParentID != "new-parent" {
		t.Errorf("parent_id = %q, want new-parent", row.ParentID)
	}
	if mode := taskWorkspaceMode(t, repo, "pa-inherit"); mode != "shared_group" {
		t.Errorf("workspace.mode = %q, want shared_group after re-parenting", mode)
	}

	// A non-inherit mode is left alone.
	if err := repo.UpdateTaskParentID(ctx, "pa-shared", "new-parent"); err != nil {
		t.Fatalf("UpdateTaskParentID(pa-shared): %v", err)
	}
	if mode := taskWorkspaceMode(t, repo, "pa-shared"); mode != "new_workspace" {
		t.Errorf("workspace.mode = %q, want new_workspace untouched", mode)
	}

	// Invalid JSON metadata is left verbatim rather than erroring.
	if err := repo.UpdateTaskParentID(ctx, "pa-badjson", "new-parent"); err != nil {
		t.Fatalf("UpdateTaskParentID(pa-badjson): %v", err)
	}
	var raw string
	if err := repo.ReaderDB().Get(&raw, `SELECT metadata FROM tasks WHERE id = 'pa-badjson'`); err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if raw != "not json" {
		t.Errorf("metadata = %q, want it preserved verbatim", raw)
	}

	err = repo.UpdateTaskParentID(ctx, "missing", "p")
	if err == nil {
		t.Fatal("UpdateTaskParentID(missing) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "task not found: missing") {
		t.Errorf("error = %q, want it to name the task", err)
	}
}

// A repeated PATCH with the same parent must not rewrite the workspace mode.
func TestUpdateTaskParentID_SameParentIsANoOpForWorkspaceMode(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, parent_id, title, metadata, created_at, updated_at)
		VALUES ('pa-same', 'ws-1', 'parent-1', 'a', '{"workspace":{"mode":"inherit_parent"}}',
		        datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := repo.UpdateTaskParentID(ctx, "pa-same", "parent-1"); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}
	if mode := taskWorkspaceMode(t, repo, "pa-same"); mode != "inherit_parent" {
		t.Errorf("workspace.mode = %q, want inherit_parent kept when the parent does not change", mode)
	}
}

func taskWorkspaceMode(t *testing.T, repo *sqlite.Repository, taskID string) string {
	t.Helper()
	var mode string
	if err := repo.ReaderDB().Get(&mode,
		`SELECT COALESCE(json_extract(metadata, '$.workspace.mode'), '') FROM tasks WHERE id = ?`,
		taskID); err != nil {
		t.Fatalf("read workspace mode for %s: %v", taskID, err)
	}
	return mode
}

func TestGetTaskBasicInfo(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, description, identifier, priority, project_id, created_at, updated_at)
		VALUES ('bi-1', 'ws-1', 'Ship it', 'the description', 'KAN-7', 'high', 'proj-2',
		        datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	got, err := repo.GetTaskBasicInfo(ctx, "bi-1")
	if err != nil {
		t.Fatalf("GetTaskBasicInfo: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want the task info")
	}
	want := sqlite.TaskBasicInfo{
		Title: "Ship it", Description: "the description",
		Identifier: "KAN-7", Priority: "high", ProjectID: "proj-2",
	}
	if *got != want {
		t.Errorf("got %+v, want %+v", *got, want)
	}

	// A missing task is nil, nil — not an error.
	got, err = repo.GetTaskBasicInfo(ctx, "missing")
	if err != nil {
		t.Fatalf("GetTaskBasicInfo(missing) = %v, want nil error", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestGetTaskBasicInfo_NullColumnsCoalesce(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('bi-null', 'ws-1', 'Bare', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET description = NULL, identifier = NULL, project_id = NULL
		WHERE id = 'bi-null'
	`); err != nil {
		t.Fatalf("null out columns: %v", err)
	}

	got, err := repo.GetTaskBasicInfo(ctx, "bi-null")
	if err != nil {
		t.Fatalf("GetTaskBasicInfo: %v", err)
	}
	want := sqlite.TaskBasicInfo{Title: "Bare", Priority: "medium"}
	if *got != want {
		t.Errorf("got %+v, want %+v (NULL columns coalesced to empty strings)", *got, want)
	}
}

func TestUpdateTaskAssignee_WritesUpdatesAndClearsRunnerRow(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, title, created_at, updated_at)
		VALUES ('as-1', 'ws-1', 'step-1', 'Assign me', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := repo.UpdateTaskAssignee(ctx, "as-1", "agent-one"); err != nil {
		t.Fatalf("UpdateTaskAssignee (insert): %v", err)
	}
	if got := taskRunner(t, repo, "as-1"); got != "agent-one" {
		t.Fatalf("runner = %q, want agent-one", got)
	}
	if n := runnerRowCount(t, repo, "as-1"); n != 1 {
		t.Fatalf("runner rows = %d, want 1", n)
	}

	// A second assignment updates the existing row instead of adding one.
	if err := repo.UpdateTaskAssignee(ctx, "as-1", "agent-two"); err != nil {
		t.Fatalf("UpdateTaskAssignee (update): %v", err)
	}
	if got := taskRunner(t, repo, "as-1"); got != "agent-two" {
		t.Errorf("runner = %q, want agent-two", got)
	}
	if n := runnerRowCount(t, repo, "as-1"); n != 1 {
		t.Errorf("runner rows = %d, want the row updated in place", n)
	}

	// An empty assignee deletes the runner row.
	if err := repo.UpdateTaskAssignee(ctx, "as-1", ""); err != nil {
		t.Fatalf("UpdateTaskAssignee (clear): %v", err)
	}
	if n := runnerRowCount(t, repo, "as-1"); n != 0 {
		t.Errorf("runner rows = %d, want 0 after clearing", n)
	}
	if got := taskRunner(t, repo, "as-1"); got != "" {
		t.Errorf("runner = %q, want empty after clearing", got)
	}

	err := repo.UpdateTaskAssignee(ctx, "missing", "agent-one")
	if err == nil {
		t.Fatal("UpdateTaskAssignee(missing) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "task not found: missing") {
		t.Errorf("error = %q, want it to name the task", err)
	}
}

// Tasks created outside a workflow have no workflow_step_id; the runner row
// is keyed on the empty step so the projection still resolves it.
func TestUpdateTaskAssignee_WorksWithoutAWorkflowStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, title, created_at, updated_at)
		VALUES ('as-nostep', 'ws-1', '', 'Channel task', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := repo.UpdateTaskAssignee(ctx, "as-nostep", "agent-channel"); err != nil {
		t.Fatalf("UpdateTaskAssignee: %v", err)
	}
	if got := taskRunner(t, repo, "as-nostep"); got != "agent-channel" {
		t.Errorf("runner = %q, want agent-channel", got)
	}
}

// Only the target task's runner row moves.
func TestUpdateTaskAssignee_LeavesOtherTasksAlone(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, title, created_at, updated_at) VALUES
			('as-a', 'ws-1', 'step-shared', 'A', datetime('now'), datetime('now')),
			('as-b', 'ws-1', 'step-shared', 'B', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	if err := repo.UpdateTaskAssignee(ctx, "as-a", "agent-a"); err != nil {
		t.Fatalf("assign as-a: %v", err)
	}
	if err := repo.UpdateTaskAssignee(ctx, "as-b", "agent-b"); err != nil {
		t.Fatalf("assign as-b: %v", err)
	}
	if err := repo.UpdateTaskAssignee(ctx, "as-b", ""); err != nil {
		t.Fatalf("clear as-b: %v", err)
	}

	if got := taskRunner(t, repo, "as-a"); got != "agent-a" {
		t.Errorf("as-a runner = %q, want agent-a to survive as-b's clear", got)
	}
	if got := taskRunner(t, repo, "as-b"); got != "" {
		t.Errorf("as-b runner = %q, want empty", got)
	}
}

func taskRunner(t *testing.T, repo *sqlite.Repository, taskID string) string {
	t.Helper()
	fields, err := repo.GetTaskExecutionFields(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskExecutionFields(%s): %v", taskID, err)
	}
	return fields.AssigneeAgentProfileID
}

func runnerRowCount(t *testing.T, repo *sqlite.Repository, taskID string) int {
	t.Helper()
	var n int
	if err := repo.ReaderDB().Get(&n,
		`SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ? AND role = 'runner'`,
		taskID); err != nil {
		t.Fatalf("count runner rows: %v", err)
	}
	return n
}

func TestGetTaskExecutionFields_NotFoundSentinel(t *testing.T) {
	repo := newSearchTestRepo(t)

	got, err := repo.GetTaskExecutionFields(context.Background(), "missing")
	if err == nil {
		t.Fatal("GetTaskExecutionFields(missing) = nil error, want ErrTaskNotFound")
	}
	if !errors.Is(err, sqlite.ErrTaskNotFound) {
		t.Errorf("error = %v, want it to wrap ErrTaskNotFound", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestGetTaskExecutionFields_ProjectsOfficeOrigin(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-office', 'wf-office'), ('ws-plain', '')
	`); err != nil {
		t.Fatalf("seed workspaces: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_id, state, title, created_at, updated_at) VALUES
			('ef-office', 'ws-office', 'wf-office', 'IN_PROGRESS', 'Office', datetime('now'), datetime('now')),
			('ef-plain',  'ws-plain',  'wf-plain',  'TODO',        'Plain',  datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	office, err := repo.GetTaskExecutionFields(ctx, "ef-office")
	if err != nil {
		t.Fatalf("GetTaskExecutionFields(office): %v", err)
	}
	if office.ID != "ef-office" || office.State != "IN_PROGRESS" || office.WorkspaceID != "ws-office" {
		t.Errorf("got %+v, want the office task's own fields", *office)
	}
	if !office.IsFromOffice {
		t.Error("IsFromOffice = false, want true for a task in the office workflow")
	}

	plain, err := repo.GetTaskExecutionFields(ctx, "ef-plain")
	if err != nil {
		t.Fatalf("GetTaskExecutionFields(plain): %v", err)
	}
	if plain.IsFromOffice {
		t.Error("IsFromOffice = true, want false for a plain Kanban task")
	}
}

func TestListChildTasks(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks
			(id, workspace_id, parent_id, workflow_step_id, title, description, state, priority,
			 project_id, labels, identifier, is_ephemeral, origin, created_at, updated_at)
		VALUES
			('c-first',  'ws-1', 'parent-1', 'step-c', 'First',  'desc one', 'TODO',      'high',
			 'proj-1', '["a"]', 'KAN-1', 0, 'manual', '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
			('c-second', 'ws-1', 'parent-1', 'step-c', 'Second', '',         'COMPLETED', 'low',
			 '',       '[]',    'KAN-2', 0, 'manual', '2026-01-02 00:00:00', '2026-01-02 00:00:00'),
			('c-eph',    'ws-1', 'parent-1', 'step-c', 'Eph',    '',         'TODO',      'medium',
			 '',       '[]',    'KAN-3', 1, 'manual', '2026-01-03 00:00:00', '2026-01-03 00:00:00'),
			('c-auto',   'ws-1', 'parent-1', 'step-c', 'Auto',   '',         'TODO',      'medium',
			 '',       '[]',    'KAN-4', 0, 'automation_run', '2026-01-04 00:00:00', '2026-01-04 00:00:00'),
			('c-arch',   'ws-1', 'parent-1', 'step-c', 'Arch',   '',         'TODO',      'medium',
			 '',       '[]',    'KAN-5', 0, 'manual', '2026-01-05 00:00:00', '2026-01-05 00:00:00'),
			('c-other',  'ws-1', 'parent-2', 'step-c', 'Other',  '',         'TODO',      'medium',
			 '',       '[]',    'KAN-6', 0, 'manual', '2026-01-06 00:00:00', '2026-01-06 00:00:00')
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET archived_at = '2026-01-06 00:00:00' WHERE id = 'c-arch'`); err != nil {
		t.Fatalf("archive c-arch: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('p-c-first', 'step-c', 'c-first', 'runner', 'agent-child', 0, 0)
	`); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	rows, err := repo.ListChildTasks(ctx, "parent-1")
	if err != nil {
		t.Fatalf("ListChildTasks: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if strings.Join(ids, ",") != "c-first,c-second" {
		t.Fatalf("ids = %v, want c-first,c-second (created_at ASC; ephemeral, automation and archived excluded)", ids)
	}

	first := rows[0]
	want := sqlite.TaskRow{
		ID: "c-first", WorkspaceID: "ws-1", Identifier: "KAN-1", Title: "First",
		Description: "desc one", Status: "TODO", Priority: "high", ParentID: "parent-1",
		ProjectID: "proj-1", AssigneeAgentProfileID: "agent-child", Labels: `["a"]`,
		CreatedAt: first.CreatedAt, UpdatedAt: first.UpdatedAt,
	}
	if *first != want {
		t.Errorf("first child = %+v, want %+v", *first, want)
	}
	if first.CreatedAt == "" || first.UpdatedAt == "" {
		t.Error("timestamps are empty, want them projected")
	}

	none, err := repo.ListChildTasks(ctx, "parent-without-children")
	if err != nil {
		t.Fatalf("ListChildTasks(no children): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("rows = %+v, want none", none)
	}
}

func TestCheckoutTask_ExclusiveLockLifecycle(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "co-1", "ws-1", "Lock me", "", "KAN-1")

	acquired, err := repo.CheckoutTask(ctx, "co-1", "agent-a")
	if err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}
	if !acquired {
		t.Fatal("first checkout = false, want the lock acquired")
	}
	if got := checkoutAgent(t, repo, "co-1"); got != "agent-a" {
		t.Errorf("checkout_agent_id = %q, want agent-a", got)
	}
	if at := checkoutAt(t, repo, "co-1"); at == "" {
		t.Error("checkout_at is empty, want it stamped")
	}

	// The holder can re-acquire its own lock.
	if acquired, err = repo.CheckoutTask(ctx, "co-1", "agent-a"); err != nil || !acquired {
		t.Fatalf("re-checkout by holder = %v, %v; want true, nil", acquired, err)
	}

	// A different agent is refused.
	if acquired, err = repo.CheckoutTask(ctx, "co-1", "agent-b"); err != nil {
		t.Fatalf("CheckoutTask(other agent): %v", err)
	}
	if acquired {
		t.Error("second agent acquired a held lock")
	}
	if got := checkoutAgent(t, repo, "co-1"); got != "agent-a" {
		t.Errorf("checkout_agent_id = %q, want agent-a to keep the lock", got)
	}

	// Releasing clears both columns and re-opens the lock.
	if err = repo.ReleaseTaskCheckout(ctx, "co-1"); err != nil {
		t.Fatalf("ReleaseTaskCheckout: %v", err)
	}
	if got := checkoutAgent(t, repo, "co-1"); got != "" {
		t.Errorf("checkout_agent_id = %q, want it cleared", got)
	}
	if at := checkoutAt(t, repo, "co-1"); at != "" {
		t.Errorf("checkout_at = %q, want it cleared", at)
	}
	if acquired, err = repo.CheckoutTask(ctx, "co-1", "agent-b"); err != nil || !acquired {
		t.Fatalf("checkout after release = %v, %v; want true, nil", acquired, err)
	}

	// A missing task yields false, not an error.
	if acquired, err = repo.CheckoutTask(ctx, "missing", "agent-a"); err != nil {
		t.Fatalf("CheckoutTask(missing): %v", err)
	}
	if acquired {
		t.Error("CheckoutTask(missing) = true, want false")
	}
	if err = repo.ReleaseTaskCheckout(ctx, "missing"); err != nil {
		t.Errorf("ReleaseTaskCheckout(missing) = %v, want nil", err)
	}
}

func TestCheckoutTaskForRun_ReleaseOnlyOwningRun(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	insertTask(t, repo, ctx, "co-run-1", "ws-1", "Run-scoped lock", "", "KAN-1")

	acquired, err := repo.CheckoutTaskForRun(ctx, "co-run-1", "agent-a", "run-a")
	if err != nil || !acquired {
		t.Fatalf("first checkout = %v, %v; want true, nil", acquired, err)
	}
	if acquired, err = repo.CheckoutTaskForRun(ctx, "co-run-1", "agent-a", "run-b"); err != nil {
		t.Fatalf("successor checkout: %v", err)
	} else if acquired {
		t.Fatal("successor run replaced the live owner's checkout")
	}

	if err := repo.ReleaseTaskCheckoutForRun(ctx, "co-run-1", "agent-a", "run-b"); err != nil {
		t.Fatalf("release by successor: %v", err)
	}
	if got := checkoutRun(t, repo, "co-run-1"); got != "run-a" {
		t.Fatalf("checkout_run_id after successor release = %q, want run-a", got)
	}

	if err := repo.ReleaseTaskCheckoutForRun(ctx, "co-run-1", "agent-a", "run-a"); err != nil {
		t.Fatalf("release by owner: %v", err)
	}
	if got := checkoutRun(t, repo, "co-run-1"); got != "" {
		t.Fatalf("checkout_run_id after owner release = %q, want empty", got)
	}
}

// An empty-string checkout_agent_id counts as unheld, same as NULL.
func TestCheckoutTask_TreatsEmptyHolderAsFree(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "co-empty", "ws-1", "Lock", "", "KAN-1")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET checkout_agent_id = '' WHERE id = 'co-empty'`); err != nil {
		t.Fatalf("blank the holder: %v", err)
	}

	acquired, err := repo.CheckoutTask(ctx, "co-empty", "agent-a")
	if err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}
	if !acquired {
		t.Error("CheckoutTask on an empty-string holder = false, want the lock acquired")
	}
}

func checkoutAgent(t *testing.T, repo *sqlite.Repository, taskID string) string {
	t.Helper()
	var got string
	if err := repo.ReaderDB().Get(&got,
		`SELECT COALESCE(checkout_agent_id, '') FROM tasks WHERE id = ?`, taskID); err != nil {
		t.Fatalf("read checkout_agent_id: %v", err)
	}
	return got
}

func checkoutAt(t *testing.T, repo *sqlite.Repository, taskID string) string {
	t.Helper()
	var got string
	if err := repo.ReaderDB().Get(&got,
		`SELECT COALESCE(CAST(checkout_at AS TEXT), '') FROM tasks WHERE id = ?`, taskID); err != nil {
		t.Fatalf("read checkout_at: %v", err)
	}
	return got
}

func checkoutRun(t *testing.T, repo *sqlite.Repository, taskID string) string {
	t.Helper()
	var got string
	if err := repo.ReaderDB().Get(&got,
		`SELECT COALESCE(checkout_run_id, '') FROM tasks WHERE id = ?`, taskID); err != nil {
		t.Fatalf("read checkout_run_id: %v", err)
	}
	return got
}

func TestGetCheckoutAgentBySession(t *testing.T) {
	repo := newSearchTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	insertTask(t, repo, ctx, "cs-held", "ws-1", "Held", "", "KAN-1")
	insertTask(t, repo, ctx, "cs-free", "ws-1", "Free", "", "KAN-2")
	if _, err := repo.CheckoutTask(ctx, "cs-held", "agent-holder"); err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES
			('sess-held', 'cs-held', 'RUNNING', datetime('now'), datetime('now')),
			('sess-free', 'cs-free', 'RUNNING', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	got, err := repo.GetCheckoutAgentBySession(ctx, "sess-held")
	if err != nil {
		t.Fatalf("GetCheckoutAgentBySession: %v", err)
	}
	if got != "agent-holder" {
		t.Errorf("got %q, want agent-holder", got)
	}

	// A task with no checkout resolves to the empty string.
	if got, err = repo.GetCheckoutAgentBySession(ctx, "sess-free"); err != nil {
		t.Fatalf("GetCheckoutAgentBySession(free): %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}

	// An unknown session surfaces sql.ErrNoRows.
	if _, err = repo.GetCheckoutAgentBySession(ctx, "sess-missing"); err == nil {
		t.Error("GetCheckoutAgentBySession(missing) = nil error, want sql.ErrNoRows")
	}
}

func TestHasOfficeAdoption(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	adopted, err := repo.HasOfficeAdoption(ctx)
	if err != nil {
		t.Fatalf("HasOfficeAdoption: %v", err)
	}
	if adopted {
		t.Fatal("HasOfficeAdoption = true on a clean database, want false")
	}

	// A workspace row with an empty office_workflow_id is not adoption.
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-plain', '')`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if adopted, err = repo.HasOfficeAdoption(ctx); err != nil || adopted {
		t.Fatalf("HasOfficeAdoption = %v, %v; want false for an unconfigured workspace", adopted, err)
	}

	// An office project alone is enough.
	if err := repo.CreateProject(ctx, &models.Project{
		ID: "p-adopt", WorkspaceID: "ws-plain", Name: "Adopted", Status: models.ProjectStatus("active"),
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if adopted, err = repo.HasOfficeAdoption(ctx); err != nil || !adopted {
		t.Fatalf("HasOfficeAdoption = %v, %v; want true once a project exists", adopted, err)
	}

	// So is a workspace with an office workflow, with no projects at all.
	if err := repo.DeleteProject(ctx, "p-adopt"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE workspaces SET office_workflow_id = 'wf-1' WHERE id = 'ws-plain'`); err != nil {
		t.Fatalf("set office workflow: %v", err)
	}
	if adopted, err = repo.HasOfficeAdoption(ctx); err != nil || !adopted {
		t.Fatalf("HasOfficeAdoption = %v, %v; want true for a configured workspace", adopted, err)
	}
}
