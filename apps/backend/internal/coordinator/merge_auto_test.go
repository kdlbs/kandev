package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func (e *mergeExec) dailyLimit(t *testing.T, limit int) {
	t.Helper()
	c, err := e.store.GetCoordinatorByID(context.Background(), e.fix.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := e.svc.policyFor(c)
	el := DefaultEligibility()
	el.DailyLimit = limit
	p.Eligibility = &el
	raw, _ := json.Marshal(p)
	mustExec(t, e.store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`, string(raw), e.fix.c.ID)
}

func (e *mergeExec) autoProposal(t *testing.T, head string) string {
	t.Helper()
	e.setPolicy(t, RepoAutomatic, 0)
	e.admit(t)
	return *e.propose(t, head).ProposalID
}

func (e *mergeExec) detail(t *testing.T, id string) *MergeProposal {
	t.Helper()
	m, err := e.store.getMergeProposal(e.ctx, e.store.ro, id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAutomaticWaitsOnPending(t *testing.T) {
	for name, set := range map[string]func(*fakeMerger){
		"pipeline pending": func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelinePending },
		"pipeline none":    func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineNone },
		"approvals":        func(f *fakeMerger) { f.snap.Approvals = mergestate.Unmet },
		"threads":          func(f *fakeMerger) { f.snap.UnresolvedThreads = 1 },
		"draft":            func(f *fakeMerger) { f.snap.Draft = mergestate.Unmet },
	} {
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			e.setPolicy(t, RepoAutomatic, 0)
			e.admit(t)
			id := *e.propose(t, "h1").ProposalID
			e.merger.set(set)
			e.duties(t)
			e.duties(t)
			if st, _ := e.status(t, id); st != "pending" {
				t.Fatalf("status = %s", st)
			}
			if e.merger.mergeCalls() != 0 {
				t.Fatal("merged while waiting")
			}
			if n := e.countActivity(t, ActivityNote); n != 1 {
				t.Fatalf("waiting notes = %d, want one per change of reason", n)
			}
		})
	}
}

func (e *mergeExec) countActivity(t *testing.T, outcome ActivityOutcome) int {
	t.Helper()
	var n int
	if err := e.store.db.QueryRow(e.store.db.Rebind(`SELECT COUNT(*) FROM coordinator_activity WHERE action_class = 'merge' AND outcome = ?`), string(outcome)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAutomaticRefusesRedAndConflict(t *testing.T) {
	for name, set := range map[string]func(*fakeMerger){
		"red":      func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineRed },
		"conflict": func(f *fakeMerger) { f.snap.Conflict = mergestate.ConflictPresent },
	} {
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			id := e.autoProposal(t, "h1")
			e.merger.set(set)
			e.duties(t)
			if st, _ := e.status(t, id); st != "rejected" {
				t.Fatalf("status = %s", st)
			}
			if e.merger.mergeCalls() != 0 {
				t.Fatal("merged")
			}
		})
	}
}

func TestAutomaticPausedManagerOnly(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	if _, err := e.store.SetPaused(context.Background(), e.fix.c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
	e.duties(t)
	m := e.detail(t, id)
	if st, _ := e.status(t, id); st != "pending" || !m.ManagerOnly || m.Note != pausedNote {
		t.Fatalf("status = %s, manager_only = %v, note = %q", st, m.ManagerOnly, m.Note)
	}
	if _, err := e.store.SetPaused(context.Background(), e.fix.c.ID, false, "u-1"); err != nil {
		t.Fatal(err)
	}
	e.duties(t)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("a manager_only proposal was evaluated automatically again")
	}
}

func TestManagerApproveIgnoresPause(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	if _, err := e.store.SetPaused(context.Background(), e.fix.c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.approve(id); err != nil {
		t.Fatal(err)
	}
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("merge calls = %d", e.merger.mergeCalls())
	}
}

func TestManagerApproveUnmetRefuses(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelinePending })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st != "rejected" || e.merger.mergeCalls() != 0 {
		t.Fatalf("status = %s, calls = %d", st, e.merger.mergeCalls())
	}
}

func TestAutomaticDailyLimitManagerOnly(t *testing.T) {
	e := newMergeExec(t)
	e.dailyLimit(t, 1)
	first := e.autoProposal(t, "h1")
	e.duties(t)
	if st, _ := e.status(t, first); st != "approved" {
		t.Fatalf("first status = %s", st)
	}
	e.merger.set(func(f *fakeMerger) { f.snap = greenSnapshot("h5") })
	second := *e.propose(t, "h5").ProposalID
	e.svc.changeMerger().(*fakeMerger).set(func(f *fakeMerger) { f.snap.Key.Number = 43 })
	e.duties(t)
	m := e.detail(t, second)
	if !m.ManagerOnly || m.Note != noteDailyLimit {
		t.Fatalf("manager_only = %v, note = %q", m.ManagerOnly, m.Note)
	}
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("merge calls = %d, the limit allows one", e.merger.mergeCalls())
	}
}

func TestAutomaticExpiresAfter7Days(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoAutomatic, 0)
	e.admit(t)
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelinePending })
	id := *e.propose(t, "h1").ProposalID
	base := e.store.now
	e.store.now = func() time.Time { return base().Add(mergeExpiry + time.Minute) }
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseExpired {
		t.Fatalf("status = %s/%s", st, code)
	}
}

func TestPolicyChangedMakesManagerOnlyAndApproveStillWorks(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.setPolicy(t, RepoRequiresApproval, 0)
	e.duties(t)
	m := e.detail(t, id)
	if !m.ManagerOnly || m.LastReason != reasonPolicyChanged {
		t.Fatalf("manager_only = %v, reason = %q", m.ManagerOnly, m.LastReason)
	}
	if _, err := e.approve(id); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.status(t, id); st != "approved" {
		t.Fatalf("status = %s", st)
	}
}

func TestDeniedPolicySettlesAuthority(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.setPolicy(t, RepoDenied, 0)
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseAuthority {
		t.Fatalf("status = %s/%s", st, code)
	}
}

func TestInvalidPrincipalRefusesAuthorityNoLower(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.svc.SetPrincipalValidator(fakePrincipals{ok: map[string]bool{}})
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseAuthority {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s", e.policy(t))
	}
}

func TestProviderRejectLowers(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected, Reason: "branch protection"} })
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseProviderReject {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.policy(t) != RepoRequiresApproval {
		t.Fatalf("policy = %s, want lowered", e.policy(t))
	}
}

func TestManagerProviderRejectDoesNotLower(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoAutomatic, 0)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected} })
	_, _ = e.approve(id)
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s", e.policy(t))
	}
}

func TestPreflightRefusalDoesNotLower(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineRed })
	e.duties(t)
	if st, _ := e.status(t, id); st != "rejected" || e.policy(t) != RepoAutomatic {
		t.Fatalf("status = %s, policy = %s", st, e.policy(t))
	}
}

func TestProviderShaMismatchIsStaleNoLower(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeStaleHead} })
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseStaleHead {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s", e.policy(t))
	}
	if e.countDispatches(t, id) != 1 {
		t.Fatal("a stale-head settlement must keep its dispatch record, which counts toward the limit")
	}
}

func (e *mergeExec) countDispatches(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := e.store.db.QueryRow(e.store.db.Rebind(`SELECT COUNT(*) FROM coordinator_merge_dispatches WHERE proposal_id = ? AND settled_at IS NOT NULL`), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNotMergedLowers(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.mergeErr = errTimeout })
	e.duties(t)
	e.ageDispatch(t, id, time.Hour)
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseNotMerged {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.policy(t) != RepoRequiresApproval {
		t.Fatalf("policy = %s", e.policy(t))
	}
}

func TestMergedElsewhereClosedUnconfirmedDoNotLower(t *testing.T) {
	cases := map[string]func(*fakeMerger){
		"merged elsewhere": func(f *fakeMerger) { f.snap.State, f.snap.MergedHeadSHA = mergestate.StateMerged, "other" },
		"closed":           func(f *fakeMerger) { f.snap.State = mergestate.StateClosed },
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			id := e.autoProposal(t, "h1")
			e.merger.set(func(f *fakeMerger) { f.mergeErr = errTimeout })
			e.duties(t)
			e.ageDispatch(t, id, time.Hour)
			e.merger.set(set)
			e.duties(t)
			if st, _ := e.status(t, id); st != "rejected" || e.policy(t) != RepoAutomatic {
				t.Fatalf("status = %s, policy = %s", st, e.policy(t))
			}
		})
	}
	t.Run("unconfirmed release", func(t *testing.T) {
		e := newMergeExec(t)
		id := e.autoProposal(t, "h1")
		e.merger.set(func(f *fakeMerger) { f.mergeErr = errTimeout })
		e.duties(t)
		e.ageDispatch(t, id, 25*time.Hour)
		if _, err := e.svc.ReleaseMerge(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id); err != nil {
			t.Fatal(err)
		}
		if e.policy(t) != RepoAutomatic {
			t.Fatalf("policy = %s", e.policy(t))
		}
	})
}

func TestLoweringBoundToAutomaticRevision(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected} })
	// A manager re-raises the repository at a later revision before the
	// lowering applies: the new raise is not undone by the old dispatch.
	e.merger.onMerge = func() { e.setPolicy(t, RepoAutomatic, 9) }
	e.duties(t)
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s, a later raise was lowered", e.policy(t))
	}
	_ = id
}

func TestLoweringOncePerDispatch(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected} })
	e.duties(t)
	e.setPolicy(t, RepoAutomatic, 4)
	e.duties(t)
	e.duties(t)
	if e.policy(t) != RepoAutomatic {
		t.Fatalf("policy = %s: the lowering ran twice for one dispatch", e.policy(t))
	}
	_ = id
}

func TestFlagOffClaimRecovery(t *testing.T) {
	for _, withDispatch := range []bool{false, true} {
		name := "no dispatch"
		if withDispatch {
			name = "dispatch"
		}
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			id := *e.propose(t, "h1").ProposalID
			mustExec(t, e.store, `UPDATE coordinator_proposals SET status = 'approving', claim_token = 'tok', claimed_at = ? WHERE id = '`+id+`'`, e.store.now().UTC().Add(-time.Hour))
			if withDispatch {
				mustExec(t, e.store, e.store.db.Rebind(`INSERT INTO coordinator_merge_dispatches (proposal_id, workspace_id, change_key, head_sha, claim_token, dispatched_at) VALUES (?, 'ws-1', ?, 'h1', 'tok', ?)`), id, testChangeKey, e.store.now().UTC().Add(-time.Hour))
			}
			e.svc.merge = false
			if _, err := e.approve(id); err != nil && !isConflict(err) {
				t.Fatal(err)
			}
			want := "pending"
			if withDispatch {
				want = "uncertain"
			}
			if st, _ := e.status(t, id); st != want {
				t.Fatalf("status = %s, want %s", st, want)
			}
			if e.merger.mergeCalls() != 0 {
				t.Fatal("recovery called the provider")
			}
		})
	}
}

func TestAutomaticPendingResumesWhenFlagReturns(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.svc.merge = false
	e.duties(t)
	if st, _ := e.status(t, id); st != "pending" || e.merger.mergeCalls() != 0 {
		t.Fatalf("status = %s", st)
	}
	e.svc.merge = true
	e.duties(t)
	if st, _ := e.status(t, id); st != "approved" {
		t.Fatalf("status = %s", st)
	}
}

func TestStaleClaimNeverFailed(t *testing.T) {
	// A claim that stayed approving past the window settles uncertain with a
	// dispatch, pending without, and never failed.
	for _, withDispatch := range []bool{true, false} {
		e := newMergeExec(t)
		id := e.freshApprovingClaim(t, "h7", withDispatch)
		if _, err := e.approve(id); err != nil && !isConflict(err) {
			t.Fatal(err)
		}
		want := "pending"
		if withDispatch {
			want = "uncertain"
		}
		if st, _ := e.status(t, id); st != want {
			t.Fatalf("with dispatch %v: status = %s, want %s", withDispatch, st, want)
		}
	}
}

func TestOwningCoordinatorGoverns(t *testing.T) {
	e := newMergeExec(t)
	first := e.propose(t, "h1")
	mustExec(t, e.store, `UPDATE coordinator_merge_proposals SET coordinator_id = 'other-coordinator'`)
	mustExec(t, e.store, `UPDATE coordinator_proposals SET coordinator_id = 'other-coordinator' WHERE kind = 'merge'`)
	second := e.propose(t, "h1")
	if second.ProposalID == nil || *second.ProposalID != *first.ProposalID || !second.Deduplicated || second.OwningCoordinatorID != "other-coordinator" {
		t.Fatalf("second = %+v", second)
	}
}

func TestNoMutationOnRefusal(t *testing.T) {
	e := newMergeExec(t)
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineRed })
	id := *e.propose(t, "h1").ProposalID
	_, _ = e.approve(id)
	var dispatches int
	_ = e.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_merge_dispatches`).Scan(&dispatches)
	if dispatches != 0 || e.merger.mergeCalls() != 0 {
		t.Fatalf("dispatches = %d, calls = %d", dispatches, e.merger.mergeCalls())
	}
}

var errTimeout = errors.New("timeout")

// freshApprovingClaim stores a merge proposal stuck approving past the claim
// window, with or without a dispatch record.
func (e *mergeExec) freshApprovingClaim(t *testing.T, head string, withDispatch bool) string {
	t.Helper()
	e.merger.set(func(f *fakeMerger) { f.snap = greenSnapshot(head) })
	id := *e.propose(t, head).ProposalID
	old := e.store.now().UTC().Add(-time.Hour)
	mustExec(t, e.store, e.store.db.Rebind(`UPDATE coordinator_proposals SET status = 'approving', claim_token = 'tok', claimed_at = ? WHERE id = ?`), old, id)
	if withDispatch {
		mustExec(t, e.store, e.store.db.Rebind(`INSERT INTO coordinator_merge_dispatches (proposal_id, workspace_id, change_key, head_sha, claim_token, dispatched_at) VALUES (?, 'ws-1', ?, ?, 'tok', ?)`), id, testChangeKey, head, old)
	}
	return id
}

func TestStricterConcurrentWins(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoProposal(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected} })
	e.merger.onMerge = func() { e.setPolicy(t, RepoDenied, 0) }
	e.duties(t)
	if e.policy(t) != RepoDenied {
		t.Fatalf("policy = %s: a lowering overwrote a stricter concurrent save", e.policy(t))
	}
	d, err := e.store.getDispatch(e.ctx, e.store.ro, id)
	if err != nil || d == nil || d.Lowering != loweringSkipped {
		t.Fatalf("dispatch = %+v, %v", d, err)
	}
}

func TestDedupeAcrossCardsAndCoordinators(t *testing.T) {
	e := newMergeExec(t)
	e.fix.addTask(t, "tn", "wf-a")
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm", "tn"}}})
	first := e.propose(t, "h1")
	res, err := e.svc.ProposeMerge(e.ctx, e.fix.c.ID, []byte(`{"task_id":"tn","expected_head_sha":"h1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ProposalID == nil || *res.ProposalID != *first.ProposalID || !res.Deduplicated {
		t.Fatalf("res = %+v", res)
	}
}

func TestManagerRejectsAPendingMergeAndFreesTheChange(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	reason := "not now"
	if _, err := e.svc.RejectProposal(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id, RejectProposalRequest{Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.status(t, id); st != "rejected" {
		t.Fatalf("status = %s", st)
	}
	if res := e.propose(t, "h1"); res.ProposalID == nil || res.Deduplicated || *res.ProposalID == id {
		t.Fatalf("re-propose = %+v", res)
	}
}
