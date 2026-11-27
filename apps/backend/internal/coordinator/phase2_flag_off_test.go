package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func statusOf(t *testing.T, store *Store, id string) string {
	t.Helper()
	var status string
	if err := store.db.Get(&status, store.db.Rebind(`SELECT status FROM coordinator_proposals WHERE id = ?`), id); err != nil {
		t.Fatalf("status read: %v", err)
	}
	return status
}

// A flag-off service never surfaces or decides a non-create proposal.
func TestFlagOff_NonCreateProposalIs404WithoutSideEffects(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	tasks.createResult = createdResult("task-new")
	p := insertKind(t, store, c, ProposalKindMessage)
	ctx := context.Background()

	if _, err := svc.GetProposal(ctx, "ws-1", c.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ApproveProposal(ctx, "ws-1", c.ID, p.ID, ApproveProposalRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approve err = %v, want ErrNotFound", err)
	}
	if _, err := svc.RejectProposal(ctx, "ws-1", c.ID, p.ID, RejectProposalRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reject err = %v, want ErrNotFound", err)
	}
	if got := statusOf(t, store, p.ID); got != string(ProposalStatusPending) {
		t.Fatalf("status = %q, want pending", got)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("create calls = %d, want 0", len(tasks.createCalls))
	}
}

// Recovery ignores an approving non-create row when the flag is off.
func TestFlagOff_ReclaimAndSweepLeaveNonCreateApprovingRowAlone(t *testing.T) {
	store, c, _, _ := approveFixture(t)
	p := insertKind(t, store, c, ProposalKindMessage)
	ctx := context.Background()
	now := time.Now()
	if _, err := store.db.Exec(store.db.Rebind(
		`UPDATE coordinator_proposals SET status = 'approving', claim_token = 'tok', claimed_at = ? WHERE id = ?`),
		now.Add(-time.Hour).UTC(), p.ID); err != nil {
		t.Fatal(err)
	}

	list, err := store.ListApprovingClaimedBefore(ctx, now, false)
	if err != nil || len(list) != 0 {
		t.Fatalf("flag-off list = %v err=%v, want empty", list, err)
	}
	ok, err := store.ReclaimStale(ctx, p.ID, "new", now, now.Add(-time.Minute), false)
	if err != nil || ok {
		t.Fatalf("flag-off reclaim ok=%v err=%v, want false", ok, err)
	}
	if got := statusOf(t, store, p.ID); got != string(ProposalStatusApproving) {
		t.Fatalf("status = %q, want approving", got)
	}
	if list, err := store.ListApprovingClaimedBefore(ctx, now, true); err != nil || len(list) != 1 {
		t.Fatalf("flag-on list = %v err=%v, want 1", list, err)
	}
}
