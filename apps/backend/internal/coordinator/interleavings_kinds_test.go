package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The tests in this file are the rows of the interleaving table in
// docs/plans/workspace-coordinator-p2/task-04-proposal-kinds-backend.md.

type kindsFixture struct {
	store *Store
	c     *Coordinator
	svc   *Service
	undo  *fakeUndoTasks
}

func newKindsFixture(t *testing.T) *kindsFixture {
	t.Helper()
	store, c, _, svc := phase2ApproveFixture(t)
	undo := &fakeUndoTasks{
		tasks:    map[string]*UndoTask{"task-0": {WorkflowID: "wf-1", WorkflowStepID: "step-1"}},
		steps:    map[string]*UndoStep{"manual-step": {Name: "Review", WorkflowID: "wf-1"}},
		admitted: true,
	}
	svc.SetUndoDeps(undo)
	mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"move": "requires_approval"}))
	return &kindsFixture{store: store, c: c, svc: svc, undo: undo}
}

func (f *kindsFixture) insertMove(t *testing.T) *Proposal {
	t.Helper()
	target := "task-0"
	p := &Proposal{
		CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID, Kind: ProposalKindMove, TargetTaskID: &target,
		RawSpec: `{"task_id":"task-0","workflow_id":"wf-1","from_step_id":"step-1","to_step_id":"manual-step","rationale":"r"}`,
	}
	if err := f.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	return p
}

func (f *kindsFixture) forceApproving(t *testing.T, p *Proposal, claimedAt time.Time) {
	t.Helper()
	if _, err := f.store.db.Exec(f.store.db.Rebind(
		`UPDATE coordinator_proposals SET status = 'approving', claim_token = 'tok', claimed_at = ? WHERE id = ?`),
		claimedAt.UTC(), p.ID); err != nil {
		t.Fatal(err)
	}
}

func (f *kindsFixture) errorOf(t *testing.T, id string) string {
	t.Helper()
	got, err := f.store.GetProposal(context.Background(), f.c.WorkspaceID, f.c.ID, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == nil {
		return ""
	}
	return *got.Error
}

// Row 1: two concurrent approves of one pending move proposal.
func TestKindsInterleaving1_ConcurrentApprovesMoveOnce(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
		}()
	}
	wg.Wait()
	if len(f.undo.moves) != 1 {
		t.Fatalf("move calls = %d, want 1", len(f.undo.moves))
	}
	if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusApproved) {
		t.Fatalf("status = %q, want approved", got)
	}
}

// Row 2: the sweep settles the claim while Execute is inside the move call.
func TestKindsInterleaving2_SweepDuringExecuteWritesNothingLate(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	f.undo.onMove = func() {
		f.svc.runApprovalSweepPass(context.Background(), time.Now().Add(time.Hour))
	}
	got, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != "outcome_unknown" {
		t.Fatalf("status=%q error=%q, want failed outcome_unknown", got.Status, f.errorOf(t, p.ID))
	}
	if len(f.undo.moves) != 1 {
		t.Fatalf("move calls = %d, want 1", len(f.undo.moves))
	}
}

// Row 3: a crash after Execute and before the settle; the startup pass
// settles outcome_unknown and never calls the move again, flag on or off.
func TestKindsInterleaving3_StartupPassSettlesOutcomeUnknown(t *testing.T) {
	for _, phase2 := range []bool{true, false} {
		f := newKindsFixture(t)
		p := f.insertMove(t)
		f.forceApproving(t, p, time.Now().Add(-time.Hour))
		f.svc.phase2 = phase2
		f.svc.StartupRecoveryPass(context.Background(), time.Now())
		if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusFailed) {
			t.Fatalf("phase2=%v status = %q, want failed", phase2, got)
		}
		if f.errorOf(t, p.ID) != "outcome_unknown" || len(f.undo.moves) != 0 {
			t.Fatalf("phase2=%v error=%q moves=%d, want outcome_unknown and 0", phase2, f.errorOf(t, p.ID), len(f.undo.moves))
		}
	}
}

// Row 4: a reject arriving while the row is approving is refused.
func TestKindsInterleaving4_RejectWhileApprovingIsConflict(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	f.undo.onMove = func() {
		_, err := f.svc.RejectProposal(context.Background(), "ws-1", f.c.ID, p.ID, RejectProposalRequest{})
		_ = assertConflict(t, err, ProposalStatusApproving)
	}
	if _, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusApproved) {
		t.Fatalf("status = %q, want approved", got)
	}
}

// Row 9: approve of a stale non-create claim returns the failed row and
// makes no move call.
func TestKindsInterleaving9_ApproveOfStaleNonCreateClaimSettlesFailed(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	f.forceApproving(t, p, time.Now().Add(-time.Hour))
	got, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != "outcome_unknown" || len(f.undo.moves) != 0 {
		t.Fatalf("status=%q error=%q moves=%d, want failed outcome_unknown and 0", got.Status, f.errorOf(t, p.ID), len(f.undo.moves))
	}
}
