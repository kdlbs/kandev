package sqlite_test

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestTaskBlocker_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	blocker := &models.TaskBlocker{
		TaskID:        "task-1",
		BlockerTaskID: "task-2",
	}
	if err := repo.CreateTaskBlocker(ctx, blocker); err != nil {
		t.Fatalf("create: %v", err)
	}

	blockers, err := repo.ListTaskBlockers(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("count = %d, want 1", len(blockers))
	}
	if blockers[0].BlockerTaskID != "task-2" {
		t.Errorf("blocker_task_id = %q, want %q", blockers[0].BlockerTaskID, "task-2")
	}

	if err := repo.DeleteTaskBlocker(ctx, "task-1", "task-2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	blockers, _ = repo.ListTaskBlockers(ctx, "task-1")
	if len(blockers) != 0 {
		t.Errorf("count after delete = %d, want 0", len(blockers))
	}
}

func TestListTasksBlockedBy(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// No edges yet: empty, non-nil result.
	ids, err := repo.ListTasksBlockedBy(ctx, "task-3")
	if err != nil {
		t.Fatalf("ListTasksBlockedBy: %v", err)
	}
	if ids == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(ids) != 0 {
		t.Errorf("expected no blocked tasks, got %v", ids)
	}

	// task-1 and task-2 are blocked by task-3; task-2 is also blocked by task-4.
	for _, b := range []*models.TaskBlocker{
		{TaskID: "task-1", BlockerTaskID: "task-3"},
		{TaskID: "task-2", BlockerTaskID: "task-3"},
		{TaskID: "task-2", BlockerTaskID: "task-4"},
	} {
		if err := repo.CreateTaskBlocker(ctx, b); err != nil {
			t.Fatalf("create blocker: %v", err)
		}
	}

	// One blocked task.
	ids, err = repo.ListTasksBlockedBy(ctx, "task-4")
	if err != nil {
		t.Fatalf("ListTasksBlockedBy: %v", err)
	}
	if !slices.Equal(ids, []string{"task-2"}) {
		t.Errorf("blocked by task-4 = %v, want [task-2]", ids)
	}

	// Several blocked tasks, ordered by insertion time.
	ids, err = repo.ListTasksBlockedBy(ctx, "task-3")
	if err != nil {
		t.Fatalf("ListTasksBlockedBy: %v", err)
	}
	if !slices.Equal(ids, []string{"task-1", "task-2"}) {
		t.Errorf("blocked by task-3 = %v, want [task-1 task-2]", ids)
	}

	// A task that only appears on the blocked side blocks nothing.
	ids, err = repo.ListTasksBlockedBy(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTasksBlockedBy: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("blocked by task-1 = %v, want none", ids)
	}

	// Deleting an edge removes it from the reverse lookup.
	if err := repo.DeleteTaskBlocker(ctx, "task-1", "task-3"); err != nil {
		t.Fatalf("DeleteTaskBlocker: %v", err)
	}
	ids, _ = repo.ListTasksBlockedBy(ctx, "task-3")
	if !slices.Equal(ids, []string{"task-2"}) {
		t.Errorf("blocked by task-3 after delete = %v, want [task-2]", ids)
	}
}

func TestGetChildSummaries(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// Create parent and children.
	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent Task", "", "")
	insertTask(t, repo, ctx, "child-1", "ws-1", "Auth service", "", "KAN-2")
	insertTask(t, repo, ctx, "child-2", "ws-1", "API gateway", "", "KAN-3")

	// Set parent_id and states.
	_, _ = repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'COMPLETED' WHERE id = 'child-1'`)
	_, _ = repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-1', state = 'CANCELLED' WHERE id = 'child-2'`)

	// Add a comment to child-1.
	_, _ = repo.ExecRaw(ctx,
		`INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		 VALUES ('c1', 'child-1', 'agent', 'a1', 'Implemented JWT generation', 'agent', datetime('now'))`)

	summaries, truncated, err := repo.GetChildSummaries(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false for 2 children")
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// child-1 should have its comment.
	if summaries[0].Title != "Auth service" {
		t.Errorf("child-1 title = %q", summaries[0].Title)
	}
	if summaries[0].LastComment != "Implemented JWT generation" {
		t.Errorf("child-1 last_comment = %q", summaries[0].LastComment)
	}
	if summaries[0].State != "COMPLETED" {
		t.Errorf("child-1 state = %q", summaries[0].State)
	}

	// child-2 should have no comment.
	if summaries[1].LastComment != "" {
		t.Errorf("child-2 should have no comment, got %q", summaries[1].LastComment)
	}
}

func TestGetChildSummaries_NoChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "lonely-parent", "ws-1", "No Kids", "", "")

	summaries, truncated, err := repo.GetChildSummaries(ctx, "lonely-parent")
	if err != nil {
		t.Fatalf("GetChildSummaries: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false")
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
	}
}

func TestListChildStates(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent Task", "", "")
	insertTask(t, repo, ctx, "child-z", "ws-1", "Child Z", "", "")
	insertTask(t, repo, ctx, "child-a", "ws-1", "Child A", "", "")
	insertTask(t, repo, ctx, "child-null", "ws-1", "Child Null", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks
		SET parent_id = 'parent-1', state = CASE id
			WHEN 'child-z' THEN 'CANCELLED'
			WHEN 'child-a' THEN 'COMPLETED'
			WHEN 'child-null' THEN NULL
		END
		WHERE id IN ('child-z', 'child-a', 'child-null')
	`); err != nil {
		t.Fatalf("set child states: %v", err)
	}

	states, err := repo.ListChildStates(ctx, "parent-1")
	if err != nil {
		t.Fatalf("ListChildStates: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("child state count = %d, want 3", len(states))
	}
	want := []struct {
		id    string
		state string
	}{
		{id: "child-a", state: "COMPLETED"},
		{id: "child-null", state: ""},
		{id: "child-z", state: "CANCELLED"},
	}
	for i, got := range states {
		if got.TaskID != want[i].id || got.State != want[i].state {
			t.Errorf("state[%d] = {%q, %q}, want {%q, %q}",
				i, got.TaskID, got.State, want[i].id, want[i].state)
		}
	}

	empty, err := repo.ListChildStates(ctx, "missing-parent")
	if err != nil {
		t.Fatalf("ListChildStates empty: %v", err)
	}
	if empty == nil {
		t.Fatal("empty child state result is nil")
	}
	if len(empty) != 0 {
		t.Fatalf("empty child state count = %d, want 0", len(empty))
	}
}

func TestListBlockersForTasks(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// task-1 is blocked by task-3
	// task-2 is blocked by task-3 and task-4
	for _, b := range []*models.TaskBlocker{
		{TaskID: "task-1", BlockerTaskID: "task-3"},
		{TaskID: "task-2", BlockerTaskID: "task-3"},
		{TaskID: "task-2", BlockerTaskID: "task-4"},
	} {
		if err := repo.CreateTaskBlocker(ctx, b); err != nil {
			t.Fatalf("create blocker: %v", err)
		}
	}

	m, err := repo.ListBlockersForTasks(ctx, []string{"task-1", "task-2", "task-5"})
	if err != nil {
		t.Fatalf("ListBlockersForTasks: %v", err)
	}
	if len(m["task-1"]) != 1 || m["task-1"][0] != "task-3" {
		t.Errorf("task-1 blockers = %v, want [task-3]", m["task-1"])
	}
	if len(m["task-2"]) != 2 {
		t.Errorf("task-2 blockers = %v, want 2 entries", m["task-2"])
	}
	if len(m["task-5"]) != 0 {
		t.Errorf("task-5 blockers = %v, want none", m["task-5"])
	}

	// Empty input returns empty map.
	empty, err := repo.ListBlockersForTasks(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty input: err=%v map=%v", err, empty)
	}
}

// TestGetTaskAssigneeTx_ReflectsCurrentRunner covers the primitive
// ParentWakeReconciler.recordReceipt (scheduler_wake_reconciler.go) uses to close the
// TOCTOU race between ListStuckParents' SELECT and its own transactional run
// insert: a runner reassignment committed on a separate connection before
// the transaction begins must be visible inside it, matching GetTaskAssignee
// (the non-transactional analog this method mirrors) rather than some stale
// snapshot taken before the reassignment.
func TestGetTaskAssigneeTx_ReflectsCurrentRunner(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	seedWakeAgentProfile(t, repo, ctx, "agent-a", "idle")
	seedWakeAgentProfile(t, repo, ctx, "agent-b", "idle")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES ('p-runner-parent-1', '', 'parent-1', 'runner', 'agent-a')
	`); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	got, err := repo.GetTaskAssignee(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetTaskAssignee: %v", err)
	}
	if got != "agent-a" {
		t.Fatalf("GetTaskAssignee = %q, want agent-a", got)
	}

	// Reassign on a separate, already-committed statement, simulating a
	// reassignment that lands between a candidate's capture (the earlier
	// ListStuckParents SELECT) and the transaction recordReceipt() opens to record it.
	if _, err := repo.ExecRaw(ctx, `
		UPDATE workflow_step_participants SET agent_profile_id = 'agent-b'
		WHERE task_id = 'parent-1' AND role = 'runner'
	`); err != nil {
		t.Fatalf("reassign runner: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	gotTx, err := repo.GetTaskAssigneeTx(ctx, tx, "parent-1")
	if err != nil {
		t.Fatalf("GetTaskAssigneeTx: %v", err)
	}
	if gotTx != "agent-b" {
		t.Fatalf("GetTaskAssigneeTx = %q, want agent-b (the reassigned runner, not the stale agent-a)", gotTx)
	}
}

func TestTaskBlocker_SelfReferenceBlocked(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	blocker := &models.TaskBlocker{
		TaskID:        "task-1",
		BlockerTaskID: "task-1",
	}
	if err := repo.CreateTaskBlocker(ctx, blocker); err == nil {
		t.Fatal("expected CHECK constraint error for self-reference")
	}
}
