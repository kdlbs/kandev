package coordinator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func (e *mergeExec) forceLowering(t *testing.T, id string, state string) {
	t.Helper()
	err := e.store.withCoordinatorLock(e.ctx, e.fix.c.ID, func(tx coordinatorExec) error {
		return e.store.insertDispatchTx(e.ctx, tx, MergeDispatch{ProposalID: id, WorkspaceID: e.fix.c.WorkspaceID,
			ChangeKey: "other", HeadSHA: "x", ClaimToken: "t", Automatic: false, AutomaticSinceRevision: 0})
	})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, e.store, `UPDATE coordinator_merge_dispatches SET lowering = '`+state+`' WHERE proposal_id = '`+id+`'`)
}

func TestPendingLoweringKeepsAutomaticProposalPendingNotManagerOnly(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.forceLowering(t, id, loweringPending)
	c, p := e.fix.c, mustProposal(t, e, id)
	for i := 0; i < 2; i++ {
		e.svc.evaluateAutomaticMerge(e.ctx, c, p, e.detail(t, id), true)
	}
	m := e.detail(t, id)
	if st, _ := e.status(t, id); st != "pending" || m.ManagerOnly || m.LastReason != reasonLoweringPending {
		t.Fatalf("status = %s manager_only = %v reason = %q", st, m.ManagerOnly, m.LastReason)
	}
	if e.merger.mergeCalls() != 0 {
		t.Fatal("merged while a lowering was pending")
	}
	mustExec(t, e.store, `DELETE FROM coordinator_merge_dispatches WHERE proposal_id = '`+id+`'`)
	e.setPolicy(t, RepoAutomatic, 0)
	e.duties(t)
	if st, _ := e.status(t, id); st == "pending" || e.merger.mergeCalls() != 1 {
		t.Fatalf("not evaluated again: status = %s calls = %d detail = %+v", st, e.merger.mergeCalls(), e.detail(t, id))
	}
}

func TestDispatchRefusesPolicyRevisionThatMovedDuringReads(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) {
		base := f.reads
		f.onRead = func(n int) {
			if n == base+2 {
				e.setPolicy(t, RepoAutomatic, 7)
			}
		}
	})
	e.duties(t)
	m := e.detail(t, id)
	if e.merger.mergeCalls() != 0 || !m.ManagerOnly || m.LastReason != reasonPolicyChanged {
		t.Fatalf("calls = %d manager_only = %v reason = %q", e.merger.mergeCalls(), m.ManagerOnly, m.LastReason)
	}
}

func TestInvalidPrincipalDoesNotRefuseWhileWaiting(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoAutomatic, 0)
	e.admit(t)
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelinePending })
	id := *e.propose(t, "h1").ProposalID
	e.svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{}})
	e.duties(t)
	if st, code := e.status(t, id); st != "pending" || code != "" {
		t.Fatalf("waiting proposal settled %s/%s on a principal check", st, code)
	}
}

func TestAutomaticMergeConsumesTheSharedDailyBudget(t *testing.T) {
	e := newMergeExec(t)
	e.autoProposal(t, "h1")
	e.duties(t)
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("merge calls = %d", e.merger.mergeCalls())
	}
	n, err := e.store.CountAutomaticDecidedTx(e.ctx, e.store.ro, e.fix.c.ID, e.store.now().Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("shared automatic count = %d err = %v, want the merge dispatch counted once", n, err)
	}
}

func TestApproveAfterExpiryRefusesWithoutMerging(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	base := e.store.now
	e.store.now = func() time.Time { return base().Add(mergeExpiry + time.Minute) }
	_, _ = e.approve(id)
	if st, code := e.status(t, id); st != "rejected" || code != refuseExpired {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.merger.mergeCalls() != 0 {
		t.Fatal("expired proposal was merged")
	}
}

func mustProposal(t *testing.T, e *mergeExec, id string) *Proposal {
	t.Helper()
	p, err := e.store.GetProposalAnyKind(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLateProviderRejectionNotesOnlyAndDoesNotLower(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) {
		f.result = MergeResult{Outcome: MergeRejected, Reason: "late"}
		f.onMerge = func() {
			// Another path (Release) settles the claim while the request is in flight.
			mustExec(t, e.store, `UPDATE coordinator_proposals SET status = 'failed', claim_token = 'other' WHERE id = '`+id+`'`)
		}
	})
	e.duties(t)
	if st, _ := e.status(t, id); st != "failed" {
		t.Fatalf("late answer changed the status to %s", st)
	}
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s: a late rejection lowered", e.policy(t))
	}
	if n := e.activity(t, ActivityNote, "late_response"); n != 1 {
		t.Fatalf("late_response notes = %d", n)
	}
}

func TestRecoveredClaimOriginalWorkerCannotDispatch(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) {
		base := f.reads
		f.onRead = func(n int) {
			// Recovery re-takes the claim under a new token between the read and the dispatch.
			if n == base+2 {
				mustExec(t, e.store, `UPDATE coordinator_proposals SET status = 'approving', claim_token = 'successor' WHERE id = '`+id+`'`)
			}
		}
	})
	e.duties(t)
	if e.merger.mergeCalls() != 0 || e.countDispatches(t, id) != 0 {
		t.Fatalf("stale worker dispatched: calls = %d dispatches = %d", e.merger.mergeCalls(), e.countDispatches(t, id))
	}
}
