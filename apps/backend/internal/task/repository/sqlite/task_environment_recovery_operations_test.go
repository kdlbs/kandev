package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

func TestRecoveryOperationProjectionCASAndClaimRelease(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, _ := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "recovery-operation", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	started, err := repo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID,
		OwnershipGeneration: env.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID, ErrorStamp: "error-stamp", Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-current", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 2,
		SelectedRepositoryIDs: []string{"repository-a", "repository-b"},
	})
	if err != nil {
		t.Fatalf("BeginTaskEnvironmentRecoveryOperation: %v", err)
	}
	if started.AttemptID == "" || started.Revision < 1 || started.RepositoryTotal != 2 {
		t.Fatalf("started projection identity = %+v", started)
	}

	updated, err := repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: claim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-current",
		ExpectedRevision: started.Revision, State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseSnapshotting, RepositoryID: "repository-a",
		RepositoryPosition: 1, RepositoryTotal: 2,
	})
	if err != nil || updated.Revision <= started.Revision || updated.Phase != recoveryoperation.PhaseSnapshotting {
		t.Fatalf("phase update = %+v, %v", updated, err)
	}
	_, err = repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: claim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-current",
		ExpectedRevision: started.Revision, State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseVerifyingSnapshot,
	})
	if !errors.Is(err, recoveryoperation.ErrStaleWriter) {
		t.Fatalf("late phase update error = %v, want stale writer", err)
	}
	endedAt := repo.nowUTC()
	terminal, err := repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: claim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-current",
		ExpectedRevision: updated.Revision, State: recoveryoperation.StateFailed,
		Phase: recoveryoperation.PhaseVerifyingSnapshot, RepositoryID: "repository-a",
		RepositoryPosition: 1, RepositoryTotal: 2, EndedAt: &endedAt, ReasonCode: "snapshot_failed",
	})
	if err != nil || terminal.State != recoveryoperation.StateFailed || terminal.EndedAt == nil {
		t.Fatalf("terminal result = %+v, %v", terminal, err)
	}

	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatalf("ReleaseTaskEnvironmentRecoveryClaim: %v", err)
	}
	persisted, err := repo.GetTaskEnvironmentRecoveryOperation(ctx, env.ID)
	if err != nil || persisted == nil || persisted.OperationID != claim.OperationID || persisted.Revision != terminal.Revision {
		t.Fatalf("projection after claim release = %+v, %v", persisted, err)
	}

	newClaim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "recovery-operation", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("re-acquire same operation claim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, newClaim) }()
	retried, err := repo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID,
		OwnershipGeneration: env.OwnershipGeneration, SessionID: newClaim.SessionID,
		OperationID: newClaim.OperationID, ErrorStamp: "error-stamp", Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-current", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 2,
		SelectedRepositoryIDs: []string{"repository-a", "repository-b"},
	})
	if err != nil || retried.AttemptID == started.AttemptID || retried.Revision <= terminal.Revision {
		t.Fatalf("new attempt = %+v, %v", retried, err)
	}
	_, err = repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: claim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-current",
		ExpectedRevision: retried.Revision, State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseVerifyingSnapshot,
	})
	if !errors.Is(err, recoveryoperation.ErrStaleWriter) {
		t.Fatalf("previous attempt update error = %v, want stale writer", err)
	}
}

func TestWorkspaceRecoveryInterruptedRunnerDoesNotExpireClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, _ := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "recovery-interrupted", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	started, err := repo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID,
		OwnershipGeneration: env.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID, Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-old", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseSnapshotting, RepositoryTotal: 1,
		SelectedRepositoryIDs: []string{"repository-a"},
	})
	if err != nil {
		t.Fatalf("BeginTaskEnvironmentRecoveryOperation: %v", err)
	}
	interrupted, err := repo.InterruptTaskEnvironmentRecoveryOperations(ctx, "runner-current")
	if err != nil || interrupted != 1 {
		t.Fatalf("InterruptTaskEnvironmentRecoveryOperations = %d, %v", interrupted, err)
	}
	persisted, err := repo.GetTaskEnvironmentRecoveryOperation(ctx, env.ID)
	if err != nil || persisted == nil || persisted.State != recoveryoperation.StateInterrupted || persisted.Revision <= started.Revision {
		t.Fatalf("interrupted projection = %+v, %v", persisted, err)
	}
	currentClaim, err := repo.GetTaskEnvironmentRecoveryClaim(ctx, env.ID)
	if err != nil || currentClaim == nil || currentClaim.OperationID != claim.OperationID {
		t.Fatalf("interrupted runner evicted durable claim: %+v, %v", currentClaim, err)
	}
}

func TestRecoveryOperationCanRestartAfterEnvironmentOwnershipTransfer(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	env, _ := seedWorkspaceInventoryRecovery(t, repo)
	oldClaim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "operation-before-transfer", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("acquire old-owner claim: %v", err)
	}
	started, err := repo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID,
		OwnershipGeneration: env.OwnershipGeneration, SessionID: oldClaim.SessionID,
		OperationID: oldClaim.OperationID, Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-old-owner", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 1,
		SelectedRepositoryIDs: []string{"repository-recovery"},
	})
	if err != nil {
		t.Fatalf("begin old-owner operation: %v", err)
	}
	endedAt := repo.nowUTC()
	terminal, err := repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: oldClaim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-old-owner",
		ExpectedRevision: started.Revision, State: recoveryoperation.StateFailed,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 1,
		EndedAt: &endedAt, ReasonCode: "original_recovery_failed",
	})
	if err != nil {
		t.Fatalf("end old-owner operation: %v", err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, oldClaim); err != nil {
		t.Fatalf("release old-owner claim: %v", err)
	}

	const newOwner = "task-recovery-new-owner"
	seedExecutorRunningCleanupTask(t, repo, newOwner)
	if err := repo.TransferTaskEnvironmentOwnership(ctx, env.ID, env.TaskID, env.OwnershipGeneration, newOwner); err != nil {
		t.Fatalf("transfer environment: %v", err)
	}
	transferred, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil || transferred.TaskID != newOwner || transferred.OwnershipGeneration != env.OwnershipGeneration+1 {
		t.Fatalf("transferred environment = %+v, %v", transferred, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery-new-owner", TaskID: newOwner, TaskEnvironmentID: env.ID,
		State: models.TaskSessionStateFailed,
	}); err != nil {
		t.Fatalf("create new-owner session: %v", err)
	}
	newClaim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, newOwner, "session-recovery-new-owner", "operation-after-transfer", transferred.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("acquire new-owner claim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, newClaim) }()
	retried, err := repo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: env.ID, OwnerTaskID: newOwner,
		OwnershipGeneration: transferred.OwnershipGeneration, SessionID: newClaim.SessionID,
		OperationID: newClaim.OperationID, Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-new-owner", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 1,
		SelectedRepositoryIDs: []string{"repository-recovery"},
	})
	if err != nil || retried.OwnerTaskID != newOwner || retried.OwnershipGeneration != transferred.OwnershipGeneration || retried.Revision <= terminal.Revision {
		t.Fatalf("new-owner operation = %+v, %v", retried, err)
	}
	_, err = repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
		TaskEnvironmentID: env.ID, OperationID: oldClaim.OperationID, AttemptID: started.AttemptID,
		OwnershipGeneration: env.OwnershipGeneration, RunnerInstanceID: "runner-old-owner",
		ExpectedRevision: terminal.Revision, State: recoveryoperation.StateFailed,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 1,
		EndedAt: &endedAt, ReasonCode: "stale_old_owner",
	})
	if !errors.Is(err, recoveryoperation.ErrStaleWriter) {
		t.Fatalf("stale old-owner write error = %v, want ErrStaleWriter", err)
	}
}
