package changes

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTurnChangePreservationKeepsPendingCheckpointRefs(t *testing.T) {
	store := newCoordinatorStore()
	client := &coordinatorCheckpointClient{}
	rows := []models.TurnRepositoryChangeSet{{
		ID: "repository-change-pending", TurnChangeSetID: "change-set-pending", CheckoutID: "checkout-pending",
		StartCommitOID: "start-commit", EndCommitOID: "end-commit", CleanupPending: true,
		StartReachabilityRef: "refs/kandev/turn-changes/start", EndReachabilityRef: "refs/kandev/turn-changes/end",
		PreservationState: models.TurnChangePreservationPending,
	}}
	coordinator := &Coordinator{repository: store}
	if err := coordinator.cleanupRepositoryCheckpointRefs(context.Background(), rows, client); err != nil {
		t.Fatalf("cleanup pending preservation refs: %v", err)
	}
	if len(client.deleteRequests) != 0 {
		t.Fatalf("pending preservation deleted %d executor refs; retry needs them", len(client.deleteRequests))
	}
	rows[0].PreservationState = models.TurnChangePreservationReady
	if err := coordinator.cleanupRepositoryCheckpointRefs(context.Background(), rows, client); err != nil {
		t.Fatalf("cleanup preserved refs: %v", err)
	}
	if len(client.deleteRequests) != 2 {
		t.Fatalf("preserved interval deleted %d refs, want both endpoints", len(client.deleteRequests))
	}
}
