package coordinator

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/kandev/kandev/internal/common/logger"
)

// The tests in this file are the rows of the interleaving table in
// docs/plans/workspace-coordinator-p2/task-04-proposal-kinds-backend.md.

type kindsFixture struct {
	store *Store
	c     *Coordinator
	svc   *Service
	undo  *fakeUndoTasks
	seq   int
	tasks *fakeKindTasks
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
	mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"move": "requires_approval", "resume": "requires_approval", "message": "requires_approval"}))
	return &kindsFixture{store: store, c: c, svc: svc, undo: undo}
}

// watchFenced counts execute_settle_fenced warnings and closes the returned
// channel on the first one.
func (f *kindsFixture) watchFenced(t *testing.T) (*atomic.Int32, <-chan struct{}) {
	t.Helper()
	var n atomic.Int32
	first := make(chan struct{})
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zap.WarnLevel)
	z := zap.New(core).WithOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Message == "execute_settle_fenced" && n.Add(1) == 1 {
			close(first)
		}
		return nil
	}))
	log, err := logger.NewFromZap(z)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.logger = log
	return &n, first
}

func (f *kindsFixture) outcomeOf(t *testing.T, id string) string {
	t.Helper()
	got, err := f.store.GetProposal(context.Background(), f.c.WorkspaceID, f.c.ID, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeJSON == nil {
		return ""
	}
	return *got.OutcomeJSON
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

// Row 1: two concurrent approves of one pending move proposal. Each caller
// gets a conflict or the settled row, and the move is called once.
func TestKindsInterleaving1_ConcurrentApprovesMoveOnce(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	type result struct {
		got *Proposal
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
			results <- result{got, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for r := range results {
		var conflict *ProposalConflictError
		if r.err != nil {
			if !errors.As(r.err, &conflict) {
				t.Fatalf("loser err = %v, want a conflict", r.err)
			}
			continue
		}
		if r.got.Status != ProposalStatusApproved {
			t.Fatalf("returned row status = %q, want approved", r.got.Status)
		}
	}
	if len(f.undo.moves) != 1 {
		t.Fatalf("move calls = %d, want 1", len(f.undo.moves))
	}
	if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusApproved) {
		t.Fatalf("status = %q, want approved", got)
	}
}

// Row 1, forced overlap: the second approve runs while the first is inside
// the move call and is refused as a conflict.
func TestKindsInterleaving1_SecondApproveDuringMoveIsConflict(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	inMove, release := make(chan struct{}), make(chan struct{})
	f.undo.onMove = func() { close(inMove); <-release }
	done := make(chan error, 1)
	go func() {
		_, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
		done <- err
	}()
	<-inMove
	_, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusApproving)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if len(f.undo.moves) != 1 || statusOf(t, f.store, p.ID) != string(ProposalStatusApproved) {
		t.Fatalf("moves = %d status = %q, want 1 and approved", len(f.undo.moves), statusOf(t, f.store, p.ID))
	}
}

// Row 2: the sweep settles the claim while Execute is inside the move call.
func TestKindsInterleaving2_SweepDuringExecuteWritesNothingLate(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	fenced, _ := f.watchFenced(t)
	var atSweep string
	f.undo.onMove = func() {
		f.svc.runApprovalSweepPass(context.Background(), time.Now().Add(time.Hour))
		atSweep = f.outcomeOf(t, p.ID)
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
	if fenced.Load() != 1 || f.outcomeOf(t, p.ID) != atSweep {
		t.Fatalf("fenced warnings = %d outcome = %q, want 1 and the outcome unchanged from %q", fenced.Load(), f.outcomeOf(t, p.ID), atSweep)
	}
}

// Row 5: an approve that read the row while it was failed loses to a reject
// that settled it: the conditional claim matches nothing and Execute is not
// called.
func TestKindsInterleaving5_StaleApproveReadLosesToReject(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	if _, err := f.store.db.Exec(f.store.db.Rebind(`UPDATE coordinator_proposals SET status = 'failed', error = 'x' WHERE id = ?`), p.ID); err != nil {
		t.Fatal(err)
	}
	stale, err := f.store.GetProposal(context.Background(), f.c.WorkspaceID, f.c.ID, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RejectProposal(context.Background(), "ws-1", f.c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.approveKind(context.Background(), f.svc.kindExecutor(ProposalKindMove), stale, ApproveProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusRejected)
	if len(f.undo.moves) != 0 || statusOf(t, f.store, p.ID) != string(ProposalStatusRejected) {
		t.Fatalf("moves = %d status = %q, want 0 and rejected", len(f.undo.moves), statusOf(t, f.store, p.ID))
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
// makes no move call, flag on or off.
func TestKindsInterleaving9_ApproveOfStaleNonCreateClaimSettlesFailed(t *testing.T) {
	for _, phase2 := range []bool{true, false} {
		f := newKindsFixture(t)
		p := f.insertMove(t)
		f.forceApproving(t, p, time.Now().Add(-time.Hour))
		f.svc.phase2 = phase2
		got, err := f.svc.ApproveProposal(context.Background(), "ws-1", f.c.ID, p.ID, ApproveProposalRequest{})
		if err != nil {
			t.Fatalf("phase2=%v approve: %v", phase2, err)
		}
		if got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != "outcome_unknown" || len(f.undo.moves) != 0 {
			t.Fatalf("phase2=%v status=%q error=%q moves=%d, want failed outcome_unknown and 0", phase2, got.Status, f.errorOf(t, p.ID), len(f.undo.moves))
		}
	}
}

// A failed read of a stale non-create row must not fall through to the
// create-path re-claim: the claim is left as it was.
func TestReclaimStale_ReadFailureLeavesNonCreateClaimAlone(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	f.forceApproving(t, p, time.Now().Add(-time.Hour))
	if _, err := f.store.db.Exec(f.store.db.Rebind(`UPDATE coordinator_proposals SET standing_order_ids = 'not json' WHERE id = ?`), p.ID); err != nil {
		t.Fatal(err)
	}
	var before string
	_ = f.store.db.Get(&before, f.store.db.Rebind(`SELECT claim_token FROM coordinator_proposals WHERE id = ?`), p.ID)
	if _, err := f.svc.reclaimStaleAndProceed(context.Background(), "ws-1", f.c.ID, p.ID, time.Now()); err == nil {
		t.Fatal("want the read error")
	}
	var after string
	_ = f.store.db.Get(&after, f.store.db.Rebind(`SELECT claim_token FROM coordinator_proposals WHERE id = ?`), p.ID)
	if before != after || statusOf(t, f.store, p.ID) != string(ProposalStatusApproving) {
		t.Fatalf("claim token %q -> %q status %q, want the claim untouched", before, after, statusOf(t, f.store, p.ID))
	}
}
