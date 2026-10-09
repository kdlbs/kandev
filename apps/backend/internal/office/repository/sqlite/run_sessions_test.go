package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// AC-OFFICE-TASKLESS-001.1: run-owned sessions have their own durable table
// instead of borrowing the task_sessions foreign key contract.
func TestRunSessionSchemaExists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES ('agent-schema-type', 'Agent', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent type: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, created_at, updated_at)
		VALUES ('agent-schema', 'agent-schema-type', 'Agent', 'Agent', 'workspace-schema', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, requested_at
		) VALUES ('run-schema', 'agent-schema', 'routine_dispatch_cron', '{}', 'claimed', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	_, err := repo.ExecRaw(ctx, `
		INSERT INTO office_run_sessions (
			id, workspace_id, agent_profile_id, run_id, attempt, state, created_at
		) VALUES ('session-schema', 'workspace-schema', 'agent-schema', 'run-schema', 1, ?, CURRENT_TIMESTAMP)
	`, "preparing")
	if err != nil {
		t.Fatalf("insert run session: %v", err)
	}
}

// AC-OFFICE-TASKLESS-001.1/.2: a duplicate reservation loses without
// replacing the predecessor, while a new attempt gets a fresh identity.
func TestRunSessionReservationCAS(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES ('agent-cas-type', 'Agent', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent type: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, created_at, updated_at)
		VALUES ('agent-cas', 'agent-cas-type', 'Agent', 'Agent', 'workspace-cas', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('run-cas', 'agent-cas', 'routine_dispatch_cron', '{}', 'claimed', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	first := &models.RunSession{
		ID: "session-cas-1", WorkspaceID: "workspace-cas", AgentProfileID: "agent-cas",
		RunID: "run-cas", Attempt: 1, State: models.RunSessionStatePreparing,
		CreatedAt: time.Now().UTC(), Version: 1,
	}
	reserved, err := repo.ReserveRunSession(ctx, first)
	if err != nil || !reserved {
		t.Fatalf("first reservation = %v, %v; want true, nil", reserved, err)
	}
	executionBound, err := repo.BindRunSessionExecution(
		ctx, first.ID, "execution-cas-1", "profile-cas", "adapter", "model", "",
	)
	if err != nil || !executionBound {
		t.Fatalf("bind execution = %v, %v; want true, nil", executionBound, err)
	}
	requested, err := repo.RequestRunSessionCancellation(ctx, first.ID)
	if err != nil || !requested {
		t.Fatalf("request cancellation = %v, %v; want true, nil", requested, err)
	}
	finished, err := repo.FinishRunSession(ctx, first.ID, models.RunSessionStateFinished, "")
	if err != nil || finished {
		t.Fatalf("finish after cancellation = %v, %v; want false, nil", finished, err)
	}
	cancelled, err := repo.FinishRunSession(ctx, first.ID, models.RunSessionStateCancelled, "cancelled")
	if err != nil || !cancelled {
		t.Fatalf("cancel after cancellation = %v, %v; want true, nil", cancelled, err)
	}
	duplicate := *first
	duplicate.ID = "session-cas-duplicate"
	reserved, err = repo.ReserveRunSession(ctx, &duplicate)
	if err != nil || reserved {
		t.Fatalf("duplicate reservation = %v, %v; want false, nil", reserved, err)
	}
	second := *first
	second.ID = "session-cas-2"
	second.Attempt = 2
	reserved, err = repo.ReserveRunSession(ctx, &second)
	if err != nil || !reserved {
		t.Fatalf("second reservation = %v, %v; want true, nil", reserved, err)
	}
	sessions, err := repo.ListRunSessions(ctx, first.RunID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 || sessions[0].ID != first.ID || sessions[1].ID != second.ID {
		t.Fatalf("sessions = %#v, want both immutable attempts", sessions)
	}
	var bound string
	if err := repo.ReaderDB().GetContext(ctx, &bound, `SELECT session_id FROM runs WHERE id = ?`, first.RunID); err != nil {
		t.Fatalf("read bound run session: %v", err)
	}
	if bound != first.ID {
		t.Fatalf("run.session_id = %q, want predecessor's first bound identity %q", bound, first.ID)
	}
}
