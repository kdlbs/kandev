package coordinator

import (
	"context"
	"testing"
)

// TestDeleteWorkspaceState_Postgres_DeletesCoordinatorsProposalsAndStalls is
// the PostgreSQL twin of
// TestDeleteWorkspaceState_DeletesCoordinatorsProposalsAndStallsScopedToWorkspace.
func TestDeleteWorkspaceState_Postgres_DeletesCoordinatorsProposalsAndStalls(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	seedWorkspaceCoordinatorState(t, store, "ws-1")
	seedWorkspaceCoordinatorState(t, store, "ws-2")

	if err := store.DeleteWorkspaceState(ctx, "ws-1"); err != nil {
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

	if err := store.DeleteWorkspaceState(ctx, "ws-1"); err != nil {
		t.Fatalf("DeleteWorkspaceState (repeated): %v", err)
	}
}
