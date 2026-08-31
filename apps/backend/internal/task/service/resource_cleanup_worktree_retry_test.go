package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestDeleteTaskCleanupRetriesWhenAuditedWorktreeRemovalIsUnsafe(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestService(t)
	const taskID = "task-audited-cleanup-retry"
	const sessionID = "session-audited-cleanup-retry"
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	repoPath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-audited-cleanup", WorkspaceID: "ws-" + taskID,
		Name: "repo-audited-cleanup", SourceType: "local", LocalPath: repoPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	mgr := newCleanupTestWorktreeManager(t, repo)
	wt, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Audited cleanup retry",
		RepositoryID: "repo-audited-cleanup", RepositoryPath: repoPath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-audited-cleanup",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(wt.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-audited-cleanup", RepositoryID: "repo-audited-cleanup",
			WorktreeID: wt.ID, WorktreePath: wt.Path, WorktreeBranch: wt.Branch, Status: "active",
		}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get task session: %v", err)
	}
	session.TaskEnvironmentID = "env-" + taskID
	session.BaseBranch = "main"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link task session: %v", err)
	}
	sentinel := filepath.Join(wt.Path, "untracked-preserved.txt")
	if err := os.WriteFile(sentinel, []byte("preserve me\n"), 0o644); err != nil {
		t.Fatalf("write untracked work: %v", err)
	}

	svc := newServiceOverRepository(t, repo)
	svc.SetWorktreeCleanup(mgr)
	if err := svc.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	var jobID string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT id FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
		ORDER BY created_at DESC LIMIT 1
	`, taskID).Scan(&jobID); err != nil {
		t.Fatalf("load delete cleanup job: %v", err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, jobID); err == nil {
		t.Fatal("unsafe worktree cleanup returned nil")
	}

	job, err := repo.GetTaskResourceCleanupJob(ctx, jobID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateRetryWait || job.NextAttemptAt == nil {
		t.Fatalf("cleanup job state = %q next=%v, want retry_wait with next attempt", job.State, job.NextAttemptAt)
	}
	if !strings.Contains(job.LastError, "uncommitted or untracked work") {
		t.Fatalf("cleanup last error = %q, want audited-work refusal", job.LastError)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve me\n" {
		t.Fatalf("preserved work changed: contents=%q err=%v", got, err)
	}
}
