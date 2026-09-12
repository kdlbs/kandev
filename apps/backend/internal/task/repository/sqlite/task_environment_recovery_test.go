package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func seedRecoveryClaimEnvironment(t *testing.T, repo *Repository, taskID, environmentID string) {
	t.Helper()
	seedExecutorRunningCleanupTask(t, repo, taskID)
	if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID:                  environmentID,
		TaskID:              taskID,
		ExecutorType:        string(models.ExecutorTypeWorktree),
		Status:              models.TaskEnvironmentStatusCreating,
		OwnershipGeneration: 1,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment(%s): %v", environmentID, err)
	}
}

func recoveryClaimRequest(environmentID, ownerTaskID, sessionID, operationID string, generation int64) models.TaskEnvironmentRecoveryClaimRequest {
	return models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID:   environmentID,
		OwnerTaskID:         ownerTaskID,
		OwnershipGeneration: generation,
		SessionID:           sessionID,
		OperationID:         operationID,
		ExecutorType:        string(models.ExecutorTypeWorktree),
	}
}

func TestTaskEnvironmentRecoveryClaimSerializesMutationsAndConsumers(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-recovery-claim"
		environmentID = "environment-recovery-claim"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-recovery-1",
		TaskID:            taskID,
		TaskEnvironmentID: environmentID,
		State:             models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create requesting session: %v", err)
	}

	request := recoveryClaimRequest(environmentID, taskID, "session-recovery-1", "operation-recovery-1", 1)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("acquire recovery claim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()

	replayed, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
	if err != nil {
		t.Fatalf("replay recovery claim: %v", err)
	}
	if replayed.OperationID != claim.OperationID || replayed.SessionID != claim.SessionID {
		t.Fatalf("replayed claim = %+v, want %+v", replayed, claim)
	}

	_, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, "session-recovery-2", "operation-recovery-2", 1))
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("competing claim error = %v, want ErrBusy", err)
	}

	stale := *claim
	stale.OperationID = "operation-recovery-stale"
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, &stale); !errors.Is(err, recoveryclaim.ErrClaimMismatch) {
		t.Fatalf("stale release error = %v, want ErrClaimMismatch", err)
	}

	if err := repo.UpdateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           environmentID,
		TaskID:       taskID,
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusFailed,
	}); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("environment mutation error = %v, want ErrBusy", err)
	}
	if err := repo.UpdateTaskEnvironment(recoveryclaim.WithClaim(ctx, claim), &models.TaskEnvironment{
		ID:           environmentID,
		TaskID:       taskID,
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusFailed,
	}); err != nil {
		t.Fatalf("guarded environment mutation: %v", err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-recovery-claim", SessionID: "session-recovery-1", TaskID: taskID,
	}); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("runtime startup mutation error = %v, want ErrBusy", err)
	}

	if err := repo.TransferTaskEnvironmentOwnership(ctx, environmentID, taskID, 1, "task-recovery-new-owner"); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("ownership transfer error = %v, want ErrBusy", err)
	}

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-recovery-attached",
		TaskID:            taskID,
		TaskEnvironmentID: environmentID,
		State:             models.TaskSessionStateWaitingForInput,
	}); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("session attachment error = %v, want ErrBusy", err)
	}

	if err := repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		ID:          "cleanup-recovery-claim",
		OperationID: "cleanup-operation-recovery-claim",
		TaskID:      taskID,
		Trigger:     models.TaskResourceCleanupTriggerArchive,
		State:       models.TaskResourceCleanupStatePrepared,
	}); !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("cleanup creation error = %v, want ErrBusy", err)
	}

	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatalf("release recovery claim: %v", err)
	}
	claim = nil

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-recovery-attached",
		TaskID:            taskID,
		TaskEnvironmentID: environmentID,
		State:             models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session after release: %v", err)
	}

	_, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, "session-recovery-new", "operation-recovery-new", 1))
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("claim with live attached session error = %v, want ErrBusy", err)
	}
}

func TestTaskEnvironmentRecoveryClaimRejectsStaleOwnership(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "task-recovery-generation", "environment-recovery-generation")
	_, err := repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), recoveryClaimRequest(
		"environment-recovery-generation", "task-recovery-generation", "session-generation", "operation-generation", 2))
	if !errors.Is(err, repoerrors.ErrTaskEnvironmentOwnershipChanged) {
		t.Fatalf("stale generation error = %v, want ErrTaskEnvironmentOwnershipChanged", err)
	}
}
