package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRelaunchDynamicTaskAfterFailure_DoesNotLaunchSuccessorWhenStopFails(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-stop-failure"
		sessionID   = "session-dynamic-stop-failure"
		executionID = "execution-dynamic-stop-failure"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	stopErr := errors.New("runtime teardown failed")
	agentManager := &mockAgentManager{
		stopAgentWithReasonErr: stopErr,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	relaunched := svc.relaunchDynamicTaskAfterFailure(
		ctx,
		watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
		},
		"fallback-profile",
		launchOriginAutomatic,
	)

	if relaunched {
		t.Fatal("relaunchDynamicTaskAfterFailure returned success after predecessor stop failed")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING while predecessor teardown is unresolved", session.State)
	}
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after stop failure", len(agentManager.startAgentProcessCalls))
	}
	if len(agentManager.stopAgentWithReasonArgs) != 1 {
		t.Fatalf("stop calls = %d, want 1", len(agentManager.stopAgentWithReasonArgs))
	}
	if agentManager.stopAgentWithReasonArgs[0] != (stopAgentCall{
		ExecutionID: executionID,
		Reason:      "dynamic route fallback",
		Force:       true,
	}) {
		t.Fatalf("unexpected stop call: %#v", agentManager.stopAgentWithReasonArgs[0])
	}
}

func TestLaunchPreparedDynamicRelaunchDispatchesPromptToResumedCandidate(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-retry-prompt"
		sessionID   = "session-dynamic-retry-prompt"
		executionID = "execution-dynamic-retry-prompt"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "dynamic-profile"
	session.ExecutionProfileID = "candidate-profile"
	session.ErrorMessage = "previous provider failure"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn:       func(context.Context, string) bool { return true },
		isAgentReadyFn:         func(context.Context, string) bool { return true },
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.turnService = &repoTurnService{repo: repo}
	prompt := capturedPrompt{text: "retry the accepted prompt"}
	svc.lastTurnPrompt.Store(sessionID, prompt)
	task, err := svc.scheduler.GetTask(ctx, taskID)
	if err != nil || task == nil {
		t.Fatalf("load scheduled task: task=%#v err=%v", task, err)
	}
	data := watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		AgentProfileID: "dynamic-profile",
	}
	preparedTask, preparedSession, preparedPrompt, launchMode, ok := svc.prepareDynamicRelaunchAfterFailure(
		ctx, data, launchOriginManual,
	)
	if !ok {
		t.Fatal("prepareDynamicRelaunchAfterFailure refused the current session")
	}
	releaseRouteAction := svc.acquireRouteActionOperationLock(sessionID)
	defer releaseRouteAction()

	outcome := svc.launchPreparedDynamicRelaunch(
		ctx,
		data,
		preparedTask,
		preparedSession,
		preparedPrompt,
		"candidate-profile",
		launchMode,
		launchOriginManual,
		nil,
	)
	if outcome != dynamicRelaunchSucceeded {
		t.Fatalf("dynamic relaunch outcome = %v, want a successfully dispatched prompt", outcome)
	}

	agentManager.mu.Lock()
	defer agentManager.mu.Unlock()
	if len(agentManager.capturedPromptCalls) != 1 {
		t.Fatalf("provider prompt calls = %#v, want exactly one retry prompt", agentManager.capturedPromptCalls)
	}
	call := agentManager.capturedPromptCalls[0]
	if call.Prompt != "retry the accepted prompt" || !call.DispatchOnly {
		t.Fatalf("provider retry prompt = %+v, want accepted content dispatched once", call)
	}
	updated, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload session after retry: %v", err)
	}
	if updated.State != models.TaskSessionStateRunning || updated.ErrorMessage != "" {
		t.Fatalf("session after retry = state %q error %q, want running with cleared prior error", updated.State, updated.ErrorMessage)
	}
}

func TestDynamicRelaunchCreatedSessionCarriesRecoveryAttemptIdentity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-dynamic-retry-attempt"
		sessionID = "session-dynamic-retry-attempt"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "dynamic-profile"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	processStarted := make(chan struct{})
	releaseProcessStart := make(chan struct{})
	var releaseProcessOnce sync.Once
	asyncCleanupDone := make(chan struct{})
	var launchAttemptID string
	agentManager := &mockAgentManager{
		launchAgentFunc: func(lctx context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchAttemptID = executor.ResumeAttemptIDFromContext(lctx)
			return &executor.LaunchAgentResponse{AgentExecutionID: "dynamic-successor-execution"}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			close(processStarted)
			<-releaseProcessStart
			return nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.executor.SetOnCancelledResumeExecutionCleanup(svc.cleanupCancelledResumeExecution)
	startFailed := svc.handleAgentProcessStartFailed
	svc.executor.SetOnAgentProcessStartFailed(func(ctx context.Context, callbackTaskID, callbackSessionID, executionID string, err error) {
		startFailed(ctx, callbackTaskID, callbackSessionID, executionID, err)
		close(asyncCleanupDone)
	})
	t.Cleanup(func() {
		releaseProcessOnce.Do(func() { close(releaseProcessStart) })
		select {
		case <-asyncCleanupDone:
		case <-time.After(5 * time.Second):
			t.Error("asynchronous cancelled-start cleanup did not finish")
		}
	})
	prior, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin prior resume attempt: owner=%v err=%v", owner, err)
	}
	prior.setExecutionID("prior-manual-retry-execution")
	if !svc.resumeAttemptStore().accept(prior, "prior-manual-retry-execution") {
		t.Fatal("accept prior manual retry attempt")
	}
	prior.finish(svc.resumeAttemptStore())

	execution, err := svc.startDynamicRelaunchCreatedSession(
		ctx, taskID, session, capturedPrompt{text: "retry the accepted prompt"},
	)
	if err != nil {
		t.Fatalf("start dynamic successor: %v", err)
	}
	if execution == nil || execution.AgentExecutionID != "dynamic-successor-execution" {
		t.Fatalf("dynamic successor execution = %+v, want mock successor execution", execution)
	}
	select {
	case <-processStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous process startup did not reach its barrier")
	}
	if launchAttemptID == "" {
		t.Fatal("dynamic successor launch omitted its recovery attempt identity")
	}
	if !svc.resumeAttemptAllowsExecution(sessionID, execution.AgentExecutionID, launchAttemptID) {
		t.Fatal("dynamic successor callback was rejected after a prior manual resume")
	}
	if svc.resumeAttemptAllowsExecution(sessionID, execution.AgentExecutionID) {
		t.Fatal("untagged dynamic successor callback was accepted after recovery history")
	}

	agentManager.mu.Lock()
	initialPromptAccepted := agentManager.initialPromptDispatchCallback
	agentManager.mu.Unlock()
	if initialPromptAccepted == nil {
		t.Fatal("dynamic successor launch did not register its initial-prompt acceptance callback")
	}
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	attempt := registry.attempts[sessionID]
	registry.mu.Unlock()
	if attempt == nil || attempt.execution() != execution.AgentExecutionID {
		t.Fatalf("dynamic successor startup owner = %+v, want active owner for %q", attempt, execution.AgentExecutionID)
	}
	if err := registry.invalidate(sessionID); !err {
		t.Fatal("cancel dynamic successor before initial prompt acceptance")
	}
	if attempt.ctx.Err() == nil {
		t.Fatal("cancelling dynamic successor did not cancel its startup context")
	}
	svc.cleanupCancelledResumeAttempt(attempt)
	initialPromptAccepted()
	if registry.accept(attempt, execution.AgentExecutionID) {
		t.Fatal("late initial-prompt acceptance transferred ownership after cancellation")
	}
	releaseProcessOnce.Do(func() { close(releaseProcessStart) })
	select {
	case <-asyncCleanupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous cancelled-start cleanup did not finish")
	}
	agentManager.mu.Lock()
	defer agentManager.mu.Unlock()
	if len(agentManager.stopAgentWithReasonArgs) != 1 ||
		agentManager.stopAgentWithReasonArgs[0].ExecutionID != execution.AgentExecutionID {
		t.Fatalf("cancelled successor cleanup = %+v, want stop of exact execution %q", agentManager.stopAgentWithReasonArgs, execution.AgentExecutionID)
	}
}

// TestRelaunchDynamicTaskAfterFailure_DoesNotResurrectCancelledSession proves
// F-A: a coordinator stop that commits CANCELLED while the predecessor
// teardown is in flight must win over the relaunch's own CREATED reset.
// Before the fix, the reset at dynamic_launch.go was an unconditional
// UpdateTaskSessionState write with no expected-state predicate, so it
// silently overwrote CANCELLED with CREATED and StartCreatedSession then
// launched a new agent on the session the user just stopped. This exercises
// relaunchDynamicTaskAfterFailure directly because LaunchDynamicRouteAction
// (the manual retry path) calls it without the detached-launch wrapper's
// shouldDropSessionFailure guard.
func TestRelaunchDynamicTaskAfterFailure_DoesNotResurrectCancelledSession(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-cancel-race"
		sessionID   = "session-dynamic-cancel-race"
		executionID = "execution-dynamic-cancel-race"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			// Simulate a coordinator stop committing CANCELLED between the
			// route-selection CAS and this relaunch's own destructive reset.
			return repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateCancelled, "")
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	relaunched := svc.relaunchDynamicTaskAfterFailure(
		ctx,
		watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
		},
		"fallback-profile",
		launchOriginAutomatic,
	)

	if relaunched {
		t.Fatal("relaunchDynamicTaskAfterFailure returned success over a concurrently cancelled session")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want it to stay CANCELLED", session.State)
	}
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after a concurrent cancel", len(agentManager.startAgentProcessCalls))
	}
}

// The agent.failed event that selects a fallback route is dispatched on the
// lifecycle completion goroutine while it holds the execution's prompt
// lifecycle lock. Stopping the predecessor needs that lock again, so the
// successor launch must leave the dispatch before the stop runs.
func TestLaunchDynamicSuccessorDetached_ReturnsWhileStopIsBlocked(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-detached"
		sessionID   = "session-dynamic-detached"
		executionID = "execution-dynamic-detached"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)

	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	stopErr := errors.New("runtime teardown failed")
	var firstStop sync.Once
	agentManager := &mockAgentManager{
		// Only the fallback stop blocks; the recoverable-failure cleanup that
		// follows a failed launch stops the same execution again and must not.
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			firstStop.Do(func() {
				close(stopEntered)
				<-releaseStop
			})
			return stopErr
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	data := watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
	}

	returned := make(chan struct{})
	go func() {
		svc.launchDynamicSuccessorDetached(ctx, data, "fallback-profile")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("launchDynamicSuccessorDetached blocked on the predecessor stop")
	}
	select {
	case <-stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor stop never started after the dispatch returned")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q before the stop resolved, want RUNNING", session.State)
	}

	close(releaseStop)
	waitForSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after stop failure", len(agentManager.startAgentProcessCalls))
	}
}

func TestRunDetachedDynamicSuccessorLaunch_ParksSessionWhenRelaunchFails(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-detached-failure"
		sessionID   = "session-dynamic-detached-failure"
		executionID = "execution-dynamic-detached-failure"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{
		stopAgentWithReasonErr: errors.New("runtime teardown failed"),
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	svc.runDetachedDynamicSuccessorLaunch(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		ErrorMessage:     "provider quota exhausted",
	}, "fallback-profile")

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT so the user can resume", session.State)
	}
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after stop failure", len(agentManager.startAgentProcessCalls))
	}
}

func waitForSessionState(t *testing.T, repo taskSessionStateReader, sessionID string, want models.TaskSessionState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := repo.GetTaskSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if session.State == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session state = %q, want %q", session.State, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type taskSessionStateReader interface {
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
}

// A coordinator stop can win the session's cancel guard between the route
// decision and the detached launch: it persists CANCELLED and releases the
// guard before the launch acquires it. Relaunching from that stale event would
// reset the session to CREATED and resurrect work the user already stopped.
func TestRunDetachedDynamicSuccessorLaunch_DropsCancelledSession(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-detached-cancelled"
		sessionID   = "session-dynamic-detached-cancelled"
		executionID = "execution-dynamic-detached-cancelled"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCancelled)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	// Dropping a stale failure also retires the predecessor, and that teardown
	// runs on its own goroutine with reason "agent completed". Count only the
	// relaunch's own stop, under a mutex, so the assertion neither races that
	// cleanup nor mistakes it for a resurrection.
	var stopMu sync.Mutex
	fallbackStops := 0
	agentManager := &mockAgentManager{
		stopAgentWithReasonFunc: func(_ context.Context, _ string, reason string, _ bool) error {
			if reason == "dynamic route fallback" {
				stopMu.Lock()
				fallbackStops++
				stopMu.Unlock()
			}
			return nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	svc.runDetachedDynamicSuccessorLaunch(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
	}, "fallback-profile")

	// The relaunch path opens by stopping the predecessor and then resets the
	// session to CREATED, both before the launch can fail. That stop is
	// synchronous, so its absence on return proves the launch was refused
	// before it touched the cancelled session — which a final-state assertion
	// cannot show, because the failure branch restores a state a resurrection
	// has already passed through.
	stopMu.Lock()
	stops := fallbackStops
	stopMu.Unlock()
	if stops != 0 {
		t.Fatalf("detached launch stopped the predecessor of a cancelled session %d times", stops)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED to survive the detached launch", session.State)
	}
}

// handleAgentFailedLocked returned early because the route was accepted, so
// the detached failure branch owns the automation finalization the synchronous
// path used to reach. Without it the run stays nonterminal and holds a
// max_concurrent_runs slot forever.
func TestRunDetachedDynamicSuccessorLaunch_FinalizesAutomationRunOnFailure(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-detached-automation"
		sessionID   = "session-dynamic-detached-automation"
		executionID = "execution-dynamic-detached-automation"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.Origin = models.TaskOriginAutomationRun
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task origin: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentManager := &mockAgentManager{
		stopAgentWithReasonErr: errors.New("runtime teardown failed"),
	}
	autoSvc := &stubAutomationService{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.SetAutomationService(autoSvc)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	svc.runDetachedDynamicSuccessorLaunch(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		ErrorMessage:     "provider quota exhausted",
	}, "fallback-profile")

	if autoSvc.failed[taskID] != "provider quota exhausted" {
		t.Fatalf("automation run failure = %q, want the launch error so the run leaves task_created",
			autoSvc.failed[taskID])
	}
}

// A service shutdown can cancel the detached worker while predecessor teardown
// is in progress. The worker must not surface a second failure after shutdown,
// because that would mutate the session after the service-owned cancellation.
func TestRunDetachedDynamicSuccessorLaunch_DoesNotRecoverAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	const (
		taskID      = "task-dynamic-detached-shutdown"
		sessionID   = "session-dynamic-detached-shutdown"
		executionID = "execution-dynamic-detached-shutdown"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)

	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	stopErr := errors.New("runtime teardown failed")
	var stopStartOnce sync.Once
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		stopAgentWithReasonFunc: func(_ context.Context, _ string, _ string, _ bool) error {
			stopStartOnce.Do(func() { close(stopEntered) })
			<-releaseStop
			return stopErr
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	done := make(chan struct{})
	go func() {
		svc.runDetachedDynamicSuccessorLaunch(ctx, watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
			ErrorMessage:     "provider quota exhausted",
		}, "fallback-profile")
		close(done)
	}()

	select {
	case <-stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor stop did not start")
	}
	cancel()
	close(releaseStop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("detached launch did not finish after cancellation")
	}

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after shutdown cancellation", session.State)
	}
	if len(agentManager.startAgentProcessCalls) != 0 {
		t.Fatalf("successor launch started %d processes after shutdown cancellation", len(agentManager.startAgentProcessCalls))
	}
}

// context.WithoutCancel would let the worker keep mutating session state after
// Stop began. The launch runs under a service-owned context instead, so a
// stopped service schedules nothing and reports the route as not taken.
func TestLaunchDynamicSuccessorDetached_SkipsWhenServiceStopped(t *testing.T) {
	const (
		taskID    = "task-dynamic-detached-stopped"
		sessionID = "session-dynamic-detached-stopped"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.stopDynamicSuccessorWorkers()

	if svc.launchDynamicSuccessorDetached(context.Background(), watcher.AgentEventData{
		TaskID:    taskID,
		SessionID: sessionID,
	}, "fallback-profile") {
		t.Fatal("a stopped service reported a detached successor launch it will never run")
	}

	svc.resetDynamicSuccessorWorkers()
	if !svc.launchDynamicSuccessorDetached(context.Background(), watcher.AgentEventData{
		TaskID:    taskID,
		SessionID: sessionID,
	}, "fallback-profile") {
		t.Fatal("a restarted service refused to schedule the detached successor launch")
	}
	svc.stopDynamicSuccessorWorkers()
}
