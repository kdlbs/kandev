package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func exerciseCleanupRequestUnderARecoveryClaim(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seedProvisionedForClaims(t, repo, seed)
	claim := acquireClaimForDescriptorEnvironment(t, repo, "op-1:recovery")
	ctx := context.Background()

	if err := repo.RecordPluginExecutorCleanupRequested(ctx, "session-descriptor-1", "execution-descriptor-1"); err != nil {
		t.Fatalf("a cleanup request must be recordable while recovery holds the claim: %v", err)
	}
	envelope, revision := storedEnvelope(t, repo)
	if envelope["cleanup_requested"] != true || revision != 3 {
		t.Fatalf("stored cleanup_requested=%v revision=%d, want true at revision 3", envelope["cleanup_requested"], revision)
	}
	if err := repo.RecordPluginExecutorCleanupRequested(ctx, "session-descriptor-1", "execution-descriptor-1"); err != nil {
		t.Fatalf("a repeated request must succeed: %v", err)
	}
	if _, again := storedEnvelope(t, repo); again != 3 {
		t.Fatalf("a repeated request wrote: revision %d", again)
	}
	if _, err := claimCheckpoint(t, repo, claim, models.PluginClaimKindRecovery, "recovery_progress", 1, "d", 2); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("the recovery holder's stale write after the request: err = %v, want conflict", err)
	}
	if err := repo.RecordPluginExecutorCleanupRequested(ctx, "session-descriptor-1", "execution-rotated"); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("another execution: err = %v, want execution_rotated", err)
	}
}

func TestCleanupRequestIsRecordableUnderARecoveryClaim(t *testing.T) {
	exerciseCleanupRequestUnderARecoveryClaim(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}
