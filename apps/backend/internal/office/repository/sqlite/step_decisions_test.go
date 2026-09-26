package sqlite_test

import (
	"context"
	"testing"
)

func TestHasActiveStepDecisionDistinguishesSupersededRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `CREATE TABLE workflow_step_decisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		decider_id TEXT NOT NULL,
		decision TEXT NOT NULL,
		superseded_at DATETIME
	)`); err != nil {
		t.Fatalf("create workflow_step_decisions: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `INSERT INTO workflow_step_decisions
		(id, task_id, step_id, decider_id, decision, superseded_at)
		VALUES
		('active', 'task-1', 'step-1', 'agent-1', 'approved', NULL),
		('superseded', 'task-1', 'step-2', 'agent-1', 'rejected', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert decisions: %v", err)
	}

	active, err := repo.HasActiveStepDecision(ctx, "task-1", "step-1", "agent-1")
	if err != nil {
		t.Fatalf("active decision lookup: %v", err)
	}
	if !active {
		t.Fatal("active decision lookup = false, want true")
	}

	superseded, err := repo.HasActiveStepDecision(ctx, "task-1", "step-2", "agent-1")
	if err != nil {
		t.Fatalf("superseded decision lookup: %v", err)
	}
	if superseded {
		t.Fatal("superseded decision lookup = true, want false")
	}
}
