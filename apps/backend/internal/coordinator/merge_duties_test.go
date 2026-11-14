package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func (e *mergeExec) duties(t *testing.T) {
	t.Helper()
	if err := e.svc.RunMergeDuties(context.Background(), e.fix.c.ID); err != nil {
		t.Fatal(err)
	}
}

func (e *mergeExec) autonomy(t *testing.T, on bool) {
	t.Helper()
	v := 0
	if on {
		v = 1
	}
	mustExec(t, e.store, `UPDATE coordinators SET autonomy_enabled = `+map[int]string{0: "0", 1: "1"}[v])
}

func TestDutiesSweepClosedMergedAndMovedHead(t *testing.T) {
	cases := map[string]struct {
		set  func(*fakeMerger)
		code string
	}{
		"closed":     {func(f *fakeMerger) { f.snap.State = mergestate.StateClosed }, refuseClosed},
		"merged":     {func(f *fakeMerger) { f.snap.State = mergestate.StateMerged }, refuseMergedElsewhere},
		"moved head": {func(f *fakeMerger) { f.snap = greenSnapshot("h2") }, refuseStaleHead},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			id := *e.propose(t, "h1").ProposalID
			e.merger.set(tc.set)
			e.duties(t)
			if st, code := e.status(t, id); st != "rejected" || code != tc.code {
				t.Fatalf("status = %s/%s", st, code)
			}
			if e.merger.mergeCalls() != 0 {
				t.Fatal("merged")
			}
		})
	}
}

func TestDutiesExpirePendingProposal(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	base := e.store.now
	e.store.now = func() time.Time { return base().Add(mergeExpiry + time.Hour) }
	e.duties(t)
	if st, code := e.status(t, id); st != "rejected" || code != refuseExpired {
		t.Fatalf("status = %s/%s", st, code)
	}
}

func TestDutiesDoNotMergeRequiresApprovalProposal(t *testing.T) {
	e := newMergeExec(t)
	e.autonomy(t, true)
	id := *e.propose(t, "h1").ProposalID
	e.duties(t)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("a requires_approval proposal was merged by the backstop")
	}
	if st, _ := e.status(t, id); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

func TestDutiesDispatchAutomaticProposalWhenAutonomous(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoAutomatic, 0)
	e.admit(t)
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelinePending })
	id := *e.propose(t, "h1").ProposalID
	e.duties(t)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("merged while the pipeline was pending")
	}
	e.merger.set(func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineGreen })
	e.duties(t)
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("merge calls = %d", e.merger.mergeCalls())
	}
	if st, _ := e.status(t, id); st != "approved" {
		t.Fatalf("status = %s", st)
	}
	e.duties(t)
	if e.merger.mergeCalls() != 1 {
		t.Fatal("a settled proposal was merged again")
	}
}

func TestDutiesAutomaticNeedsAutonomy(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoAutomatic, 0)
	e.autonomy(t, false)
	e.propose(t, "h1")
	e.duties(t)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("merged without autonomy")
	}
}

func TestDutiesFlagOffDoNothing(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.svc.merge = false
	e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
	e.duties(t)
	if st, _ := e.status(t, id); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

func TestVisitSetIncludesCoordinatorsWithOpenMerges(t *testing.T) {
	e := newMergeExec(t)
	e.autonomy(t, false)
	e.propose(t, "h1")
	ids := e.svc.backstop.visitSet(context.Background(), time.Now().UTC())
	found := false
	for _, id := range ids {
		found = found || id == e.fix.c.ID
	}
	if !found {
		t.Fatalf("visit set = %v", ids)
	}
}
