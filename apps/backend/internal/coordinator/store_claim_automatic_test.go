package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestClaimProposal_ManagerClaimNeverTouchesAutomaticColumns(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	p := insertKind(t, store, c, ProposalKindCreateTask)
	at := automaticNow
	if ok, err := store.ClaimProposalTx(context.Background(), store.db, p.ID, "tok-a", sampleSpec(), "mgr", at, &at); err != nil || !ok {
		t.Fatalf("automatic claim ok=%v err=%v", ok, err)
	}
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'failed' WHERE id = ?`, p.ID)
	if ok, err := store.ClaimProposal(context.Background(), p.ID, "tok-m", sampleSpec(), "mgr-2", at.Add(time.Hour)); err != nil || !ok {
		t.Fatalf("manager claim ok=%v err=%v", ok, err)
	}
	got, err := store.GetProposal(context.Background(), c.WorkspaceID, c.ID, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaimedAutomatically || !got.DecidedAutomatically || got.AutomaticAt == nil || !got.AutomaticAt.Equal(at) {
		t.Fatalf("proposal = %+v", got)
	}
}

func TestClaimProposal_ManagerClaimOfPendingWritesNoAutomaticStamp(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	p := insertKind(t, store, c, ProposalKindCreateTask)
	if ok, err := store.ClaimProposal(context.Background(), p.ID, "tok", sampleSpec(), "mgr", automaticNow); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	got, _ := store.GetProposal(context.Background(), c.WorkspaceID, c.ID, p.ID, true)
	if got.ClaimedAutomatically || got.DecidedAutomatically || got.AutomaticAt != nil {
		t.Fatalf("proposal = %+v", got)
	}
}
