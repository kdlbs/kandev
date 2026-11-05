package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A recovery abandons the environment its record names. A session that has
// since been re-pointed to another creating environment, or an environment
// whose generation moved on, must be left untouched.
func TestScopedFailLeavesAnEnvironmentTheRecordDoesNotNameUntouched(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-scoped-fail")
	owner := seedCreatingOwner(t, repo, "task-scoped-fail", "session-scoped-fail", models.TaskSessionStateCreated)
	env, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	status := func() models.TaskEnvironmentStatus {
		current, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
		if err != nil {
			t.Fatal(err)
		}
		return current.Status
	}
	for name, scope := range map[string]struct {
		environmentID string
		generation    int64
	}{
		"another environment": {"environment-of-the-old-record", env.OwnershipGeneration},
		"another generation":  {owner.TaskEnvironmentID, env.OwnershipGeneration + 1},
		"no scope":            {"", 0},
	} {
		changed, err := repo.FailScopedTaskEnvironmentMaterialization(ctx, owner.ID, scope.environmentID, scope.generation)
		if err != nil || changed {
			t.Fatalf("%s: changed=%v err=%v, want untouched", name, changed, err)
		}
		if got := status(); got != models.TaskEnvironmentStatusCreating {
			t.Fatalf("%s: status = %q, want creating", name, got)
		}
	}
	changed, err := repo.FailScopedTaskEnvironmentMaterialization(ctx, owner.ID, owner.TaskEnvironmentID, env.OwnershipGeneration)
	if err != nil || !changed {
		t.Fatalf("matching scope: changed=%v err=%v, want failed", changed, err)
	}
	if got := status(); got != models.TaskEnvironmentStatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
}
