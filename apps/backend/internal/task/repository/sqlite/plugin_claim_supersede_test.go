package sqlite

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func acquireSupersedingClaim(t *testing.T, repo *Repository, operationID, marker string) (*models.TaskEnvironmentRecoveryClaim, error) {
	t.Helper()
	return repo.AcquireTaskEnvironmentRecoveryClaim(t.Context(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-descriptor-1", OwnerTaskID: "task-descriptor-1", OwnershipGeneration: 7,
		SessionID: "session-descriptor-1", OperationID: operationID, ExecutorType: string(models.ExecutorTypePluginRemote),
		AllowCurrentSessionRuntime: true, SupersedeOperationPrefix: "op-1:cleanup:", LiveOperationMarker: marker,
	})
}

// A claim a previous process left behind is replaced by a new acquisition of
// the same operation, which is a distinct acquisition with its own sequence
// space; a claim of the live process stays busy.
func TestClaimOfAnEarlierProcessIsSupersededByAFreshAcquisition(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentSQLite)
	crashed := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup:epoch-old:n1")
	if _, err := claimCheckpoint(t, repo, crashed, models.PluginClaimKindCleanup, "cleanup_progress", 1, "digest-a", 2); err != nil {
		t.Fatalf("checkpoint before the crash: %v", err)
	}

	live, err := acquireSupersedingClaim(t, repo, "op-1:cleanup:epoch-new:n1", "epoch-new")
	if err != nil {
		t.Fatalf("a stale claim blocked the new process: %v", err)
	}
	if live.OperationID == crashed.OperationID {
		t.Fatalf("the successor reused the crashed claim id %q", live.OperationID)
	}
	if _, err := claimCheckpoint(t, repo, crashed, models.PluginClaimKindCleanup, "cleanup_progress", 2, "digest-b", 3); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("the crashed acquisition still checkpoints: err = %v", err)
	}
	if applied, err := claimCheckpoint(t, repo, live, models.PluginClaimKindCleanup, "cleanup_progress", 1, "digest-c", 3); err != nil || !applied {
		t.Fatalf("a fresh acquisition starts at sequence 1: applied=%v err=%v", applied, err)
	}
	if _, err := acquireSupersedingClaim(t, repo, "op-1:cleanup:epoch-new:n2", "epoch-new"); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("a live holder's claim was superseded: err = %v", err)
	}
}
