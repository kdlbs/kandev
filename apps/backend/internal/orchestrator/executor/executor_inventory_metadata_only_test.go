package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func newLocalMetadataRepairFixture(t *testing.T) (*Executor, *mockAgentManager, *models.TaskSession) {
	t.Helper()
	db, _ := remoteSelectionSQLiteTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(db, db, nil)
	ctx := t.Context()
	source, checkout := createExecutorPreservationFixture(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository", WorkspaceID: "workspace", Name: "repository", SourceType: sourceTypeLocal, LocalPath: source,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository", TaskID: "task", RepositoryID: "repository", BaseBranch: "main",
	}))
	env := &models.TaskEnvironment{
		ID: "environment", TaskID: "task", ExecutorType: "worktree", ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: checkout,
	}
	env.Repos = []*models.TaskEnvironmentRepo{{
		ID: "environment-repo", TaskEnvironmentID: env.ID, RepositoryID: "repository", BranchSlug: "stale",
		WorktreeID: "worktree-recovery", WorktreePath: checkout, WorktreeBranch: "feature/recovery", Status: "active",
	}}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
	session := &models.TaskSession{
		ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, RepositoryID: "repository",
		State: models.TaskSessionStateFailed, ExecutorID: models.ExecutorIDWorktree, AgentProfileID: "profile-123",
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	manager := &mockAgentManager{}
	return newTestExecutor(t, manager, repo), manager, session
}

func TestResumeAfterLocalMetadataRepairAdmitsRepairedCheckout(t *testing.T) {
	executor, manager, session := newLocalMetadataRepairFixture(t)
	receipt, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", "repair")
	require.NoError(t, err)
	require.NotNil(t, receipt.Preservation.Fence)
	require.True(t, receipt.PostRepairMatched)

	execution, err := executor.ResumeSessionWithOptions(t.Context(), session, true, ResumeOptions{})
	require.NoError(t, err, "the ordinary retry must admit the checkout repaired by the metadata-only request")
	require.Equal(t, 1, manager.launchAgentCallCount)
	require.Equal(t, receipt.ID, execution.WorkspaceInventoryRecoveryReceipt.ID)
}
