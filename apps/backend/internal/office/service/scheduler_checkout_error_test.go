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

// TestSchedulerTick_CheckoutErrorRetriesInsteadOfFalseFinish pins the
// PR #2830 review fix for tryCheckout's error branch: a transient
// CheckoutTask error (e.g. a busy database) is not a successful
// completion. Before the fix, tryCheckout called FinishRun on any
// CheckoutTask error, exactly like the checkout-contention bug this PR
// already fixes for the sibling "lost the race" branch — the run was
// marked finished with no indication it never executed, and nothing ever
// retried it.
func TestSchedulerTick_CheckoutErrorRetriesInsteadOfFalseFinish(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-checkout-db-error",
		WorkspaceID: "ws-1",
		Name:        "checkout-db-error",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-checkout-db-error-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Drop the tasks table so CheckoutTask's UPDATE fails with a genuine
	// SQL error (not a lost-race "0 rows affected"). ClaimNextEligibleRun
	// only touches the runs table, so the run still gets claimed before
	// processRun reaches checkoutTask.
	svc.ExecSQL(t, "DROP TABLE tasks")

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 0 {
		t.Fatalf("StartTask calls = %d, want 0 (checkout error should have blocked launch)", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want queued (retried after a checkout error, not silently finished)", runs[0].Status)
	}
	if runs[0].RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", runs[0].RetryCount)
	}
}

// TestSchedulerTick_AgentCompletedKeepsCheckoutWhenFinishRunFails pins the
// PR #2830 review fix for handleAgentCompleted's release ordering: the
// task checkout must only be released once FinishRun's DB update has
// actually landed. Before the fix, the checkout was released first and
// FinishRun second — a failed FinishRun left the run row non-terminal
// while the checkout was already gone, so a second agent could start work
// on the task while the original run's own bookkeeping still called it
// live. ReapStaleCheckouts is the intended backstop for a checkout that
// outlives its run; it must not be needed for a run that is still,
// as far as the database is concerned, in flight.
func TestSchedulerTick_AgentCompletedKeepsCheckoutWhenFinishRunFails(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := &models.AgentInstance{
		ID:          "agent-finish-order",
		WorkspaceID: "ws-1",
		Name:        "finish-order",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-finish-order-1', 'ws-1', 'Finish Order Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	ok, err := svc.CheckoutTask(ctx, "task-finish-order-1", agent.ID)
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-finish-order-1', ?, 'task_assigned', '{"task_id":"task-finish-order-1"}',
			'claimed', 1, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, agent.ID)

	// Force FinishRun's UPDATE to fail (it sets finished_at) while leaving
	// the tasks table, and every other runs column, writable — a targeted
	// fault instead of a global read-only pragma, so this test actually
	// distinguishes "release before finish" from "finish before release"
	// rather than failing both writes identically.
	svc.ExecSQL(t, "ALTER TABLE runs DROP COLUMN finished_at")

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          "task-finish-order-1",
		"session_id":       "session-finish-order-1",
		"agent_profile_id": agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	// FinishRun's write never landed, so the checkout must still be held.
	stillHeld, err := svc.CheckoutTask(ctx, "task-finish-order-1", "agent-third")
	if err != nil {
		t.Fatalf("checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("checkout was released even though FinishRun's DB update failed")
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusClaimed {
		t.Fatalf("run status = %q, want claimed (FinishRun's update never landed)", runs[0].Status)
	}
}
