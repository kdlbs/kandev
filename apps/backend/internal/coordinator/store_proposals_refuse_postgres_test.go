package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnswerFencesPostgres(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	p := insertAnswerRow(t, store, c)
	if ok, err := store.RefuseOpenTx(ctx, store.db, p.ID, p.UpdatedAt.Add(time.Second), "stale", time.Now()); err != nil || ok {
		t.Fatalf("a changed observation must not match: %v %v", ok, err)
	}
	if ok, err := store.RefuseOpenTx(ctx, store.db, p.ID, p.UpdatedAt, "stale", time.Now()); err != nil || !ok {
		t.Fatalf("observed row must match: %v %v", ok, err)
	}
	got, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, p.ID)
	if got.Status != ProposalStatusRejected || got.Error == nil || *got.Error != "stale" || got.DecidedBy != nil {
		t.Fatalf("refused row = %+v", got)
	}

	// Refusing frees the one-open-per-task slot.
	q := insertAnswerRow(t, store, c)
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`UPDATE coordinator_proposals SET status='failed', error='outcome_unknown' WHERE id=?`), q.ID); err != nil {
		t.Fatal(err)
	}
	cur, _ := store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, q.ID)
	if ok, _ := store.RefuseOpenTx(ctx, store.db, q.ID, cur.UpdatedAt, "stale", time.Now()); ok {
		t.Fatal("outcome_unknown must never be refused")
	}
	if ok, _ := store.ClaimProposalFenced(ctx, q.ID, "t", `{}`, "u1", time.Now(), cur.UpdatedAt); ok {
		t.Fatal("outcome_unknown must not be claimable")
	}
}

func TestClaimProposalFencedConcurrentPostgres(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	p := insertAnswerRow(t, store, c)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.ClaimProposalFenced(ctx, p.ID, "tok"+string(rune('a'+i)), `{}`, "u1", time.Now(), p.UpdatedAt)
			if err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("exactly one claim must win, got %d", wins.Load())
	}
}
