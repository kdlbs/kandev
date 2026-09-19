package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// insertRunnerSetTask inserts an actionable task assigned to agentID via a
// runner participant row, with an explicit updated_at so ordering tests are
// deterministic.
func insertRunnerSetTask(
	t *testing.T, repo *sqlite.Repository, ctx context.Context,
	id, wsID, agentID, state, updatedAt string,
) {
	t.Helper()
	stepID := "step-" + id
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, state, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, wsID, stepID, state, id, updatedAt, updatedAt); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)
	`, "p-"+id, stepID, id, agentID); err != nil {
		t.Fatalf("insert runner participant for %s: %v", id, err)
	}
}

func TestListRunnerSetTaskIDs_OrdersByUpdatedAtThenIDDescending(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertRunnerSetTask(t, repo, ctx, "task-a", "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	insertRunnerSetTask(t, repo, ctx, "task-b", "ws-1", "agent-1", "TODO", "2025-01-02 00:00:00")
	// Same updated_at as task-b: tie-break on id descending.
	insertRunnerSetTask(t, repo, ctx, "task-c", "ws-1", "agent-1", "IN_PROGRESS", "2025-01-02 00:00:00")

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-1", "ws-1", 500)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	want := []string{"task-c", "task-b", "task-a"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestListRunnerSetTaskIDs_FiltersByWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertRunnerSetTask(t, repo, ctx, "task-ws1", "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	insertRunnerSetTask(t, repo, ctx, "task-ws2", "ws-2", "agent-1", "TODO", "2025-01-01 00:00:00")

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-1", "ws-1", 500)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 1 || len(ids) != 1 || ids[0] != "task-ws1" {
		t.Fatalf("ids/total = %v/%d, want [task-ws1]/1", ids, total)
	}
}

func TestListRunnerSetTaskIDs_ExcludesArchivedAndInactiveStates(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertRunnerSetTask(t, repo, ctx, "task-todo", "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	insertRunnerSetTask(t, repo, ctx, "task-done", "ws-1", "agent-1", "DONE", "2025-01-01 00:00:00")
	insertRunnerSetTask(t, repo, ctx, "task-archived", "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	if _, err := repo.ExecRaw(ctx, `UPDATE tasks SET archived_at = ? WHERE id = 'task-archived'`, "2025-01-02 00:00:00"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-1", "ws-1", 500)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 1 || len(ids) != 1 || ids[0] != "task-todo" {
		t.Fatalf("ids/total = %v/%d, want [task-todo]/1", ids, total)
	}
}

func TestListRunnerSetTaskIDs_ReportsTrueTotalWhenCapped(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		id := "task-" + string(rune('a'+i))
		insertRunnerSetTask(t, repo, ctx, id, "ws-1", "agent-1", "TODO", "2025-01-01 00:00:00")
	}

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-1", "ws-1", 2)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want len 2", ids)
	}
}

func TestListRunnerSetTaskIDs_ReturnsEmptyForNoTasks(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	ids, total, err := repo.ListRunnerSetTaskIDs(ctx, "agent-none", "ws-1", 500)
	if err != nil {
		t.Fatalf("ListRunnerSetTaskIDs: %v", err)
	}
	if total != 0 || len(ids) != 0 {
		t.Fatalf("ids/total = %v/%d, want empty/0", ids, total)
	}
}
