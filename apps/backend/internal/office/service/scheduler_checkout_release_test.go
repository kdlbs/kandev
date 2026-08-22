package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerTick_AgentCompletedReleasesTaskCheckout is the regression
// test for the checkout-leak defect: checkout_agent_id must be released
// when a run reaches a terminal state via the AgentCompleted event
// subscriber — the path that finishes most real launched runs, since
// prepareAndLaunch returns early once taskStarter != nil and leaves the
// synchronous SchedulerIntegration.finishRun (which does release the
// checkout) never reached. Before the fix, handleAgentCompleted called
// Service.FinishRun directly, which never released the checkout, so
// every other agent was permanently blocked from acquiring the task.
//
// It also covers the sibling divergence in the same async path: the
// SAME early return skips SchedulerIntegration.finishRun's
// UpdateRuntimeLastRunFinished stamp, so office_agent_runtime.
// last_run_finished_at is never written for any run that actually
// launches an agent. Asserting only that the run reached "finished"
// (the pre-existing assertion below) passes today and proves nothing
// about either leak.
func TestSchedulerTick_AgentCompletedReleasesTaskCheckout(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	beforePublish := time.Now().UTC()

	agent := &models.AgentInstance{
		ID:                 "profile-checkout-release",
		WorkspaceID:        "ws-1",
		Name:               "checkout-release-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	other := &models.AgentInstance{
		ID:          "profile-checkout-other",
		WorkspaceID: "ws-1",
		Name:        "other-worker",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, other); err != nil {
		t.Fatalf("create other agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-checkout-release-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-checkout-release-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// Sanity: the launch acquired the checkout, so a second agent must be
	// blocked from taking it right now.
	acquiredByOther, err := svc.CheckoutTask(ctx, "task-checkout-release-1", other.ID)
	if err != nil {
		t.Fatalf("checkout probe: %v", err)
	}
	if acquiredByOther {
		t.Fatal("expected checkout to be held by the launching agent after launch")
	}

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          "task-checkout-release-1",
		"session_id":       "session-checkout-release",
		"agent_profile_id": agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	ok, err := svc.CheckoutTask(ctx, "task-checkout-release-1", other.ID)
	if err != nil {
		t.Fatalf("checkout after completion: %v", err)
	}
	if !ok {
		t.Fatal("expected task checkout to be released after AgentCompleted, but another agent still cannot check it out")
	}

	runtime, err := svc.GetAgentRuntimeForTest(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent runtime: %v", err)
	}
	if runtime == nil || runtime.LastRunFinishedAt == nil {
		t.Fatal("expected last_run_finished_at to be stamped after AgentCompleted, but it is still unset")
	}
	if runtime.LastRunFinishedAt.Before(beforePublish) {
		t.Errorf("last_run_finished_at = %v, want at/after %v", runtime.LastRunFinishedAt, beforePublish)
	}
}

// TestSchedulerTick_AgentFailedReleasesTaskCheckout is the failure-path
// counterpart: HandleAgentFailure calls repo.MarkRunFailed directly
// (not Service.FailRun / transitionRunTerminal), so it needs the same
// checkout-release treatment as the completion path.
func TestSchedulerTick_AgentFailedReleasesTaskCheckout(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := &models.AgentInstance{
		ID:                 "profile-checkout-fail",
		WorkspaceID:        "ws-1",
		Name:               "checkout-fail-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	other := &models.AgentInstance{
		ID:          "profile-checkout-fail-other",
		WorkspaceID: "ws-1",
		Name:        "other-worker",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, other); err != nil {
		t.Fatalf("create other agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-checkout-fail-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-checkout-fail-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	acquiredByOther, err := svc.CheckoutTask(ctx, "task-checkout-fail-1", other.ID)
	if err != nil {
		t.Fatalf("checkout probe: %v", err)
	}
	if acquiredByOther {
		t.Fatal("expected checkout to be held by the launching agent after launch")
	}

	event := bus.NewEvent(events.AgentFailed, "test", map[string]string{
		"task_id":          "task-checkout-fail-1",
		"session_id":       "session-checkout-fail",
		"error_message":    "boom",
		"agent_profile_id": agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish agent failed: %v", err)
	}

	ok, err := svc.CheckoutTask(ctx, "task-checkout-fail-1", other.ID)
	if err != nil {
		t.Fatalf("checkout after failure: %v", err)
	}
	if !ok {
		t.Fatal("expected task checkout to be released after AgentFailed, but another agent still cannot check it out")
	}
}

// TestSchedulerTick_TasklessAgentCompletedStampsRuntime is the taskless-path
// parity check for the same async completion divergence: handleAgentCompleted
// routes a task_id-less event to handleTasklessAgentCompleted (the
// agent_heartbeat-cron completion path), which must also stamp
// office_agent_runtime.last_run_finished_at. The run is claimed directly via
// ClaimNextRun rather than a full scheduler tick, because a taskless run's
// own processRun→prepareAndLaunch path finishes it synchronously before any
// launch (taskID == "" bypasses the "leave it claimed for the async
// subscriber" branch) — going through the tick would finish the run before
// the event ever had anything claimed to resolve.
func TestSchedulerTick_TasklessAgentCompletedStampsRuntime(t *testing.T) {
	svc := newTestService(t)
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	beforePublish := time.Now().UTC()

	agent := &models.AgentInstance{
		ID:          "profile-taskless-stamp",
		WorkspaceID: "ws-1",
		Name:        "taskless-stamp-worker",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	claimed, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a run to be claimed")
	}

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"session_id": "session-taskless-stamp",
		"agent_id":   agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	runtime, err := svc.GetAgentRuntimeForTest(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent runtime: %v", err)
	}
	if runtime == nil || runtime.LastRunFinishedAt == nil {
		t.Fatal("expected last_run_finished_at to be stamped after taskless AgentCompleted, but it is still unset")
	}
	if runtime.LastRunFinishedAt.Before(beforePublish) {
		t.Errorf("last_run_finished_at = %v, want at/after %v", runtime.LastRunFinishedAt, beforePublish)
	}
}

// TestSchedulerTick_ReapsStaleCheckoutWithNoInFlightRun pins the tick-loop
// wiring for the stale-checkout reaper (backstop for
// releaseTaskCheckoutForRun): a checkout with nothing behind it must be
// cleared by an ordinary scheduler tick, not just by a direct repository
// call.
func TestSchedulerTick_ReapsStaleCheckoutWithNoInFlightRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-tick-reap-1', 'ws-1', 'Stale checkout', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	svc.ExecSQL(t, `UPDATE tasks SET checkout_agent_id = 'agent-gone', checkout_at = datetime('now', '-1 hour')
		WHERE id = 'task-tick-reap-1'`)

	service.RunSchedulerTick(svc, ctx)

	ok, err := svc.CheckoutTask(ctx, "task-tick-reap-1", "agent-new")
	if err != nil {
		t.Fatalf("checkout after tick: %v", err)
	}
	if !ok {
		t.Fatal("expected a scheduler tick to reap the stale checkout, but a new agent still cannot check it out")
	}
}
