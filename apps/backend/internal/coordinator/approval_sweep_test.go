package coordinator

import (
	"context"
	"testing"
	"time"
)

// TestStartApprovalSweep_RecoversOnTick covers proposal-recovery.md#recovery's
// sweep: a claim stale by more than two minutes at tick time is recovered on
// that tick, sharing the same stale re-claim path the startup pass uses, with
// cutoff computed from the injected tick time rather than wall-clock time.
func TestStartApprovalSweep_RecoversOnTick(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tickTime := time.Now()
	claimDirectly(t, store, p, "old-tok", sampleSpec(), tickTime.Add(-3*time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	passDone := make(chan struct{}, 1)
	svc.afterSweepPass = func() { passDone <- struct{}{} }
	tick := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); svc.sweepWG.Wait() })

	svc.startApprovalSweep(ctx, tick, func() {})
	tick <- tickTime
	<-passDone

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if got.TaskID == nil || *got.TaskID != "task-new" {
		t.Fatalf("TaskID = %v, want task-new", got.TaskID)
	}
}

// TestStartApprovalSweep_LeavesClaimsYoungerThanTwoMinutesAlone covers the
// sweep's cutoff: a claim younger than two minutes at tick time is left
// untouched, exactly like the startup pass's own cutoff rule.
func TestStartApprovalSweep_LeavesClaimsYoungerThanTwoMinutesAlone(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tickTime := time.Now()
	claimDirectly(t, store, p, "recent-tok", sampleSpec(), tickTime.Add(-time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	passDone := make(chan struct{}, 1)
	svc.afterSweepPass = func() { passDone <- struct{}{} }
	tick := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); svc.sweepWG.Wait() })

	svc.startApprovalSweep(ctx, tick, func() {})
	tick <- tickTime
	<-passDone

	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (untouched)", got.Status)
	}
}

// TestStartApprovalSweep_StopsOnContextCancel covers the sweep's shutdown:
// cancelling ctx stops the loop and the stop callback (the real ticker's
// Stop) runs exactly once.
func TestStartApprovalSweep_StopsOnContextCancel(t *testing.T) {
	_, _, _, svc := approveFixture(t)
	tick := make(chan time.Time, 1)
	stopped := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())

	svc.startApprovalSweep(ctx, tick, func() { stopped <- struct{}{} })
	cancel()
	svc.sweepWG.Wait()

	select {
	case <-stopped:
	default:
		t.Fatal("stop callback was not called after context cancellation")
	}
}

// TestStartApprovalSweep_SecondCallIsNoop covers idempotent start: calling
// startApprovalSweep again while one loop is already running must not spawn
// a second goroutine (a second stop callback would fire on cancel if it had).
func TestStartApprovalSweep_SecondCallIsNoop(t *testing.T) {
	_, _, _, svc := approveFixture(t)
	tick := make(chan time.Time, 1)
	stopCalls := make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(context.Background())

	svc.startApprovalSweep(ctx, tick, func() { stopCalls <- struct{}{} })
	svc.startApprovalSweep(ctx, tick, func() { stopCalls <- struct{}{} })
	cancel()
	svc.sweepWG.Wait()

	if len(stopCalls) != 1 {
		t.Fatalf("stop callback fired %d times, want exactly 1 (second start must be a no-op)", len(stopCalls))
	}
}
