package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// acceptedTurnPromptManager lets the first provider turn report acceptance
// before it waits for the cancellation owner to release the turn.
type acceptedTurnPromptManager struct {
	*mockAgentManager
	accepted       chan struct{}
	release        chan struct{}
	releaseOnce    sync.Once
	firstPrompt    atomic.Bool
	firstPromptErr error
	beforeDispatch func()
}

func (m *acceptedTurnPromptManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	_, err := m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if err != nil {
		return nil, err
	}
	first := m.firstPrompt.CompareAndSwap(false, true)
	if first && m.beforeDispatch != nil {
		m.beforeDispatch()
	}
	if onDispatched != nil {
		onDispatched()
	}
	if !first {
		return &executor.PromptResult{
			StopReason:   "end_turn",
			AgentMessage: "follow-up response",
		}, nil
	}
	close(m.accepted)
	<-m.release
	return &executor.PromptResult{StopReason: "cancelled"}, m.firstPromptErr
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.9
func TestPromptTask_ResumedTurnCancelPreservesExecution(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-resumed-turn-cancel"
		sessionID   = "session-resumed-turn-cancel"
		executionID = "execution-resumed-turn-cancel"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-before-resume")

	var launches atomic.Int32
	var currentExecution atomic.Value
	currentExecution.Store(executionID)
	agentRunning := &atomic.Bool{}
	baseManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return currentExecution.Load().(string), nil
		},
		isAgentRunningFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		isAgentReadyFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches.Add(1)
			currentExecution.Store(executionID)
			agentRunning.Store(true)
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: executionID, Status: "ready",
			}); err != nil {
				return nil, err
			}
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
	}
	manager := &acceptedTurnPromptManager{
		mockAgentManager: baseManager,
		accepted:         make(chan struct{}),
		release:          make(chan struct{}),
	}
	baseManager.cancelAgentFunc = func(context.Context, string) error {
		manager.releaseOnce.Do(func() { close(manager.release) })
		return nil
	}
	baseManager.stopAgentWithReasonFunc = func(_ context.Context, _ string, _ string, _ bool) error {
		agentRunning.Store(false)
		return nil
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	firstPromptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.PromptTask(ctx, taskID, sessionID, "first resumed prompt", "", false, nil, false)
		firstPromptDone <- promptErr
	}()
	select {
	case <-manager.accepted:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("resumed prompt did not reach provider acceptance")
	}
	acceptedAttempt, active := svc.resumeAttemptStore().current(sessionID)
	if !active || acceptedAttempt == nil {
		t.Fatal("accepted resume attempt was removed before the provider turn returned")
	}
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	isAccepted := acceptedAttempt.accepted
	registry.mu.Unlock()
	if !isAccepted {
		t.Fatal("provider acceptance did not transfer startup ownership")
	}

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.CancelAgent(ctx, sessionID) }()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle accepted turn")
	}
	select {
	case promptErr := <-firstPromptDone:
		if promptErr != nil {
			t.Fatalf("accepted cancelled prompt: %v", promptErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("accepted prompt did not return after cancellation")
	}

	baseManager.mu.Lock()
	forcedStops := append([]stopAgentCall(nil), baseManager.stopAgentWithReasonArgs...)
	baseManager.mu.Unlock()
	if len(forcedStops) != 0 {
		t.Fatalf("forced execution stops after accepted cancellation = %d, want 0: %#v", len(forcedStops), forcedStops)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("resume launches after accepted cancellation = %d, want 1", got)
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("load preserved execution: %v", err)
	}
	if running == nil || running.AgentExecutionID != executionID {
		t.Fatalf("preserved execution = %#v, want %q", running, executionID)
	}

	if _, err := svc.PromptTask(ctx, taskID, sessionID, "follow-up prompt", "", false, nil, false); err != nil {
		t.Fatalf("follow-up prompt: %v", err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("follow-up caused %d resume launches, want 1", got)
	}
	baseManager.mu.Lock()
	promptCalls := append([]promptCall(nil), baseManager.capturedPromptCalls...)
	baseManager.mu.Unlock()
	if len(promptCalls) != 2 {
		t.Fatalf("prompt calls = %d, want 2: %#v", len(promptCalls), promptCalls)
	}
	for index, call := range promptCalls {
		if call.ExecutionID != executionID {
			t.Errorf("prompt %d execution ID = %q, want %q", index, call.ExecutionID, executionID)
		}
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.9
func TestPromptTask_QueuedAcceptedTurnIdentityReadFailurePreservesExecution(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-queued-accepted-identity-failure"
		sessionID   = "session-queued-accepted-identity-failure"
		executionID = "execution-queued-accepted-identity-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.AgentExecutionID = executionID
	if session.QueueIncarnationID == "" {
		session.QueueIncarnationID = "queue-incarnation-accepted-identity-failure"
	}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-before-queued-resume")

	var launches atomic.Int32
	var currentExecution atomic.Value
	currentExecution.Store(executionID)
	agentRunning := &atomic.Bool{}
	identityReadFailure := &atomic.Bool{}
	var identityReadFailures atomic.Int32
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(
		func(ctx context.Context, requestedTaskID, requestedSessionID string) (messagequeue.QueueSessionIdentity, error) {
			if identityReadFailure.Load() {
				identityReadFailures.Add(1)
				return messagequeue.QueueSessionIdentity{}, errors.New("injected session identity read failure")
			}
			current, resolveErr := repo.GetTaskSession(ctx, requestedSessionID)
			if resolveErr != nil {
				return messagequeue.QueueSessionIdentity{}, resolveErr
			}
			if current == nil || current.TaskID != requestedTaskID || current.QueueIncarnationID == "" {
				return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
			}
			return messagequeue.QueueSessionIdentity{
				TaskID:               requestedTaskID,
				SessionID:            requestedSessionID,
				SessionIncarnationID: current.QueueIncarnationID,
			}, nil
		},
	)
	baseManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return currentExecution.Load().(string), nil
		},
		isAgentRunningFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		isAgentReadyFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches.Add(1)
			currentExecution.Store(executionID)
			agentRunning.Store(true)
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: executionID, Status: "ready",
			}); err != nil {
				return nil, err
			}
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
	}
	manager := &acceptedTurnPromptManager{
		mockAgentManager: baseManager,
		accepted:         make(chan struct{}),
		release:          make(chan struct{}),
		beforeDispatch:   func() { identityReadFailure.Store(true) },
	}
	baseManager.cancelAgentFunc = func(context.Context, string) error {
		manager.releaseOnce.Do(func() { close(manager.release) })
		return nil
	}
	baseManager.stopAgentWithReasonFunc = func(_ context.Context, _ string, _ string, _ bool) error {
		agentRunning.Store(false)
		return nil
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	t.Cleanup(func() {
		svc.clearAcceptedQueuedDispatch(sessionID)
		svc.stopSendNowWorkers()
	})

	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve queued session identity: %v", err)
	}
	if identity.SessionIncarnationID == "" {
		t.Fatal("queued session identity has no incarnation")
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued accepted identity failure prompt", "",
		messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	if reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued); reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}

	var afterDispatchSawAccepted atomic.Bool
	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.promptTask(
			ctx, taskID, sessionID, "queued accepted identity failure prompt", "", false, nil, true,
			launchOriginAutomatic,
			promptTaskOptions{
				claimEntryID:            queued.ID,
				expectedSessionIdentity: &identity,
				afterDispatch: func() error {
					afterDispatchSawAccepted.Store(activeResumeAttemptAccepted(svc, sessionID))
					return nil
				},
			},
		)
		promptDone <- promptErr
	}()
	select {
	case <-manager.accepted:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("queued resumed prompt did not reach provider acceptance")
	}
	if identityReadFailures.Load() == 0 {
		t.Fatal("acceptance callback did not exercise the injected identity read failure")
	}
	if !afterDispatchSawAccepted.Load() {
		t.Fatal("afterDispatch ran before startup ownership transferred")
	}
	assertActiveResumeAttemptAccepted(t, svc, sessionID)

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.CancelAgent(ctx, sessionID) }()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle queued accepted turn")
	}
	select {
	case promptErr := <-promptDone:
		if promptErr != nil {
			t.Fatalf("queued accepted prompt: %v", promptErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("queued accepted prompt did not return after cancellation")
	}

	baseManager.mu.Lock()
	forcedStops := append([]stopAgentCall(nil), baseManager.stopAgentWithReasonArgs...)
	baseManager.mu.Unlock()
	if len(forcedStops) != 0 {
		t.Fatalf("forced execution stops after queued accepted cancellation = %d, want 0: %#v", len(forcedStops), forcedStops)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("queued resume launches after accepted cancellation = %d, want 1", got)
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("load preserved queued execution: %v", err)
	}
	if running == nil || running.AgentExecutionID != executionID {
		t.Fatalf("preserved queued execution = %#v, want %q", running, executionID)
	}

	if _, err := svc.PromptTask(ctx, taskID, sessionID, "queued identity follow-up", "", false, nil, false); err != nil {
		t.Fatalf("queued identity follow-up prompt: %v", err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("queued identity follow-up caused %d resume launches, want 1", got)
	}
	baseManager.mu.Lock()
	promptCalls := append([]promptCall(nil), baseManager.capturedPromptCalls...)
	baseManager.mu.Unlock()
	if len(promptCalls) != 2 {
		t.Fatalf("queued identity prompt calls = %d, want 2: %#v", len(promptCalls), promptCalls)
	}
	for index, call := range promptCalls {
		if call.ExecutionID != executionID {
			t.Errorf("queued identity prompt %d execution ID = %q, want %q", index, call.ExecutionID, executionID)
		}
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.9
func TestResumeTaskSessionAndPrompt_AcceptedTurnCancelPreservesExecution(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-compound-resumed-turn-cancel"
		sessionID   = "session-compound-resumed-turn-cancel"
		executionID = "execution-compound-resumed-turn-cancel"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	var launches atomic.Int32
	agentRunning := &atomic.Bool{}
	baseManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		isAgentReadyFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches.Add(1)
			agentRunning.Store(true)
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
	}
	manager := &acceptedTurnPromptManager{
		mockAgentManager: baseManager,
		accepted:         make(chan struct{}),
		release:          make(chan struct{}),
		firstPromptErr:   fmt.Errorf("prompt abandoned after cancel: %w", lifecycle.ErrCancelEscalated),
	}
	baseManager.cancelAgentFunc = func(context.Context, string) error {
		manager.releaseOnce.Do(func() { close(manager.release) })
		return nil
	}
	baseManager.stopAgentWithReasonFunc = func(_ context.Context, _ string, _ string, _ bool) error {
		agentRunning.Store(false)
		return nil
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	resultDone := make(chan struct {
		result *PromptResult
		err    error
	}, 1)
	go func() {
		result, promptErr := svc.ResumeTaskSessionAndPrompt(
			ctx, taskID, sessionID, "first compound prompt", "", false, nil,
		)
		resultDone <- struct {
			result *PromptResult
			err    error
		}{result: result, err: promptErr}
	}()
	select {
	case <-manager.accepted:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("compound resume did not reach provider acceptance")
	}
	assertActiveResumeAttemptAccepted(t, svc, sessionID)

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.CancelAgent(ctx, sessionID) }()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle the accepted compound turn")
	}
	select {
	case outcome := <-resultDone:
		if outcome.err == nil || !errors.Is(outcome.err, lifecycle.ErrCancelEscalated) {
			t.Fatalf("accepted compound error = %v, want wrapped ErrCancelEscalated", outcome.err)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("accepted compound prompt did not return after cancellation")
	}

	baseManager.mu.Lock()
	forcedStops := append([]stopAgentCall(nil), baseManager.stopAgentWithReasonArgs...)
	baseManager.mu.Unlock()
	if len(forcedStops) != 0 {
		t.Fatalf("forced execution stops after accepted compound cancellation = %d, want 0: %#v", len(forcedStops), forcedStops)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("compound resume launches = %d, want 1", got)
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)

	if _, err := svc.PromptTask(ctx, taskID, sessionID, "compound follow-up", "", false, nil, false); err != nil {
		t.Fatalf("compound follow-up prompt: %v", err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("compound follow-up caused %d resume launches, want 1", got)
	}
}

func TestResumeAttempt_AcceptanceCancellationOrdering(t *testing.T) {
	t.Run("provider acceptance wins", func(t *testing.T) {
		registry := newResumeAttemptRegistry()
		attempt, owner := registry.begin(context.Background(), "task-acceptance-order", "session-acceptance-order")
		if !owner {
			t.Fatal("acceptance-order attempt was not admitted")
		}
		attempt.setExecutionID("execution-acceptance-order")
		if !registry.accept(attempt, "execution-acceptance-order") {
			t.Fatal("provider acceptance did not win before cancellation")
		}
		if registry.invalidate(attempt.sessionID) {
			t.Fatal("accepted attempt remained startup-cancellable")
		}
		if err := attempt.context().Err(); err != nil {
			t.Fatalf("accepted attempt context was cancelled: %v", err)
		}
		if !registry.accept(attempt, "execution-acceptance-order") {
			t.Fatal("repeated acceptance was not idempotent")
		}
		if registry.canCleanup(attempt) {
			t.Fatal("accepted attempt retained startup cleanup authority")
		}
		attempt.finish(registry)
		if registry.canCleanupIdentity(
			attempt.sessionID, "execution-acceptance-order", attempt.identity(),
		) {
			t.Fatal("accepted tombstone retained startup cleanup authority")
		}
	})

	t.Run("startup cancellation wins", func(t *testing.T) {
		registry := newResumeAttemptRegistry()
		attempt, owner := registry.begin(context.Background(), "task-cancellation-order", "session-cancellation-order")
		if !owner {
			t.Fatal("cancellation-order attempt was not admitted")
		}
		attempt.setExecutionID("execution-cancellation-order")
		if !registry.invalidate(attempt.sessionID) {
			t.Fatal("startup attempt was not invalidated")
		}
		if registry.accept(attempt, "execution-cancellation-order") {
			t.Fatal("late provider acceptance revived cancelled startup")
		}
		if attempt.context().Err() == nil {
			t.Fatal("startup cancellation did not cancel the attempt context")
		}
		if !registry.canCleanup(attempt) {
			t.Fatal("cancelled startup lost cleanup authority")
		}
		attempt.finish(registry)
	})
}

func TestResumeAttempt_AcceptedExecutionEventsRemainValid(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-accepted-events"
		sessionID   = "session-accepted-events"
		executionID = "execution-accepted-events"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin accepted event attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID(executionID)
	if !svc.resumeAttemptStore().accept(attempt, executionID) {
		t.Fatal("accepted event attempt was not accepted")
	}
	attemptID := attempt.identity()
	attempt.finish(svc.resumeAttemptStore())

	if !svc.resumeAttemptAllowsExecution(sessionID, executionID, attemptID) {
		t.Fatal("accepted execution event was rejected after attempt completion")
	}
	if svc.resumeAttemptAllowsExecution(sessionID, executionID, "resume-999999") {
		t.Fatal("unknown accepted event identity was allowed")
	}
	svc.handleACPSessionCreated(ctx, watcher.ACPSessionEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        attemptID,
		ACPSessionID:     "accepted-provider-session",
	})
	assertResumeToken(t, repo, sessionID, "accepted-provider-session")
	svc.handleACPSessionCreated(ctx, watcher.ACPSessionEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        "resume-999999",
		ACPSessionID:     "stale-provider-session",
	})
	assertResumeToken(t, repo, sessionID, "accepted-provider-session")
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
func TestResumeAttempt_AcceptedPublicationFailureDoesNotCleanup(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-accepted-publication-failure"
		sessionID   = "session-accepted-publication-failure"
		executionID = "execution-accepted-publication-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	var launches atomic.Int32
	agentRunning := &atomic.Bool{}
	publicationErr := errors.New("accepted turn publication failed")
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		isAgentReadyFn: func(context.Context, string) bool {
			return agentRunning.Load()
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches.Add(1)
			agentRunning.Store(true)
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
		promptResult: &executor.PromptResult{StopReason: "end_turn", AgentMessage: "accepted"},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = failingReservedTurnPublisher{
		TurnService: &repoBackedTurnService{repo: repo},
		err:         publicationErr,
	}

	_, err = svc.promptTask(
		ctx, taskID, sessionID, "accepted publication prompt", "", false, nil, false,
		launchOriginManual,
		promptTaskOptions{reserveTurnUntilDispatch: true},
	)
	if !errors.Is(err, publicationErr) {
		t.Fatalf("promptTask error = %v, want publication error %v", err, publicationErr)
	}
	var accepted interface{ DetachedResumeAccepted() bool }
	if !errors.As(err, &accepted) || !accepted.DetachedResumeAccepted() {
		t.Fatalf("promptTask error = %v, want accepted-dispatch marker", err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("resume launches = %d, want 1", got)
	}
	agentManager.mu.Lock()
	forcedStops := append([]stopAgentCall(nil), agentManager.stopAgentWithReasonArgs...)
	agentManager.mu.Unlock()
	if len(forcedStops) != 0 {
		t.Fatalf("accepted publication failure forced %d startup stops: %#v", len(forcedStops), forcedStops)
	}
	if pending := svc.reservedPromptTurnID(sessionID); pending != "" {
		t.Fatalf("accepted publication failure left turn rollback-eligible: %q", pending)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
func TestResumeAttempt_ModelSwitchFallbackTransfersAcceptance(t *testing.T) {
	ctx := context.Background()
	const (
		taskID       = "task-model-switch-acceptance"
		sessionID    = "session-model-switch-acceptance"
		oldExecution = "execution-model-switch-old"
		newExecution = "execution-model-switch-new"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session model: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, oldExecution)

	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: newExecution, Status: "ready",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: newExecution}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin model-switch attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID(oldExecution)
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	result, handled, err := svc.trySwitchModelForPrompt(
		ctx, taskID, sessionID, "new-model", "model-switch prompt", session,
		&foregroundDispatch{}, attempt,
	)
	if err != nil {
		t.Fatalf("model-switch fallback: %v", err)
	}
	if !handled {
		t.Fatal("model-switch fallback was not handled")
	}
	if result == nil || result.StopReason != "model_switched" {
		t.Fatalf("model-switch result = %#v, want model_switched", result)
	}
	manager.mu.Lock()
	onDispatched := manager.initialPromptDispatchCallback
	manager.mu.Unlock()
	if onDispatched == nil {
		t.Fatal("model-switch fallback did not register its initial-prompt acceptance callback")
	}
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	acceptedBeforeDispatch := attempt.accepted
	registry.mu.Unlock()
	if acceptedBeforeDispatch {
		t.Fatal("model-switch fallback transferred startup ownership before provider acceptance")
	}
	onDispatched()
	if got := attempt.execution(); got != newExecution {
		t.Fatalf("resume attempt execution = %q, want %q", got, newExecution)
	}
	registry.mu.Lock()
	accepted := attempt.accepted
	registry.mu.Unlock()
	if !accepted {
		t.Fatal("model-switch fallback did not transfer startup ownership")
	}
	if err := attempt.validate(registry); err != nil {
		t.Fatalf("accepted model-switch attempt validation: %v", err)
	}
}

func assertActiveResumeAttemptAccepted(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	if !activeResumeAttemptAccepted(svc, sessionID) {
		t.Fatalf("provider acceptance did not transfer startup ownership for %s", sessionID)
	}
}

func activeResumeAttemptAccepted(svc *Service, sessionID string) bool {
	attempt, active := svc.resumeAttemptStore().current(sessionID)
	if !active || attempt == nil {
		return false
	}
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return attempt.accepted
}
