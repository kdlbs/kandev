package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type borrowingReclaimCleaner struct {
	*worktree.Manager
	repo          *sqliterepo.Repository
	environmentID string
}

func (c *borrowingReclaimCleaner) CleanupArchivedWorktree(ctx context.Context, wt *worktree.Worktree, taskID, path, repositoryPath string) error {
	if err := c.Manager.CleanupArchivedWorktree(ctx, wt, taskID, path, repositoryPath); err != nil {
		return err
	}
	return c.repo.CreateTaskSession(ctx, &models.TaskSession{ID: "late-borrower-session", TaskID: "late-borrower-task", TaskEnvironmentID: c.environmentID, State: models.TaskSessionStateRunning})
}

func TestArchiveTaskDirectoryDeferredReclaimSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, manager, _, wt := createArchiveReclaimFixture(t)
	root := filepath.Dir(wt.Path)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("task output"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty := filepath.Join(wt.Path, "untracked.txt")
	if err := os.WriteFile(dirty, []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, wt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "late-borrower-task", WorkspaceID: "ws-" + wt.TaskID, WorkflowID: "wf-" + wt.TaskID, Title: "Late borrower"}); err != nil {
		t.Fatal(err)
	}
	svc.SetWorktreeCleanup(&borrowingReclaimCleaner{Manager: manager, repo: repo, environmentID: env.ID})
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("in-use directory obligation completed without retry")
	}
	job = latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if job.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("deferred state = %s", job.State)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("checkout still exists: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("borrowed root removed: %v", err)
	}
	borrower, err := repo.GetTaskSession(ctx, "late-borrower-session")
	if err != nil {
		t.Fatal(err)
	}
	borrower.State = models.TaskSessionStateCompleted
	if err := repo.UpdateTaskSession(ctx, borrower); err != nil {
		t.Fatal(err)
	}
	restarted := newServiceOverRepository(t, repo)
	restarted.StopTaskResourceCleanupWorker()
	restarted.SetWorktreeCleanup(manager)
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := restarted.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("retry left root: %v", err)
	}
}

func TestArchiveTaskDirectoryMissingReclaimRowKeepsRetry(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, manager, _, wt := createArchiveReclaimFixture(t)
	root := filepath.Dir(wt.Path)
	dirty := filepath.Join(wt.Path, "untracked.txt")
	for _, path := range []string{dirty, filepath.Join(root, "notes.txt")} {
		if err := os.WriteFile(path, []byte("task output"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
	svc.SetWorktreeCleanup(&failingTaskDirectoryCleaner{Manager: manager})
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("expected removal failure")
	}
	if _, err := repo.DB().ExecContext(ctx, "DELETE FROM task_environment_repos WHERE worktree_id = ?", wt.ID); err != nil {
		t.Fatal(err)
	}
	svc.SetWorktreeCleanup(manager)
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("missing identity silently completed pending root obligation")
	}
	job = latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if job.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("unknown row state = %s", job.State)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("unknown identity removed root: %v", err)
	}
}
