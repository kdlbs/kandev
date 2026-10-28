package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func acquireClaimForDescriptorEnvironment(t *testing.T, repo *Repository, operationID string) *models.TaskEnvironmentRecoveryClaim {
	t.Helper()
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-descriptor-1", OwnerTaskID: "task-descriptor-1", OwnershipGeneration: 7,
		SessionID: "session-descriptor-1", OperationID: operationID, ExecutorType: string(models.ExecutorTypePluginRemote),
		AllowCurrentSessionRuntime: true,
	})
	if err != nil {
		t.Fatalf("acquire claim %s: %v", operationID, err)
	}
	return claim
}

func claimCheckpoint(t *testing.T, repo *Repository, claim *models.TaskEnvironmentRecoveryClaim, kind, phase string, sequence uint64, digest string, revision uint64) (bool, error) {
	t.Helper()
	running := descriptorFixture(1, "install-a", phase, "/srv/work", "worker-1")
	running.ExpectedPluginExecutorRevision = revision
	return repo.CheckpointPluginExecutorClaimInventory(context.Background(), running, models.PluginExecutorClaimCheckpoint{Claim: claim, Kind: kind, Sequence: sequence, Digest: digest})
}

func storedEnvelope(t *testing.T, repo *Repository) (map[string]interface{}, uint64) {
	t.Helper()
	stored, err := repo.GetExecutorRunningBySessionID(context.Background(), "session-descriptor-1")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	envelope, _ := stored.Metadata["plugin_executor"].(map[string]interface{})
	revision, _ := pluginExecutorInventoryRevision(stored.Metadata)
	return envelope, revision
}

func seedProvisionedForClaims(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "allocating", "", ""), 0); err != nil {
		t.Fatalf("allocating: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1"), 1); err != nil {
		t.Fatalf("provisioned: %v", err)
	}
}

func exerciseClaimCheckpointSequence(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seedProvisionedForClaims(t, repo, seed)
	claim := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")

	for name, tc := range map[string]struct {
		kind, phase string
		sequence    uint64
	}{
		"recovery phase under a cleanup kind": {models.PluginClaimKindCleanup, "recovery_progress", 1},
		"cleanup phase under a recovery kind": {models.PluginClaimKindRecovery, "cleanup_progress", 1},
		"launch phase":                        {models.PluginClaimKindCleanup, "ready", 1},
		"missing sequence":                    {models.PluginClaimKindCleanup, "cleanup_progress", 0},
	} {
		if _, err := claimCheckpoint(t, repo, claim, tc.kind, tc.phase, tc.sequence, "d", 2); !errors.Is(err, models.ErrExecutorInvalidConfig) {
			t.Fatalf("%s: err = %v, want invalid_config", name, err)
		}
	}

	if applied, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 3, "digest-a", 2); err != nil || !applied {
		t.Fatalf("first checkpoint: applied=%v err=%v", applied, err)
	}
	envelope, revision := storedEnvelope(t, repo)
	if revision != 3 || envelope["phase"] != "cleanup_progress" {
		t.Fatalf("stored revision=%d phase=%v, want 3 cleanup_progress", revision, envelope["phase"])
	}
	state, _ := envelope["claim"].(map[string]interface{})
	if state["kind"] != "cleanup" || state["operation_id"] != "op-1:cleanup" || state["sequence"] != float64(3) || state["digest"] != "digest-a" {
		t.Fatalf("claim state = %v", state)
	}

	if applied, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 3, "digest-a", 2); err != nil || applied {
		t.Fatalf("equal sequence and digest after the revision moved: applied=%v err=%v, want an idempotent success", applied, err)
	}
	if _, after := storedEnvelope(t, repo); after != 3 {
		t.Fatalf("an idempotent replay wrote: revision %d", after)
	}
	if _, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 3, "digest-b", 3); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("equal sequence, different digest: err = %v, want conflict", err)
	}
	if _, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 2, "digest-c", 3); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("stale sequence: err = %v, want conflict", err)
	}
	if _, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 4, "digest-d", 2); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("stale revision: err = %v, want conflict", err)
	}
	if applied, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", 4, "digest-d", 3); err != nil || !applied {
		t.Fatalf("next sequence: applied=%v err=%v", applied, err)
	}
}

func exerciseClaimCheckpointFencing(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seedProvisionedForClaims(t, repo, seed)
	first := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 1, "d1", 2); err != nil {
		t.Fatalf("under the active claim: %v", err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), first); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 2, "d2", 3); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("released claim: err = %v, want not_found", err)
	}
	successor := acquireClaimForDescriptorEnvironment(t, repo, "op-1:recovery")
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 2, "d2", 3); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("delayed writer after a successor claim: err = %v, want not_found", err)
	}
	if _, revision := storedEnvelope(t, repo); revision != 3 {
		t.Fatalf("a fenced callback changed the inventory: revision %d", revision)
	}
	if _, err := claimCheckpoint(t, repo, successor, models.PluginClaimKindRecovery, "recovery_progress", 1, "r1", 3); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("cleanup_progress must not regress to recovery_progress: err = %v", err)
	}
}

func exerciseClaimCheckpointRace(t *testing.T, repoA, repoB *Repository, seed descriptorSeeder) {
	t.Helper()
	seedProvisionedForClaims(t, repoA, seed)
	claim := acquireClaimForDescriptorEnvironment(t, repoA, "op-1:cleanup")
	results := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, repo := range []*Repository{repoA, repoB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, results[i] = claimCheckpoint(t, repo, claim, models.PluginClaimKindCleanup, "cleanup_progress", uint64(i+1), "digest", 2)
		}()
	}
	close(start)
	wg.Wait()
	wins, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, models.ErrExecutorConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("results = %v; two callbacks at one revision must give one winner and one conflict", results)
	}
}

func TestPluginExecutorClaimCheckpointSequence(t *testing.T) {
	exerciseClaimCheckpointSequence(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginExecutorClaimCheckpointFencing(t *testing.T) {
	exerciseClaimCheckpointFencing(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginExecutorClaimCheckpointRace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exerciseClaimCheckpointRace(t, repo, repo, seedDescriptorEnvironmentSQLite)
}

// A claim re-acquired with the same operation id is a new acquisition: the
// provider's destroy call starts its sequence again, and the previous
// acquisition's accepted sequence must not fence it.
func exerciseReacquiredClaimStartsAFreshSequence(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seedProvisionedForClaims(t, repo, seed)
	first := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 1, "d1", 2); err != nil {
		t.Fatalf("first acquisition: %v", err)
	}
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 2, "d2", 3); err != nil {
		t.Fatalf("first acquisition, sequence 2: %v", err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), first); err != nil {
		t.Fatalf("release: %v", err)
	}
	second := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	if second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("two acquisitions of one claim id must be distinguishable")
	}
	if applied, err := claimCheckpoint(t, repo, second, models.PluginClaimKindCleanup, "cleanup_progress", 1, "e1", 4); err != nil || !applied {
		t.Fatalf("sequence 1 of the re-acquired claim: applied=%v err=%v, want accepted", applied, err)
	}
	if _, err := claimCheckpoint(t, repo, first, models.PluginClaimKindCleanup, "cleanup_progress", 3, "d3", 5); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("delayed writer of the first acquisition: err = %v, want not_found", err)
	}
}

func TestReacquiredClaimStartsAFreshSequence(t *testing.T) {
	exerciseReacquiredClaimStartsAFreshSequence(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}
