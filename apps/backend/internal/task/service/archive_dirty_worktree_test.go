package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001

// setUpDirtyArchiveWorktree wires a real worktree manager, repository, task
// environment and session so an archive-triggered cleanup exercises the same
// code path production traffic takes.
func setUpDirtyArchiveWorktree(t *testing.T, taskID, sessionID, repositoryID, environmentID string) (*Service, *worktree.Worktree, string) {
	t.Helper()
	ctx := context.Background()
	svc, _, repo := createTestService(t)
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
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Archive dirty worktree",
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
			ID: "env-repo-" + taskID, RepositoryID: repositoryID,
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
	return svc, wt, sourcePath
}

// TestArchiveTaskPreservesDirtyWorktreeUntrackedFile covers
// AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.1: Service.ArchiveTask must not destroy
// an untracked file left in a task's worktree.
func TestArchiveTaskPreservesDirtyWorktreeUntrackedFile(t *testing.T) {
	const (
		taskID        = "task-archive-dirty-untracked"
		sessionID     = "session-archive-dirty-untracked"
		repositoryID  = "repo-archive-dirty-untracked"
		environmentID = "env-archive-dirty-untracked"
	)
	ctx := context.Background()
	svc, wt, sourcePath := setUpDirtyArchiveWorktree(t, taskID, sessionID, repositoryID, environmentID)

	sentinel := filepath.Join(wt.Path, "untracked-archive-preserved.txt")
	if err := os.WriteFile(sentinel, []byte("preserve me\n"), 0o644); err != nil {
		t.Fatalf("write untracked work: %v", err)
	}

	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertDirtyArchiveWorktreePreserved(t, svc, taskID, environmentID, wt, sourcePath, sentinel, "preserve me\n")
}

// TestArchiveTaskPreservesDirtyWorktreeTrackedModification covers
// AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.2: Service.ArchiveTask must not discard
// an uncommitted modification to a tracked file.
func TestArchiveTaskPreservesDirtyWorktreeTrackedModification(t *testing.T) {
	const (
		taskID        = "task-archive-dirty-tracked"
		sessionID     = "session-archive-dirty-tracked"
		repositoryID  = "repo-archive-dirty-tracked"
		environmentID = "env-archive-dirty-tracked"
	)
	ctx := context.Background()
	svc, wt, sourcePath := setUpDirtyArchiveWorktree(t, taskID, sessionID, repositoryID, environmentID)

	tracked := filepath.Join(wt.Path, "README.md")
	if err := os.WriteFile(tracked, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("write tracked modification: %v", err)
	}

	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	assertDirtyArchiveWorktreePreserved(t, svc, taskID, environmentID, wt, sourcePath, tracked, "modified\n")
}

func assertDirtyArchiveWorktreePreserved(
	t *testing.T, svc *Service, taskID, environmentID string,
	wt *worktree.Worktree, sourcePath, sentinelPath, wantContents string,
) {
	t.Helper()
	if got, err := os.ReadFile(sentinelPath); err != nil || string(got) != wantContents {
		t.Fatalf("dirty worktree destroyed: contents=%q err=%v", got, err)
	}
	if got := strings.TrimSpace(string(runGitTestCmd(t, sourcePath, "branch", "--list", wt.Branch))); got == "" {
		t.Fatalf("dirty archive deleted branch %q", wt.Branch)
	}
	env, err := svc.taskEnvironments.GetTaskEnvironment(context.Background(), environmentID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment after dirty archive: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].DeletedAt != nil || env.Repos[0].WorktreeID != wt.ID {
		t.Fatalf("dirty worktree env repo after archive = %+v, want retained active record", env.Repos)
	}
	task, err := svc.tasks.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask after dirty archive: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("dirty worktree blocked task archive; want archive to always succeed")
	}
}

// TestArchiveTaskStillRemovesCleanWorktree is the control case: a clean
// worktree must still be force-removed on archive exactly as before.
func TestArchiveTaskStillRemovesCleanWorktree(t *testing.T) {
	const (
		taskID        = "task-archive-clean-control"
		sessionID     = "session-archive-clean-control"
		repositoryID  = "repo-archive-clean-control"
		environmentID = "env-archive-clean-control"
	)
	ctx := context.Background()
	svc, wt, _ := setUpDirtyArchiveWorktree(t, taskID, sessionID, repositoryID, environmentID)

	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("clean worktree directory stat = %v, want removed", err)
	}
	env, err := svc.taskEnvironments.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment after clean archive: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].DeletedAt == nil {
		t.Fatalf("clean worktree env repo after archive = %+v, want tombstoned", env.Repos)
	}
}

// TestCleanupTaskResourcesCascadePreservesDirtyWorktree covers the cascade
// entry point (handoff_cascade.go -> CleanupTaskResources) rather than the
// single-task ArchiveTask call.
func TestCleanupTaskResourcesCascadePreservesDirtyWorktree(t *testing.T) {
	const (
		taskID        = "task-cascade-archive-dirty"
		sessionID     = "session-cascade-archive-dirty"
		repositoryID  = "repo-cascade-archive-dirty"
		environmentID = "env-cascade-archive-dirty"
	)
	ctx := context.Background()
	svc, wt, _ := setUpDirtyArchiveWorktree(t, taskID, sessionID, repositoryID, environmentID)

	sentinel := filepath.Join(wt.Path, "untracked-cascade-preserved.txt")
	if err := os.WriteFile(sentinel, []byte("preserve me\n"), 0o644); err != nil {
		t.Fatalf("write untracked work: %v", err)
	}

	svc.CleanupTaskResources(ctx, taskID, false)
	waitForCleanupDone(t, svc)

	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("cascade archive removed dirty worktree directory: %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve me\n" {
		t.Fatalf("cascade archive destroyed dirty work: contents=%q err=%v", got, err)
	}
}

// failingDirtyInspectionCleanup implements WorktreeArchiveBatchCleaner and
// WorktreeDirtyInspector but always fails inspection, so the admission filter
// must fail closed rather than fall through to a force-remove.
type failingDirtyInspectionCleanup struct {
	preservingCalls int
	inspectErr      error
}

func (*failingDirtyInspectionCleanup) OnTaskDeleted(context.Context, string) error { return nil }

func (*failingDirtyInspectionCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (*failingDirtyInspectionCleanup) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return nil
}

func (c *failingDirtyInspectionCleanup) CleanupWorktreesPreservingBranches(context.Context, []*worktree.Worktree) error {
	c.preservingCalls++
	return nil
}

func (c *failingDirtyInspectionCleanup) InspectDirtyWorktrees(
	context.Context, []*worktree.Worktree,
) ([]worktree.DirtyWorktree, error) {
	return nil, c.inspectErr
}

// TestCleanupDestructiveTaskResourcesArchiveInspectionFailurePreservesWholeSet
// covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.5: a dirty-inspection failure
// must preserve every worktree in the batch, never fall through to removal.
func TestCleanupDestructiveTaskResourcesArchiveInspectionFailurePreservesWholeSet(t *testing.T) {
	svc, _, _ := createTestService(t)
	cleanup := &failingDirtyInspectionCleanup{inspectErr: errors.New("git status boom")}
	svc.SetWorktreeCleanup(cleanup)

	errs := svc.cleanupDestructiveTaskResources(
		context.Background(), "task-archive-inspect-fail", nil,
		[]*worktree.Worktree{{ID: "worktree-archive-inspect-fail", TaskID: "task-archive-inspect-fail"}},
		taskEnvironmentCleanup{preserveBranches: true}, nil,
	)

	joined := errors.Join(errs...)
	if joined == nil || !strings.Contains(joined.Error(), "inspect worktrees before archive cleanup") {
		t.Fatalf("archive cleanup error = %v, want inspection failure", joined)
	}
	if cleanup.preservingCalls != 0 {
		t.Fatalf("preserving cleanup calls = %d, want 0 after inspection failure", cleanup.preservingCalls)
	}
}

// recordingDirtyAwareArchiveCleanup implements WorktreeArchiveBatchCleaner and
// WorktreeDirtyInspector, recording exactly which worktrees reach the batch
// cleaner so per-worktree filtering (not all-or-nothing) can be asserted.
type recordingDirtyAwareArchiveCleanup struct {
	dirty          []worktree.DirtyWorktree
	preservedCalls [][]*worktree.Worktree
}

func (*recordingDirtyAwareArchiveCleanup) OnTaskDeleted(context.Context, string) error { return nil }

func (*recordingDirtyAwareArchiveCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (*recordingDirtyAwareArchiveCleanup) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return nil
}

func (c *recordingDirtyAwareArchiveCleanup) CleanupWorktreesPreservingBranches(
	_ context.Context, worktrees []*worktree.Worktree,
) error {
	c.preservedCalls = append(c.preservedCalls, worktrees)
	return nil
}

func (c *recordingDirtyAwareArchiveCleanup) InspectDirtyWorktrees(
	_ context.Context, _ []*worktree.Worktree,
) ([]worktree.DirtyWorktree, error) {
	return c.dirty, nil
}

// TestCleanupDestructiveTaskResourcesArchiveFiltersDirtyPerWorktree covers
// AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.6: in a multi-repo task, a dirty
// worktree must be dropped while its clean sibling is still reclaimed.
func TestCleanupDestructiveTaskResourcesArchiveFiltersDirtyPerWorktree(t *testing.T) {
	svc, _, _ := createTestService(t)
	cleanup := &recordingDirtyAwareArchiveCleanup{
		dirty: []worktree.DirtyWorktree{{WorktreeID: "worktree-multi-dirty"}},
	}
	svc.SetWorktreeCleanup(cleanup)

	errs := svc.cleanupDestructiveTaskResources(
		context.Background(), "task-archive-multi-repo", nil,
		[]*worktree.Worktree{
			{ID: "worktree-multi-dirty", TaskID: "task-archive-multi-repo"},
			{ID: "worktree-multi-clean", TaskID: "task-archive-multi-repo"},
		},
		taskEnvironmentCleanup{preserveBranches: true}, nil,
	)
	if len(errs) != 0 {
		t.Fatalf("cleanup errors = %v, want none", errs)
	}
	if len(cleanup.preservedCalls) != 1 {
		t.Fatalf("preserving cleanup calls = %d, want 1", len(cleanup.preservedCalls))
	}
	got := cleanup.preservedCalls[0]
	if len(got) != 1 || got[0].ID != "worktree-multi-clean" {
		t.Fatalf("cleaned worktrees = %+v, want only the clean sibling", got)
	}
}
