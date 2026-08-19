package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerTick_ContendedCheckoutRequeuesRun is the regression test
// for the checkout-contention defect: a run that loses the atomic task
// checkout must be re-queued for retry, not finished as if it had
// completed successfully. Before the fix, tryCheckout called FinishRun
// on a lost checkout — RunStatus has no dedicated "skipped" state, so
// the run landed in 'finished' with an empty failure_reason,
// indistinguishable from a run that actually executed, and nothing ever
// re-queued it once the holder released the task.
func TestSchedulerTick_ContendedCheckoutRequeuesRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-checkout-loser",
		WorkspaceID: "ws-1",
		Name:        "checkout-loser",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-contended-1', 'ws-1', 'Contended Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// Simulate a different agent (e.g. the runner, woken by the same
	// status transition) having already taken the checkout.
	ok, err := svc.CheckoutTask(ctx, "task-contended-1", "agent-checkout-holder")
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-contended-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 0 {
		t.Fatalf("StartTask calls = %d, want 0 (checkout should have blocked launch)", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want queued (retried, not silently dropped as success)", runs[0].Status)
	}
	if runs[0].RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", runs[0].RetryCount)
	}

	evts, err := svc.ListRunEventsForTest(ctx, runs[0].ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	found := false
	for _, e := range evts {
		if e.EventType == "checkout.contended" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a checkout.contended run event, found none")
	}
}

// TestSchedulerTick_ContendedCheckoutEscalatesPastMaxRetries pins the
// bound on the re-queue loop: a run that keeps losing the checkout past
// MaxRetryCount must eventually be marked failed rather than retried
// forever.
func TestSchedulerTick_ContendedCheckoutEscalatesPastMaxRetries(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-checkout-exhausted",
		WorkspaceID: "ws-1",
		Name:        "checkout-exhausted",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-contended-2', 'ws-1', 'Contended Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	ok, err := svc.CheckoutTask(ctx, "task-contended-2", "agent-checkout-holder-2")
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, retry_count, requested_at
		) VALUES (
			'run-contended-exhausted', ?, 'task_assigned', '{"task_id":"task-contended-2"}',
			'queued', 1, '{}', ?, CURRENT_TIMESTAMP
		)
	`, agent.ID, service.MaxRetryCount)

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want failed after exhausting retries", runs[0].Status)
	}
}
