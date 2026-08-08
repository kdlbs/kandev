package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

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
