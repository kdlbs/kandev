package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestListApprovingClaimedBefore_FiltersByCutoffAndOrders(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	cutoff := time.Now().UTC()

	// Claimed well before cutoff: must be returned.
	early := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, early, false); err != nil {
		t.Fatalf("InsertProposal(early): %v", err)
	}
	earlyClaimedAt := cutoff.Add(-10 * time.Minute)
	if _, err := store.ClaimProposal(ctx, early.ID, "token-early", sampleSpec(), "user-1", earlyClaimedAt); err != nil {
		t.Fatalf("ClaimProposal(early): %v", err)
	}

	// Claimed just before cutoff: must be returned, and ordered after early.
	late := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, late, false); err != nil {
		t.Fatalf("InsertProposal(late): %v", err)
	}
	lateClaimedAt := cutoff.Add(-time.Minute)
	if _, err := store.ClaimProposal(ctx, late.ID, "token-late", sampleSpec(), "user-1", lateClaimedAt); err != nil {
		t.Fatalf("ClaimProposal(late): %v", err)
	}

	// Claimed at or after cutoff: must not be returned.
	atCutoff := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, atCutoff, false); err != nil {
		t.Fatalf("InsertProposal(atCutoff): %v", err)
	}
	if _, err := store.ClaimProposal(ctx, atCutoff.ID, "token-at", sampleSpec(), "user-1", cutoff); err != nil {
		t.Fatalf("ClaimProposal(atCutoff): %v", err)
	}

	// A pending row (never claimed): must not be returned.
	pending := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, pending, false); err != nil {
		t.Fatalf("InsertProposal(pending): %v", err)
	}

	// A row in a different workspace, also claimed before cutoff: must be
	// returned too (every workspace, no scoping).
	c2 := newTestCoordinator(t, store, "ws-2")
	otherWorkspace := &Proposal{CoordinatorID: c2.ID, WorkspaceID: "ws-2", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, otherWorkspace, false); err != nil {
		t.Fatalf("InsertProposal(otherWorkspace): %v", err)
	}
	if _, err := store.ClaimProposal(ctx, otherWorkspace.ID, "token-other", sampleSpec(), "user-1", earlyClaimedAt); err != nil {
		t.Fatalf("ClaimProposal(otherWorkspace): %v", err)
	}

	got, err := store.ListApprovingClaimedBefore(ctx, cutoff, false)
	if err != nil {
		t.Fatalf("ListApprovingClaimedBefore: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListApprovingClaimedBefore returned %d rows, want 3", len(got))
	}

	// Ordered by claimed_at ascending: the two rows sharing earlyClaimedAt
	// (early and otherWorkspace) come before late, ordered by id as a
	// tiebreaker.
	if got[0].ClaimedAt == nil || !got[0].ClaimedAt.Equal(earlyClaimedAt) {
		t.Fatalf("got[0].ClaimedAt = %v, want %v", got[0].ClaimedAt, earlyClaimedAt)
	}
	if got[1].ClaimedAt == nil || !got[1].ClaimedAt.Equal(earlyClaimedAt) {
		t.Fatalf("got[1].ClaimedAt = %v, want %v", got[1].ClaimedAt, earlyClaimedAt)
	}
	if got[0].ID >= got[1].ID {
		t.Fatalf("got[0].ID = %q, got[1].ID = %q, want ascending id tiebreak", got[0].ID, got[1].ID)
	}
	if got[2].ID != late.ID {
		t.Fatalf("got[2].ID = %q, want %q (latest claimed_at last)", got[2].ID, late.ID)
	}
}

func TestListApprovingClaimedBefore_EmptyWhenNoneStuck(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.ListApprovingClaimedBefore(ctx, time.Now().UTC(), false)
	if err != nil {
		t.Fatalf("ListApprovingClaimedBefore: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListApprovingClaimedBefore returned %d rows, want 0", len(got))
	}
}
