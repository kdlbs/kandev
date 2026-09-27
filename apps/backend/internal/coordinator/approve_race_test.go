package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// This file covers the task-07 Verification section's named gaps that
// approve_test.go's status-routing and edits-merge groups do not reach: a
// plain read error surfacing through the real ApproveProposal entry point
// before any claim, a non-ErrTaskNotFound lookup error on a failed row, a
// step-graph read error at the pre-create eligibility check (both the
// original-claimer and stale-reclaim paths), and the claimRaceResult/
// settleWriteRace re-read branch when the row is gone
// (docs/plans/workspace-coordinator/task-07-proposals-backend.md#verification).

// errAfterNStepReader returns steps for the first n calls, then err for
// every call after. Used to fail only a later step-graph read (the
// post-claim eligibility re-check) while leaving an earlier one (spec
// validation) unaffected; fakeStepReader's err field fails every call
// unconditionally, which cannot express that split.
type errAfterNStepReader struct {
	steps []*workflowmodels.WorkflowStep
	n     int
	err   error
	calls int
}

func (r *errAfterNStepReader) ListStepsByWorkflow(context.Context, string) ([]*workflowmodels.WorkflowStep, error) {
	r.calls++
	if r.calls > r.n {
		return nil, r.err
	}
	return r.steps, nil
}

// TestApproveProposal_WorkflowReadErrorPropagatesBeforeClaim proves a plain
// GetWorkflow read error reaches the real ApproveProposal entry point as-is
// (not a FieldError, not a conflict), leaves the row pending, and publishes
// nothing ("a read error before the claim returns 500 with the row
// unchanged and nothing published").
func TestApproveProposal_WorkflowReadErrorPropagatesBeforeClaim(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	wantErr := errors.New("workflow lookup boom")
	tasks.workflowErr = map[string]error{"wf-1": wantErr}
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(tasks, svc.decisionSteps, eventBus)
	received, cleanup := subscribeCoordinatorUpdated(t, eventBus)
	defer cleanup()

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var conflict *ProposalConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a plain error, not a conflict", err)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusPending {
		t.Fatalf("Status = %q, want pending (no write should have happened)", reread.Status)
	}

	select {
	case payload := <-received:
		t.Fatalf("coordinator.updated published on a pre-claim read error: %+v", payload)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestApproveFailed_LookupErrorPropagatesRowUnchanged proves approveFailed's
// GetTaskByExternalID lookup propagates a non-ErrTaskNotFound error as-is and
// leaves the failed row untouched ("a lookup error on a failed row returns
// 500 with the row unchanged").
func TestApproveFailed_LookupErrorPropagatesRowUnchanged(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	if _, err := store.FailProposal(context.Background(), p.ID, "tok", "boom", time.Now()); err != nil {
		t.Fatalf("FailProposal (setup): %v", err)
	}
	wantErr := errors.New("lookup boom")
	tasks.lookupErr = map[string]error{proposalExternalID(p.ID): wantErr}

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var conflict *ProposalConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a plain error, not a conflict", err)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed (unchanged)", reread.Status)
	}
}

// TestApproveProposal_StepGraphReadErrorPreCreateLeavesApproving proves a
// step-graph read error at completeClaimedApproval's pre-create eligibility
// check (reached via the original-claimer approvePending path) propagates as
// a plain error and leaves the row claimed ("approving" with this request's
// own token) rather than failing it, since the create call never ran ("a
// step-graph read error at that check leaves the row approving and returns
// 500").
func TestApproveProposal_StepGraphReadErrorPreCreateLeavesApproving(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	wantErr := errors.New("step graph boom")
	svc.SetDecisionDeps(tasks, &errAfterNStepReader{
		steps: []*workflowmodels.WorkflowStep{{ID: "step-1", IsStartStep: true}},
		n:     1,
		err:   wantErr,
	}, nil)

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var conflict *ProposalConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a plain error, not a conflict", err)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (claimed but left for recovery)", reread.Status)
	}
	if reread.ClaimToken == nil {
		t.Fatal("ClaimToken = nil, want this request's own claim token")
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

// TestApproveProposal_StaleReclaimLookupErrorLeavesApprovingWithNewToken
// proves reclaimStaleAndProceed's GetTaskByExternalID lookup, run right
// after a successful stale re-claim, propagates a non-ErrTaskNotFound error
// and leaves the row approving with the re-claim's fresh token rather than
// the original stale one ("a lookup ... error after a re-claim leaves the
// row approving with the new token and returns 500").
func TestApproveProposal_StaleReclaimLookupErrorLeavesApprovingWithNewToken(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-3*time.Minute))
	wantErr := errors.New("lookup boom")
	tasks.lookupErr = map[string]error{proposalExternalID(p.ID): wantErr}

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving", reread.Status)
	}
	if reread.ClaimToken == nil || *reread.ClaimToken == "old-tok" {
		t.Fatalf("ClaimToken = %v, want a fresh token from this re-claim, not old-tok", reread.ClaimToken)
	}
}

// TestApproveProposal_StaleReclaimStepGraphReadErrorLeavesApprovingWithNewToken
// proves a step-graph read error at the pre-create eligibility check reached
// via the stale-reclaim path (lookup misses with ErrTaskNotFound, so
// completeClaimedApproval runs stepStillEligible) behaves the same way: a
// plain error, row left approving with the new token, no create call.
func TestApproveProposal_StaleReclaimStepGraphReadErrorLeavesApprovingWithNewToken(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-3*time.Minute))
	wantErr := errors.New("step graph boom")
	svc.SetDecisionDeps(tasks, &fakeStepReader{err: wantErr}, nil)

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving", reread.Status)
	}
	if reread.ClaimToken == nil || *reread.ClaimToken == "old-tok" {
		t.Fatalf("ClaimToken = %v, want a fresh token from this re-claim, not old-tok", reread.ClaimToken)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

// TestClaimRaceResult_ProposalDeletedReturnsNotFound proves claimRaceResult's
// re-read branch reports ErrNotFound when the zero-row CAS it is handling
// raced against the proposal's own deletion, not just a status change
// (proposals.md#approve step 3, #stale-re-claim). claimRaceResult is
// exercised directly (white-box, same package) because the CAS's atomicity
// makes any concurrent-delete interleaving equivalent to the row already
// being gone by the time the race handler re-reads it — a real
// concurrent-goroutine test would prove nothing this deterministic ordering
// does not already prove.
func TestClaimRaceResult_ProposalDeletedReturnsNotFound(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if err := store.DeleteCoordinator(context.Background(), "ws-1", c.ID); err != nil {
		t.Fatalf("DeleteCoordinator (setup): %v", err)
	}

	_, err := svc.claimRaceResult(context.Background(), "ws-1", c.ID, p.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestSettleWriteRace_ProposalDeletedReturnsNotFound is
// TestClaimRaceResult_ProposalDeletedReturnsNotFound's twin for step 5's
// complete/fail fence race handler.
func TestSettleWriteRace_ProposalDeletedReturnsNotFound(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if err := store.DeleteCoordinator(context.Background(), "ws-1", c.ID); err != nil {
		t.Fatalf("DeleteCoordinator (setup): %v", err)
	}

	_, err := svc.settleWriteRace(context.Background(), "ws-1", c.ID, p.ID, "task-orphaned")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
