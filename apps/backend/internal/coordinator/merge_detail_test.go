package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func (e *mergeExec) open(t *testing.T, id string) (*Proposal, *MergeDetail) {
	t.Helper()
	p, err := e.store.GetProposalAnyKind(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return e.svc.OpenMergeDetail(e.ctx, p)
}

func TestDetailOpenRunsUncachedReadAndShowsConditions(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	before := e.merger.reads
	p, d := e.open(t, id)
	if d == nil || d.Live == nil || d.Live.Pipeline != "green" || d.HeadSHA != "h1" || d.Method != "squash" || p.Status != ProposalStatusPending {
		t.Fatalf("detail = %+v", d)
	}
	if e.merger.reads <= before {
		t.Fatal("opening the detail made no provider read")
	}
}

func TestDetailOpenSettlesAClosedChange(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
	p, _ := e.open(t, id)
	if p.Status != ProposalStatusRejected {
		t.Fatalf("status = %s", p.Status)
	}
}

func TestDetailShowsDispatchActions(t *testing.T) {
	e := newMergeExec(t)
	id := e.uncertain(t, 25*time.Hour)
	_, d := e.open(t, id)
	if d == nil || d.Dispatch == nil || !d.Dispatch.CanCheck || !d.Dispatch.CanRelease {
		t.Fatalf("detail = %+v", d)
	}
}

func TestDetailAbsentForOtherKindsAndFlagOff(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	p, _ := e.store.GetProposalAnyKind(context.Background(), e.fix.c.WorkspaceID, e.fix.c.ID, id)
	e.svc.merge = false
	if _, d := e.svc.OpenMergeDetail(context.Background(), p); d != nil {
		t.Fatal("detail returned with the flag off")
	}
	e.svc.merge = true
	other := *p
	other.Kind = ProposalKindMessage
	if _, d := e.svc.OpenMergeDetail(context.Background(), &other); d != nil {
		t.Fatal("detail returned for another kind")
	}
}

func TestDetailOpenByReaderChangesNothing(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.svc.authz = &identityAuthorizer{err: errors.New("forbidden")}
	e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
	before := e.merger.reads
	p, d := e.open(t, id)
	if p.Status != ProposalStatusPending || d == nil || d.Live != nil || e.merger.reads != before {
		t.Fatalf("status = %s, detail = %+v, reads %d -> %d", p.Status, d, before, e.merger.reads)
	}
}
