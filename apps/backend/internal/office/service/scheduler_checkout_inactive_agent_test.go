package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerTick_InactiveAgentRunDoesNotStealCheckout pins the second
// reproduction path from the Review round-1 lock-stealing regression
// (BLOCKING FINDING 1): processRun finishes a run for a paused/stopped/
// pending-approval agent via the "run skipped (agent not active)" branch
// BEFORE that run ever reaches checkoutTask. That run never held the task's
// checkout. Before releaseTaskCheckoutForRun was scoped to the run's own
// agent, transitionRunTerminal's unconditional release cleared
// checkout_agent_id by task ID alone, so finishing this run stole a
// different, currently-active agent's live lock out from under it.
func TestSchedulerTick_InactiveAgentRunDoesNotStealCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	holder := &models.AgentInstance{
		ID:          "agent-inactive-holder",
		WorkspaceID: "ws-1",
		Name:        "checkout-holder",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, holder); err != nil {
		t.Fatalf("create holder agent: %v", err)
	}

	inactive := &models.AgentInstance{
		ID:          "agent-inactive-paused",
		WorkspaceID: "ws-1",
		Name:        "paused-agent",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, inactive); err != nil {
		t.Fatalf("create inactive agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-inactive-1', 'ws-1', 'Inactive Agent Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// A different, currently-active agent already holds the checkout.
	ok, err := svc.CheckoutTask(ctx, "task-inactive-1", holder.ID)
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	// Queue while idle (QueueRun itself refuses to queue for a paused
	// agent), then go paused before the tick runs - reproducing the race
	// where an agent is deactivated between queueing and processing.
	if err := svc.QueueRun(ctx, inactive.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-inactive-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	svc.ExecSQL(t, `UPDATE agent_profiles SET status = 'paused' WHERE id = 'agent-inactive-paused'`)

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusFinished {
		t.Fatalf("run status = %q, want finished (skipped for inactivity)", runs[0].Status)
	}

	// The real holder must still own the checkout: the skipped run
	// (agent-inactive-paused) never held it, so finishing it must not
	// release agent-inactive-holder's lock.
	stillHeld, err := svc.CheckoutTask(ctx, "task-inactive-1", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a third agent acquired the checkout while agent-inactive-holder still held it")
	}
}
