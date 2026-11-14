package coordinator

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// uncertain proposes and approves a merge whose request gave no answer, then
// ages its dispatch record.
func (e *mergeExec) uncertain(t *testing.T, age time.Duration) string {
	t.Helper()
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.mergeErr = errors.New("timeout") })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st != "uncertain" {
		t.Fatalf("status = %s", st)
	}
	e.ageDispatch(t, id, age)
	return id
}

func (e *mergeExec) ageDispatch(t *testing.T, id string, age time.Duration) {
	t.Helper()
	mustExec(t, e.store, e.store.db.Rebind(`UPDATE coordinator_merge_dispatches SET dispatched_at = ? WHERE proposal_id = ?`), e.store.now().UTC().Add(-age), id)
}

func (e *mergeExec) check(t *testing.T, id string) {
	t.Helper()
	if _, err := e.svc.CheckMergeNow(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileMergedOpenClosedElsewhere(t *testing.T) {
	cases := []struct {
		name   string
		snap   func(*fakeMerger)
		status string
		code   string
	}{
		{"merged with expected head", func(f *fakeMerger) {
			f.snap.State, f.snap.MergedHeadSHA, f.snap.MergeCommit = mergestate.StateMerged, "h1", "mc9"
		}, "approved", ""},
		{"merged from another head", func(f *fakeMerger) {
			f.snap.State, f.snap.MergedHeadSHA = mergestate.StateMerged, "other"
		}, "rejected", refuseMergedElsewhere},
		{"closed", func(f *fakeMerger) { f.snap.State = mergestate.StateClosed }, "rejected", refuseClosed},
		{"open", func(f *fakeMerger) {}, "rejected", refuseNotMerged},
		{"open and in progress", func(f *fakeMerger) { f.snap.InProgress = true }, "uncertain", ""},
		{"read unavailable", func(f *fakeMerger) { f.snapErr = errors.New("down") }, "uncertain", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newMergeExec(t)
			id := e.uncertain(t, time.Hour)
			e.merger.set(tc.snap)
			e.check(t, id)
			if st, code := e.status(t, id); st != tc.status || (tc.code != "" && code != tc.code) {
				t.Fatalf("status = %s/%s", st, code)
			}
			if e.merger.mergeCalls() != 1 {
				t.Fatalf("merge calls = %d", e.merger.mergeCalls())
			}
		})
	}
}

func TestQueuedDispatchStaysUncertainWhenOpen(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeQueued} })
	_, _ = e.approve(id)
	e.ageDispatch(t, id, time.Hour)
	e.check(t, id)
	if st, _ := e.status(t, id); st != "uncertain" {
		t.Fatalf("status = %s", st)
	}
}

func TestReconcileMinimumAge(t *testing.T) {
	e := newMergeExec(t)
	id := e.uncertain(t, 2*time.Minute)
	e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
	before := e.merger.reads
	e.check(t, id)
	if st, _ := e.status(t, id); st != "uncertain" || e.merger.reads != before {
		t.Fatalf("status = %s, reads %d -> %d", st, before, e.merger.reads)
	}
}

func TestBackstopReconcilesUncertainWithAutonomyOff(t *testing.T) {
	e := newMergeExec(t)
	id := e.uncertain(t, time.Hour)
	e.autonomy(t, false)
	e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseClosed {
		t.Fatalf("status = %s/%s", st, code)
	}
}

func TestManagerReleaseUncertain(t *testing.T) {
	e := newMergeExec(t)
	id := e.uncertain(t, 25*time.Hour)
	before := e.merger.mergeCalls()
	if _, err := e.svc.ReleaseMerge(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id); err != nil {
		t.Fatal(err)
	}
	if st, code := e.status(t, id); st != "rejected" || code != reasonUnconfirmed {
		t.Fatalf("status = %s/%s", st, code)
	}
	if e.merger.mergeCalls() != before {
		t.Fatal("release sent a provider request")
	}
	// The head may be proposed again once the dispatch is released.
	if res := e.propose(t, "h1"); res.ProposalID == nil || res.Deduplicated {
		t.Fatalf("re-propose = %+v", res)
	}
}

func TestReleaseWindowFromDispatch(t *testing.T) {
	e := newMergeExec(t)
	id := e.uncertain(t, 23*time.Hour)
	if _, err := e.svc.ReleaseMerge(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id); err == nil {
		t.Fatal("released before 24 hours")
	}
	if st, _ := e.status(t, id); st != "uncertain" {
		t.Fatalf("status = %s", st)
	}
}

// autoMerged dispatches an automatic merge that the provider accepted.
func (e *mergeExec) autoMerged(t *testing.T) string {
	t.Helper()
	e.setPolicy(t, RepoAutomatic, 0)
	e.admit(t)
	id := *e.propose(t, "h1").ProposalID
	e.duties(t)
	if st, _ := e.status(t, id); st != "approved" {
		t.Fatalf("status = %s", st)
	}
	return id
}

func (e *mergeExec) policy(t *testing.T) RepoPolicy {
	t.Helper()
	rows, err := e.store.loadRepoPolicies(e.ctx, e.store.ro, e.fix.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return rows["r1"].Policy
}

func (e *mergeExec) ageWatch(t *testing.T, id string) {
	t.Helper()
	mustExec(t, e.store, `UPDATE coordinator_merge_dispatches SET revert_checked_at = NULL WHERE proposal_id = '`+id+`'`)
}

func TestRevertLowers(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoMerged(t)
	e.merger.set(func(f *fakeMerger) { f.pages = [][]string{{"unrelated"}, {"Revert\n\nThis reverts commit mc1."}} })
	e.ageWatch(t, id)
	e.duties(t)
	if got := e.policy(t); got != RepoRequiresApproval {
		t.Fatalf("policy = %s", got)
	}
	e.duties(t)
	if got := e.policy(t); got != RepoRequiresApproval {
		t.Fatalf("policy after a second pass = %s", got)
	}
}

func TestRevertUnknownLowersNothing(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoMerged(t)
	e.merger.set(func(f *fakeMerger) { f.pagesErr = errors.New("down") })
	e.ageWatch(t, id)
	e.duties(t)
	if got := e.policy(t); got != RepoAutomatic {
		t.Fatalf("policy = %s", got)
	}
}

func TestRevertScanBounds(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoMerged(t)
	pages := make([][]string, 5)
	pages[4] = []string{"This reverts commit mc1"}
	e.merger.set(func(f *fakeMerger) { f.pages = pages })
	e.ageWatch(t, id)
	e.duties(t)
	if e.merger.pageCalls != revertPages {
		t.Fatalf("pages read = %d, want %d", e.merger.pageCalls, revertPages)
	}
	if got := e.policy(t); got != RepoAutomatic {
		t.Fatalf("a revert past the bound lowered the policy: %s", got)
	}
}

func TestRevertScanOncePerInterval(t *testing.T) {
	e := newMergeExec(t)
	e.autoMerged(t)
	e.merger.set(func(f *fakeMerger) { f.pages = [][]string{{"nothing"}} })
	e.duties(t)
	first := e.merger.pageCalls
	e.duties(t)
	if e.merger.pageCalls != first {
		t.Fatalf("pages read %d -> %d inside the interval", first, e.merger.pageCalls)
	}
}

func TestRevertWindowEndsAfterSevenDays(t *testing.T) {
	e := newMergeExec(t)
	id := e.autoMerged(t)
	mustExec(t, e.store, e.store.db.Rebind(`UPDATE coordinator_merge_dispatches SET settled_at = ? WHERE proposal_id = ?`), e.store.now().UTC().Add(-8*24*time.Hour), id)
	e.merger.set(func(f *fakeMerger) { f.pages = [][]string{{"This reverts commit mc1"}} })
	e.ageWatch(t, id)
	e.duties(t)
	if got := e.policy(t); got != RepoAutomatic {
		t.Fatalf("policy = %s", got)
	}
}
