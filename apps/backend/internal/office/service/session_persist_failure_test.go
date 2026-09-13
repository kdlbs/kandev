package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// rejectSessionIDWrites installs a trigger that aborts only an UPDATE
// setting runs.session_id to wantSessionID — the exact value the
// launch under test will produce — so it targets persistLaunchedSession's
// post-launch SetRunSessionID write without also catching the
// pre-launch runtime-context build, which writes session_id too (see
// runtime/context_builder.go) but with the run's pre-launch id.
func rejectSessionIDWrites(t *testing.T, svc *service.Service, wantSessionID string) {
	t.Helper()
	svc.ExecSQL(t, fmt.Sprintf(`
		CREATE TRIGGER reject_session_id_write
		BEFORE UPDATE OF session_id ON runs
		WHEN NEW.session_id = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'session_id write rejected for test');
		END;
	`, wantSessionID))
}

// AC-002.11: on a direct (non-routed) launch, a session persist failure
// is counted separately from a without-session launch, and does not
// fail the launch itself or overwrite the run's session_id — the agent
// is already running by the time this write happens.
func TestSchedulerTick_DirectLaunchSessionPersistFailureCountsSeparately(t *testing.T) {
	mock := &mockTaskStarterWithSession{mockTaskStarter: &mockTaskStarter{}, sessionID: "sess-direct-persist-fail"}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "direct-persist-fail-agent",
		WorkspaceID:        "ws-1",
		Name:               "direct-persist-fail-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-direct-persist-fail-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-direct-persist-fail-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	rejectSessionIDWrites(t, svc, "sess-direct-persist-fail")

	beforeFailed := directHopExpvarInt(t, "office_loop_session_persist_failed_total", "workspace=ws-1")
	beforeWithoutSession := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 launch call, got %d", mock.callCount())
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].SessionID != "" {
		t.Fatalf("session_id = %q, want empty after a rejected write", runs[0].SessionID)
	}
	// The launch itself must still count as claimed/in-flight, not failed —
	// the agent is already running despite the persist failure.
	if runs[0].Status != service.RunStatusClaimed {
		t.Fatalf("run status = %q, want claimed (launch succeeded despite persist failure)", runs[0].Status)
	}

	afterFailed := directHopExpvarInt(t, "office_loop_session_persist_failed_total", "workspace=ws-1")
	if afterFailed != beforeFailed+1 {
		t.Fatalf("office_loop_session_persist_failed_total delta = %d, want 1", afterFailed-beforeFailed)
	}
	afterWithoutSession := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")
	if afterWithoutSession != beforeWithoutSession {
		t.Fatalf("office_loop_launch_without_session_total delta = %d, want 0", afterWithoutSession-beforeWithoutSession)
	}
}

// ignoreSessionIDWrites installs a trigger that uses RAISE(IGNORE)
// rather than RAISE(ABORT): the UPDATE setting runs.session_id to
// wantSessionID matches zero rows and returns no error, the shape a run
// row that vanished between claim and launch would produce — a
// different failure mode than rejectSessionIDWrites' hard DB error.
func ignoreSessionIDWrites(t *testing.T, svc *service.Service, wantSessionID string) {
	t.Helper()
	svc.ExecSQL(t, fmt.Sprintf(`
		CREATE TRIGGER ignore_session_id_write
		BEFORE UPDATE OF session_id ON runs
		WHEN NEW.session_id = '%s'
		BEGIN
			SELECT RAISE(IGNORE);
		END;
	`, wantSessionID))
}

// AC-002.11: on a direct (non-routed) launch, a real, non-empty session
// id whose write affects zero rows (no error) must be counted as a
// session persist failure, not miscounted as "launch yielded no session
// id" (AC-002.8's counter).
func TestSchedulerTick_DirectLaunchZeroRowSessionWriteCountsAsPersistFailure(t *testing.T) {
	mock := &mockTaskStarterWithSession{mockTaskStarter: &mockTaskStarter{}, sessionID: "sess-direct-vanished"}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "direct-vanished-agent",
		WorkspaceID:        "ws-1",
		Name:               "direct-vanished-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-direct-vanished-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-direct-vanished-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	ignoreSessionIDWrites(t, svc, "sess-direct-vanished")

	beforeFailed := directHopExpvarInt(t, "office_loop_session_persist_failed_total", "workspace=ws-1")
	beforeWithoutSession := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 launch call, got %d", mock.callCount())
	}

	afterFailed := directHopExpvarInt(t, "office_loop_session_persist_failed_total", "workspace=ws-1")
	if afterFailed != beforeFailed+1 {
		t.Fatalf("office_loop_session_persist_failed_total delta = %d, want 1 (AC-002.11: a real session id that failed to persist)", afterFailed-beforeFailed)
	}
	afterWithoutSession := directHopExpvarInt(t, "office_loop_launch_without_session_total", "workspace=ws-1")
	if afterWithoutSession != beforeWithoutSession {
		t.Fatalf("office_loop_launch_without_session_total delta = %d, want 0 (this counter is reserved for AC-002.8's empty-session-id case)", afterWithoutSession-beforeWithoutSession)
	}
}
