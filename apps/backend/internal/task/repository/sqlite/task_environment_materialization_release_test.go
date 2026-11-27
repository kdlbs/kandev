package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
func TestFailTaskEnvironmentMaterializationChangesOnlyTheCurrentOwnersClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-owner-release")
	owner := seedCreatingOwner(t, repo, "task-owner-release", "session-release-owner", models.TaskSessionStateCreated)

	changed, err := repo.FailTaskEnvironmentMaterialization(ctx, "session-not-owner")
	if err != nil || changed {
		t.Fatalf("non-owner release = %v, %v; want no change", changed, err)
	}
	assertCreatingOwner(t, repo, owner.TaskEnvironmentID, owner.ID)

	changed, err = repo.FailTaskEnvironmentMaterialization(ctx, owner.ID)
	if err != nil || !changed {
		t.Fatalf("owner release = %v, %v; want a change", changed, err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Status != models.TaskEnvironmentStatusFailed || environment.MaterializationSessionID != "" {
		t.Fatalf("released environment = status %q owner %q, want failed without owner", environment.Status, environment.MaterializationSessionID)
	}

	changed, err = repo.FailTaskEnvironmentMaterialization(ctx, owner.ID)
	if err != nil || changed {
		t.Fatalf("repeated release = %v, %v; want no change", changed, err)
	}
}
