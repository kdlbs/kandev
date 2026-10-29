package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// A launch that failed with its resource unrecorded leaves cleanup_pending; a
// later pass that recovers the resource records it without leaving that phase,
// because cleanup_pending may not move back to provisioned.
func TestCleanupPendingNeverReturnsToProvisioned(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentSQLite)
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "cleanup_pending", "", ""), 2); err != nil {
		t.Fatalf("cleanup_pending: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1"), 3); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("cleanup_pending -> provisioned: err = %v, want a rejected transition", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "cleanup_pending", "/srv/work", "worker-1"), 3); err != nil {
		t.Fatalf("recording the recovered resource under cleanup_pending: %v", err)
	}
	if !PluginExecutorInventoryPhaseMayFollow("cleanup_pending", "cleanup_progress") || PluginExecutorInventoryPhaseMayFollow("cleanup_pending", "ready") {
		t.Fatal("the transition table exposed to test doubles disagrees with the repository")
	}
}

// A host write built from an older envelope must not drop the claim state a
// provider checkpoint recorded: the per-claim sequence and acquisition stamp
// outlive it.
func TestHostEnvelopeWriteKeepsTheRecordedClaimState(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentSQLite)
	claim := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup:epoch:n1")
	if applied, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 1, "d1", 2); err != nil || !applied {
		t.Fatalf("claim checkpoint: applied=%v err=%v", applied, err)
	}
	running := descriptorFixture(1, "install-a", "cleanup_pending", "/srv/work", "worker-1")
	running.ExpectedPluginExecutorRevision = 3
	if err := repo.CheckpointPluginExecutorInventory(recoveryclaim.WithClaim(context.Background(), claim), running); err != nil {
		t.Fatalf("host write: %v", err)
	}
	envelope, _ := storedEnvelope(t, repo)
	state, _ := envelope["claim"].(map[string]interface{})
	if state["operation_id"] != "op-1:cleanup:epoch:n1" || state["sequence"] != float64(1) {
		t.Fatalf("claim state after a host write = %v, want the recorded checkpoint", state)
	}
}
