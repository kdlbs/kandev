package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPromptTask_IdleSession_IsPromptable reproduces the office-mode IDLE
// re-engagement bug. An IDLE office session has its conversation parked
// (ACP session preserved) but no live agent process. Sending a prompt MUST
// wake the session up rather than be bounced by the "agent is currently
// processing a prompt" guard.
//
// Before the fix, checkSessionPromptable's default branch returned
// ErrAgentPromptInProgress for IDLE — the misleading error the user reported.
func TestPromptTask_IdleSession_IsPromptable(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()

	// Model the post-resume state: agent is running, executors_running row
	// exists. This keeps the test focused on the checkSessionPromptable bug;
	// the IDLE→resume path itself is covered by
	// TestEnsureSessionRunning_IdleSessionTriggersResume.
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-idle-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-idle-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "follow-up", "", false, nil, false)
	if err != nil {
		if errors.Is(err, ErrAgentPromptInProgress) {
			t.Fatalf("IDLE session must not surface ErrAgentPromptInProgress: %v", err)
		}
		if strings.Contains(err.Error(), "agent is currently processing a prompt") {
			t.Fatalf("must not return 'agent is currently processing a prompt' for IDLE: %v", err)
		}
		t.Fatalf("expected IDLE session to accept prompt, got: %v", err)
	}

	// The prompt was forwarded to the (post-resume) live agent.
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected one captured prompt, got %d", len(agentMgr.capturedPrompts))
	}
}

// TestCheckSessionPromptable_StateMatrix exercises every state to lock in the
// IDLE acceptance and document which states are rejected and with which error.
// RUNNING → ErrAgentPromptInProgress (someone else is mid-turn).
// IDLE / COMPLETED / WAITING_FOR_INPUT → accepted.
// Everything else → ErrSessionNotPromptable (NOT ErrAgentPromptInProgress).
func TestCheckSessionPromptable_StateMatrix(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	cases := []struct {
		state   models.TaskSessionState
		wantErr bool
		wantIs  error
	}{
		{models.TaskSessionStateWaitingForInput, false, nil},
		{models.TaskSessionStateCompleted, false, nil},
		{models.TaskSessionStateIdle, false, nil},
		{models.TaskSessionStateRunning, true, ErrAgentPromptInProgress},
		{models.TaskSessionStateStarting, true, ErrSessionNotPromptable},
		{models.TaskSessionStateFailed, true, ErrSessionNotPromptable},
		{models.TaskSessionStateCancelled, true, ErrSessionNotPromptable},
		{models.TaskSessionStateCreated, true, ErrSessionNotPromptable},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			err := svc.checkSessionPromptable("t", "s", tc.state)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for state %q", tc.state)
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Fatalf("state %q: expected errors.Is(%v); got %v", tc.state, tc.wantIs, err)
				}
				// Non-RUNNING states must NOT be classified as
				// "agent in progress" — that misleads the UI and any caller
				// doing errors.Is(err, ErrAgentPromptInProgress) checks.
				if tc.state != models.TaskSessionStateRunning && errors.Is(err, ErrAgentPromptInProgress) {
					t.Fatalf("state %q must not wrap ErrAgentPromptInProgress: %v", tc.state, err)
				}
			} else if err != nil {
				t.Fatalf("expected nil for state %q, got: %v", tc.state, err)
			}
		})
	}
}

// TestEnsureSessionRunning_IdleSessionTriggersResume confirms that an IDLE
// session with a preserved executors_running row (the normal office-mode IDLE
// shape, since handleOfficeTurnComplete tears down the agent process but leaves
// the row intact) routes through ResumeSession when the in-memory execution
// store has no live entry. This pins down the second half of the bug: even
// with checkSessionPromptable fixed, ensureSessionRunning must not bounce IDLE
// — it must resume.
func TestEnsureSessionRunning_IdleSessionTriggersResume(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-idle-1"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-idle-1")

	// Mock launch: report success and capture the call so the test can confirm
	// the resume path triggered. isAgentRunning starts false (true IDLE shape).
	// We flip session→WAITING_FOR_INPUT in a goroutine AFTER LaunchAgent so it
	// happens after ResumeSession's persistResumeState (which forces STARTING).
	// waitForSessionReady polls every 500ms — a short delay is enough.
	launchCalled := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		isAgentRunning:         false,
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			select {
			case launchCalled <- struct{}{}:
			default:
			}
			go func(sessID string) {
				// Wait long enough for persistResumeState (sets STARTING) to
				// land, then flip to WAITING_FOR_INPUT to mirror the
				// AgentBootReady → handleAgentBootReady flow.
				time.Sleep(50 * time.Millisecond)
				sess, err := repo.GetTaskSession(context.Background(), sessID)
				if err == nil && sess != nil {
					sess.State = models.TaskSessionStateWaitingForInput
					sess.UpdatedAt = time.Now().UTC()
					_ = repo.UpdateTaskSession(context.Background(), sess)
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

	// Re-load
	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if err := svc.ensureSessionRunning(ctx, "session1", session); err != nil {
		t.Fatalf("ensureSessionRunning failed for IDLE session: %v", err)
	}
	select {
	case <-launchCalled:
	default:
		t.Fatal("expected ResumeSession to call LaunchAgent on IDLE session, but it never fired")
	}
}
