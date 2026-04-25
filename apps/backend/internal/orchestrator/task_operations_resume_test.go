package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestGetTaskSessionStatus_NoAutoResumeOnErrorRecovery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	// Set ErrorMessage to simulate error-recovery state (set by handleRecoverableFailure)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.ErrorMessage = "Agent encountered an error: context deadline exceeded"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "er1",
		SessionID: "session1",
		TaskID:    "task1",
		Status:    "ready",
		Resumable: true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateReview}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.NeedsResume {
		t.Fatal("expected NeedsResume=false for error-recovery session")
	}
	if !resp.IsResumable {
		t.Fatal("expected IsResumable=true so manual resume buttons still work")
	}
	if resp.ResumeReason != resumeReasonErrorRecovery {
		t.Fatalf("expected ResumeReason=%q, got %q", resumeReasonErrorRecovery, resp.ResumeReason)
	}
}

func TestGetTaskSessionStatus_AutoResumesNormalWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	// No ErrorMessage — normal idle session
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "er1",
		SessionID: "session1",
		TaskID:    "task1",
		Status:    "ready",
		Resumable: true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if !resp.NeedsResume {
		t.Fatal("expected NeedsResume=true for normal waiting session")
	}
	if !resp.IsResumable {
		t.Fatal("expected IsResumable=true")
	}
	if resp.ResumeReason != "agent_not_running_fresh_start" {
		t.Fatalf("expected ResumeReason=%q, got %q", "agent_not_running_fresh_start", resp.ResumeReason)
	}
}

// TestGetTaskSessionStatus_AutoResumesFailedSessionWithResumeToken verifies the
// failed-but-recoverable path: a FAILED session that still has a resumable
// runtime + resume token reports NeedsResume=true so the frontend retries
// transparently instead of showing the historical error to the user.
func TestGetTaskSessionStatus_AutoResumesFailedSessionWithResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.ErrorMessage = "execution already running"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "er1",
		SessionID:   "session1",
		TaskID:      "task1",
		Status:      "ready",
		Resumable:   true,
		ResumeToken: "acp-session-123",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateReview}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if !resp.NeedsResume {
		t.Fatal("expected NeedsResume=true so frontend auto-resumes a failed session")
	}
	if !resp.IsResumable {
		t.Fatal("expected IsResumable=true for failed session with resumable runtime")
	}
	if resp.NeedsWorkspaceRestore {
		t.Fatal("expected NeedsWorkspaceRestore=false when auto-resuming")
	}
	if resp.ResumeReason != resumeReasonFailedSessionResumable {
		t.Fatalf("expected ResumeReason=%q for FAILED auto-resume, got %q",
			resumeReasonFailedSessionResumable, resp.ResumeReason)
	}
}

// TestGetTaskSessionStatus_FailedSessionWithoutResumableFlagFallsBack verifies
// that a FAILED session whose runtime is NOT resumable still goes through the
// workspace-restore path (no auto-resume attempt to avoid certain failure).
func TestGetTaskSessionStatus_FailedSessionWithoutResumableFlagFallsBack(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	// Seed a worktree so canRestoreWorkspace returns true.
	now := time.Now().UTC()
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "er1",
		SessionID:   "session1",
		TaskID:      "task1",
		Status:      "ready",
		Resumable:   false,
		ResumeToken: "acp-session-123",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateReview}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.NeedsResume {
		t.Fatal("expected NeedsResume=false when runtime is not Resumable")
	}
	if !resp.NeedsWorkspaceRestore {
		t.Fatal("expected NeedsWorkspaceRestore=true as fallback")
	}
}

// TestGetTaskSessionStatus_CancelledSessionStaysWorkspaceRestore verifies that
// CANCELLED sessions are NOT auto-resumed (the user explicitly stopped them).
func TestGetTaskSessionStatus_CancelledSessionStaysWorkspaceRestore(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCancelled)

	now := time.Now().UTC()
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "er1",
		SessionID:   "session1",
		TaskID:      "task1",
		Status:      "ready",
		Resumable:   true,
		ResumeToken: "acp-session-123",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateReview}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.NeedsResume {
		t.Fatal("expected NeedsResume=false for CANCELLED — user stopped intentionally")
	}
	if !resp.NeedsWorkspaceRestore {
		t.Fatal("expected NeedsWorkspaceRestore=true for CANCELLED with worktree")
	}
}

func TestGetTaskSessionStatus_NoAutoResumeWithResumeTokenOnError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	// Set ErrorMessage and a resume token
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.ErrorMessage = "Agent encountered an error: context deadline exceeded"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "er1",
		SessionID:   "session1",
		TaskID:      "task1",
		Status:      "ready",
		Resumable:   true,
		ResumeToken: "acp-session-123",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateReview}
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.NeedsResume {
		t.Fatal("expected NeedsResume=false for error-recovery session with resume token")
	}
	if !resp.IsResumable {
		t.Fatal("expected IsResumable=true so manual resume buttons still work")
	}
	if resp.ResumeReason != resumeReasonErrorRecovery {
		t.Fatalf("expected ResumeReason=%q, got %q", resumeReasonErrorRecovery, resp.ResumeReason)
	}
}
