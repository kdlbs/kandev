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

// TestSchedulerTick_SameAgentPreCheckoutRunDoesNotStealOwnLiveCheckout is
// the same-agent variant of the regression above (Review round 3, BLOCKING
// FINDING 1): releaseTaskCheckoutForRun is scoped by run.AgentProfileID,
// which guards the cross-agent case but not this one. Agent X already
// holds the checkout via a live, executing run (run B). A second run (run
// A) for that SAME agent + task is queued (e.g. a comment or status
// change) and then terminates via the "agent not active" branch before it
// ever reaches checkoutTask, because the agent went paused between queue
// and tick. Run A's release matches X's own checkout_agent_id and, before
// this fix, cleared run B's still-live lock out from under it.
func TestSchedulerTick_SameAgentPreCheckoutRunDoesNotStealOwnLiveCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	holder := &models.AgentInstance{
		ID:          "agent-same-holder",
		WorkspaceID: "ws-1",
		Name:        "checkout-same-holder",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, holder); err != nil {
		t.Fatalf("create holder agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-same-1', 'ws-1', 'Same Agent Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// Run B: the holder's own live, executing run already owns the
	// checkout (simulates a launched run mid-turn, checked out earlier).
	ok, err := svc.CheckoutTask(ctx, "task-same-1", holder.ID)
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	// Run A: a second run for the SAME agent + task (e.g. a comment wake),
	// queued while idle, then the agent goes paused before the tick picks
	// it up - reproducing the race where the holder is deactivated after
	// a second run was already queued behind its own live run.
	if err := svc.QueueRun(ctx, holder.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-same-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	svc.ExecSQL(t, `UPDATE agent_profiles SET status = 'paused' WHERE id = 'agent-same-holder'`)

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

	// Run B's checkout must still be held: run A never held it, so
	// finishing run A must not release the holder's own live lock.
	stillHeld, err := svc.CheckoutTask(ctx, "task-same-1", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a third agent acquired the checkout that agent-same-holder's live run still held")
	}
}
