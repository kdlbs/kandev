package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
