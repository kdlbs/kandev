package sqlite

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// A holder killed between a provider side effect and its checkpoint leaves the
// claim row behind. Recovery with the same operation adopts that claim, keeps
// its sequence, and nothing else can take the environment meanwhile.
func TestClaimLeftHeldByACrashIsAdoptedByTheSameOperation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentSQLite)
	crashed := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	if applied, err := claimCheckpoint(t, repo, crashed, models.PluginClaimKindCleanup, "cleanup_progress", 1, "digest-a", 2); err != nil || !applied {
		t.Fatalf("checkpoint before the crash: applied=%v err=%v", applied, err)
	}

	if _, err := repo.AcquireTaskEnvironmentRecoveryClaim(t.Context(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-descriptor-1", OwnerTaskID: "task-descriptor-1", OwnershipGeneration: 7,
		SessionID: "session-descriptor-1", OperationID: "op-2:cleanup", ExecutorType: string(models.ExecutorTypePluginRemote),
		AllowCurrentSessionRuntime: true,
	}); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("another operation took a claim held by a crashed one: err = %v", err)
	}

	adopted := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	if !adopted.CreatedAt.Equal(crashed.CreatedAt) {
		t.Fatalf("recovery got a new acquisition (%v), want the crashed holder's claim (%v)", adopted.CreatedAt, crashed.CreatedAt)
	}
	if applied, err := claimCheckpoint(t, repo, adopted, models.PluginClaimKindCleanup, "cleanup_progress", 1, "digest-a", 2); err != nil || applied {
		t.Fatalf("replaying the crashed checkpoint: applied=%v err=%v, want an idempotent success", applied, err)
	}
	if applied, err := claimCheckpoint(t, repo, adopted, models.PluginClaimKindCleanup, "cleanup_progress", 2, "digest-b", 3); err != nil || !applied {
		t.Fatalf("next checkpoint after adoption: applied=%v err=%v", applied, err)
	}
	if envelope, revision := storedEnvelope(t, repo); revision != 4 || envelope["phase"] != "cleanup_progress" {
		t.Fatalf("stored revision=%d phase=%v, want 4 cleanup_progress", revision, envelope["phase"])
	}
}
