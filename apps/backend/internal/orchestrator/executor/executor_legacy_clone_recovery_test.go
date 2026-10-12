package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.5
func TestPreflightSessionWorktreeRecoveryReusesRegisteredLegacyClone(t *testing.T) {
	ctx := context.Background()
	dbConn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "legacy.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	root := filepath.Join(t.TempDir(), "repos")
	source := filepath.Join(root, "acme", "widget")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	seed := initExecutorRecoveryGitRepository(t)
	runExecutorRecoveryGit(t, seed, "clone", "--no-hardlinks", seed, source)
	runExecutorRecoveryGit(t, source, "remote", "add", "upstream-local", seed)
	runExecutorRecoveryGit(t, source, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	taskRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cfg := worktree.Config{TasksBasePath: filepath.Join(taskRoot, "tasks")}
	checkout := filepath.Join(cfg.TasksBasePath, "legacy-task", "widget")
	require.NoError(t, os.MkdirAll(filepath.Dir(checkout), 0o755))
	runExecutorRecoveryGit(t, source, "worktree", "add", "-b", "feature/legacy", checkout)
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "untracked.txt"), []byte("keep me\n"), 0o600))
	wt := &worktree.Worktree{ID: "wt-legacy", TaskID: "task-legacy", SessionID: "session-legacy", TaskEnvironmentID: "env-legacy", RepositoryID: "repo-legacy", BranchSlug: "main", Path: checkout, RepositoryPath: source, Branch: "feature/legacy", Status: worktree.StatusActive}
	session := seedLegacyCloneRecoveryRows(t, repo, wt)
	store, err := worktree.NewSQLiteStore(db, db)
	require.NoError(t, err)
	require.NoError(t, store.CreateWorktree(ctx, wt))
	manager, err := worktree.NewManager(cfg, store, logger.Default())
	require.NoError(t, err)
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	executor.SetRepoCloner(repoclone.NewCloner(repoclone.Config{BasePath: root}, repoclone.ProtocolHTTPS, "", nil), nil)
	executor.SetSelectedWorktreeRecoveryAdmission(manager.AdmitRecovery)
	before, err := repo.GetTaskEnvironment(ctx, wt.TaskEnvironmentID)
	require.NoError(t, err)
	head := strings.TrimSpace(runExecutorRecoveryGit(t, checkout, "rev-parse", "HEAD"))
	admission, err := executor.PreflightSessionWorktreeRecovery(ctx, wt.TaskID, session, false)
	require.NoError(t, err)
	require.Nil(t, admission)
	after, err := repo.GetTaskEnvironment(ctx, wt.TaskEnvironmentID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	persisted, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, session.Metadata, persisted.Metadata)
	require.Equal(t, session.State, persisted.State)
	require.Equal(t, head, strings.TrimSpace(runExecutorRecoveryGit(t, checkout, "rev-parse", "HEAD")))
	data, err := os.ReadFile(filepath.Join(checkout, "untracked.txt"))
	require.NoError(t, err)
	require.Equal(t, "keep me\n", string(data))
	require.NoDirExists(t, filepath.Join(root, "workspaces"))
}

func seedLegacyCloneRecoveryRows(t *testing.T, repo *tasksqlite.Repository, wt *worktree.Worktree) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-legacy", Name: "Legacy"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: wt.RepositoryID, WorkspaceID: "workspace-legacy", Name: "widget", SourceType: "provider", LocalPath: wt.RepositoryPath, Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: wt.TaskID, WorkspaceID: "workspace-legacy", Title: "Legacy task"}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-legacy", TaskID: wt.TaskID, RepositoryID: wt.RepositoryID, BaseBranch: "main"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: wt.TaskEnvironmentID, TaskID: wt.TaskID, OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree, Status: models.TaskEnvironmentStatusReady, WorkspacePath: wt.Path, Repos: []*models.TaskEnvironmentRepo{{ID: "env-repo-legacy", TaskEnvironmentID: wt.TaskEnvironmentID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug, WorktreeID: wt.ID, WorktreePath: wt.Path, WorktreeBranch: wt.Branch, Status: "active"}}}))
	session := &models.TaskSession{ID: wt.SessionID, TaskID: wt.TaskID, TaskEnvironmentID: wt.TaskEnvironmentID, State: models.TaskSessionStateCancelled, Metadata: map[string]interface{}{"resume_token": "provider-conversation"}}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	return session
}
