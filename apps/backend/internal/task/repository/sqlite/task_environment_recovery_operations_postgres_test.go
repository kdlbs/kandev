package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

func TestPostgresRecoveryOperationSerializesWriters(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID        = "task-postgres-recovery-operation"
		environmentID = "environment-postgres-recovery-operation"
		sessionID     = "session-postgres-recovery-operation"
		operationID   = "operation-postgres-recovery-operation"
	)
	seedRecoveryClaimEnvironment(t, repoA, taskID, environmentID)
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateFailed,
	}); err != nil {
		t.Fatalf("create recovery session: %v", err)
	}
	claim, err := repoA.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		environmentID, taskID, sessionID, operationID, 1,
	))
	if err != nil {
		t.Fatalf("acquire recovery claim: %v", err)
	}
	defer func() { _ = repoA.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	started, err := repoA.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: environmentID, OwnerTaskID: taskID, OwnershipGeneration: 1,
		SessionID: sessionID, OperationID: operationID, Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-postgres-recovery-operation", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseChecking, RepositoryTotal: 1,
		SelectedRepositoryIDs: []string{"repository-postgres-recovery-operation"},
	})
	if err != nil {
		t.Fatalf("begin recovery operation: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, repo := range []*Repository{repoA, repoB} {
		wait.Add(1)
		go func(index int, repo *Repository) {
			defer wait.Done()
			<-start
			phase := recoveryoperation.PhaseSnapshotting
			if index == 1 {
				phase = recoveryoperation.PhaseRestoring
			}
			_, updateErr := repo.UpdateTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperationUpdate{
				TaskEnvironmentID: environmentID, OperationID: operationID,
				AttemptID: started.AttemptID, OwnershipGeneration: 1,
				RunnerInstanceID: "runner-postgres-recovery-operation",
				ExpectedRevision: started.Revision, State: recoveryoperation.StateRunning, Phase: phase,
			})
			results <- updateErr
		}(index, repo)
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	stale := 0
	for updateErr := range results {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, recoveryoperation.ErrStaleWriter):
			stale++
		default:
			t.Fatalf("concurrent phase update: %v", updateErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent writers: successes=%d stale=%d, want one each", successes, stale)
	}
	stored, err := repoA.GetTaskEnvironmentRecoveryOperation(ctx, environmentID)
	if err != nil || stored == nil || stored.Revision != started.Revision+1 {
		t.Fatalf("stored operation = %+v, err=%v", stored, err)
	}
}
