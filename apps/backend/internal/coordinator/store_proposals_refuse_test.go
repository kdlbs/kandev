package coordinator

import (
	"context"
	"testing"
	"time"
)

func insertAnswerRow(t *testing.T, store *Store, c *Coordinator) *Proposal {
	t.Helper()
	task := "t1"
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindAnswer, TargetTaskID: &task, RawSpec: `{"task_id":"t1"}`}
	if err := store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetProposalAnyKind(context.Background(), c.WorkspaceID, c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRefuseOpenFencedByObservedUpdatedAt(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	p := insertAnswerRow(t, store, c)

	ok, err := store.RefuseOpenTx(ctx, store.db, p.ID, p.UpdatedAt.Add(time.Second), "stale", time.Now())
	if err != nil || ok {
		t.Fatalf("a stale observation must not match: %v %v", ok, err)
	}
	ok, err = store.RefuseOpenTx(ctx, store.db, p.ID, p.UpdatedAt, "stale", time.Now())
	if err != nil || !ok {
		t.Fatalf("observed row must match: %v %v", ok, err)
	}
	got, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, p.ID)
	if got.Status != ProposalStatusRejected || got.Error == nil || *got.Error != "stale" || got.DecidedBy != nil {
		t.Fatalf("refused row = %+v", got)
	}
	if again, _ := store.RefuseOpenTx(ctx, store.db, p.ID, p.UpdatedAt, "stale", time.Now()); again {
		t.Fatal("a refused row must not refuse twice")
	}
}

func TestRefuseOpenSkipsOutcomeUnknown(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	p := insertAnswerRow(t, store, c)
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinator_proposals SET status='failed', error='outcome_unknown' WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	cur, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, p.ID)
	if ok, _ := store.RefuseOpenTx(ctx, store.db, p.ID, cur.UpdatedAt, "stale", time.Now()); ok {
		t.Fatal("outcome_unknown must never be refused")
	}
}

func TestRefuseClaimedFencedByToken(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	p := insertAnswerRow(t, store, c)
	if ok, err := store.ClaimProposalRaw(ctx, p.ID, "tok", `{}`, "u1", time.Now()); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if ok, _ := store.RefuseClaimedTx(ctx, store.db, p.ID, "other", "task_archived", time.Now()); ok {
		t.Fatal("wrong token matched")
	}
	if ok, err := store.RefuseClaimedTx(ctx, store.db, p.ID, "tok", "task_archived", time.Now()); err != nil || !ok {
		t.Fatalf("refuse: %v %v", ok, err)
	}
	got, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, p.ID)
	if got.Status != ProposalStatusRejected || got.DecidedBy != nil || *got.Error != "task_archived" {
		t.Fatalf("row = %+v", got)
	}
}

func TestClaimFencedLosesToChangeAndOutcomeUnknown(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	ctx := context.Background()
	p := insertAnswerRow(t, store, c)
	if ok, _ := store.ClaimProposalFenced(ctx, p.ID, "t", `{}`, "u1", time.Now(), p.UpdatedAt.Add(time.Second)); ok {
		t.Fatal("a changed row must not be claimed")
	}
	if ok, err := store.ClaimProposalFenced(ctx, p.ID, "t", `{}`, "u1", time.Now(), p.UpdatedAt); err != nil || !ok {
		t.Fatalf("fenced claim: %v %v", ok, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinator_proposals SET status='failed', error='outcome_unknown' WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	cur, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, p.ID)
	if ok, _ := store.ClaimProposalFenced(ctx, p.ID, "t2", `{}`, "u1", time.Now(), cur.UpdatedAt); ok {
		t.Fatal("outcome_unknown must not be claimable")
	}
}
