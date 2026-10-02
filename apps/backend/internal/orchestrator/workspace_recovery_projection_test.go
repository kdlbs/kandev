package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type workspaceRecoveryErrorReporterFunc func(context.Context, models.WorkspaceRecoveryErrorObservation) (string, error)

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
