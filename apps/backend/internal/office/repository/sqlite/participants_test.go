package sqlite_test

import (
	"context"
	"testing"
)

func TestListAllTaskParticipants_PrefersPerTaskParticipant(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT DEFAULT '',
			workflow_step_id TEXT DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL,
			task_id TEXT DEFAULT '',
			role TEXT NOT NULL,
			agent_profile_id TEXT DEFAULT '',
			decision_required INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0
		)
	`); err != nil {
		t.Fatalf("create workflow_step_participants table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_step_id)
		VALUES ('task-participants', 'ws-1', 'step-participants')
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('a-template', 'step-participants', '', 'reviewer', 'agent-reviewer', 0, 0),
			('z-task', 'step-participants', 'task-participants', 'reviewer', 'agent-reviewer', 1, 0)
	`); err != nil {
		t.Fatalf("insert participants: %v", err)
	}

	participants, err := repo.ListAllTaskParticipants(ctx, "task-participants")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("participants = %d, want 1: %#v", len(participants), participants)
	}
	got := participants[0]
	if got.TaskID != "task-participants" {
		t.Fatalf("TaskID = %q, want projected task id", got.TaskID)
	}
	if !got.DecisionRequired {
		t.Fatalf("DecisionRequired = false, want per-task row to win: %#v", got)
	}
}
