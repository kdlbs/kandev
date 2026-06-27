package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
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

// TestIsSessionBusyError pins down the requeue umbrella: STARTING / CREATED /
// RUNNING all need to come back true so executeQueuedMessage and the auto-start
// loop in event_handlers_workflow.go requeue rather than drop. Without this
// helper, splitting ErrAgentPromptInProgress and ErrSessionNotPromptable
// would silently lose queued messages that hit a session mid-startup.
func TestIsSessionBusyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
		{"in-progress sentinel", ErrAgentPromptInProgress, true},
		{"wrapped in-progress", fmt.Errorf("outer: %w", ErrAgentPromptInProgress), true},
		{"not-promptable sentinel", ErrSessionNotPromptable, true},
		{"wrapped not-promptable", fmt.Errorf("outer: %w", ErrSessionNotPromptable), true},
		{"reset is NOT busy", ErrSessionResetInProgress, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSessionBusyError(tc.err); got != tc.want {
				t.Errorf("isSessionBusyError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
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
	// After LaunchAgent returns, ResumeSession's persistResumeState forces the
	// session into STARTING; we then need to flip to WAITING_FOR_INPUT so the
	// downstream waitForSessionReady poll exits. We watch for STARTING to
	// appear (deterministic — no time.Sleep race against persistResumeState)
	// then flip, mirroring the real AgentBootReady → handleAgentBootReady flow.
	launchCalled := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		isAgentRunning:         false,
		repoForExecutionLookup: repo,
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return true
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			select {
			case launchCalled <- struct{}{}:
			default:
			}
			go func(sessID string) {
				// Watch for STARTING (committed by persistResumeState) then flip
				// — guarantees ordering without time.Sleep. Ticker + select per
				// AGENTS.md "channel-based synchronization over sleep-based
				// waits" guidance (testing/synctest doesn't help here because
				// the SQLite repo runs real I/O goroutines that synctest can't
				// fully observe as idle).
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

func TestEnsureSessionRunning_WaitsForPromptReadyAfterResume(t *testing.T) {
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

	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.ensureSessionRunning(ctx, "session1", session)
	}()

	select {
	case <-checked:
	case <-time.After(3 * time.Second):
		t.Fatal("expected ensureSessionRunning to check prompt readiness after resume")
	}

	select {
	case err := <-done:
		t.Fatalf("ensureSessionRunning returned before prompt readiness: %v", err)
	default:
	}

	close(ready)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ensureSessionRunning failed after prompt readiness: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ensureSessionRunning did not return after prompt readiness")
	}
}

func TestEnsureSessionRunning_WaitsForPromptReadyWhenExecutionExists(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-ready-race-1"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-ready-race-1")

	ready := make(chan struct{})
	checked := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
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
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	done := make(chan error, 1)
	go func() {
		done <- svc.ensureSessionRunning(ctx, "session1", session)
	}()

	select {
	case <-checked:
	case <-time.After(2 * time.Second):
		t.Fatal("expected ensureSessionRunning to check prompt readiness")
	}

	select {
	case err := <-done:
		t.Fatalf("ensureSessionRunning returned before prompt readiness: %v", err)
	default:
	}

	close(ready)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ensureSessionRunning failed after prompt readiness: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ensureSessionRunning did not return after prompt readiness")
	}
}

func TestPromptTask_LazyResumeExecutionNotFoundFallsBackToFreshLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-before-restart"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-before-restart")

	var launchCalls atomic.Int32
	launchPrompts := make(chan string, 2)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr:              lifecycle.ErrExecutionNotFound,
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			call := launchCalls.Add(1)
			launchPrompts <- req.TaskDescription
			if call == 1 {
				go func(sessionID string) {
					tick := time.NewTicker(5 * time.Millisecond)
					defer tick.Stop()
					timeout := time.After(5 * time.Second)
					for {
						select {
						case <-tick.C:
							sess, err := repo.GetTaskSession(context.Background(), sessionID)
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
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: fmt.Sprintf("exec-resumed-%d", call)}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())

	prompt := "follow-up after restart"
	if _, err := svc.PromptTask(ctx, "task1", "session1", prompt, "", false, nil, false); err != nil {
		t.Fatalf("expected fresh-launch fallback to recover missing execution, got: %v", err)
	}

	if got := launchCalls.Load(); got != 2 {
		t.Fatalf("expected resume launch plus fresh fallback launch, got %d launches", got)
	}
	<-launchPrompts // resume prompt; empty for the lazy resume path.
	freshPrompt := <-launchPrompts
	if !strings.Contains(freshPrompt, prompt) {
		t.Fatalf("fresh launch prompt %q does not contain original prompt %q", freshPrompt, prompt)
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected one failed PromptAgent attempt before fallback, got %d", len(agentMgr.capturedPromptCalls))
	}
}

func TestPromptTask_LazyResumeMissingACPSessionFallsBackToFreshLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-before-restart"
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-before-restart")

	var launchCalls atomic.Int32
	launchPrompts := make(chan string, 2)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		promptErr: fmt.Errorf(
			"%w: session not found: %w",
			lifecycle.ErrAgentReported,
			lifecycle.ErrExecutionNotFound,
		),
		isAgentRunningFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		isAgentReadyFn: func(_ context.Context, _ string) bool {
			return launchCalls.Load() > 0
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			call := launchCalls.Add(1)
			launchPrompts <- req.TaskDescription
			if call == 1 {
				go func(sessionID string) {
					tick := time.NewTicker(5 * time.Millisecond)
					defer tick.Stop()
					timeout := time.After(5 * time.Second)
					for {
						select {
						case <-tick.C:
							sess, err := repo.GetTaskSession(context.Background(), sessionID)
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
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: fmt.Sprintf("exec-resumed-%d", call)}, nil
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:    "task1",
		Title: "Test Task",
		State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())

	prompt := "follow-up after missing ACP session"
	if _, err := svc.PromptTask(ctx, "task1", "session1", prompt, "", false, nil, false); err != nil {
		t.Fatalf("expected fresh-launch fallback to recover missing ACP session, got: %v", err)
	}

	if got := launchCalls.Load(); got != 2 {
		t.Fatalf("expected resume launch plus fresh fallback launch, got %d launches", got)
	}
	<-launchPrompts // resume prompt; empty for the lazy resume path.
	freshPrompt := <-launchPrompts
	if !strings.Contains(freshPrompt, prompt) {
		t.Fatalf("fresh launch prompt %q does not contain original prompt %q", freshPrompt, prompt)
	}
	if len(agentMgr.capturedPromptCalls) != 1 {
		t.Fatalf("expected one failed PromptAgent attempt before fallback, got %d", len(agentMgr.capturedPromptCalls))
	}
}
