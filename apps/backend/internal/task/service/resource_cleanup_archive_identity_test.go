package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type archiveManagerEnvironmentDestroyer struct {
	mgr *worktree.Manager
}

type policyRecordingWorktreeCleanup struct {
	worktrees       []*worktree.Worktree
	defaultCalls    int
	preservingCalls int
}

func (*policyRecordingWorktreeCleanup) OnTaskDeleted(context.Context, string) error {
	return nil
}

func (c *policyRecordingWorktreeCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return c.worktrees, nil
}

func (c *policyRecordingWorktreeCleanup) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	c.defaultCalls++
	return nil
}

func (c *policyRecordingWorktreeCleanup) CleanupWorktreesPreservingBranches(context.Context, []*worktree.Worktree) error {
	c.preservingCalls++
	return nil
}

func (*archiveManagerEnvironmentDestroyer) DestroyContainer(context.Context, string) error {
	return nil
}

func (*archiveManagerEnvironmentDestroyer) DestroySandbox(context.Context, string, string) error {
	return nil
}

func (d *archiveManagerEnvironmentDestroyer) DestroyWorktree(ctx context.Context, worktreeID string) error {
	return d.mgr.RemoveByID(ctx, worktreeID, false)
}

func (*archiveManagerEnvironmentDestroyer) PushEnvironmentBranch(context.Context, *models.TaskEnvironment) error {
	return nil
}

func (*archiveManagerEnvironmentDestroyer) GetContainerLiveStatus(context.Context, string) (*ContainerLiveStatus, error) {
	return nil, nil
}

// TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity covers
// AC-TASKS-RUNTIME-CLEANUP-001.6 and AC-TASKS-RUNTIME-CLEANUP-001.7.
func TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	const (
		taskID        = "task-archive-identity"
		sessionID     = "session-archive-identity"
		repositoryID  = "repo-archive-identity"
		environmentID = "env-archive-identity"
	)
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	sourcePath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: "ws-" + taskID, Name: repositoryID,
		SourceType: "local", LocalPath: sourcePath,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	mgr := newCleanupTestWorktreeManager(t, repo)
	wt, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Archive identity",
		RepositoryID: repositoryID, RepositoryPath: sourcePath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: repositoryID,
	})
	if err != nil {
		t.Fatalf("Create worktree: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(wt.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-archive-identity", RepositoryID: repositoryID,
			BranchSlug: wt.BranchSlug, WorktreeID: wt.ID, WorktreePath: wt.Path,
			WorktreeBranch: wt.Branch, Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.TaskEnvironmentID = environmentID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	svc.SetWorktreeCleanup(mgr)
	svc.SetEnvironmentDestroyer(&archiveManagerEnvironmentDestroyer{mgr: mgr})
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	var cleanupState models.TaskResourceCleanupState
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT state FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'archive'
		ORDER BY created_at DESC LIMIT 1
	`, taskID).Scan(&cleanupState); err != nil {
		t.Fatalf("load archive cleanup state: %v", err)
	}
	if cleanupState != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("cleanup state = %q, want %q", cleanupState, models.TaskResourceCleanupStateSucceeded)
	}

	env, err := repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment after archive: %v", err)
	}
	if env.TaskID != taskID || len(env.Repos) != 1 {
		t.Fatalf("environment after archive = %+v, want retained owner and repository row", env)
	}
	gotRepo := env.Repos[0]
	if gotRepo.WorktreeID != wt.ID || gotRepo.WorktreePath != wt.Path ||
		gotRepo.WorktreeBranch != wt.Branch || gotRepo.BranchSlug != wt.BranchSlug {
		t.Fatalf("repository identity after archive = %+v, want worktree %+v", gotRepo, wt)
	}
	if gotRepo.DeletedAt == nil {
		t.Fatal("repository row remains active after physical archive cleanup")
	}
}

// TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch covers
// AC-TASKS-RUNTIME-CLEANUP-001.7 at the service, SQLite, and Git boundaries.
func TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	const (
		taskID        = "task-archive-resume"
		sessionID     = "session-archive-resume"
		repositoryID  = "repo-archive-resume"
		environmentID = "env-archive-resume"
	)
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	sourcePath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: "ws-" + taskID, Name: repositoryID,
		SourceType: "local", LocalPath: sourcePath,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	mgr := newCleanupTestWorktreeManager(t, repo)
	request := worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Archive resume",
		RepositoryID: repositoryID, RepositoryPath: sourcePath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: repositoryID,
	}
	wt, err := mgr.Create(ctx, request)
	if err != nil {
		t.Fatalf("Create worktree: %v", err)
	}
	runGitTestCmd(t, wt.Path, "commit", "--allow-empty", "-m", "local-only archive work")
	wantHead := strings.TrimSpace(string(runGitTestCmd(t, wt.Path, "rev-parse", "HEAD")))

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(wt.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-archive-resume", RepositoryID: repositoryID,
			BranchSlug: wt.BranchSlug, WorktreeID: wt.ID, WorktreePath: wt.Path,
			WorktreeBranch: wt.Branch, Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.TaskEnvironmentID = environmentID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	svc.SetWorktreeCleanup(mgr)
	svc.SetEnvironmentDestroyer(&archiveManagerEnvironmentDestroyer{mgr: mgr})
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	if got := strings.TrimSpace(string(runGitTestCmd(t, sourcePath, "branch", "--list", wt.Branch))); got == "" {
		t.Fatalf("archive cleanup deleted local-only branch %q", wt.Branch)
	}
	if unarchived, err := repo.UnarchiveTask(ctx, taskID); err != nil || !unarchived {
		t.Fatalf("UnarchiveTask = %t, %v", unarchived, err)
	}

	request.WorktreeID = wt.ID
	restored, err := mgr.Create(ctx, request)
	if err != nil {
		t.Fatalf("resume worktree after unarchive: %v", err)
	}
	if restored.ID != wt.ID || restored.Path != wt.Path {
		t.Fatalf("restored worktree = %+v, want ID %q and path %q", restored, wt.ID, wt.Path)
	}
	if got := strings.TrimSpace(string(runGitTestCmd(t, restored.Path, "rev-parse", "HEAD"))); got != wantHead {
		t.Fatalf("restored HEAD = %q, want %q", got, wantHead)
	}
	env, err := repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment after resume: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != wt.ID || env.Repos[0].DeletedAt != nil {
		t.Fatalf("environment repository after resume = %+v, want reactivated worktree %q", env.Repos, wt.ID)
	}
}

func TestCleanupTaskResources_CascadeArchivePreservesBranches(t *testing.T) {
	svc, _, repo := createTestService(t)
	const taskID = "task-cascade-archive-policy"
	seedCleanupTaskAndSession(t, repo, taskID, "session-cascade-archive-policy")
	cleanup := &policyRecordingWorktreeCleanup{worktrees: []*worktree.Worktree{{
		ID: "worktree-cascade-archive-policy", TaskID: taskID,
	}}}
	svc.SetWorktreeCleanup(cleanup)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	svc.CleanupTaskResources(context.Background(), taskID, false)
	waitForCleanupDone(t, svc)

	if cleanup.preservingCalls != 1 || cleanup.defaultCalls != 0 {
		t.Fatalf("cleanup calls = preserving %d, default %d; want preserving 1, default 0",
			cleanup.preservingCalls, cleanup.defaultCalls)
	}
}

// This contract test covers the retry-safe fallback after the preserving
// manager path is in place: archive cleanup must never call a branch-deleting
// compatibility method.
func TestArchiveCleanupFailsClosedWithoutBranchPreservingCleaner(t *testing.T) {
	svc, _, _ := createTestService(t)
	cleanup := &recordingWorktreeCleanup{}
	svc.SetWorktreeCleanup(cleanup)

	errs := svc.cleanupDestructiveTaskResources(
		context.Background(), "task-archive-fail-closed", nil,
		[]*worktree.Worktree{{ID: "worktree-archive-fail-closed", TaskID: "task-archive-fail-closed"}},
		taskEnvironmentCleanup{preserveBranches: true}, nil,
	)

	joined := errors.Join(errs...)
	if joined == nil || !strings.Contains(joined.Error(), "cannot preserve branches") {
		t.Fatalf("archive cleanup error = %v, want branch-preservation failure", joined)
	}
	if got := cleanup.cleanedIDs(); len(got) != 0 {
		t.Fatalf("branch-deleting cleanup calls = %v, want none", got)
	}
}
