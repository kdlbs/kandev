package coordinator

import (
	"context"
	"errors"
	"testing"
)

func insertRaw(t *testing.T, store *Store, c *Coordinator, kind string, phase2 bool) *Proposal {
	t.Helper()
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec(), Kind: kind}
	if err := store.InsertProposal(context.Background(), p, phase2); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	return p
}

func TestInsertProposalWith_PreShortCircuits(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	existing := insertRaw(t, store, c, ProposalKindCreateTask, true)
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	inTxRan := false
	err := store.InsertProposalWith(context.Background(), p, true,
		func(context.Context, coordinatorExec) (*Proposal, error) { return existing, nil },
		func(context.Context, coordinatorExec, *Proposal) error { inTxRan = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != existing.ID || inTxRan {
		t.Fatalf("p.ID=%q existing=%q inTx=%v", p.ID, existing.ID, inTxRan)
	}
	list, _ := store.ListProposals(context.Background(), "ws-1", c.ID, ListProposalsAll, true)
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
}

func TestInsertProposalWith_InTxErrorRollsBackInsert(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	boom := errors.New("boom")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	err := store.InsertProposalWith(context.Background(), p, true, nil,
		func(context.Context, coordinatorExec, *Proposal) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if list, _ := store.ListProposals(context.Background(), "ws-1", c.ID, ListProposalsAll, true); len(list) != 0 {
		t.Fatalf("len = %d, want 0", len(list))
	}
}

func TestInsertProposal_StoresPhase2Fields(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	target := "task-9"
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec(), Kind: ProposalKindMove,
		TargetTaskID: &target, StandingOrderIDs: []string{"o1", "o2"}, StartsAgent: true}
	if err := store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != ProposalKindMove || got.TargetTaskID == nil || *got.TargetTaskID != target ||
		len(got.StandingOrderIDs) != 2 || !got.StartsAgent {
		t.Fatalf("got %+v", got)
	}
}

func TestPhase2OffHidesOtherKinds(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	insertRaw(t, store, c, ProposalKindCreateTask, true)
	msg := insertRaw(t, store, c, ProposalKindMessage, true)
	ctx := context.Background()
	if list, _ := store.ListProposals(ctx, "ws-1", c.ID, ListProposalsAll, false); len(list) != 1 {
		t.Fatalf("off list = %d, want 1", len(list))
	}
	if list, _ := store.ListProposals(ctx, "ws-1", c.ID, ListProposalsAll, true); len(list) != 2 {
		t.Fatalf("on list = %d, want 2", len(list))
	}
	if _, err := store.GetProposal(ctx, "ws-1", c.ID, msg.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("off get err = %v", err)
	}
	if n, _ := store.CountOpenProposals(ctx, c.ID, false); n != 1 {
		t.Fatalf("off count = %d", n)
	}
	if n, _ := store.CountOpenProposals(ctx, c.ID, true); n != 2 {
		t.Fatalf("on count = %d", n)
	}
}

func TestInsertProposal_CapCountsAllKindsWhenPhase2On(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	for i := 0; i < maxOpenProposals; i++ {
		insertRaw(t, store, c, ProposalKindMessage, true)
	}
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(context.Background(), p, true); !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("on err = %v", err)
	}
	q := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(context.Background(), q, false); err != nil {
		t.Fatalf("off err = %v (other kinds must not count)", err)
	}
}
