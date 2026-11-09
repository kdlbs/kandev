package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const (
	metadataOnlyTaskID    = "task-metadata-only"
	metadataOnlySessionID = "session-metadata-only"
	metadataOnlyEnvID     = "environment-metadata-only"
)

// newMetadataOnlyRecoveryService seeds a failed session whose ready worktree
// environment holds one local repository checkout with a stale inventory slug.
func newMetadataOnlyRecoveryService(t *testing.T) (*Service, *sqliterepo.Repository, *int) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	source := filepath.Join(t.TempDir(), "repository")
	checkout := filepath.Join(t.TempDir(), "checkout")
	require.NoError(t, os.Mkdir(source, 0o755))
	metadataOnlyGit(t, source, "init", "-b", "main")
	metadataOnlyGit(t, source, "config", "user.email", "metadata-only@example.test")
	metadataOnlyGit(t, source, "config", "user.name", "Metadata Only")
	require.NoError(t, os.WriteFile(filepath.Join(source, "README.md"), []byte("clean\n"), 0o644))
	metadataOnlyGit(t, source, "add", "README.md")
	metadataOnlyGit(t, source, "commit", "-m", "fixture")
	metadataOnlyGit(t, source, "branch", "feature/recovery")
	metadataOnlyGit(t, source, "worktree", "add", checkout, "feature/recovery")

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-metadata-only", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-metadata-only", WorkspaceID: "ws-metadata-only", Name: "repository",
		SourceType: "local", LocalPath: source, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: metadataOnlyTaskID, WorkspaceID: "ws-metadata-only", Title: "Task", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-metadata-only", TaskID: metadataOnlyTaskID, RepositoryID: "repository-metadata-only",
		BaseBranch: "main", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: metadataOnlyEnvID, TaskID: metadataOnlyTaskID, ExecutorType: string(models.ExecutorTypeWorktree),
		ExecutorID: models.ExecutorIDWorktree, Status: models.TaskEnvironmentStatusReady, WorkspacePath: checkout,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-metadata-only", RepositoryID: "repository-metadata-only", BranchSlug: "stale",
			WorktreeID: "worktree-metadata-only", WorktreePath: checkout, WorktreeBranch: "feature/recovery",
			Status: "active", CreatedAt: now, UpdatedAt: now,
		}},
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: metadataOnlySessionID, TaskID: metadataOnlyTaskID, TaskEnvironmentID: metadataOnlyEnvID,
		RepositoryID: "repository-metadata-only", State: models.TaskSessionStateFailed,
		ExecutorID: models.ExecutorIDWorktree, AgentProfileID: "profile-metadata-only", StartedAt: now, UpdatedAt: now,
	}))

	launchCalls := 0
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return nil, errors.New("a metadata-only repair must not launch an agent")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	return svc, repo, &launchCalls
}

func TestRecoverSessionWorkspaceOnlyRepairsInventoryWithoutLaunching(t *testing.T) {
	ctx := context.Background()
	svc, repo, launchCalls := newMetadataOnlyRecoveryService(t)

	response, err := svc.RecoverSessionWithOptions(ctx, metadataOnlyTaskID, metadataOnlySessionID, recoveryActionRepairWorkspaceInventory,
		RecoverSessionOptions{WorkspaceOnly: true, IdempotencyKey: "  metadata-only  "})
	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "workspace_repaired", response.State)
	require.Equal(t, "not_started", response.ActivationDisposition)
	require.Equal(t, "metadata_only", response.ActivationReason)
	require.NotNil(t, response.WorkspaceInventoryRecoveryReceipt)
	require.NotNil(t, response.WorkspaceInventoryRecoveryReceipt.Preservation.Fence)
	require.Zero(t, *launchCalls)

	stored, err := repo.GetWorkspaceInventoryRepairReceipt(ctx, metadataOnlyTaskID, "metadata-only")
	require.NoError(t, err, "the idempotency key is trimmed before it reaches the executor")
	require.NotNil(t, stored)
	require.Equal(t, response.WorkspaceInventoryRecoveryReceipt.ID, stored.ID)
	session, err := repo.GetTaskSession(ctx, metadataOnlySessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State, "a metadata-only repair leaves the session terminal")
	rows, err := repo.ListTaskEnvironmentRepos(ctx, metadataOnlyEnvID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotEqual(t, "stale", rows[0].BranchSlug)
}

func TestRecoverSessionWorkspaceOnlyRejectsOtherActionsAndMissingKey(t *testing.T) {
	tests := []struct {
		name   string
		action string
		key    string
	}{
		{name: "resume", action: recoveryActionResume, key: "metadata-only"},
		{name: "fresh start", action: "fresh_start", key: "metadata-only"},
		{name: "blank key", action: recoveryActionRepairWorkspaceInventory, key: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, launchCalls := newMetadataOnlyRecoveryService(t)

			_, err := svc.RecoverSessionWithOptions(ctx, metadataOnlyTaskID, metadataOnlySessionID, tt.action,
				RecoverSessionOptions{WorkspaceOnly: true, IdempotencyKey: tt.key})
			require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryInvalid)
			require.Zero(t, *launchCalls)
			rows, err := repo.ListTaskEnvironmentRepos(ctx, metadataOnlyEnvID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "stale", rows[0].BranchSlug)
		})
	}
}

func metadataOnlyGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, output)
}
