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

// TestSchedulerTick_TasklessRunsDoNotAutoPauseAgent is the WO-35 Review
// round 1 regression test. A taskless run is a scheduler capability gap
// (the taskless-launch seam does not exist yet — see the SCOPE-1 decision
// in the task plan), not an agent failure, so failing it must not touch
// the agent's consecutive-failure counter. The pre-installed "Coordinator
// heartbeat" routine is taskless by design and fires every 5 minutes
// (routines/service.go:133,155): if taskless failures counted toward
// DefaultAgentFailureThreshold (3), every default install would
// auto-pause its coordinator within ~15 minutes of a fresh boot. Once
// paused, scheduler_integration.go's !isAgentActive branch silently
// FinishRuns every subsequent run for that agent — including task-bound,
// event-driven ones that work today — which is exactly the
// reports-success-but-does-nothing pathology this card exists to kill,
// now applied to the path the card's own SYMPTOM section says is
// unaffected.
func TestSchedulerTick_TasklessRunsDoNotAutoPauseAgent(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35-pause",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35-pause",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// DefaultAgentFailureThreshold consecutive taskless heartbeat fires —
	// mirrors 3 ticks of the pre-installed coordinator heartbeat routine.
	const firesAtThreshold = 3
	for i := 0; i < firesAtThreshold; i++ {
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue taskless run %d: %v", i, err)
		}
		service.RunSchedulerTick(svc, ctx)
	}

	got, err := svc.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status == models.AgentStatusPaused {
		t.Fatalf("agent auto-paused after %d taskless failures — a taskless run is a scheduler "+
			"capability gap, not an agent failure, and must not count toward auto-pause", firesAtThreshold)
	}
	if got.PauseReason != "" {
		t.Fatalf("pause_reason = %q, want empty", got.PauseReason)
	}
	if got.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0 — taskless failures must not accumulate", got.ConsecutiveFailures)
	}

	// The event-driven path must still work after repeated taskless
	// failures: a task-bound run queued afterwards must still launch.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-wo35-pause-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-wo35-pause-1"}`, ""); err != nil {
		t.Fatalf("queue task-bound run: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call for the task-bound run after taskless failures, got %d — "+
			"the event-driven path must survive repeated taskless failures", mock.callCount())
	}
}
