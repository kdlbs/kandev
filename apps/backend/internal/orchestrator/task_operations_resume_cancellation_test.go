package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestResumeAttemptCancellationInvalidatesOnlyTheCancelledAttempt(t *testing.T) {
	registry := newResumeAttemptRegistry()
	first, firstOwner := registry.begin(context.Background(), "task-1", "session-1")
	if !firstOwner {
		t.Fatal("first resume attempt was not admitted as the owner")
	}

	registry.invalidate("session-1")
	second, secondOwner := registry.begin(context.Background(), "task-1", "session-1")
	if !secondOwner {
		t.Fatal("retry resume attempt was not admitted after cancellation")
	}

	if err := first.validate(registry); !errors.Is(err, ErrResumeAttemptCancelled) {
		t.Fatalf("cancelled attempt validation error = %v, want ErrResumeAttemptCancelled", err)
	}
	if err := second.validate(registry); err != nil {
		t.Fatalf("new attempt validation error = %v", err)
	}

	first.finish(registry)
	if current, ok := registry.current("session-1"); !ok || current != second {
		t.Fatal("late completion of the old attempt replaced the active retry")
	}
	second.finish(registry)
}

func TestResumeAttemptRegistryRetainsCancelledAttemptOnce(t *testing.T) {
	registry := newResumeAttemptRegistry()
	first, owner := registry.begin(context.Background(), "task-retain-once", "session-retain-once")
	if !owner {
		t.Fatal("first resume attempt was not admitted as the owner")
	}
	if !registry.invalidate(first.sessionID) {
		t.Fatal("first resume attempt was not cancelled")
	}
	second, owner := registry.begin(context.Background(), first.taskID, first.sessionID)
	if !owner {
		t.Fatal("replacement resume attempt was not admitted")
	}
	if got := len(registry.tombstones[first.sessionID]); got != 1 {
		t.Fatalf("tombstones after replacement = %d, want 1", got)
	}
	first.finish(registry)
	if got := len(registry.tombstones[first.sessionID]); got != 1 {
		t.Fatalf("tombstones after old owner returned = %d, want 1", got)
	}
	second.finish(registry)
}

func TestResumeAttemptCancellationInterruptsDetachedContext(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	registry := newResumeAttemptRegistry()
	attempt, owner := registry.begin(requestCtx, "task-1", "session-1")
	if !owner {
		t.Fatal("resume attempt was not admitted as the owner")
	}

	cancelRequest()
	select {
	case <-attempt.context().Done():
		t.Fatal("browser request cancellation cancelled the accepted resume attempt")
	default:
	}

	registry.invalidate("session-1")
	select {
	case <-attempt.context().Done():
	case <-context.Background().Done():
		t.Fatal("unreachable")
	}
	if err := attempt.validate(registry); !errors.Is(err, ErrResumeAttemptCancelled) {
		t.Fatalf("cancelled attempt validation error = %v, want ErrResumeAttemptCancelled", err)
	}
	attempt.finish(registry)
}

func TestResumeAttemptRegistryFencesEvictedCancelledIdentities(t *testing.T) {
	const sessionID = "session-evicted-attempt"
	registry := newResumeAttemptRegistry()
	first, owner := registry.begin(context.Background(), "task-evicted-attempt", sessionID)
	if !owner {
		t.Fatal("first resume attempt was not admitted as the owner")
	}
	first.setExecutionID("execution-reused")
	firstID := first.identity()
	if !registry.invalidate(sessionID) {
		t.Fatal("first resume attempt was not cancelled")
	}
	first.finish(registry)

	for i := 0; i < maxResumeAttemptTombstones; i++ {
		attempt, admitted := registry.begin(context.Background(), "task-evicted-attempt", sessionID)
		if !admitted {
			t.Fatalf("replacement resume attempt %d was not admitted", i)
		}
		if !registry.invalidate(sessionID) {
			t.Fatalf("replacement resume attempt %d was not cancelled", i)
		}
		attempt.setExecutionID("execution-reused")
		attempt.finish(registry)
	}

	if got := len(registry.tombstones[sessionID]); got != maxResumeAttemptTombstones {
		t.Fatalf("tombstone count = %d, want bounded count %d", got, maxResumeAttemptTombstones)
	}
	service := &Service{resumeAttempts: registry}
	if service.resumeAttemptAllowsExecution(sessionID, "execution-reused", firstID) {
		t.Fatal("an evicted cancelled attempt regained execution ownership")
	}
	if service.resumeAttemptAllowsExecution(sessionID, "execution-reused", "resume-18446744073709551615") {
		t.Fatal("an unknown numeric callback identity bypassed recovery history fencing")
	}
	if registry.canCleanup(first) {
		t.Fatal("an evicted cancelled attempt regained cleanup ownership")
	}
}

func TestResumeAttemptBindsFirstCallbackExecutionAndFencesUntaggedCompletion(t *testing.T) {
	const sessionID = "session-first-callback-execution"
	registry := newResumeAttemptRegistry()
	attempt, owner := registry.begin(context.Background(), "task-first-callback-execution", sessionID)
	if !owner {
		t.Fatal("resume attempt was not admitted as the owner")
	}
	service := &Service{resumeAttempts: registry}
	attemptID := attempt.identity()

	if !service.resumeAttemptAllowsExecution(sessionID, "execution-first", attemptID) {
		t.Fatal("the current attempt did not bind its first callback execution")
	}
	if got := attempt.execution(); got != "execution-first" {
		t.Fatalf("bound execution = %q, want execution-first", got)
	}
	if service.resumeAttemptAllowsExecution(sessionID, "execution-other", attemptID) {
		t.Fatal("the current attempt accepted a callback from a different execution")
	}

	attempt.finish(registry)
	if service.resumeAttemptAllowsExecution(sessionID, "execution-first") {
		t.Fatal("an untagged callback was accepted after the attempt finished")
	}
}

func TestResumeTaskSessionAndPrompt_CancelAtContinuationBarrierDoesNotDispatchPrompt(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-resume-prompt-barrier"
		sessionID   = "session-resume-prompt-barrier"
		executionID = "execution-resume-prompt-barrier"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-before-resume")

	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn: func(context.Context, string) bool {
			return true
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			// The real launch path persists STARTING before it calls the runtime.
			// Marking the fake runtime ready here makes the readiness wait
			// deterministic without a timer or a second goroutine.
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
		promptResult: &executor.PromptResult{
			StopReason:   "end_turn",
			AgentMessage: "should not be dispatched",
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})

	continuationReached := make(chan struct{})
	allowPrompt := make(chan struct{})
	resumeDone := make(chan error, 1)
	go func() {
		_, resumeErr := svc.resumeTaskSessionWithContinuation(
			ctx,
			taskID,
			sessionID,
			executor.ResumeOptions{},
			func(resumeCtx context.Context, attempt *resumeAttempt, _ *executor.TaskExecution) error {
				close(continuationReached)
				<-allowPrompt
				_, promptErr := svc.promptTask(
					resumeCtx,
					taskID,
					sessionID,
					"cancelled prompt",
					"",
					false,
					nil,
					false,
					launchOriginManual,
					promptTaskOptions{resumeAttempt: attempt},
				)
				return promptErr
			},
		)
		resumeDone <- resumeErr
	}()

	select {
	case <-continuationReached:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("resume did not reach the prompt continuation barrier")
	}

	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- svc.CancelAgent(ctx, sessionID)
	}()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle while the continuation was waiting")
	}

	close(allowPrompt)
	select {
	case resumeErr := <-resumeDone:
		if !errors.Is(resumeErr, ErrResumeAttemptCancelled) {
			t.Fatalf("resume continuation error = %v, want ErrResumeAttemptCancelled", resumeErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("resume did not return after the cancelled continuation was released")
	}

	agentManager.mu.Lock()
	promptCount := len(agentManager.capturedPrompts)
	agentManager.mu.Unlock()
	if promptCount != 0 {
		t.Fatalf("cancelled continuation dispatched %d prompts", promptCount)
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
}

func TestPromptTask_ResumeAttemptKeepsCancellationOutOfAcceptanceCallback(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-resume-acceptance-barrier"
		sessionID = "session-resume-acceptance-barrier"
		execID    = "execution-resume-acceptance-barrier"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, execID)
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		promptResult: &executor.PromptResult{
			StopReason:   "end_turn",
			AgentMessage: "accepted",
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})

	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin resume attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID(execID)
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	acceptanceEntered := make(chan struct{})
	allowAcceptance := make(chan struct{})
	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.promptTask(
			ctx,
			taskID,
			sessionID,
			"accepted prompt",
			"",
			false,
			nil,
			false,
			launchOriginManual,
			promptTaskOptions{
				resumeAttempt: attempt,
				afterDispatch: func() error {
					close(acceptanceEntered)
					<-allowAcceptance
					return nil
				},
			},
		)
		promptDone <- promptErr
	}()
	select {
	case <-acceptanceEntered:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("prompt did not reach the provider acceptance callback")
	}

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.CancelAgent(ctx, sessionID) }()
	select {
	case cancelErr := <-cancelDone:
		t.Fatalf("CancelAgent settled before acceptance callback completed: %v", cancelErr)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowAcceptance)
	select {
	case promptErr := <-promptDone:
		if promptErr != nil && !errors.Is(promptErr, ErrResumeAttemptCancelled) {
			t.Fatalf("prompt after acceptance barrier: %v", promptErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("prompt did not return after acceptance callback completed")
	}
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle after acceptance callback completed")
	}

	agentManager.mu.Lock()
	promptCount := len(agentManager.capturedPrompts)
	agentManager.mu.Unlock()
	if promptCount != 1 {
		t.Fatalf("accepted prompt count = %d, want 1", promptCount)
	}
}

func TestResumeTaskSession_CancelDuringReadyWaitReturnsTypedCancellation(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-resume-ready-cancel"
		sessionID   = "session-resume-ready-cancel"
		executionID = "execution-resume-ready-cancel"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-before-resume")

	readyChecked := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn: func(context.Context, string) bool {
			select {
			case readyChecked <- struct{}{}:
			default:
			}
			return false
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpdateTaskSessionState(
				context.Background(), req.SessionID, models.TaskSessionStateWaitingForInput, "",
			); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})

	resumeDone := make(chan error, 1)
	go func() {
		_, resumeErr := svc.ResumeTaskSession(ctx, taskID, sessionID)
		resumeDone <- resumeErr
	}()
	select {
	case <-readyChecked:
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("ResumeTaskSession did not reach the readiness wait")
	}

	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- svc.CancelAgent(ctx, sessionID)
	}()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent: %v", cancelErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("CancelAgent did not settle the readiness wait")
	}

	select {
	case resumeErr := <-resumeDone:
		if !errors.Is(resumeErr, ErrResumeAttemptCancelled) {
			t.Fatalf("ResumeTaskSession error = %v, want ErrResumeAttemptCancelled", resumeErr)
		}
	case <-time.After(resumeCancellationTestTimeout(t)):
		t.Fatal("ResumeTaskSession did not return after cancellation")
	}
}

func TestResumeCallbacksRejectCancelledAttemptBeforeAndAfterReplacement(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-resume-callbacks"
		sessionID   = "session-resume-callbacks"
		executionID = "execution-reused"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	registry := svc.resumeAttemptStore()

	first, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin first resume attempt: attempt=%v owner=%v err=%v", first, owner, err)
	}
	first.setExecutionID(executionID)
	firstID := first.identity()
	svc.invalidateResumeAttempt(sessionID)

	// A cancellation leaves the attempt visible until its owner settles. A
	// delayed boot-success callback must already be rejected in that window.
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        firstID,
	})
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateStarting)
	first.finish(registry)
	// The callback is still stale after the cancelled owner has settled and
	// removed itself from the active registry.
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        firstID,
	})
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateStarting)

	second, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin replacement resume attempt: attempt=%v owner=%v err=%v", second, owner, err)
	}
	second.setExecutionID(executionID)
	secondID := second.identity()
	t.Cleanup(func() {
		first.finish(registry)
		second.finish(registry)
	})

	// The execution ID is intentionally reused. Attempt identity, rather than
	// the current registry entry or the execution ID alone, must fence the old
	// callback after the replacement is installed.
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        firstID,
	})
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateStarting)

	// The replacement callback is accepted and settles startup. An unrelated
	// queued prompt remains parked because Auto-run is explicitly OFF.
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, false); err != nil {
		t.Fatalf("disable queue auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessage(
		ctx, sessionID, taskID, "unrelated queued work", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue unrelated work: %v", err)
	}
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        secondID,
	})
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status.AutoRun || len(status.Entries) != 1 {
		t.Fatalf("queue status after accepted replacement boot = %+v, want Auto-run OFF with one parked entry", status)
	}
}

func TestResumeCallbacksRejectOldTokenAndFailureOnReusedExecution(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-resume-token-failure"
		sessionID   = "session-resume-token-failure"
		executionID = "execution-reused-token"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	registry := svc.resumeAttemptStore()

	first, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin first resume attempt: attempt=%v owner=%v err=%v", first, owner, err)
	}
	first.setExecutionID(executionID)
	firstID := first.identity()
	svc.invalidateResumeAttempt(sessionID)

	oldCallback := func() {
		svc.handleACPSessionCreated(ctx, watcher.ACPSessionEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
			AttemptID:        firstID,
			ACPSessionID:     "old-acp-session",
		})
		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
			AttemptID:        firstID,
			ErrorMessage:     "old failure",
		})
	}
	oldCallback()
	assertResumeToken(t, repo, sessionID, "")
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateRunning)
	first.finish(registry)
	// The finished tombstone must continue to reject both callback types before
	// a replacement attempt is installed.
	oldCallback()
	assertResumeToken(t, repo, sessionID, "")
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateRunning)

	second, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin replacement resume attempt: attempt=%v owner=%v err=%v", second, owner, err)
	}
	second.setExecutionID(executionID)
	secondID := second.identity()
	t.Cleanup(func() {
		first.finish(registry)
		second.finish(registry)
	})
	oldCallback()
	assertResumeToken(t, repo, sessionID, "")
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateRunning)

	// The current attempt can publish its token on the same execution ID.
	svc.handleACPSessionCreated(ctx, watcher.ACPSessionEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        secondID,
		ACPSessionID:     "replacement-acp-session",
	})
	assertResumeToken(t, repo, sessionID, "replacement-acp-session")
}

func TestResumeAttemptServiceFencesBrowserDisconnectAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	attempt, owner, err := svc.beginResumeAttempt(ctx, "task-browser", "session-browser")
	if err != nil || !owner {
		t.Fatalf("begin browser resume attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	cancel()
	if attempt.context().Err() != nil {
		t.Fatal("browser disconnect cancelled the accepted detached resume attempt")
	}
	if !svc.resumeAttemptAllowsExecution("session-browser", "execution-browser", attempt.identity()) {
		t.Fatal("browser disconnect incorrectly invalidated the accepted attempt")
	}
	svc.invalidateResumeAttempt("session-browser")
	attempt.finish(svc.resumeAttemptStore())

	shutdownAttempt, owner, err := svc.beginResumeAttempt(context.Background(), "task-shutdown", "session-shutdown")
	if err != nil || !owner {
		t.Fatalf("begin shutdown resume attempt: attempt=%v owner=%v err=%v", shutdownAttempt, owner, err)
	}
	svc.cancelResumeAttempts()
	if shutdownAttempt.context().Err() == nil {
		t.Fatal("service shutdown did not cancel the in-flight resume attempt")
	}
	if svc.resumeAttemptAllowsExecution("session-shutdown", "execution-shutdown", shutdownAttempt.identity()) {
		t.Fatal("shutdown-cancelled callback retained execution ownership")
	}
	shutdownAttempt.finish(svc.resumeAttemptStore())
}

func TestCancelledResumeCleanupCannotStopSameExecutionOwnedByReplacement(t *testing.T) {
	repo := setupTestRepo(t)
	agentManager := &mockAgentManager{}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)

	first, owner, err := svc.beginResumeAttempt(context.Background(), "task-cleanup", "session-cleanup")
	if err != nil || !owner {
		t.Fatalf("begin first cleanup attempt: attempt=%v owner=%v err=%v", first, owner, err)
	}
	first.setExecutionID("execution-reused-cleanup")
	svc.invalidateResumeAttempt("session-cleanup")
	second, owner, err := svc.beginResumeAttempt(context.Background(), "task-cleanup", "session-cleanup")
	if err != nil || !owner {
		t.Fatalf("begin replacement cleanup attempt: attempt=%v owner=%v err=%v", second, owner, err)
	}
	second.setExecutionID("execution-reused-cleanup")
	t.Cleanup(func() {
		first.finish(svc.resumeAttemptStore())
		second.finish(svc.resumeAttemptStore())
	})

	// The first attempt is cancelled after the replacement has reused the same
	// execution ID. Its cleanup callback must not stop the replacement runtime.
	svc.cleanupCancelledResumeAttempt(first)
	svc.cleanupCancelledResumeExecution(
		context.Background(), first.taskID, first.sessionID, first.execution(), first.identity(),
	)
	agentManager.mu.Lock()
	stopCalls := len(agentManager.stopAgentWithReasonArgs)
	agentManager.mu.Unlock()
	if stopCalls != 0 {
		t.Fatalf("cancelled predecessor cleanup stopped %d replacement executions", stopCalls)
	}
}

func TestCancelledResumeCleanupReleasesFailedTeardownClaimForRetry(t *testing.T) {
	repo := setupTestRepo(t)
	stopErr := errors.New("runtime teardown failed")
	var stopCalls int
	agentManager := &mockAgentManager{
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls++
			if stopCalls == 1 {
				return stopErr
			}
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)
	attempt, owner, err := svc.beginResumeAttempt(context.Background(), "task-retry-cleanup", "session-retry-cleanup")
	if err != nil || !owner {
		t.Fatalf("begin resume attempt: owner=%v err=%v", owner, err)
	}
	attempt.setExecutionID("execution-retry-cleanup")
	if !svc.resumeAttemptStore().invalidate("session-retry-cleanup") {
		t.Fatal("resume attempt was not invalidated")
	}
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	svc.cleanupCancelledResumeAttempt(attempt)
	key := terminalExecutionKey("session-retry-cleanup", "execution-retry-cleanup")
	if _, claimed := svc.executionTeardownClaims.Load(key); claimed {
		t.Fatal("failed runtime teardown retained its claim and blocked retry")
	}
	if svc.cancelledResumeTeardownForSession("session-retry-cleanup") != nil {
		t.Fatal("failed runtime teardown retained its admission fence and blocked retry")
	}

	svc.cleanupCancelledResumeAttempt(attempt)
	if stopCalls != 2 {
		t.Fatalf("runtime stop calls = %d after retry, want 2", stopCalls)
	}
	claim, found := svc.executionTeardownClaimFor("session-retry-cleanup", "execution-retry-cleanup")
	if !found || !claim.cleanupCompleted {
		t.Fatalf("successful retry claim = %+v, found=%v, want completed", claim, found)
	}
}

func TestCancelledResumeCleanupRevalidatesAttemptAfterGuardWait(t *testing.T) {
	const (
		taskID      = "task-cleanup-guard-recheck"
		sessionID   = "session-cleanup-guard-recheck"
		executionID = "execution-reused-guard-recheck"
	)
	stopCalls := 0
	agentManager := &mockAgentManager{
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls++
			return nil
		},
	}
	svc := newCoordinatorStopTestService(setupTestRepo(t), newMockTaskRepo(), agentManager)
	registry := svc.resumeAttemptStore()
	first, owner := registry.begin(context.Background(), taskID, sessionID)
	if !owner {
		t.Fatal("cancelled attempt did not become startup owner")
	}
	first.setExecutionID(executionID)
	if !registry.invalidate(sessionID) {
		t.Fatal("resume attempt was not invalidated")
	}
	guard, releaseGuard := svc.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	guardLocked := true
	var replacement *resumeAttempt
	done := make(chan struct{})
	go func() {
		svc.cleanupCancelledResumeExecution(context.Background(), taskID, sessionID, executionID, first.identity())
		close(done)
	}()
	t.Cleanup(func() {
		if guardLocked {
			guard.Unlock()
		}
		releaseGuard()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("cancelled cleanup did not settle")
		}
		first.finish(registry)
		if replacement != nil {
			replacement.finish(registry)
		}
	})
	waitForCancelInFlightMutexWaiters(t, guard, 1)
	replacement, owner = registry.begin(context.Background(), taskID, sessionID)
	if !owner {
		t.Fatal("same-execution replacement did not become startup owner")
	}
	replacement.setExecutionID(executionID)
	guard.Unlock()
	guardLocked = false
	releaseGuard()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled cleanup remained blocked after guard release")
	}
	if stopCalls != 0 {
		t.Fatalf("stale cancelled cleanup stopped the replacement %d times", stopCalls)
	}
	if _, claimed := svc.executionTeardownClaims.Load(terminalExecutionKey(sessionID, executionID)); claimed {
		t.Fatal("stale cancelled cleanup claimed its replacement execution")
	}
}

func TestResumeAttemptWaitsForCancelledStartupTeardownAfterClaim(t *testing.T) {
	const (
		taskID      = "task-cleanup-before-retry"
		sessionID   = "session-cleanup-before-retry"
		executionID = "execution-cleanup-before-retry"
	)
	stopEntered := make(chan struct{})
	allowStopReturn := make(chan struct{})
	var releaseStop sync.Once
	stopCalls := 0
	agentManager := &mockAgentManager{
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls++
			close(stopEntered)
			<-allowStopReturn
			return nil
		},
	}
	svc := newCoordinatorStopTestService(setupTestRepo(t), newMockTaskRepo(), agentManager)
	registry := svc.resumeAttemptStore()
	first, owner := registry.begin(context.Background(), taskID, sessionID)
	if !owner {
		t.Fatal("cancelled attempt did not become startup owner")
	}
	first.setExecutionID(executionID)
	if !registry.invalidate(sessionID) {
		t.Fatal("resume attempt was not invalidated")
	}

	cleanupDone := make(chan struct{})
	go func() {
		svc.cleanupCancelledResumeAttempt(first)
		close(cleanupDone)
	}()
	type beginResult struct {
		attempt *resumeAttempt
		owner   bool
		err     error
	}
	beginDone := make(chan beginResult, 1)
	beginWorkerDone := make(chan struct{})
	probeDone := make(chan struct{})
	probeTeardown := make(chan *cancelledResumeTeardown, 1)
	beginStarted := false
	beginResultReceived := false
	probeStarted := false
	var replacement *resumeAttempt
	var guard *cancelInFlightMutex
	var releaseGuard func()
	guardLocked := false
	t.Cleanup(func() {
		releaseStop.Do(func() { close(allowStopReturn) })
		if guardLocked {
			guard.Unlock()
		}
		if releaseGuard != nil {
			releaseGuard()
		}
		if probeStarted {
			select {
			case <-probeDone:
			case <-time.After(5 * time.Second):
				t.Error("session guard probe did not settle")
			}
		}
		select {
		case <-cleanupDone:
		case <-time.After(5 * time.Second):
			t.Error("cancelled startup teardown did not settle")
		}
		if beginStarted {
			select {
			case <-beginWorkerDone:
			case <-time.After(5 * time.Second):
				t.Error("resume admission worker did not settle")
			}
			if !beginResultReceived {
				select {
				case result := <-beginDone:
					replacement = result.attempt
					beginResultReceived = true
				case <-time.After(5 * time.Second):
					t.Error("resume admission result was not delivered")
				}
			}
		}
		first.finish(registry)
		if replacement != nil {
			replacement.finish(registry)
		}
	})
	select {
	case <-stopEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled startup teardown did not reach runtime stop")
	}
	if attempt, owner, err := svc.beginResumeAttempt(
		withCancelInFlightGuardHeld(context.Background()), taskID, sessionID,
	); !errors.Is(err, ErrResumeAttemptCancelled) || owner || attempt != nil {
		t.Fatalf("guard-held resume admission during teardown = (attempt=%v owner=%v err=%v), want closed admission", attempt, owner, err)
	}

	guard, releaseGuard = svc.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	guardLocked = true
	beginStarted = true
	go func() {
		defer close(beginWorkerDone)
		attempt, owner, err := svc.beginResumeAttempt(context.Background(), taskID, sessionID)
		beginDone <- beginResult{attempt: attempt, owner: owner, err: err}
	}()
	waitForCancelInFlightMutexWaiters(t, guard, 1)
	probeStarted = true
	go func() {
		guard.Lock()
		teardown := svc.cancelledResumeTeardownForSession(sessionID)
		guard.Unlock()
		probeTeardown <- teardown
		close(probeDone)
	}()
	waitForCancelInFlightMutexWaiters(t, guard, 2)
	guard.Unlock()
	guardLocked = false
	releaseGuard()
	releaseGuard = nil
	select {
	case <-probeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("resume admission did not yield the session guard")
	}
	if teardown := <-probeTeardown; teardown == nil {
		t.Fatal("guard probe did not observe the active cancelled-startup fence")
	}
	current, currentExists := registry.current(sessionID)
	if !currentExists || current != first {
		replacement = current
		t.Fatalf("resume attempt replaced cancelled owner before runtime stop returned: current=%p first=%p", current, first)
	}
	select {
	case result := <-beginDone:
		beginResultReceived = true
		replacement = result.attempt
		t.Fatalf("resume attempt was admitted before cancelled startup teardown returned: owner=%v err=%v", result.owner, result.err)
	default:
	}

	releaseStop.Do(func() { close(allowStopReturn) })
	select {
	case <-cleanupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled startup teardown did not finish after stop release")
	}
	select {
	case result := <-beginDone:
		beginResultReceived = true
		replacement = result.attempt
		if result.err != nil || !result.owner || replacement == nil {
			t.Fatalf("resume after teardown = (attempt=%v owner=%v err=%v), want new owner", replacement, result.owner, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resume admission did not continue after cancelled startup teardown")
	}
	if stopCalls != 1 {
		t.Fatalf("cancelled startup runtime stop calls = %d, want 1", stopCalls)
	}
}

func TestHasActiveSessionRecoveryForFailureRequiresMatchingIdentity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-recovery-correlation"
		sessionID = "session-recovery-correlation"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	historical := models.LastAgentError{
		Message:          "historical bootstrap failure",
		AgentExecutionID: "execution-old",
		ExecutionID:      "execution-old",
		AttemptID:        "1",
		StampValue:       "failure-old",
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, historical); err != nil {
		t.Fatalf("store historical recovery error: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	unrelated := &SessionRecoveryFailure{
		Err: errors.New("new resume failed"),
		Identity: SessionRecoveryIdentity{
			AttemptID:   "2",
			ExecutionID: "execution-new",
		},
	}
	if svc.HasActiveSessionRecoveryForFailure(ctx, taskID, sessionID, unrelated) {
		t.Fatal("historical recovery error suppressed an unrelated current failure")
	}

	matching := &SessionRecoveryFailure{
		Err: errors.New("same resume failed"),
		Identity: SessionRecoveryIdentity{
			AttemptID:   historical.AttemptID,
			ExecutionID: historical.ExecutionID,
			ErrorStamp:  historical.Stamp(),
		},
	}
	if !svc.HasActiveSessionRecoveryForFailure(ctx, taskID, sessionID, matching) {
		t.Fatal("matching recovery error did not retain ownership of the current failure")
	}
}

func assertResumeSessionState(t *testing.T, repo interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}, sessionID string, want models.TaskSessionState) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load session %s: %v", sessionID, err)
	}
	if session.State != want {
		t.Fatalf("session %s state = %q, want %q", sessionID, session.State, want)
	}
}

func assertResumeToken(t *testing.T, repo interface {
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
}, sessionID, want string) {
	t.Helper()
	running, err := repo.GetExecutorRunningBySessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load executor row for %s: %v", sessionID, err)
	}
	if running.ResumeToken != want {
		t.Fatalf("resume token for %s = %q, want %q", sessionID, running.ResumeToken, want)
	}
}

func resumeCancellationTestTimeout(t *testing.T) time.Duration {
	t.Helper()
	const safetyMargin = time.Second
	const defaultTimeout = 10 * time.Second
	if deadline, ok := t.Deadline(); ok {
		if remaining := time.Until(deadline) - safetyMargin; remaining > 0 && remaining < defaultTimeout {
			return remaining
		}
	}
	return defaultTimeout
}
