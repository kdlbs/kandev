package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

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

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.2
func TestCleanupTaskResourcesCascadeRemovesDirtyWorktree(t *testing.T) {
	const taskID = "task-cascade-archive-dirty"
	svc, wt, _ := setUpDirtyArchiveWorktree(t, taskID, "session-"+taskID, "repo-"+taskID, "env-"+taskID)
	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("local work"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.CleanupTaskResources(t.Context(), taskID, false)
	waitForCleanupDone(t, svc)
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("cascade left dirty worktree: %v", err)
	}
}
