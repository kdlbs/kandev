package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	orchestratorexec "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type coordinatorStopCallOutcome struct {
	result CoordinatorTaskStopResult
	err    error
}

type coordinatorStopRepoHooks struct {
	repoStore
	listActiveFunc   func(context.Context, string) ([]*models.TaskSession, error)
	getSessionFunc   func(context.Context, string) (*models.TaskSession, error)
	cancelActiveFunc func(context.Context, string, string) (bool, time.Time, error)
	getTaskFunc      func(context.Context, string) (*models.Task, error)
	updateFullRowCAS func(context.Context, *models.TaskSession, models.TaskSessionState) (bool, error)
}

func (r *coordinatorStopRepoHooks) UpdateTaskSessionIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	if r.updateFullRowCAS != nil {
		return r.updateFullRowCAS(ctx, session, expected)
	}
	return r.repoStore.UpdateTaskSessionIfCurrentState(ctx, session, expected)
}

func (r *coordinatorStopRepoHooks) ListActiveTaskSessionsByTaskID(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSession, error) {
	if r.listActiveFunc != nil {
		return r.listActiveFunc(ctx, taskID)
	}
	return r.repoStore.ListActiveTaskSessionsByTaskID(ctx, taskID)
}

func (r *coordinatorStopRepoHooks) GetTaskSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	if r.getSessionFunc != nil {
		return r.getSessionFunc(ctx, sessionID)
	}
	return r.repoStore.GetTaskSession(ctx, sessionID)
}

func (r *coordinatorStopRepoHooks) CancelActiveTaskSession(
	ctx context.Context,
	sessionID, reason string,
) (bool, time.Time, error) {
	if r.cancelActiveFunc != nil {
		return r.cancelActiveFunc(ctx, sessionID, reason)
	}
	canceller, ok := r.repoStore.(activeTaskSessionCanceller)
	if !ok {
		return false, time.Time{}, errors.New("coordinator stop test repository cannot cancel an active session")
	}
	return canceller.CancelActiveTaskSession(ctx, sessionID, reason)
}

func (r *coordinatorStopRepoHooks) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	if r.getTaskFunc != nil {
		return r.getTaskFunc(ctx, taskID)
	}
	return r.repoStore.GetTask(ctx, taskID)
}

type coordinatorStopReadyReadKey struct{}

func TestScheduleTaskForSession_CancelledSessionDoesNotOverwriteReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-cancelled", "session-cancelled", models.TaskSessionStateCancelled)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-cancelled", v1.TaskStateReview)
	svc := newCoordinatorStopTestService(repo, taskRepo, &mockAgentManager{})

	err := svc.scheduleTaskForSession(ctx, "task-cancelled", "session-cancelled")

	require.ErrorIs(t, err, orchestratorexec.ErrSessionStateSuperseded)
	state, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-cancelled")
	require.Equal(t, v1.TaskStateReview, state)
	require.Empty(t, history, "cancelled session must not write task SCHEDULING")
}

func TestStopTaskForCoordinator_NotRunningDisarmsTransientRetry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-no-execution", "session-no-execution", models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-no-execution", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "", lifecycle.ErrNoExecutionForSession
		},
	})
	cancelled := make(chan struct{}, 1)
	svc.transientRetries.Store("session-no-execution", &transientRetryEntry{
		attempt: 1,
		cancel: func() {
			cancelled <- struct{}{}
		},
	})
	svc.rememberTurnPrompt("session-no-execution", "retry me", "", false, nil)

	result, err := svc.StopTaskForCoordinator(ctx, "task-no-execution")

	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusNotRunning, result.Status)
	coordinatorStopAwaitSignal(t, cancelled, "transient retry cancellation")
	_, retryArmed := svc.transientRetries.Load("session-no-execution")
	require.False(t, retryArmed)
	_, promptCached := svc.lastTurnPrompt.Load("session-no-execution")
	require.False(t, promptCached)
	session, err := repo.GetTaskSession(ctx, "session-no-execution")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	state, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-no-execution")
	require.Equal(t, v1.TaskStateInProgress, state)
	require.Empty(t, history, "not_running stop must not change task state")
}

func TestStopTaskForCoordinator_ReleasesSessionGuardBeforeTeardown(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-teardown-order", "session-teardown-order", models.TaskSessionStateRunning)

	guardHeldAtTeardown := make(chan bool, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-teardown-order", nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-teardown-order", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, manager)
	manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		guardHeldAtTeardown <- svc.isCancelInFlight("session-teardown-order")
		return nil
	}

	result, err := svc.StopTaskForCoordinator(ctx, "task-teardown-order")
	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, result.Status)
	select {
	case held := <-guardHeldAtTeardown:
		require.False(t, held, "detached teardown entered before the session guard was released")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for teardown ordering check")
	}
}

func TestStopTaskForCoordinator_FirstForceTeardownIntentWins(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-teardown-intent", "session-teardown-intent", models.TaskSessionStateRunning)

	stopEntered := make(chan stopAgentCall, 2)
	allowForceStop := make(chan struct{})
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-teardown-intent", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopEntered <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			if force {
				<-allowForceStop
			}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-teardown-intent", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, manager)

	cleanupDone := make(chan struct{})
	go func() {
		svc.cleanupAgentExecution(
			"execution-teardown-intent",
			"task-teardown-intent",
			"session-teardown-intent",
		)
		close(cleanupDone)
	}()
	first := <-stopEntered
	require.True(t, first.Force)

	result, err := svc.StopTaskForCoordinator(ctx, "task-teardown-intent")
	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, result.Status)
	close(allowForceStop)
	coordinatorStopAwaitSignal(t, cleanupDone, "force teardown completion")
	select {
	case duplicate := <-stopEntered:
		t.Fatalf("duplicate teardown intent reached runtime: %#v", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
	session, err := repo.GetTaskSession(ctx, "session-teardown-intent")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
}

func TestHandleAgentFailed_UnavailableSessionStateStillCleansExecution(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-failure-unavailable", "session-failure-unavailable", models.TaskSessionStateRunning)
	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
			return nil, errors.New("session store unavailable")
		},
	}
	stopCalled := make(chan stopAgentCall, 1)
	manager := &mockAgentManager{
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopCalled <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "task-failure-unavailable",
		SessionID:        "session-failure-unavailable",
		AgentExecutionID: "execution-failure-unavailable",
		ErrorMessage:     "agent failed",
	})

	select {
	case call := <-stopCalled:
		require.Equal(t, "execution-failure-unavailable", call.ExecutionID)
		require.True(t, call.Force)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for execution-safe cleanup")
	}
}

func TestStopTaskForCoordinator_SerializesStaleAgentFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-failure-race", "session-failure-race", models.TaskSessionStateRunning)

	lookupEntered := make(chan struct{})
	allowLookup := make(chan struct{})
	var lookupOnce sync.Once
	var allowLookupOnce sync.Once
	releaseLookup := func() { allowLookupOnce.Do(func() { close(allowLookup) }) }
	t.Cleanup(releaseLookup)
	stopCalls := make(chan stopAgentCall, 2)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			lookupOnce.Do(func() { close(lookupEntered) })
			<-allowLookup
			return "execution-failure-race", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopCalls <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-failure-race", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	stopDone := make(chan coordinatorStopCallOutcome, 1)
	go func() {
		result, err := svc.StopTaskForCoordinator(ctx, "task-failure-race")
		stopDone <- coordinatorStopCallOutcome{result: result, err: err}
	}()
	coordinatorStopAwaitSignal(t, lookupEntered, "coordinator lifecycle lookup")

	failureDone := make(chan struct{})
	go func() {
		defer close(failureDone)
		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           "task-failure-race",
			SessionID:        "session-failure-race",
			AgentExecutionID: "execution-failure-race",
			ErrorMessage:     "agent crashed",
		})
	}()
	coordinatorStopWaitForGuardRefs(t, svc, "session-failure-race", 2)

	releaseLookup()
	outcome := coordinatorStopAwaitCall(t, stopDone)
	require.NoError(t, outcome.err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, outcome.result.Status)
	coordinatorStopAwaitSignal(t, failureDone, "stale agent.failed completion")

	select {
	case call := <-stopCalls:
		require.False(t, call.Force, "coordinator graceful teardown must remain non-force")
		require.Equal(t, coordinatorMCPStopReason, call.Reason)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coordinator graceful teardown")
	}
	select {
	case call := <-stopCalls:
		t.Fatalf("stale agent.failed launched extra cleanup: %#v", call)
	case <-time.After(100 * time.Millisecond):
	}

	session, err := repo.GetTaskSession(ctx, "session-failure-race")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	require.Empty(t, messages.sessionMessages, "stale failure must not emit recovery messages")
	state, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-failure-race")
	require.Equal(t, v1.TaskStateReview, state)
	require.Equal(t, []v1.TaskState{v1.TaskStateReview}, history)
}

func TestStopTaskForCoordinator_ProcessesCandidatesInStableIDOrder(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-order", "session-c", models.TaskSessionStateRunning)
	coordinatorStopAddSession(t, baseRepo, "task-order", "session-a", models.TaskSessionStateRunning)
	coordinatorStopAddSession(t, baseRepo, "task-order", "session-b", models.TaskSessionStateRunning)

	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		listActiveFunc: func(context.Context, string) ([]*models.TaskSession, error) {
			return []*models.TaskSession{
				{ID: "session-c", TaskID: "task-order", State: models.TaskSessionStateRunning},
				{ID: "session-a", TaskID: "task-order", State: models.TaskSessionStateRunning},
				{ID: "session-b", TaskID: "task-order", State: models.TaskSessionStateRunning},
			}, nil
		},
	}
	var lookupMu sync.Mutex
	var lookupOrder []string
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			lookupMu.Lock()
			lookupOrder = append(lookupOrder, sessionID)
			lookupMu.Unlock()
			return "", lifecycle.ErrNoExecutionForSession
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-order", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-order")

	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusNotRunning, result.Status)
	lookupMu.Lock()
	gotOrder := append([]string(nil), lookupOrder...)
	lookupMu.Unlock()
	require.Equal(t, []string{"session-a", "session-b", "session-c"}, gotOrder)
}

func TestStopTaskForCoordinator_AcceptedAndAbsentReturnsStopped(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-mixed", "session-a", models.TaskSessionStateRunning)
	coordinatorStopAddSession(t, repo, "task-mixed", "session-b", models.TaskSessionStateRunning)

	teardownCalled := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			if sessionID == "session-a" {
				return "execution-a", nil
			}
			return "", lifecycle.ErrNoExecutionForSession
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			teardownCalled <- struct{}{}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-mixed", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-mixed")

	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, result.Status)
	coordinatorStopAwaitSignal(t, teardownCalled, "accepted runtime teardown")
	accepted, err := repo.GetTaskSession(ctx, "session-a")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, accepted.State)
	absent, err := repo.GetTaskSession(ctx, "session-b")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, absent.State)
}

func TestStopTaskForCoordinator_CommittedCancellationSurvivesPostWriteReadFailure(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-post-write-read", "session-post-write-read", models.TaskSessionStateRunning)

	postWriteReadFailure := errors.New("post-cancellation read failed")
	var cancelCommitted atomic.Bool
	var failedRead atomic.Bool
	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(readCtx context.Context, sessionID string) (*models.TaskSession, error) {
			if cancelCommitted.Load() && failedRead.CompareAndSwap(false, true) {
				return nil, postWriteReadFailure
			}
			return baseRepo.GetTaskSession(readCtx, sessionID)
		},
		cancelActiveFunc: func(writeCtx context.Context, sessionID, reason string) (bool, time.Time, error) {
			changed, updatedAt, err := baseRepo.CancelActiveTaskSession(writeCtx, sessionID, reason)
			if changed && err == nil {
				cancelCommitted.Store(true)
			}
			return changed, updatedAt, err
		},
	}
	teardownCalled := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-post-write-read", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			teardownCalled <- struct{}{}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-post-write-read", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-post-write-read")

	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, result.Status)
	coordinatorStopAwaitSignal(t, teardownCalled, "teardown after committed cancellation")
	session, getErr := baseRepo.GetTaskSession(ctx, "session-post-write-read")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
}

func TestStopTaskForCoordinator_DelayedStaleStateWriterCannotResurrectCancelledSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stale-writer", "session-stale-writer", models.TaskSessionStateRunning)
	staleSession, err := repo.GetTaskSession(ctx, "session-stale-writer")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, staleSession.State)

	teardownCalled := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-stale-writer", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			teardownCalled <- struct{}{}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-stale-writer", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-stale-writer")
	require.NoError(t, err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, result.Status)
	coordinatorStopAwaitSignal(t, teardownCalled, "stale-writer runtime teardown")

	// Model a delayed event handler that loaded RUNNING before the stop, then
	// attempts to commit an active state after cancellation was accepted.
	svc.updateTaskSessionState(
		ctx,
		"task-stale-writer",
		"session-stale-writer",
		models.TaskSessionStateWaitingForInput,
		"",
		false,
		staleSession,
	)

	finalSession, getErr := repo.GetTaskSession(ctx, "session-stale-writer")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, finalSession.State)
}

func TestCompleteAndStopSession_DoesNotRestoreStaleRunningStateAfterCancellation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-workflow-cleanup", "session-workflow-cleanup", models.TaskSessionStateRunning)

	staleSession, err := repo.GetTaskSession(ctx, "session-workflow-cleanup")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, staleSession.State)
	changed, _, err := repo.CancelActiveTaskSession(ctx, staleSession.ID, coordinatorMCPStopReason)
	require.NoError(t, err)
	require.True(t, changed)

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-workflow-cleanup", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, &mockAgentManager{repoForExecutionLookup: repo})

	// Model workflow cleanup resuming after coordinator stop committed. The
	// cleanup owns a stale RUNNING row and must not write it back wholesale.
	svc.completeAndStopSession(ctx, "task-workflow-cleanup", staleSession)

	stored, err := repo.GetTaskSession(ctx, staleSession.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
	require.Equal(t, coordinatorMCPStopReason, stored.ErrorMessage)
}

func TestSetSessionStarting_CoordinatorCancellationWinsAfterCurrentStateRead(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-start-race", "session-start-race", models.TaskSessionStateRunning)
	stale, err := baseRepo.GetTaskSession(ctx, "session-start-race")
	require.NoError(t, err)
	stale.State = models.TaskSessionStateStarting

	repo := &coordinatorStopRepoHooks{repoStore: baseRepo}
	repo.updateFullRowCAS = func(
		writeCtx context.Context,
		session *models.TaskSession,
		expected models.TaskSessionState,
	) (bool, error) {
		changed, _, cancelErr := baseRepo.CancelActiveTaskSession(
			writeCtx, session.ID, coordinatorMCPStopReason,
		)
		require.NoError(t, cancelErr)
		require.True(t, changed)
		return baseRepo.UpdateTaskSessionIfCurrentState(writeCtx, session, expected)
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), &mockAgentManager{})

	err = svc.setSessionStarting(ctx, stale.TaskID, stale, true)
	require.Error(t, err)
	stored, getErr := baseRepo.GetTaskSession(ctx, stale.ID)
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
	require.Equal(t, coordinatorMCPStopReason, stored.ErrorMessage)
}

func TestStopTaskForCoordinator_PartialFailureAttemptsEveryCandidateAndSkipsReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-partial", "session-a", models.TaskSessionStateRunning)
	coordinatorStopAddSession(t, repo, "task-partial", "session-b", models.TaskSessionStateRunning)
	coordinatorStopAddSession(t, repo, "task-partial", "session-c", models.TaskSessionStateRunning)

	lookupFailure := errors.New("lifecycle lookup failed")
	var lookupMu sync.Mutex
	var lookupOrder []string
	teardownCalled := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			lookupMu.Lock()
			lookupOrder = append(lookupOrder, sessionID)
			lookupMu.Unlock()
			switch sessionID {
			case "session-a":
				return "execution-a", nil
			case "session-b":
				return "", lookupFailure
			default:
				return "", lifecycle.ErrNoExecutionForSession
			}
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			teardownCalled <- struct{}{}
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-partial", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	result, err := svc.StopTaskForCoordinator(ctx, "task-partial")

	require.ErrorIs(t, err, lookupFailure)
	require.Empty(t, result.Status)
	coordinatorStopAwaitSignal(t, teardownCalled, "successful candidate teardown")
	lookupMu.Lock()
	gotOrder := append([]string(nil), lookupOrder...)
	lookupMu.Unlock()
	require.Equal(t, []string{"session-a", "session-b", "session-c"}, gotOrder)
	state, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-partial")
	require.Equal(t, v1.TaskStateInProgress, state)
	require.Empty(t, history, "partial failure must not reconcile the task to REVIEW")
	accepted, getErr := repo.GetTaskSession(ctx, "session-a")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateCancelled, accepted.State)
}

func TestStopTaskForCoordinator_TerminalRereadWinsAfterCandidateSnapshot(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-terminal", "session-terminal", models.TaskSessionStateRunning)

	snapshotReturned := make(chan struct{})
	var snapshotOnce sync.Once
	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		listActiveFunc: func(context.Context, string) ([]*models.TaskSession, error) {
			snapshotOnce.Do(func() { close(snapshotReturned) })
			return []*models.TaskSession{{
				ID: "session-terminal", TaskID: "task-terminal", State: models.TaskSessionStateRunning,
			}}, nil
		},
	}
	var lookupCalls atomic.Int32
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			lookupCalls.Add(1)
			return "execution-terminal", nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-terminal", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)

	guard, releaseGuard := svc.acquireCancelInFlightGuard("session-terminal")
	guard.Lock()
	var unlockOnce sync.Once
	unlockGuard := func() {
		unlockOnce.Do(func() {
			guard.Unlock()
			releaseGuard()
		})
	}
	t.Cleanup(unlockGuard)

	stopDone := make(chan coordinatorStopCallOutcome, 1)
	go func() {
		result, err := svc.StopTaskForCoordinator(ctx, "task-terminal")
		stopDone <- coordinatorStopCallOutcome{result: result, err: err}
	}()
	coordinatorStopAwaitSignal(t, snapshotReturned, "active-session snapshot")
	require.NoError(t, baseRepo.UpdateTaskSessionState(
		ctx, "session-terminal", models.TaskSessionStateCompleted, "completed naturally",
	))
	unlockGuard()

	outcome := coordinatorStopAwaitCall(t, stopDone)
	require.NoError(t, outcome.err)
	require.Equal(t, CoordinatorTaskStopStatusNotRunning, outcome.result.Status)
	require.Zero(t, lookupCalls.Load(), "terminal guarded reread must skip lifecycle lookup")
	session, err := baseRepo.GetTaskSession(ctx, "session-terminal")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCompleted, session.State)
	_, history := coordinatorStopTaskStateSnapshot(taskRepo, "task-terminal")
	require.Empty(t, history)
}

func TestStopTaskForCoordinator_SerializesReadyBeforeQueueDrain(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-ready", "session-ready", models.TaskSessionStateRunning)

	cancelWriteEntered := make(chan struct{})
	allowCancelWrite := make(chan struct{})
	readyInitialRead := make(chan struct{})
	var cancelWriteOnce sync.Once
	var readyReadOnce sync.Once
	repo := &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(readCtx context.Context, sessionID string) (*models.TaskSession, error) {
			session, err := baseRepo.GetTaskSession(readCtx, sessionID)
			if readCtx.Value(coordinatorStopReadyReadKey{}) == true {
				readyReadOnce.Do(func() { close(readyInitialRead) })
			}
			return session, err
		},
		cancelActiveFunc: func(writeCtx context.Context, sessionID, reason string) (bool, time.Time, error) {
			cancelWriteOnce.Do(func() { close(cancelWriteEntered) })
			<-allowCancelWrite
			return baseRepo.CancelActiveTaskSession(writeCtx, sessionID, reason)
		},
	}
	var releaseCancelOnce sync.Once
	releaseCancelWrite := func() { releaseCancelOnce.Do(func() { close(allowCancelWrite) }) }
	t.Cleanup(releaseCancelWrite)

	teardownCalled := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-ready", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			teardownCalled <- struct{}{}
			return nil
		},
		promptDone: make(chan struct{}),
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-ready", v1.TaskStateInProgress)
	svc := newCoordinatorStopTestService(repo, taskRepo, agentManager)
	_, err := svc.messageQueue.QueueMessage(
		ctx,
		"session-ready",
		"task-ready",
		"must remain queued",
		"",
		messagequeue.QueuedByAgent,
		false,
		nil,
	)
	require.NoError(t, err)

	stopDone := make(chan coordinatorStopCallOutcome, 1)
	go func() {
		result, stopErr := svc.StopTaskForCoordinator(ctx, "task-ready")
		stopDone <- coordinatorStopCallOutcome{result: result, err: stopErr}
	}()
	coordinatorStopAwaitSignal(t, cancelWriteEntered, "blocked cancellation write")

	readyDone := make(chan struct{})
	readyCtx := context.WithValue(ctx, coordinatorStopReadyReadKey{}, true)
	go func() {
		svc.handleAgentReady(readyCtx, watcher.AgentEventData{
			TaskID: "task-ready", SessionID: "session-ready", AgentExecutionID: "execution-ready",
		})
		close(readyDone)
	}()
	coordinatorStopAwaitSignal(t, readyInitialRead, "ready event initial session read")
	coordinatorStopWaitForGuardRefs(t, svc, "session-ready", 2)
	select {
	case <-readyDone:
		t.Fatal("ready event completed while coordinator cancellation owned the shared guard")
	default:
	}

	releaseCancelWrite()
	stopOutcome := coordinatorStopAwaitCall(t, stopDone)
	coordinatorStopAwaitSignal(t, readyDone, "ready event guarded reread")
	coordinatorStopAwaitSignal(t, teardownCalled, "runtime teardown")

	require.NoError(t, stopOutcome.err)
	require.Equal(t, CoordinatorTaskStopStatusStopped, stopOutcome.result.Status)
	session, err := baseRepo.GetTaskSession(ctx, "session-ready")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-ready").Count)
	agentManager.mu.Lock()
	promptCalls := append([]promptCall(nil), agentManager.capturedPromptCalls...)
	agentManager.mu.Unlock()
	require.Empty(t, promptCalls, "ready event must not dispatch replacement work after cancellation")
}
