package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestAsyncStartFailure_RequeuesArmedStepPromptExactlyOnce is the regression
// test for issue #3753: a step with reset_agent_context + auto_start_agent
// launches a CREATED session, admission succeeds synchronously, but the
// agent process then fails to start inside the goroutine spawned by
// startAgentProcessAsync — after autoStartStepPrompt's call into
// StartCreatedSession has already returned nil. Before this fix the prompt
// armed for that launch was silently dropped: it existed only as a chat row
// tied to an execution that never started.
func TestAsyncStartFailure_RequeuesArmedStepPromptExactlyOnce(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-async-fail"
		sessionID = "session-async-fail"
		profile   = "profile-async-fail"
		execID    = "exec-async-fail"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, Title: "Async Fail Task", State: v1.TaskStateInProgress,
	}

	startAgentProcessCalled := make(chan struct{}, 1)
	var launchCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			return &executor.LaunchAgentResponse{
				AgentExecutionID: execID,
				Status:           v1.AgentStatusStarting,
				WorkspacePath:    t.TempDir(),
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			select {
			case startAgentProcessCalled <- struct{}{}:
			default:
			}
			return errors.New("agent subprocess failed to boot")
		},
	}

	step := &wfmodels.WorkflowStep{
		ID: "step-work", WorkflowID: "wf1", Name: "Work",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
			{Type: wfmodels.OnEnterAutoStartAgent},
		}},
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps[step.ID] = step

	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.messageCreator = &mockMessageCreator{}
	// createTestServiceWithScheduler does not wire the async-start callbacks;
	// production wiring lives in service.go next to the executor's construction.
	svc.executor.SetOnAgentStartFailed(svc.handleAgentStartFailed)
	svc.executor.SetOnAgentProcessStarted(svc.handleAgentProcessStarted)

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = profile
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session profile: %v", err)
	}

	if err := svc.autoStartStepPrompt(ctx, taskID, session, step, "Do the work", false, true, nil); err != nil {
		t.Fatalf("expected the synchronous launch admission to succeed, got %v", err)
	}

	select {
	case <-startAgentProcessCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("expected StartAgentProcess to be called")
	}

	// The async failure runs on the goroutine spawned by runAgentProcessAsync,
	// well after autoStartStepPrompt already returned — poll instead of
	// asserting immediately.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.messageQueue.GetStatus(ctx, sessionID).Count > 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status.Count != 1 {
		t.Fatalf("expected exactly 1 requeued message after the asynchronous start failure, got %d: %+v",
			status.Count, status.Entries)
	}
	queued := status.Entries[0]
	if queued.QueuedBy != messagequeue.QueuedByWorkflow {
		t.Errorf("queued_by = %q, want %q", queued.QueuedBy, messagequeue.QueuedByWorkflow)
	}
	if !strings.Contains(queued.Content, "Do the work") {
		t.Errorf("queued content = %q, want it to contain the step prompt", queued.Content)
	}
	if recorded, _ := queued.Metadata[metaKeyUserMessageRecorded].(bool); !recorded {
		t.Error("expected user_message_recorded=true: recordAutoStartMessage already wrote the chat row")
	}

	// No double start: exactly one StartAgentProcess call for this launch.
	agentMgr.mu.Lock()
	startCalls := append([]string(nil), agentMgr.startAgentProcessCalls...)
	agentMgr.mu.Unlock()
	if len(startCalls) != 1 {
		t.Fatalf("expected exactly 1 StartAgentProcess call, got %d: %v", len(startCalls), startCalls)
	}

	// No scheduled auto-resume: queueAsyncStartFailurePrompt must not trigger
	// a second LaunchAgent call. Give any wrongly-scheduled goroutine a window
	// to fire before asserting its absence.
	time.Sleep(150 * time.Millisecond)
	if got := launchCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 LaunchAgent call (no auto-resume double start), got %d", got)
	}
}

// TestAsyncStartSuccess_DiscardsArmedStepPromptHandle covers the second half
// of REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005: once the asynchronous
// start succeeds, the armed prompt already reached the agent as the execution
// description, so the handle must be discarded — an unrelated later failure
// on the same session must find nothing to re-queue.
func TestAsyncStartSuccess_DiscardsArmedStepPromptHandle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.armPendingStepPrompt("s1", pendingStepPrompt{taskID: "t1", prompt: "Do the work"})

	svc.handleAgentProcessStarted(ctx, "t1", "s1", "exec-success")

	if _, ok := svc.pendingStepPromptStore().take("s1"); ok {
		t.Fatal("expected the handle to be discarded after a successful async start")
	}

	// Re-arm to prove discard, not a one-shot take, is what happened above —
	// then discard again the way handleAgentProcessStarted does, and confirm
	// an unrelated later failure on the same session queues nothing.
	svc.armPendingStepPrompt("s1", pendingStepPrompt{taskID: "t1", prompt: "should not be queued"})
	svc.handleAgentProcessStarted(ctx, "t1", "s1", "exec-success")

	svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-success",
		errors.New("ACP initialize handshake failed"), false)

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("expected no queued messages after the handle was discarded on success, got %d", got)
	}
}

// TestAsyncStartFailure_GuardedFailureLeavesHandleArmed proves the recovery
// call sits strictly after handleAgentStartFailed's existing ownership
// guards: a failure that the cancel-in-flight/stale-resume-attempt/terminal-
// session checks reject must never reach recoverPendingStepPromptOnAsyncStartFailure,
// so an armed handle for the execution that actually owns the session survives.
func TestAsyncStartFailure_GuardedFailureLeavesHandleArmed(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.UpdateTaskSessionState(
		ctx, "s1", models.TaskSessionStateCancelled, "cancelled outside coordinator stop",
	); err != nil {
		t.Fatalf("seed cancelled session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.armPendingStepPrompt("s1", pendingStepPrompt{taskID: "t1", prompt: "Do the work"})

	svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-unclaimed",
		errors.New("ACP initialize handshake failed"), false)
	// shouldDropSessionFailure's terminal-session branch spawns a bounded
	// cleanup goroutine; wait for it so goleak does not see it as a leak.
	waitForStopCall(t, agentMgr)

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("a cancelled session's stale start failure must not requeue anything, got %d", got)
	}
	if _, ok := svc.pendingStepPromptStore().take("s1"); !ok {
		t.Fatal("expected the handle to remain armed for the execution that actually owns the session")
	}
}
