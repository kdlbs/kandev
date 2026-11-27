package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestClaimProposalPostgres_ManagerAndAutomaticClaims(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	manager := insertKind(t, store, c, ProposalKindCreateTask)
	if ok, err := store.ClaimProposal(ctx, manager.ID, "tok-m", sampleSpec(), "mgr", automaticNow); err != nil || !ok {
		t.Fatalf("manager claim ok=%v err=%v", ok, err)
	}
	auto := insertKind(t, store, c, ProposalKindCreateTask)
	at := automaticNow.Add(time.Minute)
	if ok, err := store.ClaimProposalTx(ctx, store.db, auto.ID, "tok-a", sampleSpec(), "mgr", at, &at); err != nil || !ok {
		t.Fatalf("automatic claim ok=%v err=%v", ok, err)
	}
	got, err := store.GetProposal(ctx, c.WorkspaceID, c.ID, auto.ID, true)
	if err != nil || !got.ClaimedAutomatically || !got.DecidedAutomatically {
		t.Fatalf("automatic proposal = %+v err = %v", got, err)
	}
	got, err = store.GetProposal(ctx, c.WorkspaceID, c.ID, manager.ID, true)
	if err != nil || got.ClaimedAutomatically || got.DecidedAutomatically {
		t.Fatalf("manager proposal = %+v err = %v", got, err)
	}
}
