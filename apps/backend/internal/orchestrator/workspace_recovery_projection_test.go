package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type workspaceRecoveryErrorReporterFunc func(context.Context, models.WorkspaceRecoveryErrorObservation) (string, error)

type workspaceRecoveryStatusReaderFunc func(context.Context, string) (*models.TaskEnvironmentRecoveryOperation, bool, error)

func (f workspaceRecoveryStatusReaderFunc) WorkspaceRecoveryProjection(
	ctx context.Context,
	environmentID string,
) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
	return f(ctx, environmentID)
}

func (f workspaceRecoveryErrorReporterFunc) ReportManagedCloneRelocationRequired(
	ctx context.Context,
	observation models.WorkspaceRecoveryErrorObservation,
) (string, error) {
	return f(ctx, observation)
}

func TestRecoverSessionProjectsManagedCloneRefusalWithCapturedIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedTaskAndSession(t, repo, "task-recovery-projection", "session-recovery-projection", models.TaskSessionStateCancelled)
	now := time.Now().UTC()
	const environmentID = "environment-recovery-projection"
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-recovery-projection", WorkspaceID: "ws1", Name: "recovery",
		SourceType: "local", LocalPath: t.TempDir(), CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: "task-recovery-projection", ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: t.TempDir(), OwnershipGeneration: 7,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-recovery-projection", RepositoryID: "repository-recovery-projection",
			WorktreeID: "worktree-recovery-projection", WorktreePath: t.TempDir(), WorktreeBranch: "feature/recovery",
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-recovery-projection")
	require.NoError(t, err)
	session.TaskEnvironmentID = environmentID
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "old generic failure", OccurredAt: now, Scope: models.ErrorScopeSession, StampValue: "generic-stamp",
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-recovery-projection", SessionID: session.ID, TaskID: session.TaskID,
		AgentExecutionID: "execution-observed", CreatedAt: now, UpdatedAt: now,
	}))

	var captured models.WorkspaceRecoveryErrorObservation
	svc.SetWorkspaceRecoveryErrorReporter(workspaceRecoveryErrorReporterFunc(func(
		_ context.Context,
		observation models.WorkspaceRecoveryErrorObservation,
	) (string, error) {
		captured = observation
		return "relocation-projection-stamp", nil
	}))
	svc.executor.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, request worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		require.Equal(t, environmentID, request.TaskEnvironmentID)
		return nil, &worktree.ManagedCloneRelocationRequiredError{TaskID: request.TaskID}
	})

	_, err = svc.RecoverSession(ctx, session.TaskID, session.ID, "resume")
	var recoveryErr *ManagedCloneRelocationRecoveryError
	require.ErrorAs(t, err, &recoveryErr)
	require.Equal(t, "relocation-projection-stamp", recoveryErr.Stamp)
	require.Equal(t, models.TaskSessionStateCancelled, captured.SessionState)
	require.Equal(t, environmentID, captured.TaskEnvironmentID)
	require.Equal(t, "task-recovery-projection", captured.EnvironmentOwnerTaskID)
	require.EqualValues(t, 7, captured.OwnershipGeneration)
	require.Equal(t, "execution-observed", captured.AgentExecutionID)
	require.Equal(t, "generic-stamp", captured.ExpectedErrorStamp)
}

func TestWorkspaceRecoveryStatusReadDoesNotBootstrapAgent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	launchCalls := 0
	agentMgr := &mockAgentManager{launchAgentFunc: func(
		context.Context,
		*executor.LaunchAgentRequest,
	) (*executor.LaunchAgentResponse, error) {
		launchCalls++
		return nil, nil
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	const taskID = "task-recovery-status-read"
	const sessionID = "session-recovery-status-read"
	const environmentID = "environment-recovery-status-read"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 12,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/synthetic/recovery-status-read",
	}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.TaskEnvironmentID = environmentID
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	want := &models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: environmentID, OwnerTaskID: taskID, OwnershipGeneration: 12,
		SessionID: sessionID, OperationID: "operation-status-read", AttemptID: "attempt-status-read",
		Kind: recoveryoperation.KindManagedCloneRelocation, Revision: 4,
		State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting,
	}
	readerCalls := 0
	svc.SetWorkspaceRecoveryStatusReader(workspaceRecoveryStatusReaderFunc(func(
		_ context.Context,
		gotEnvironmentID string,
	) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
		readerCalls++
		require.Equal(t, environmentID, gotEnvironmentID)
		return want, true, nil
	}))

	got, live, err := svc.GetWorkspaceRecoveryStatus(ctx, taskID, sessionID)
	require.NoError(t, err)
	require.Same(t, want, got)
	require.True(t, live)
	require.Equal(t, 1, readerCalls)
	require.Zero(t, launchCalls, "a status read must not initialize or launch an agent")
}

func TestRecoverSessionReturnsLiveWorkspaceRecoveryWithoutPreflight(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	launchCalls := 0
	agentMgr := &mockAgentManager{launchAgentFunc: func(
		context.Context,
		*executor.LaunchAgentRequest,
	) (*executor.LaunchAgentResponse, error) {
		launchCalls++
		return nil, nil
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	const taskID = "task-recovery-in-progress"
	const sessionID = "session-recovery-in-progress"
	const environmentID = "environment-recovery-in-progress"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 5,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/synthetic/recovery-in-progress",
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.TaskEnvironmentID = environmentID
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	svc.SetWorkspaceRecoveryStatusReader(workspaceRecoveryStatusReaderFunc(func(
		_ context.Context,
		gotEnvironmentID string,
	) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
		require.Equal(t, environmentID, gotEnvironmentID)
		return &models.TaskEnvironmentRecoveryOperation{
			TaskEnvironmentID: environmentID, OwnerTaskID: taskID,
			OwnershipGeneration: 5, SessionID: sessionID,
			OperationID: "operation-recovery-in-progress", AttemptID: "attempt-recovery-in-progress",
			Kind: recoveryoperation.KindManagedCloneRelocation, Revision: 14,
			State: recoveryoperation.StateRunning, Phase: recoveryoperation.PhaseSnapshotting,
		}, true, nil
	}))
	preflightCalls := 0
	svc.executor.SetSelectedWorktreeRecoveryAdmission(func(
		context.Context,
		worktree.RecoveryAdmissionRequest,
	) (*worktree.RecoveryAdmission, error) {
		preflightCalls++
		return nil, errors.New("live recovery should return before filesystem preflight")
	})

	response, err := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, "resume", RecoverSessionOptions{})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.InProgress)
	require.True(t, response.Success)
	require.Equal(t, "snapshotting", response.WorkspaceRecovery.Phase)
	require.True(t, response.WorkspaceRecovery.RunnerLive)
	require.Zero(t, preflightCalls)
	require.Zero(t, launchCalls)
}
