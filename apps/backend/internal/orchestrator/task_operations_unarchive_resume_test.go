package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestResumeTaskSession_RecreatesMissingWorktreeAfterUnarchive pins the
// unarchive → resume path. Archiving a task deletes the worktree *directory*
// but keeps the environment repository row and the git branch, so every
// resume after an unarchive sees a recorded worktree path that no longer
// exists on disk. Rejecting the resume there made unarchive a dead end — the
// session could never be resumed even though the branch was still intact.
// The resume must reach the launch instead, where the worktree manager
// recreates the directory from the stored record.
func TestResumeTaskSession_RecreatesMissingWorktreeAfterUnarchive(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// CANCELLED with the archive reason and no executors_running row: exactly
	// what archive cleanup leaves behind for an unarchived task.
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCancelled)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.ErrorMessage = models.SessionArchiveCancelReason
	session.TaskEnvironmentID = "env1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env1", TaskID: "task1", ExecutorType: "worktree",
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	// The directory below is never created — the archive removed it.
	missingPath := filepath.Join(t.TempDir(), "archived-task", "kandev")
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                "ter1",
		TaskEnvironmentID: "env1",
		WorktreeID:        "wid1",
		RepositoryID:      "repo1",
		WorktreePath:      missingPath,
		WorktreeBranch:    "feature/archived-work",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	if _, statErr := os.Stat(missingPath); statErr == nil {
		t.Fatalf("test setup is wrong: %s must not exist on disk", missingPath)
	}

	var launched atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn:         func(_ context.Context, _ string) bool { return true },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launched.Add(1)
			sess, getErr := repo.GetTaskSession(context.Background(), req.SessionID)
			if getErr == nil && sess != nil {
				sess.State = models.TaskSessionStateWaitingForInput
				sess.UpdatedAt = time.Now().UTC()
				_ = repo.UpdateTaskSession(context.Background(), sess)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-unarchived"}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", Title: "Test Task", State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	exec, err := svc.ResumeTaskSession(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("resume of an unarchived session must not be blocked by the missing worktree directory: %v", err)
	}
	if exec == nil {
		t.Fatal("ResumeTaskSession returned nil execution")
	}
	if launched.Load() == 0 {
		t.Fatal("expected the resume to reach LaunchAgent, where the worktree is recreated")
	}
}

// TestGetTaskSessionStatus_ArchivedSessionResumableWithMissingWorktree covers
// the status half of the unarchive path: an archive-cancelled session whose
// executors_running row survived cleanup must still report IsResumable, even
// though its worktree directory is gone. Reporting "worktree not found" here
// hid both the auto-resume and the manual resume button, so an unarchived task
// had no way back at all.
func TestGetTaskSessionStatus_ArchivedSessionResumableWithMissingWorktree(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCancelled)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.ErrorMessage = models.SessionArchiveCancelReason
	session.TaskEnvironmentID = "env1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env1", TaskID: "task1", ExecutorType: "worktree",
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	// Recorded path the archive already deleted.
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		ID:                "ter1",
		TaskEnvironmentID: "env1",
		WorktreeID:        "wid1",
		RepositoryID:      "repo1",
		WorktreePath:      filepath.Join(t.TempDir(), "archived-task", "kandev"),
		WorktreeBranch:    "feature/archived-work",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "er1",
		SessionID:   "session1",
		TaskID:      "task1",
		Status:      "ready",
		Resumable:   true,
		ResumeToken: "acp-session-archived",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("expected no status error for a recreatable worktree, got %q", resp.Error)
	}
	if !resp.IsResumable {
		t.Fatal("expected IsResumable=true: the worktree is recreated by the resume itself")
	}
	if !resp.NeedsResume {
		t.Fatal("expected NeedsResume=true so the unarchived session auto-resumes")
	}
	if resp.ResumeReason != resumeReasonArchiveCancelledResumable {
		t.Fatalf("expected ResumeReason=%q, got %q", resumeReasonArchiveCancelledResumable, resp.ResumeReason)
	}
}
