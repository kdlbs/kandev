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

// TestEnsureSessionRunning_ColdResumeReapsAndRetriesOnPromptReadyTimeout pins
// down a self-healing gap specific to the "kandev restart" cold-resume path:
// when the backend restarts, the in-memory execution store is empty even
// though a resumable executors_running row survives in the DB. The very
// first ensureSessionRunning call for such a session (the task-open
// auto-resume in session_ensure.go's tryEnsureExecution, which has no
// external retry — failures are only logged) takes the "no execution
// tracked yet" branch: it calls ResumeSession once and waits once for
// prompt-readiness.
//
// The sibling branch just above it (an execution IS already tracked but
// isn't prompt-ready) self-heals on the same timeout by reaping the stuck
// execution and relaunching — see reapPromptUnreadyExecution's call site a
// few lines up. The cold-resume branch had no equivalent: a launch that
// hangs past agentPromptReadyTimeout on its first attempt returned the bare
// timeout error with no reap-and-retry, even though the exact same recovery
// this test drives (stop the wedged execution, relaunch, wait again) is
// already proven safe elsewhere (TestResumeTaskSession_ReapsPromptDeadExecutionAndRelaunches).
func TestEnsureSessionRunning_ColdResumeReapsAndRetriesOnPromptReadyTimeout(t *testing.T) {
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

	// Surviving executors_running row, no in-memory execution — the shape
	// left behind by a backend restart.
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-wedged",
		Status:           "running",
		Resumable:        true,
		ResumeToken:      "resume-token-123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to seed executor running: %v", err)
	}

	// Both launch attempts flip the session's DB state to WAITING_FOR_INPUT
	// promptly (so waitForSessionReady never falls back to the 6-minute
	// constants.AgentLaunchTimeout in this test) — only agent-level
	// prompt-readiness (isAgentReadyFn) distinguishes the wedged first
	// attempt from the healthy replacement.
	var launchCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return false
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			// Only the reap-and-retry relaunch (attempt 2+) ever reports ready.
			return launchCalls.Load() >= 2
		},
		stopAgentWithReasonFunc: func(_ context.Context, _ string, _ string, _ bool) error {
			return nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			n := launchCalls.Add(1)
			go func(sessID string) {
				tick := time.NewTicker(time.Millisecond)
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
			if n == 1 {
				// First attempt: session state flips fine, but the agent
				// process never reports prompt-ready — the wedged-resume
				// shape this test targets.
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-wedged-relaunch"}, nil
			}
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

	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if err := svc.ensureSessionRunning(ctx, "session1", session); err != nil {
		t.Fatalf("expected cold resume to reap the wedged launch and retry, got: %v", err)
	}
	if got := launchCalls.Load(); got != 2 {
		t.Fatalf("expected a reap-and-retry relaunch (2 LaunchAgent calls), got %d", got)
	}
}
