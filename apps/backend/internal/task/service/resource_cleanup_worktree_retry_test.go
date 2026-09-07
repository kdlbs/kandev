package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-RUNTIME-CLEANUP-001.10
// @covers AC-TASKS-RUNTIME-CLEANUP-001.11
func TestDeleteTaskRejectsDirtyWorktreeBeforeMutation(t *testing.T) {
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
	if err := svc.DeleteTask(ctx, taskID); err == nil {
		t.Fatal("DeleteTask succeeded without discard consent")
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("dirty delete removed task: %v", err)
	}
	if task == nil {
		t.Fatal("dirty delete removed task")
	}
	var cleanupJobs int
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
	`, taskID).Scan(&cleanupJobs); err != nil {
		t.Fatalf("count delete cleanup jobs: %v", err)
	}
	if cleanupJobs != 0 {
		t.Fatalf("dirty delete created %d cleanup jobs, want none", cleanupJobs)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve me\n" {
		t.Fatalf("preserved work changed: contents=%q err=%v", got, err)
	}
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.12
func TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestService(t)
	const taskID = "task-audited-cleanup-discard"
	const sessionID = "session-audited-cleanup-discard"
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	repoPath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-audited-cleanup-discard", WorkspaceID: "ws-" + taskID,
		Name: "repo-audited-cleanup-discard", SourceType: "local", LocalPath: repoPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	mgr := newCleanupTestWorktreeManager(t, repo)
	wt, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Audited cleanup discard",
		RepositoryID: "repo-audited-cleanup-discard", RepositoryPath: repoPath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-audited-cleanup-discard",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(wt.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-audited-cleanup-discard", RepositoryID: "repo-audited-cleanup-discard",
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
	sentinel := filepath.Join(wt.Path, "untracked-discarded.txt")
	if err := os.WriteFile(sentinel, []byte("discard me\n"), 0o644); err != nil {
		t.Fatalf("write untracked work: %v", err)
	}

	svc := newServiceOverRepository(t, repo)
	svc.SetWorktreeCleanup(mgr)
	svc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: mgr})
	if err := svc.DeleteTaskWithOptions(ctx, taskID, DeleteTaskOptions{
		DiscardWorktreeChanges: true,
	}); err != nil {
		t.Fatalf("delete task with discard consent: %v", err)
	}
	var encodedSnapshot string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT resource_snapshot FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
		ORDER BY created_at DESC LIMIT 1
	`, taskID).Scan(&encodedSnapshot); err != nil {
		t.Fatalf("load delete cleanup snapshot: %v", err)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(encodedSnapshot), &snapshot); err != nil {
		t.Fatalf("decode delete cleanup snapshot: %v", err)
	}
	if !snapshot.DiscardWorktreeChanges {
		t.Fatal("cleanup snapshot omitted discard consent")
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
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Fatalf("consented cleanup left worktree on disk: %s (stat err = %v)", wt.Path, statErr)
	}
}

type dirtyWorktreeCleanupFailure struct{}

func (dirtyWorktreeCleanupFailure) OnTaskDeleted(context.Context, string) error { return nil }

func (dirtyWorktreeCleanupFailure) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (dirtyWorktreeCleanupFailure) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return fmt.Errorf("cleanup worktrees: %w", worktree.ErrDirtyWorktreeCleanup)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.11
func TestDirtyWorktreeCleanupRefusalBecomesTerminalAfterAdmission(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	svc.SetWorktreeCleanup(dirtyWorktreeCleanupFailure{})
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "wt-race", TaskID: "task-race"}},
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "dirty-worktree-terminal", OperationID: "delete:dirty-worktree-terminal",
		TaskID: "task-race", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("create cleanup job: %v", err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); !errors.Is(err, worktree.ErrDirtyWorktreeCleanup) {
		t.Fatalf("process cleanup error = %v, want dirty-worktree refusal", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get cleanup job: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateFailed {
		t.Fatalf("cleanup state = %q, want failed", got.State)
	}
	if got.NextAttemptAt != nil {
		t.Fatalf("terminal cleanup scheduled retry at %v", got.NextAttemptAt)
	}
}

func TestDirtyWorktreeCleanupMixedFailureRemainsRetryable(t *testing.T) {
	dirtyErr := fmt.Errorf("cleanup worktrees: %w", worktree.ErrDirtyWorktreeCleanup)
	mixedErr := errors.Join(dirtyErr, errors.New("temporary environment cleanup failure"))
	if isDirtyWorktreeCleanupError(mixedErr) {
		t.Fatal("mixed cleanup failure was classified as a sole dirty-worktree refusal")
	}
	if !isDirtyWorktreeCleanupError(dirtyErr) {
		t.Fatal("wrapped dirty-worktree refusal was not classified as terminal")
	}
}

func TestDirtyWorktreeCleanupBatchOrderRemainsRetryable(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestService(t)
	const taskID = "task-batch-cleanup-order"
	const sessionID = "session-batch-cleanup-order"
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	repoPath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-batch-cleanup-order", WorkspaceID: "ws-" + taskID,
		Name: "repo-batch-cleanup-order", SourceType: "local", LocalPath: repoPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	mgr := newCleanupTestWorktreeManager(t, repo)
	wt, err := mgr.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Batch cleanup order",
		RepositoryID: "repo-batch-cleanup-order", RepositoryPath: repoPath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-batch-cleanup-order",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	dirtyPath := filepath.Join(wt.Path, "dirty-before-retry.txt")
	if err := os.WriteFile(dirtyPath, []byte("preserve me\n"), 0o644); err != nil {
		t.Fatalf("write dirty worktree file: %v", err)
	}

	// The first inventory row fails for a retryable metadata reason. The second
	// row refuses cleanup because it is dirty. Both failures must reach the job
	// classifier; returning only the final sentinel would incorrectly terminalize
	// this batch.
	firstFailure := &worktree.Worktree{ID: "wt-retryable-first", TaskID: taskID}
	cleanupErr := mgr.CleanupWorktrees(ctx, []*worktree.Worktree{firstFailure, wt})
	if cleanupErr == nil {
		t.Fatal("batch cleanup succeeded despite two failures")
	}
	if !errors.Is(cleanupErr, worktree.ErrDirtyWorktreeCleanup) {
		t.Fatalf("batch cleanup error = %v, want dirty refusal included", cleanupErr)
	}
	if isDirtyWorktreeCleanupError(cleanupErr) {
		t.Fatalf("batch cleanup was classified as terminal: %v", cleanupErr)
	}
}
