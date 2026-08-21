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

	// The holder must still own the checkout: requeuing the loser must not
	// touch a lock it never acquired.
	stillHeld, err := svc.CheckoutTask(ctx, "task-contended-1", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a third agent acquired the checkout while agent-checkout-holder still held it")
	}
}

// TestSchedulerTick_ContendedCheckoutEscalatesPastMaxRetries pins the
// bound on the re-queue loop: a run that keeps losing the checkout past
// MaxRetryCount must eventually be marked failed rather than retried
// forever. It also pins the fix for a Review round-1 regression: escalation
// routes through escalateFailure -> FailRun -> transitionRunTerminal ->
// releaseTaskCheckoutForRun, and the escalating run is by construction the
// LOSER of the checkout race. Before the release was scoped to the run's
// own agent, this call cleared checkout_agent_id unconditionally by task
// ID, so the escalation of a run that never held the lock stole it out
// from under the real holder — the exact inverse of the leak this defect
// board exists to fix.
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

	// The real holder must still own the checkout: the escalated run
	// (agent-checkout-exhausted) never held it, so failing it must not
	// release agent-checkout-holder-2's lock.
	stillHeld, err := svc.CheckoutTask(ctx, "task-contended-2", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a third agent acquired the checkout while agent-checkout-holder-2 still held it")
	}
}

// TestSchedulerTick_StaleContendedRunDoesNotStealADifferentAgentsLiveCheckout
// pins the Review round-4 BLOCKING FINDING 1 fix: requeueContendedCheckout
// (the fix for the checkout-contention defect above) calls ScheduleRetry
// with RetryCount+1, so a run that has lost the checkout race even once can
// accumulate RetryCount >= 1. If that run's original RequestedAt is older
// than staleRunThreshold, evaluateRunStaleness now routes it to
// cancelStaleRun on a later tick - a path reachable BEFORE the run ever
// calls checkoutTask, exactly like the "agent not active" branch in
// scheduler_checkout_inactive_agent_test.go. Before this fix, cancelStaleRun
// called the unscoped releaseCheckoutIfNeeded(ctx, taskID), which cleared
// checkout_agent_id by task ID alone and stole a different, currently-active
// agent's live lock out from under it - reopening the exact leak this
// defect board exists to fix.
func TestSchedulerTick_StaleContendedRunDoesNotStealADifferentAgentsLiveCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	holder := &models.AgentInstance{
		ID:          "agent-stale-holder",
		WorkspaceID: "ws-1",
		Name:        "checkout-stale-holder",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, holder); err != nil {
		t.Fatalf("create holder agent: %v", err)
	}

	staleAgent := &models.AgentInstance{
		ID:          "agent-stale-contended",
		WorkspaceID: "ws-1",
		Name:        "checkout-stale-contended",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, staleAgent); err != nil {
		t.Fatalf("create stale agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-stale-1', 'ws-1', 'Stale Contention Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// The holder's own live, executing run already owns the checkout.
	ok, err := svc.CheckoutTask(ctx, "task-stale-1", holder.ID)
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	// A second run, for a DIFFERENT agent, already lost the checkout race
	// once (retry_count=1, simulating requeueContendedCheckout having run)
	// and was originally requested over 2 hours ago - old enough that
	// evaluateRunStaleness now routes it to cancelStaleRun on this tick
	// instead of another contended retry.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, retry_count, requested_at
		) VALUES (
			'run-stale-contended', ?, 'task_assigned', '{"task_id":"task-stale-1"}',
			'queued', 1, '{}', 1, datetime('now', '-3 hours')
		)
	`, staleAgent.ID)

	service.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var staleRun models.Run
	found := false
	for _, r := range runs {
		if r.ID == "run-stale-contended" {
			staleRun = *r
			found = true
		}
	}
	if !found {
		t.Fatalf("run-stale-contended not found among runs: %v", runs)
	}
	if staleRun.Status != service.RunStatusCancelled {
		t.Fatalf("run status = %q, want cancelled (stale)", staleRun.Status)
	}

	// The real holder must still own the checkout: the stale run never held
	// it (it is a different agent, and evaluateRunStaleness fires before
	// checkoutTask on this tick), so cancelling it must not release
	// agent-stale-holder's live lock.
	stillHeld, err := svc.CheckoutTask(ctx, "task-stale-1", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a third agent acquired the checkout while agent-stale-holder still held it")
	}
}

// TestSchedulerTick_StaleSameAgentRunDoesNotStealLiveCheckout pins the
// run-identity half of the checkout ownership contract. A retrying run can
// become stale before it reaches checkoutTask while an earlier run for the
// same agent still owns the task. Agent-only release cannot distinguish those
// runs and clears the live checkout.
func TestSchedulerTick_StaleSameAgentRunDoesNotStealLiveCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-stale-same-holder",
		WorkspaceID: "ws-1",
		Name:        "checkout-stale-same-holder",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-stale-same-1', 'ws-1', 'Stale Same-Agent Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// An earlier live run for this agent already owns the checkout.
	ok, err := svc.CheckoutTaskForRun(ctx, "task-stale-same-1", agent.ID, "run-live-same")
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	// A newer retry for the same agent never reaches checkoutTask. It is old
	// enough for the staleness guard to cancel it before launch.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, retry_count, requested_at
		) VALUES (
			'run-stale-same', ?, 'task_assigned', '{"task_id":"task-stale-same-1"}',
			'queued', 1, '{}', 1, datetime('now', '-3 hours')
		)`, agent.ID)

	service.RunSchedulerTick(svc, ctx)

	stillHeld, err := svc.CheckoutTask(ctx, "task-stale-same-1", "agent-third")
	if err != nil {
		t.Fatalf("holder-checkout probe: %v", err)
	}
	if stillHeld {
		t.Fatal("LOCK STOLEN: a stale same-agent run cleared the live checkout")
	}
}
