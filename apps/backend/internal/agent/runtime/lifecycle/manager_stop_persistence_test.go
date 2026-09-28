package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestStopTaskExecutionPersistsStoppedRuntimeAndResumeToken(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	execution := &AgentExecution{
		ID: "execution-task-stop", TaskID: "task-stop-persisted-runtime", SessionID: "session-stop-persisted-runtime",
		Owner:  ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task-stop-persisted-runtime", SessionID: "session-stop-persisted-runtime"},
		Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
	running, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	running.ResumeToken = "saved-conversation-token"
	running.Resumable = true
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

	require.NoError(t, mgr.StopAgentWithReason(ctx, execution.ID, "user stop", true))
	running, err = repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, running.Status)
	require.Equal(t, "saved-conversation-token", running.ResumeToken)
	require.True(t, running.Resumable)
}

func TestSuspendIdlePreservesRuntimeOwnershipWithoutAgentStopped(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "workspace-idle-suspension", Name: "Idle suspension",
		ACPIdleSuspensionEnabled: true, ACPIdleTimeoutMinutes: 120,
	}))
	workspace, err := repo.GetWorkspace(ctx, "workspace-idle-suspension")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-idle-suspension", WorkspaceID: workspace.ID, Title: "Idle suspension",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-suspension", TaskID: "task-idle-suspension", State: models.TaskSessionStateWaitingForInput,
	}))
	mgr, eventBus := createTestManagerWithTracking()
	mgr.SetExecutorRunningWriter(repo)
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	execution := &AgentExecution{
		ID: "execution-idle-suspension", TaskID: "task-idle-suspension", SessionID: "session-idle-suspension",
		Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
		promptGeneration: 3, promptActivityEpoch: 5,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
	running, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	running.ResumeToken = "saved-conversation-token"
	running.Resumable = true
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

	require.NoError(t, mgr.SuspendIdle(ctx, IdleSuspensionIdentity{
		ExecutionID: execution.ID, SessionID: execution.SessionID,
		WorkspaceID: workspace.ID, PolicyUpdatedAt: workspace.UpdatedAt,
		PromptGeneration: 3, ActivityEpoch: 5,
	}))

	running, err = repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err, "idle suspension must retain its recovery inventory")
	require.Equal(t, models.ExecutorRunningStatusStopped, running.Status)
	require.Equal(t, models.ExecutorIdleSuspensionSuspended, running.IdleSuspensionState)
	require.Equal(t, "saved-conversation-token", running.ResumeToken)
	_, executionTracked := mgr.executionStore.Get(execution.ID)
	require.False(t, executionTracked)
	for _, published := range eventBus.PublishedEvents {
		require.NotEqual(t, events.AgentStopped, published.Event.Type,
			"idle suspension must not flow through generic cancellation handling")
	}
}

func TestSuspendIdleDoesNotStopWhenWorkspacePolicyChangesAfterClaim(t *testing.T) {
	ctx := context.Background()
	repo, db := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "workspace-idle-policy-race", Name: "Idle policy race",
		ACPIdleSuspensionEnabled: true, ACPIdleTimeoutMinutes: 120,
	}))
	workspace, err := repo.GetWorkspace(ctx, "workspace-idle-policy-race")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-idle-policy-race", WorkspaceID: workspace.ID, Title: "Idle policy race",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-idle-policy-race", TaskID: "task-idle-policy-race", State: models.TaskSessionStateWaitingForInput,
	}))
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	execution := &AgentExecution{
		ID: "execution-idle-policy-race", TaskID: "task-idle-policy-race", SessionID: "session-idle-policy-race",
		Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
	running, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	running.Resumable = true
	running.ResumeToken = "retained-token"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER change_idle_policy_after_claim
		AFTER UPDATE OF idle_suspension_state ON executors_running
		WHEN NEW.idle_suspension_state = 'suspending'
		BEGIN
			UPDATE workspaces
			   SET acp_idle_suspension_enabled = FALSE, updated_at = CURRENT_TIMESTAMP
			 WHERE id = 'workspace-idle-policy-race';
		END;
	`)
	require.NoError(t, err)

	err = mgr.SuspendIdle(ctx, IdleSuspensionIdentity{
		ExecutionID: execution.ID, SessionID: execution.SessionID,
		WorkspaceID: workspace.ID, PolicyUpdatedAt: workspace.UpdatedAt,
	})
	require.ErrorIs(t, err, ErrIdleSuspensionRejected)
	running, err = repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorIdleSuspensionNone, running.IdleSuspensionState)
	require.Empty(t, backend.stoppedID, "runtime teardown must not start after policy invalidation")
	_, tracked := mgr.executionStore.Get(execution.ID)
	require.True(t, tracked)
}

func TestSuspendIdleRejectsStaleIdentityAndMissingRestoreData(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resumeData bool
		identity   IdleSuspensionIdentity
	}{
		{
			name: "changed activity epoch", resumeData: true,
			identity: IdleSuspensionIdentity{
				ExecutionID: "execution-idle-rejected", SessionID: "session-idle-rejected",
				PromptGeneration: 3, ActivityEpoch: 6,
			},
		},
		{
			name: "missing resume token", resumeData: false,
			identity: IdleSuspensionIdentity{
				ExecutionID: "execution-idle-rejected", SessionID: "session-idle-rejected",
				PromptGeneration: 3, ActivityEpoch: 5,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo, _ := runOwnerTestRepository(t)
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
				ID: "workspace-idle-rejected", Name: "Idle suspension",
				ACPIdleSuspensionEnabled: true, ACPIdleTimeoutMinutes: 120,
			}))
			workspace, err := repo.GetWorkspace(ctx, "workspace-idle-rejected")
			require.NoError(t, err)
			tc.identity.WorkspaceID = workspace.ID
			tc.identity.PolicyUpdatedAt = workspace.UpdatedAt
			mgr, eventBus := createTestManagerWithTracking()
			mgr.SetExecutorRunningWriter(repo)
			backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
			mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
			mgr.executorRegistry.Register(backend)
			execution := &AgentExecution{
				ID: "execution-idle-rejected", TaskID: "task-idle-rejected", SessionID: "session-idle-rejected",
				Owner:  ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task-idle-rejected", SessionID: "session-idle-rejected"},
				Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
				promptGeneration: 3, promptActivityEpoch: 5,
			}
			require.NoError(t, mgr.executionStore.Add(execution))
			require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
			running, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
			require.NoError(t, err)
			if tc.resumeData {
				running.ResumeToken = "retained-token"
				running.Resumable = true
			}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

			err = mgr.SuspendIdle(ctx, tc.identity)
			require.ErrorIs(t, err, ErrIdleSuspensionRejected)
			failureStage, ok := IdleSuspensionFailureStage(err)
			require.True(t, ok, "failed suspensions expose a bounded failure stage")
			if tc.resumeData {
				require.Equal(t, "prompt_activity_changed", failureStage)
			} else {
				require.Equal(t, "restore_data", failureStage)
			}
			stillRunning, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.ExecutorIdleSuspensionNone, stillRunning.IdleSuspensionState)
			_, stillTracked := mgr.executionStore.Get(execution.ID)
			require.True(t, stillTracked)
			for _, published := range eventBus.PublishedEvents {
				require.NotEqual(t, events.AgentStopped, published.Event.Type)
			}
		})
	}
}
