package service_test

import (
	"context"
	"testing"

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
func TestSchedulerTick_AgentCompletedReleasesTaskCheckout(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

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
