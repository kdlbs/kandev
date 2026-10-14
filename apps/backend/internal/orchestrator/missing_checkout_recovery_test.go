package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6
func TestMissingCheckoutRecoveryFreshStartPreflightPreservesProviderState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	launchCalls := 0
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return nil, errors.New("agent must not start after recovery refusal")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	const taskID = "task-missing-checkout-preflight"
	const sessionID = "session-missing-checkout-preflight"
	const environmentID = "environment-missing-checkout-preflight"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	now := time.Now().UTC()
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	worktreePath := filepath.Join(t.TempDir(), "tasks", "task-root", "repository", "main")
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-missing-checkout-preflight", WorkspaceID: "ws1", Name: "repository",
		SourceType: "local", LocalPath: repositoryPath, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		ExecutorID: models.ExecutorIDWorktree, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: worktreePath, TaskDirName: "task-root", OwnershipGeneration: 1,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-missing-checkout-preflight", RepositoryID: "repository-missing-checkout-preflight",
			BranchSlug: "main", WorktreeID: "worktree-missing-checkout-preflight", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Status: "active", Position: 0, CreatedAt: now, UpdatedAt: now,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-missing-checkout-preflight"
	session.TaskEnvironmentID = environmentID
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-missing-checkout-preflight", SessionID: sessionID, TaskID: taskID,
		ResumeToken: "provider-conversation-preserved", Resumable: true, CreatedAt: now, UpdatedAt: now,
	}))

	var beforeMetadata map[string]interface{}
	metadataJSON, err := json.Marshal(session.Metadata)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(metadataJSON, &beforeMetadata))
	preflightCalls := 0
	svc.executor.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		preflightCalls++
		require.Equal(t, taskID, req.TaskID)
		require.Equal(t, sessionID, req.SessionID)
		require.Equal(t, environmentID, req.TaskEnvironmentID)
		require.Equal(t, string(models.ExecutorTypeWorktree), req.ExecutorType)
		require.True(t, req.SelectionSnapshot.Valid())
		require.Len(t, req.SelectionSnapshot.Slots, 1)
		require.Len(t, req.Slots, 1)
		return nil, &worktree.WorktreeRecoveryError{
			TaskID: taskID, Checkout: worktreePath, State: "admission",
			Reason: "selected worktree inventory changed while waiting for inspection",
		}
	})

	_, err = svc.RecoverSession(ctx, taskID, sessionID, "fresh_start")
	var recoveryErr *worktree.WorktreeRecoveryError
	require.ErrorAs(t, err, &recoveryErr)
	require.Equal(t, "admission", recoveryErr.State)
	require.Contains(t, recoveryErr.Reason, "inventory changed")
	require.Equal(t, 1, preflightCalls)
	require.Zero(t, launchCalls, "recovery refusal must stop before agent startup")

	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "provider-conversation-preserved", running.ResumeToken,
		"fresh start must preserve provider identity until selected-workspace admission succeeds")
	storedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(beforeMetadata, storedSession.Metadata),
		"recovery refusal must preserve session recovery metadata")
	storedEnvironment, err := repo.GetTaskEnvironment(ctx, environmentID)
	require.NoError(t, err)
	require.Equal(t, models.TaskEnvironmentStatusReady, storedEnvironment.Status)
	require.NotContains(t, taskRepo.updatedStates, taskID, "refusal must not mutate task state")
	storedSession, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, storedSession.State)
}
