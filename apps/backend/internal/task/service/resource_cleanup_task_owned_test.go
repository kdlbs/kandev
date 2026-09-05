package service

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type managerEnvironmentDestroyer struct {
	mgr *worktree.Manager
}

func (d *managerEnvironmentDestroyer) DestroyContainer(context.Context, string) error {
	return nil
}

func (d *managerEnvironmentDestroyer) DestroySandbox(context.Context, string, string) error {
	return nil
}

func (d *managerEnvironmentDestroyer) DestroyWorktree(ctx context.Context, worktreeID string) error {
	return d.mgr.RemoveByID(ctx, worktreeID, true)
}

func (d *managerEnvironmentDestroyer) PushEnvironmentBranch(context.Context, *models.TaskEnvironment) error {
	return nil
}

func (d *managerEnvironmentDestroyer) GetContainerLiveStatus(context.Context, string) (*ContainerLiveStatus, error) {
	return nil, nil
}

// TestDeleteTaskCleanupFindsWorktreeAfterLastSessionDeletedAndRestart is the
// core task-owned cleanup contract: after the task's only session is deleted
// and the backend restarts (a fresh service over the same database), delete
// inventory must still discover the task-owned worktree through the
// environment, and the durable snapshot must carry it after the task row is
// gone.
func TestDeleteTaskCleanupFindsWorktreeAfterLastSessionDeletedAndRestart(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestService(t)
	seedCleanupTaskAndSession(t, repo, "task-zero-session", "session-zero-session")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-zero-session", TaskID: "task-zero-session", ExecutorType: "worktree",
		WorkspacePath: "/tmp/zero", Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-zero-session", RepositoryID: "repo-zero-session",
			WorktreeID: "wt-zero-session", WorktreePath: "/tmp/zero/repo",
			WorktreeBranch: "feature/zero", Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-zero-session")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.TaskEnvironmentID = "env-zero-session"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link session: %v", err)
	}

	// Delete the only session through the orchestrator-style deletion.
	if err := repo.DeleteTaskSession(ctx, "session-zero-session"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	// Restart: a fresh service over the same database, wired to a real
	// worktree manager backed by the same schema.
	svc2 := newServiceOverRepository(t, repo)
	mgr := newCleanupTestWorktreeManager(t, repo)
	svc2.SetWorktreeCleanup(mgr)
	worktrees, err := svc2.gatherWorktreesForDelete(ctx, "task-zero-session")
	if err != nil {
		t.Fatalf("gather worktrees: %v", err)
	}
	if len(worktrees) != 1 || worktrees[0].ID != "wt-zero-session" {
		t.Fatalf("zero-session inventory = %+v, want wt-zero-session", worktrees)
	}

	if err := svc2.DeleteTask(ctx, "task-zero-session"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	var encoded string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT resource_snapshot FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
	`, "task-zero-session").Scan(&encoded); err != nil {
		t.Fatalf("load delete snapshot: %v", err)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.Worktrees) != 1 || snapshot.Worktrees[0].ID != "wt-zero-session" {
		t.Fatalf("snapshot worktrees = %+v, want wt-zero-session after task-row deletion", snapshot.Worktrees)
	}
}

// TestDeleteTaskCleanupRemovesEveryWorktreeAfterLastSessionDeletedAndRestart
// covers the complete task-owned lifecycle for a multi-repository task. The
// session rows disappear first, then a fresh service inventories both durable
// environment-repository rows, persists them in the delete snapshot, and
// finally removes both worktrees through the normal batch cleanup path.
func TestDeleteTaskCleanupRemovesEveryWorktreeAfterLastSessionDeletedAndRestart(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestService(t)
	taskID := "task-multi-repo-cleanup"
	sessionID := "session-multi-repo-cleanup"
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	repoAPath := initSimpleGitRepo(t)
	repoBPath := initSimpleGitRepo(t)
	workspaceID := "ws-" + taskID
	for id, path := range map[string]string{"repo-a": repoAPath, "repo-b": repoBPath} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: workspaceID, Name: id, SourceType: "local", LocalPath: path,
		}); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
	}

	mgr := newCleanupTestWorktreeManager(t, repo)
	primary, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Multi repo cleanup",
		RepositoryID: "repo-a", RepositoryPath: repoAPath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-a",
	})
	if err != nil {
		t.Fatalf("create primary worktree: %v", err)
	}
	secondary, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Multi repo cleanup",
		RepositoryID: "repo-b", RepositoryPath: repoBPath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-b",
	})
	if err != nil {
		t.Fatalf("create secondary worktree: %v", err)
	}

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(primary.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{ID: "env-repo-a", RepositoryID: "repo-a", WorktreeID: primary.ID, WorktreePath: primary.Path, WorktreeBranch: primary.Branch, Status: "active"},
			{ID: "env-repo-b", RepositoryID: "repo-b", WorktreeID: secondary.ID, WorktreePath: secondary.Path, WorktreeBranch: secondary.Branch, Status: "active"},
		},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get task session: %v", err)
	}
	session.TaskEnvironmentID = "env-" + taskID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link task session: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, sessionID); err != nil {
		t.Fatalf("delete task session: %v", err)
	}

	// Use fresh service and manager instances over the same database to model a
	// backend restart after the last session has disappeared.
	svc := newServiceOverRepository(t, repo)
	cleanupMgr := newCleanupTestWorktreeManager(t, repo)
	svc.SetWorktreeCleanup(cleanupMgr)
	svc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: cleanupMgr})
	worktrees, err := svc.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		t.Fatalf("gather worktrees: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("zero-session inventory = %+v, want both repository worktrees", worktrees)
	}

	if err := svc.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var encodedSnapshot string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT resource_snapshot FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
		ORDER BY created_at DESC LIMIT 1
	`, taskID).Scan(&encodedSnapshot); err != nil {
		t.Fatalf("load delete cleanup snapshot: %v", err)
	}
	var cleanupSnapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(encodedSnapshot), &cleanupSnapshot); err != nil {
		t.Fatalf("decode delete cleanup snapshot: %v", err)
	}
	if len(cleanupSnapshot.WorktreeHeadOIDs) != 2 {
		t.Fatalf("snapshot worktree identities = %#v, want both repository commits", cleanupSnapshot.WorktreeHeadOIDs)
	}
	var jobID string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT id FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
		ORDER BY created_at DESC LIMIT 1
	`, taskID).Scan(&jobID); err != nil {
		t.Fatalf("load delete cleanup job: %v", err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, jobID); err != nil {
		t.Fatalf("process delete cleanup job: %v", err)
	}

	for name, path := range map[string]string{"primary": primary.Path, "secondary": secondary.Path} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("%s worktree directory still on disk: %s (stat err = %v)", name, path, statErr)
		}
	}
	assertNoWorktreeRegistration(t, repoAPath, primary.Path)
	assertNoWorktreeRegistration(t, repoBPath, secondary.Path)
}

// TestArchiveCleanupFindsWorktreeAfterLastSessionDeleted proves the archive
// path inventories the task-owned worktree with zero sessions and schedules
// its asynchronous removal.
func TestArchiveCleanupFindsWorktreeAfterLastSessionDeleted(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedCleanupTaskAndSession(t, repo, "task-archive-zero", "session-archive-zero")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-archive-zero", TaskID: "task-archive-zero", ExecutorType: "worktree",
		WorkspacePath: "/tmp/archive-zero", Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-archive-zero", RepositoryID: "repo-archive-zero",
			WorktreeID: "wt-archive-zero", WorktreePath: "/tmp/archive-zero/repo",
			WorktreeBranch: "feature/archive-zero", Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-archive-zero")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.TaskEnvironmentID = "env-archive-zero"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link session: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-archive-zero"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := svc.ArchiveTask(ctx, "task-archive-zero"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	// The env-repo row remains (physical removal is async via the manager;
	// the archive marks the task archived and schedules cleanup).
	task, err := repo.GetTask(ctx, "task-archive-zero")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("task must be archived")
	}
	env, err := repo.GetTaskEnvironment(ctx, "env-archive-zero")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if env == nil || len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-archive-zero" {
		t.Fatalf("env after archive = %+v, want retained worktree row", env)
	}
}

// newCleanupTestWorktreeManager wires a real worktree manager over the test
// repository's database so cleanup inventory reads the actual
// task_environment_repos rows.
func newCleanupTestWorktreeManager(t *testing.T, repo *sqliterepo.Repository) *worktree.Manager {
	t.Helper()
	store, err := worktree.NewSQLiteStore(sqlx.NewDb(repo.DB(), "sqlite3"), sqlx.NewDb(repo.DB(), "sqlite3"))
	if err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	mgr, err := worktree.NewManager(worktree.Config{
		Enabled:       true,
		TasksBasePath: t.TempDir(),
	}, store, log)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	return mgr
}

// newServiceOverRepository builds a fresh task service over an existing test
// repository, simulating a backend restart on the same database.
func newServiceOverRepository(t *testing.T, repo *sqliterepo.Repository) *Service {
	t.Helper()
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(Repos{
		Workspaces:        repo,
		Tasks:             repo,
		TaskRepos:         repo,
		Workflows:         repo,
		Messages:          repo,
		Turns:             repo,
		Sessions:          repo,
		GitSnapshots:      repo,
		RepoEntities:      repo,
		RepositoryCleanup: repo,
		Executors:         repo,
		Environments:      repo,
		TaskEnvironments:  repo,
		Reviews:           repo,
		ResourceCleanups:  repo,
	}, eventBus, log, RepositoryDiscoveryConfig{})
	svc.SetWorkspaceBootstrapper(repo)
	svc.SetWorkflowStepGetter(&testWorkflowStepGetter{repo: repo})
	return svc
}

func initSimpleGitRepo(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	runGitTestCmd(t, repoPath, "init", "-b", "main")
	runGitTestCmd(t, repoPath, "config", "user.email", "test@example.com")
	runGitTestCmd(t, repoPath, "config", "user.name", "Test User")
	runGitTestCmd(t, repoPath, "config", "commit.gpgsign", "false")
	readme := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readme, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGitTestCmd(t, repoPath, "add", "README.md")
	runGitTestCmd(t, repoPath, "commit", "-m", "initial commit")
	return repoPath
}

func runGitTestCmd(t *testing.T, repoPath string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func assertNoWorktreeRegistration(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	out := string(runGitTestCmd(t, repoPath, "worktree", "list", "--porcelain"))
	if strings.Contains(out, worktreePath) {
		t.Errorf("stale worktree registration for %s remains in source repo %s:\n%s", worktreePath, repoPath, out)
	}
}
