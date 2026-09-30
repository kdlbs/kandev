package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6
func TestMissingCheckoutRecoveryLifecycleRestoresAndProjectsSelectedWorkspace(t *testing.T) {
	ctx := context.Background()
	fixture := newLifecycleMissingCheckoutRecoveryFixture(t)

	manager := newTestManager(t)
	manager.SetWorktreeManager(fixture.manager)
	info := &WorkspaceInfo{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		EnvironmentOwnerTaskID: fixture.taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), TaskDirName: fixture.taskDirName,
		WorkspacePath: filepath.Join(fixture.tasksBasePath, fixture.taskDirName),
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath,
			WorktreeID: fixture.worktreeID, BranchSlug: fixture.branchSlug,
			RepoName: "repository", BaseBranch: "main", DefaultBranch: "main",
		}},
	}

	admission, err := manager.admitWorkspaceRecovery(ctx, info)
	require.NoError(t, err)
	require.NotNil(t, admission)
	t.Cleanup(func() { _ = admission.Release(context.Background()) })
	require.True(t, info.WorktreeRecoveryAdmitted)
	require.Equal(t, fixture.worktreePath, info.WorkspacePath,
		"single-repository workspace path must project the restored canonical checkout")
	require.Equal(t, fixture.worktreePath, info.WorkspaceRepositories[0].WorktreePath)
	require.Equal(t, fixture.branch, info.WorkspaceRepositories[0].WorktreeBranch)

	startCtx := worktree.WithRecoveryAdmission(ctx, admission)
	require.NoError(t, manager.reconcileWorkspaceWorktrees(startCtx, fixture.taskID, info),
		"lifecycle workspace reconciliation must attach the recovered row under the held claim")
	attached, err := fixture.store.GetWorktreeByID(ctx, fixture.worktreeID)
	require.NoError(t, err)
	require.NotNil(t, attached)
	require.Equal(t, fixture.worktreePath, attached.Path)
	require.Equal(t, fixture.branch, attached.Branch)
	require.Equal(t, fixture.worktreeID, attached.ID)
	head, err := lifecycleMissingCheckoutGit(fixture.worktreePath, "rev-parse", "HEAD")
	require.NoError(t, err)
	require.Equal(t, fixture.branchHead, strings.TrimSpace(head))
	branch, err := lifecycleMissingCheckoutGit(fixture.worktreePath, "branch", "--show-current")
	require.NoError(t, err)
	require.Equal(t, fixture.branch, strings.TrimSpace(branch))
}

type lifecycleMissingCheckoutRecoveryFixture struct {
	taskID         string
	sessionID      string
	environmentID  string
	worktreeID     string
	taskDirName    string
	repositoryID   string
	branchSlug     string
	branch         string
	repositoryPath string
	worktreePath   string
	branchHead     string
	tasksBasePath  string
	store          *worktree.SQLiteStore
	manager        *worktree.Manager
}

func newLifecycleMissingCheckoutRecoveryFixture(t *testing.T) *lifecycleMissingCheckoutRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &lifecycleMissingCheckoutRecoveryFixture{
		taskID: "task-lifecycle-missing-checkout", sessionID: "session-lifecycle-missing-checkout",
		environmentID: "environment-lifecycle-missing-checkout", worktreeID: "worktree-lifecycle-missing-checkout",
		taskDirName: "task-root-lifecycle-missing-checkout", repositoryID: "repository-lifecycle-missing-checkout",
		branchSlug: "main", branch: "feature/recovery",
	}

	dbConn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "recovery.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-lifecycle-recovery", Name: "Lifecycle recovery"}))
	fixture.repositoryPath = initLifecycleMissingCheckoutGitRepository(t)
	now := time.Now().UTC()
	require.NoError(t, taskRepo.CreateRepository(ctx, &models.Repository{
		ID: fixture.repositoryID, WorkspaceID: "workspace-lifecycle-recovery", Name: "repository",
		SourceType: "local", LocalPath: fixture.repositoryPath, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, taskRepo.CreateTask(ctx, &models.Task{
		ID: fixture.taskID, WorkspaceID: "workspace-lifecycle-recovery", Title: "Lifecycle recovery",
	}))
	fixture.tasksBasePath = filepath.Join(t.TempDir(), "tasks")
	cfg := worktree.Config{TasksBasePath: fixture.tasksBasePath}
	fixture.worktreePath, err = cfg.TaskWorktreePath(fixture.taskDirName, "repository", fixture.branchSlug)
	require.NoError(t, err)
	taskRoot := filepath.Dir(fixture.worktreePath)
	require.NoError(t, os.MkdirAll(taskRoot, 0o755))
	require.NoError(t, workspaces.WriteOwnershipMarker(taskRoot, workspaces.OwnershipMarker{
		TaskID: fixture.taskID, TaskDirName: fixture.taskDirName, LayoutVersion: workspaces.LayoutVersionSemantic,
	}))
	runLifecycleMissingCheckoutGit(t, fixture.repositoryPath, "branch", fixture.branch, "main")
	runLifecycleMissingCheckoutGit(t, fixture.repositoryPath, "worktree", "add", fixture.worktreePath, fixture.branch)
	head, err := lifecycleMissingCheckoutGit(fixture.repositoryPath, "rev-parse", "refs/heads/"+fixture.branch)
	require.NoError(t, err)
	fixture.branchHead = strings.TrimSpace(head)
	runLifecycleMissingCheckoutGit(t, fixture.repositoryPath, "worktree", "remove", "--force", fixture.worktreePath)
	runLifecycleMissingCheckoutGit(t, fixture.repositoryPath, "worktree", "prune")

	require.NoError(t, taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: fixture.environmentID, TaskID: fixture.taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: fixture.worktreePath, TaskDirName: fixture.taskDirName,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repository-lifecycle-recovery", TaskEnvironmentID: fixture.environmentID,
			RepositoryID: fixture.repositoryID, BranchSlug: fixture.branchSlug, WorktreeID: fixture.worktreeID,
			WorktreePath: fixture.worktreePath, WorktreeBranch: fixture.branch, Status: "active", Position: 0,
		}},
	}))
	require.NoError(t, taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: fixture.sessionID, TaskID: fixture.taskID, TaskEnvironmentID: fixture.environmentID,
		State: models.TaskSessionStateWaitingForInput,
	}))
	fixture.store, err = worktree.NewSQLiteStore(db, db)
	require.NoError(t, err)
	require.NoError(t, fixture.store.CreateWorktree(ctx, &worktree.Worktree{
		ID: fixture.worktreeID, SessionID: fixture.sessionID, TaskID: fixture.taskID, TaskDirName: fixture.taskDirName,
		TaskEnvironmentID: fixture.environmentID, RepositoryID: fixture.repositoryID, BranchSlug: fixture.branchSlug,
		RepositoryPath: fixture.repositoryPath, Path: fixture.worktreePath, Branch: fixture.branch,
		BaseBranch: "main", Status: worktree.StatusActive,
	}))
	fixture.manager, err = worktree.NewManager(cfg, fixture.store, logger.Default())
	require.NoError(t, err)
	return fixture
}

func initLifecycleMissingCheckoutGitRepository(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, os.MkdirAll(path, 0o755))
	runLifecycleMissingCheckoutGit(t, path, "init", "-b", "main")
	runLifecycleMissingCheckoutGit(t, path, "config", "user.email", "test@example.com")
	runLifecycleMissingCheckoutGit(t, path, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(path, "README.md"), []byte("main branch\n"), 0o644))
	runLifecycleMissingCheckoutGit(t, path, "add", "README.md")
	runLifecycleMissingCheckoutGit(t, path, "commit", "-m", "initial commit")
	return path
}

func runLifecycleMissingCheckoutGit(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	output, err := lifecycleMissingCheckoutGit(repoPath, args...)
	require.NoError(t, err, "git %s output: %s", strings.Join(args, " "), output)
}

func lifecycleMissingCheckoutGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
