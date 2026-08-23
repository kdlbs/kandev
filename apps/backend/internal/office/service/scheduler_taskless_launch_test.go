package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerTick_TasklessRunFailsInsteadOfFinishing is the primary WO-35
// regression test. wakeup/dispatcher.go's createFreshRun inserts a
// `Payload: "{}"` run for every lightweight-routine fire (the pre-installed
// coordinator heartbeat, among others) — no task_id, ever. Before the fix,
// SchedulerIntegration.launchOrLog returned true for taskID=="" without
// calling the task starter, and prepareAndLaunch's fall-through called
// finishRun, so the run reached status=finished with an empty session_id
// and an empty error_message: a run that never launched an agent, reported
// as a success. That is exactly the shape of the card's "323 consecutive
// successful runs, zero agent sessions" measurement.
//
// The scheduler cannot currently launch a taskless run at all (that
// requires a taskless session seam — task_sessions.task_id is NOT NULL —
// which is a follow-up feature, not part of this card). So the correct
// terminal state today is a loud, immediate failure, not silent success.
func TestSchedulerTick_TasklessRunFailsInsteadOfFinishing(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Mirrors wakeup/dispatcher.go's createFreshRun: reason from the
	// routine trigger, payload literally "{}" (no task_id).
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 0 {
		t.Fatalf("expected 0 StartTask calls for a taskless run, got %d", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q — an un-launched run must not report success",
			runs[0].Status, service.RunStatusFailed)
	}
	if runs[0].ErrorMessage == "" {
		t.Error("expected a non-empty error_message explaining the run could not launch")
	}
	if runs[0].SessionID != "" {
		t.Errorf("session_id = %q, want empty — no agent was launched", runs[0].SessionID)
	}
}

// TestSchedulerTick_TaskBoundRunStillLaunches is the regression guard
// alongside the taskless-failure fix above: an ordinary task-bound run with
// a wired task starter must still launch normally and stay `claimed` (not
// reach a terminal state synchronously) — proving launchAgent's new
// taskID=="" / taskStarter==nil failure branches did not swallow the
// legitimate launch path.
func TestSchedulerTick_TaskBoundRunStillLaunches(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "worker-wo35",
		WorkspaceID:        "ws-1",
		Name:               "worker-wo35",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-wo35-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-wo35-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusClaimed {
		t.Fatalf("run status = %q, want %q — a launched run stays claimed for the async completion subscriber",
			runs[0].Status, service.RunStatusClaimed)
	}
}
