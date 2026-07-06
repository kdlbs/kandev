package orchestrator

import (
	"context"
	"sync/atomic"
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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

func TestResumeTaskSession_WaitsForPromptReady(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-old-1"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-old-1")

	ready := make(chan struct{})
	checked := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		isAgentRunning:         false,
		repoForExecutionLookup: repo,
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			select {
			case checked <- struct{}{}:
			default:
			}
			select {
			case <-ready:
				return true
			default:
				return false
			}
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			go func(sessID string) {
				tick := time.NewTicker(5 * time.Millisecond)
				defer tick.Stop()
				timeout := time.After(5 * time.Second)
				for {
					select {
					case <-tick.C:
						sess, err := repo.GetTaskSession(context.Background(), sessID)
						if err == nil && sess != nil && sess.State == models.TaskSessionStateStarting {
							sess.State = models.TaskSessionStateWaitingForInput
							sess.UpdatedAt = time.Now().UTC()
							_ = repo.UpdateTaskSession(context.Background(), sess)
							return
						}
					case <-timeout:
						return
					}
				}
			}(req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-resumed-1"}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	done := make(chan struct {
		exec *executor.TaskExecution
		err  error
	}, 1)
	go func() {
		exec, err := svc.ResumeTaskSession(ctx, "task1", "session1")
		done <- struct {
			exec *executor.TaskExecution
			err  error
		}{exec: exec, err: err}
	}()

	select {
	case <-checked:
	case <-time.After(3 * time.Second):
		t.Fatal("expected ResumeTaskSession to check prompt readiness")
	}

	select {
	case result := <-done:
		t.Fatalf("ResumeTaskSession returned before prompt readiness: %v", result.err)
	default:
	}

	close(ready)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("ResumeTaskSession failed after prompt readiness: %v", result.err)
		}
		if result.exec == nil {
			t.Fatal("ResumeTaskSession returned nil execution")
		}
		if result.exec.SessionState != v1.TaskSessionStateWaitingForInput {
			t.Fatalf("expected WAITING_FOR_INPUT response state, got %s", result.exec.SessionState)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResumeTaskSession did not return after prompt readiness")
	}
}

func TestResumeTaskSession_ReapsPromptDeadExecutionAndRelaunches(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

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
		ID:               "er1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-zombie",
		Status:           "running",
		Resumable:        true,
		ResumeToken:      "resume-token-123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to seed executor running: %v", err)
	}

	var zombieRunning atomic.Bool
	zombieRunning.Store(true)
	var replacementReady atomic.Bool
	var launchCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return zombieRunning.Load()
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return replacementReady.Load()
		},
		stopAgentWithReasonFunc: func(_ context.Context, _ string, _ string, _ bool) error {
			zombieRunning.Store(false)
			return nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			if req.ACPSessionID != "resume-token-123" {
				t.Errorf("expected preserved resume token, got %q", req.ACPSessionID)
			}
			go func(sessID string) {
				tick := time.NewTicker(5 * time.Millisecond)
				defer tick.Stop()
				timeout := time.After(5 * time.Second)
				for {
					select {
					case <-tick.C:
						sess, err := repo.GetTaskSession(context.Background(), sessID)
						if err == nil && sess != nil && sess.State == models.TaskSessionStateStarting {
							sess.State = models.TaskSessionStateWaitingForInput
							sess.UpdatedAt = time.Now().UTC()
							_ = repo.UpdateTaskSession(context.Background(), sess)
							replacementReady.Store(true)
							return
						}
					case <-timeout:
						return
					}
				}
			}(req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-replacement"}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	exec, err := svc.ResumeTaskSession(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("expected prompt-dead execution to be reaped and relaunched, got: %v", err)
	}
	if exec == nil {
		t.Fatal("expected replacement execution")
	}
	if exec.AgentExecutionID != "exec-replacement" {
		t.Fatalf("expected replacement execution id, got %q", exec.AgentExecutionID)
	}
	if got := launchCalls.Load(); got != 1 {
		t.Fatalf("expected one replacement launch, got %d", got)
	}

	agentMgr.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), agentMgr.stopAgentWithReasonArgs...)
	agentMgr.mu.Unlock()
	if len(stopCalls) != 1 {
		t.Fatalf("expected one zombie stop call, got %d", len(stopCalls))
	}
	if stopCalls[0].ExecutionID != "exec-zombie" || !stopCalls[0].Force {
		t.Fatalf("unexpected zombie stop call: %#v", stopCalls[0])
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to reload executor running row: %v", err)
	}
	if running.ResumeToken != "resume-token-123" {
		t.Fatalf("expected resume token to be preserved, got %q", running.ResumeToken)
	}
}
