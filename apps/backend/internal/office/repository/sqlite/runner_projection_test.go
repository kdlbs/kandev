package sqlite_test

import (
	"context"
	"testing"
	"time"
)

// TestGetTaskAssignee_MostRecentlyAssignedRunnerWins locks the task-scoped
// fallback arm of RunnerProjection (base.go): when the task's current step
// has neither a per-step runner row nor a step primary, the projection
// falls back to the task's runner row with the highest created_at across
// any step — not the physically most-recently-inserted row (rowid is not
// portable to Postgres). Seeds the later-created_at row first and the
// earlier-created_at row second, so a rowid-ordered fallback would pick
// the wrong (stale) agent.
func TestGetTaskAssignee_MostRecentlyAssignedRunnerWins(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_steps (id, agent_profile_id) VALUES ('step-current', '')
	`); err != nil {
		t.Fatalf("insert step: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, title, created_at, updated_at)
		VALUES ('task-x', 'ws-1', 'step-current', 'Task X', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	recent := time.Now().UTC()
	stale := recent.Add(-time.Hour)

	// Inserted in reverse chronological order: agent-recent's row (higher
	// created_at) is written first, so it gets the lower rowid. If the
	// fallback still ordered by rowid, agent-stale would win instead.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES ('p-recent', 'step-old-a', 'task-x', 'runner', 'agent-recent', 0, 0, ?)
	`, recent); err != nil {
		t.Fatalf("insert recent runner: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES ('p-stale', 'step-old-b', 'task-x', 'runner', 'agent-stale', 0, 0, ?)
	`, stale); err != nil {
		t.Fatalf("insert stale runner: %v", err)
	}

	got, err := repo.GetTaskAssignee(ctx, "task-x")
	if err != nil {
		t.Fatalf("GetTaskAssignee: %v", err)
	}
	if got != "agent-recent" {
		t.Fatalf("GetTaskAssignee = %q, want agent-recent (highest created_at, not highest rowid)", got)
	}
}
