package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.10
func TestWorkspaceBindingFailedEnvironmentWithEvidenceRequiresReset(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedMaterializationClaimTask(t, repo, "task-failed-reset")
	environmentID := seedFailedEnvironment(t, repo, "task-failed-reset", "session-failed-reset-owner")
	addWorktreeEvidence(t, repo, environmentID)

	_, err := bindRetry(repo, "task-failed-reset", "session-after-reset-evidence")
	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) || !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind error = %v, want reset required wrapping workspace reuse unsafe", err)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.4
// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
func TestReelectFailedTaskEnvironmentMaterialization(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-launch-reelect")
	environmentID := seedFailedEnvironment(t, repo, "task-launch-reelect", "session-launch-reelect")

	reelected, err := repo.ReelectFailedTaskEnvironmentMaterialization(ctx, environmentID, "session-launch-reelect")
	if err != nil || !reelected {
		t.Fatalf("re-elect failed environment = %v, %v; want re-elected", reelected, err)
	}
	assertCreatingOwner(t, repo, environmentID, "session-launch-reelect")

	reelected, err = repo.ReelectFailedTaskEnvironmentMaterialization(ctx, environmentID, "session-launch-late")
	if err != nil || reelected {
		t.Fatalf("second re-election = %v, %v; want no change", reelected, err)
	}

	seedMaterializationClaimTask(t, repo, "task-launch-evidence")
	evidenceID := seedFailedEnvironment(t, repo, "task-launch-evidence", "session-launch-evidence")
	addWorktreeEvidence(t, repo, evidenceID)
	reelected, err = repo.ReelectFailedTaskEnvironmentMaterialization(ctx, evidenceID, "session-launch-evidence")
	if err != nil || reelected {
		t.Fatalf("re-elect environment with a worktree = %v, %v; want no change", reelected, err)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9
func TestReelectFailedTaskEnvironmentMaterializationRespectsCleanupBarrier(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-launch-barrier")
	environmentID := seedFailedEnvironment(t, repo, "task-launch-barrier", "session-launch-barrier")
	reserveBarrier(t, repo, "task-launch-barrier", "operation-launch-barrier")

	reelected, err := repo.ReelectFailedTaskEnvironmentMaterialization(ctx, environmentID, "session-launch-barrier")
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) || reelected {
		t.Fatalf("re-elect during cleanup = %v, %v; want cleanup in progress", reelected, err)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9
func TestReelectFailedTaskEnvironmentMaterializationRespectsRecoveryClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-launch-recovery"
		environmentID = "environment-launch-recovery"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-launch-recovery", TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	environment.Status = models.TaskEnvironmentStatusFailed
	if err := repo.UpdateTaskEnvironment(ctx, environment); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		environmentID, taskID, "session-launch-recovery", "operation-launch-recovery", 1,
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	reelected, err := repo.ReelectFailedTaskEnvironmentMaterialization(ctx, environmentID, "session-launch-recovery")
	if !errors.Is(err, recoveryclaim.ErrBusy) || reelected {
		t.Fatalf("re-elect during recovery claim = %v, %v; want recovery claim busy", reelected, err)
	}
}
