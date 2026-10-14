package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestReplyStore_Postgres exercises the reply store's conditional updates on
// the real dialect: one winner for concurrent returns, the claim/finalise
// timestamps round-trip, and a delivered reply cannot be claimed again.
func TestReplyStore_Postgres(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec(), Kind: ProposalKindCreateTask}
	if err := store.InsertProposal(ctx, p, true); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.ReturnProposalTx(ctx, store.db, p.ID, "narrower please", "manager-1", time.Now())
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("return winners = %d, want 1", wins.Load())
	}

	got, err := store.GetProposal(ctx, c.WorkspaceID, c.ID, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalStatusReturned || got.ReplyText == nil || *got.ReplyText != "narrower please" || got.ReplyDeliveredAt != nil {
		t.Fatalf("returned row = %+v", got)
	}

	if ok, err := store.ClaimReplyDelivery(ctx, p.ID, time.Now()); err != nil || !ok {
		t.Fatalf("claim = %v, %v", ok, err)
	}
	if ok, err := store.FinaliseReplyDelivery(ctx, p.ID, time.Now()); err != nil || !ok {
		t.Fatalf("finalise = %v, %v", ok, err)
	}
	if ok, _ := store.FinaliseReplyDelivery(ctx, p.ID, time.Now()); ok {
		t.Fatal("second finalise changed the row")
	}
	if ok, _ := store.ClaimReplyDelivery(ctx, p.ID, time.Now()); ok {
		t.Fatal("claim matched a delivered reply")
	}
	got, err = store.GetProposal(ctx, c.WorkspaceID, c.ID, p.ID, true)
	if err != nil || got.ReplyDeliveredAt == nil {
		t.Fatalf("delivered row = %+v, %v", got, err)
	}
}
