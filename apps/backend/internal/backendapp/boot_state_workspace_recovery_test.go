package backendapp

import (
	"context"
	"testing"
	"time"

	taskdto "github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestTaskDetailBootHydratesWorkspaceRecoveryProjection(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-boot-recovery", Name: "Recovery"}))
	require.NoError(t, harness.taskRepo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-boot-recovery", WorkspaceID: "workspace-boot-recovery", Name: "Flow",
	}))
	require.NoError(t, harness.taskRepo.CreateTask(ctx, &models.Task{
		ID: "task-boot-recovery", WorkspaceID: "workspace-boot-recovery", WorkflowID: "workflow-boot-recovery",
		WorkflowStepID: "step-boot-recovery", Title: "Recovery", State: v1.TaskStateCreated, Priority: "medium",
	}))
	require.NoError(t, harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-boot-recovery", TaskID: "task-boot-recovery", OwnershipGeneration: 19,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/synthetic/boot-recovery",
	}))
	for _, sessionID := range []string{"session-boot-recovery", "session-boot-recovery-sibling"} {
		require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-boot-recovery", TaskEnvironmentID: "environment-boot-recovery",
			State: models.TaskSessionStateFailed, StartedAt: now, UpdatedAt: now,
		}))
	}
	claim, err := harness.taskRepo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: "environment-boot-recovery", OwnerTaskID: "task-boot-recovery",
		OwnershipGeneration: 19, SessionID: "session-boot-recovery", OperationID: "operation-boot-recovery",
		ExecutorType: string(models.ExecutorTypeWorktree),
	})
	require.NoError(t, err)
	defer func() { _ = harness.taskRepo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	_, err = harness.taskRepo.BeginTaskEnvironmentRecoveryOperation(ctx, models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID, Kind: recoveryoperation.KindManagedCloneRelocation,
		RunnerInstanceID: "runner-boot-recovery", State: recoveryoperation.StateRunning,
		Phase: recoveryoperation.PhaseSnapshotting, RepositoryTotal: 1,
		SelectedRepositoryIDs: []string{"repository-boot-recovery"},
	})
	require.NoError(t, err)
	sessions, err := harness.taskRepo.ListTaskSessions(ctx, "task-boot-recovery")
	require.NoError(t, err)

	state := map[string]any{}
	builder := bootStateBuilder{p: routeParams{taskSvc: harness.taskSvc}}
	builder.addTaskDetailSessionsState(ctx, state, "task-boot-recovery", sessions, "session-boot-recovery")
	taskSessions := state["taskSessions"].(map[string]any)
	items := taskSessions["items"].(map[string]taskdto.TaskSessionDTO)
	for _, sessionID := range []string{"session-boot-recovery", "session-boot-recovery-sibling"} {
		projection := items[sessionID].WorkspaceRecovery
		require.NotNil(t, projection)
		require.Equal(t, "environment-boot-recovery", projection.EnvironmentID)
		require.Equal(t, "snapshotting", projection.Phase)
		require.False(t, projection.RunnerLive, "a boot read does not invent an in-process runner")
	}
}
