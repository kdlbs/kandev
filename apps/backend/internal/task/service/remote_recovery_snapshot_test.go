package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type staleRemoteRecoverySessionReader struct {
	*sqliterepo.Repository
	sessionID string
}

type remoteRecoverySnapshotProvider interface {
	GetRemoteRecoverySnapshot(context.Context, string) (*agentruntime.RemoteRecoverySnapshot, error)
}

func (r staleRemoteRecoverySessionReader) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := r.Repository.GetTaskSession(ctx, sessionID)
	if err == nil && sessionID == r.sessionID {
		session.AgentExecutionID = "stale-execution"
	}
	return session, err
}

func TestServiceProvidesReadOnlyRemoteRecoverySnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-remote-recovery", Name: "Remote", OwnerID: "owner-before", OrgID: "org-before",
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-remote-recovery", WorkspaceID: "ws-remote-recovery", Name: "Remote",
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-remote-recovery", WorkspaceID: "ws-remote-recovery", WorkflowID: "wf-remote-recovery",
		WorkflowStepID: "step-remote-recovery", Title: "Remote", State: "IN_PROGRESS",
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-remote-recovery", TaskID: "task-remote-recovery", OwnershipGeneration: 4,
		ExecutorType: "ssh", ExecutorID: "executor-remote-recovery", Status: models.TaskEnvironmentStatusReady,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-remote-recovery", TaskID: "task-remote-recovery", TaskEnvironmentID: "env-remote-recovery",
		AgentExecutionID: "execution-remote-recovery", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-remote-recovery", SessionID: "session-remote-recovery", TaskID: "task-remote-recovery",
		AgentExecutionID: "execution-remote-recovery", Runtime: "ssh", ExecutorID: "executor-remote-recovery",
		Status: models.ExecutorRunningStatusReady,
	}))

	provider, ok := any(svc).(remoteRecoverySnapshotProvider)
	require.True(t, ok, "task service must expose the narrow read-only recovery snapshot capability")
	snapshot, err := provider.GetRemoteRecoverySnapshot(ctx, "session-remote-recovery")
	require.NoError(t, err)
	require.Equal(t, &agentruntime.RemoteRecoverySnapshot{
		TaskID:                     "task-remote-recovery",
		SessionID:                  "session-remote-recovery",
		SessionState:               models.TaskSessionStateRunning,
		AgentExecutionID:           "execution-remote-recovery",
		Runtime:                    "ssh",
		TaskWorkspaceID:            "ws-remote-recovery",
		WorkspaceOwnerID:           "owner-before",
		WorkspaceOrgID:             "org-before",
		TaskEnvironmentID:          "env-remote-recovery",
		EnvironmentOwnerTaskID:     "task-remote-recovery",
		EnvironmentOwnershipGen:    4,
		EnvironmentExecutorType:    "ssh",
		EnvironmentExecutorID:      "executor-remote-recovery",
		EnvironmentStatus:          string(models.TaskEnvironmentStatusReady),
		TaskResourceCleanupRunning: false,
	}, snapshot)

	workspace, err := repo.GetWorkspace(ctx, "ws-remote-recovery")
	require.NoError(t, err)
	require.Equal(t, "owner-before", workspace.OwnerID, "snapshot reads must not reassign workspace ownership")
	require.Equal(t, "org-before", workspace.OrgID)

	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-remote-recovery-drift", TaskID: "task-remote-recovery", TaskEnvironmentID: "env-remote-recovery",
		AgentExecutionID: "stale-execution", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-remote-recovery-drift", SessionID: "session-remote-recovery-drift", TaskID: "task-remote-recovery",
		AgentExecutionID: "current-execution", Runtime: "ssh", ExecutorID: "executor-remote-recovery",
		Status: models.ExecutorRunningStatusReady,
	}))
	svc.sessions = staleRemoteRecoverySessionReader{
		Repository: repo, sessionID: "session-remote-recovery-drift",
	}
	_, err = provider.GetRemoteRecoverySnapshot(ctx, "session-remote-recovery-drift")
	require.Error(t, err, "a remote owner snapshot must reject disagreement with the task-session execution identity")
}

func TestServiceRemoteRecoverySnapshotPinsInheritedEnvironmentOwner(t *testing.T) {
	for _, ownerCleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_workspace_owner_is_valid", true: "owner_cleanup_blocks_attach"}[ownerCleanup], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			const workspaceID = "ws-inherited-remote-recovery"
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Shared workspace"}))
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-inherited-remote-recovery", WorkspaceID: workspaceID, Name: "Shared"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{
				ID: "task-environment-owner", WorkspaceID: workspaceID, WorkflowID: "wf-inherited-remote-recovery",
				WorkflowStepID: "step-inherited-owner", Title: "Environment owner", State: "IN_PROGRESS",
			}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{
				ID: "task-environment-borrower", WorkspaceID: workspaceID, WorkflowID: "wf-inherited-remote-recovery",
				WorkflowStepID: "step-inherited-borrower", ParentID: "task-environment-owner",
				Title: "Environment borrower", State: "IN_PROGRESS",
			}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: "env-inherited-remote-recovery", TaskID: "task-environment-owner", OwnershipGeneration: 9,
				ExecutorType: "ssh", ExecutorID: "executor-inherited", Status: models.TaskEnvironmentStatusReady,
			}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session-inherited-remote-recovery", TaskID: "task-environment-borrower",
				TaskEnvironmentID: "env-inherited-remote-recovery", State: models.TaskSessionStateWaitingForInput,
				AgentExecutionID: "execution-inherited-remote-recovery",
				StartedAt:        time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}))
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "running-inherited-remote-recovery", SessionID: "session-inherited-remote-recovery",
				TaskID: "task-environment-borrower", AgentExecutionID: "execution-inherited-remote-recovery",
				Runtime: "ssh", Status: models.ExecutorRunningStatusReady,
			}))
			if ownerCleanup {
				require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
					ID: "cleanup-inherited-owner", OperationID: "archive:task-environment-owner",
					TaskID: "task-environment-owner", Trigger: models.TaskResourceCleanupTriggerArchive,
					State: models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
				}))
			}

			provider, ok := any(svc).(remoteRecoverySnapshotProvider)
			require.True(t, ok)
			snapshot, err := provider.GetRemoteRecoverySnapshot(ctx, "session-inherited-remote-recovery")
			require.NoError(t, err)
			require.Equal(t, "task-environment-borrower", snapshot.TaskID)
			require.Equal(t, workspaceID, snapshot.TaskWorkspaceID)
			require.Equal(t, "task-environment-owner", snapshot.EnvironmentOwnerTaskID)
			require.Equal(t, int64(9), snapshot.EnvironmentOwnershipGen)
			require.Equal(t, ownerCleanup, snapshot.EnvironmentOwnerCleanupRunning,
				"the caller can refuse attach while the borrowed environment owner is cleaning its resources")
			require.False(t, snapshot.TaskResourceCleanupRunning)
		})
	}
}
