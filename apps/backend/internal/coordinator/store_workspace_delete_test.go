package coordinator

import (
	"context"
	"testing"
	"time"
)

// seedWorkspaceCoordinatorState creates one coordinator in workspaceID with
// a proposal in every ProposalStatus and a stall row, exercising the
// coordinators.md#workspace-deletion cascade across all three tables.
func seedWorkspaceCoordinatorState(t *testing.T, store *Store, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	c := newTestCoordinator(t, store, workspaceID)

	pending := &Proposal{CoordinatorID: c.ID, WorkspaceID: workspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, pending); err != nil {
		t.Fatalf("InsertProposal(pending): %v", err)
	}

	approving := &Proposal{CoordinatorID: c.ID, WorkspaceID: workspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, approving); err != nil {
		t.Fatalf("InsertProposal(approving): %v", err)
	}
	if _, err := store.ClaimProposal(ctx, approving.ID, "token-approving", sampleSpec(), "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimProposal(approving): %v", err)
	}

	approved := &Proposal{CoordinatorID: c.ID, WorkspaceID: workspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, approved); err != nil {
		t.Fatalf("InsertProposal(approved): %v", err)
	}
	if _, err := store.ClaimProposal(ctx, approved.ID, "token-approved", sampleSpec(), "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimProposal(approved): %v", err)
	}
	if _, err := store.CompleteProposal(ctx, approved.ID, "token-approved", "task-approved", time.Now().UTC()); err != nil {
		t.Fatalf("CompleteProposal: %v", err)
	}

	failed := &Proposal{CoordinatorID: c.ID, WorkspaceID: workspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, failed); err != nil {
		t.Fatalf("InsertProposal(failed): %v", err)
	}
	if _, err := store.ClaimProposal(ctx, failed.ID, "token-failed", sampleSpec(), "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("ClaimProposal(failed): %v", err)
	}
	if _, err := store.FailProposal(ctx, failed.ID, "token-failed", "boom", time.Now().UTC()); err != nil {
		t.Fatalf("FailProposal: %v", err)
	}

	rejected := &Proposal{CoordinatorID: c.ID, WorkspaceID: workspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, rejected); err != nil {
		t.Fatalf("InsertProposal(rejected): %v", err)
	}
	if _, err := store.RejectProposal(ctx, rejected.ID, "no thanks", "user-1", time.Now().UTC()); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}

	if _, err := store.UpsertStall(ctx, sampleStall("task-stalled", workspaceID, time.Now().UTC())); err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}
}

func countWorkspaceCoordinatorState(t *testing.T, store *Store, workspaceID string) (coordinators, proposals, stalls int) {
	t.Helper()
	ctx := context.Background()

	cs, err := store.ListCoordinators(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListCoordinators: %v", err)
	}
	if err := store.db.GetContext(ctx, &proposals, store.db.Rebind(
		`SELECT COUNT(*) FROM coordinator_proposals WHERE workspace_id = ?`), workspaceID); err != nil {
		t.Fatalf("count proposals: %v", err)
	}
	ss, err := store.ListStalls(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	return len(cs), proposals, len(ss)
}

func TestDeleteWorkspaceState_DeletesCoordinatorsProposalsAndStallsScopedToWorkspace(t *testing.T) {
	store := newTestStore(t)
	seedWorkspaceCoordinatorState(t, store, "ws-1")
	seedWorkspaceCoordinatorState(t, store, "ws-2")

	if err := store.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceState: %v", err)
	}

	coordinators, proposals, stalls := countWorkspaceCoordinatorState(t, store, "ws-1")
	if coordinators != 0 || proposals != 0 || stalls != 0 {
		t.Fatalf("ws-1 after delete: coordinators=%d proposals=%d stalls=%d, want all 0", coordinators, proposals, stalls)
	}

	coordinators, proposals, stalls = countWorkspaceCoordinatorState(t, store, "ws-2")
	if coordinators != 1 || proposals != 5 || stalls != 1 {
		t.Fatalf("ws-2 after ws-1 delete: coordinators=%d proposals=%d stalls=%d, want 1/5/1 (untouched)", coordinators, proposals, stalls)
	}
}

func TestDeleteWorkspaceState_NoRowsIsSuccessAndIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.DeleteWorkspaceState(ctx, "ws-missing"); err != nil {
		t.Fatalf("DeleteWorkspaceState (no rows): %v", err)
	}

	seedWorkspaceCoordinatorState(t, store, "ws-1")
	if err := store.DeleteWorkspaceState(ctx, "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceState (first): %v", err)
	}
	if err := store.DeleteWorkspaceState(ctx, "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceState (repeated): %v", err)
	}

	coordinators, proposals, stalls := countWorkspaceCoordinatorState(t, store, "ws-1")
	if coordinators != 0 || proposals != 0 || stalls != 0 {
		t.Fatalf("ws-1 after repeated delete: coordinators=%d proposals=%d stalls=%d, want all 0", coordinators, proposals, stalls)
	}
}
