package orchestrator

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// TestOwnedTeardownSettlesOpenBackgroundWork covers the replacement path: the
// process that owned the workloads ended, so the rows settle before the
// successor can take over.
func TestOwnedTeardownSettlesOpenBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-owned-teardown"
		sessionID   = "session-owned-teardown"
		executionID = "execution-owned-teardown"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)

	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), &mockAgentManager{})
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)
	svc.RegisterExecutionStopOwner(sessionID, executionID, true)

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
	})

	require.Equal(t, []string{sessionID}, observer.settledSessions())
}

// TestOwnedTeardownSettlementDoesNotWaitForHeldLifecycleLock reproduces the
// incident's replacement path: the resume owner holds the session lifecycle
// lock across a prompt-readiness reap, and the reap's StopExecution publishes
// agent.stopped synchronously on the same goroutine. Settlement must not wait
// for the lock the caller already holds, and the rows must settle in place
// before the caller can launch a successor.
func TestOwnedTeardownSettlementDoesNotWaitForHeldLifecycleLock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-reap-held-lock"
		sessionID   = "session-reap-held-lock"
		executionID = "execution-reap-held-lock"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)

	observer := &settlingBackgroundWorkObserver{}
	published := make(chan struct{})
	var stopCalls atomic.Int32
	var svc *Service
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		stopAgentWithReasonFunc: func(stopCtx context.Context, execID, _ string, _ bool) error {
			stopCalls.Add(1)
			// MemoryEventBus runs this handler synchronously on the stop
			// publisher's goroutine, exactly as the prompt-readiness stop does.
			svc.handleAgentStopped(stopCtx, watcher.AgentEventData{
				TaskID: taskID, SessionID: sessionID, AgentExecutionID: execID,
			})
			close(published)
			return nil
		},
	}
	svc = newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.SetBackgroundWorkObserver(observer)

	type reapResult struct {
		err     error
		settled []string
	}
	done := make(chan reapResult, 1)
	go func() {
		// Mirror ensureSessionRunningWithAttemptAndBinding and the resume
		// pipeline, which hold this lock across the reap and the relaunch.
		release := svc.acquireSessionLifecycleLock(sessionID)
		defer release()
		err := svc.reapPromptUnreadyExecution(ctx, sessionID, errors.New("agent never became prompt-ready"))
		done <- reapResult{err: err, settled: observer.settledSessions()}
	}()

	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Equal(t, []string{sessionID}, result.settled,
			"rows must settle before the lifecycle owner can relaunch")
	case <-time.After(5 * time.Second):
		t.Fatal("prompt-readiness reclaim waited for the lifecycle lock it already held")
	}
	select {
	case <-published:
	default:
		t.Fatal("the prompt-readiness stop did not publish agent.stopped")
	}
	require.EqualValues(t, 1, stopCalls.Load())
}

// TestOwnedTeardownKeepsWorkWithLiveSuccessorOpen covers the retained-successor
// boundary: a live process still owns its workloads.
func TestOwnedTeardownKeepsWorkWithLiveSuccessorOpen(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-live-successor"
		sessionID   = "session-live-successor"
		executionID = "execution-live-successor"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	agentManager := &mockAgentManager{
		isAgentRunningFn: func(context.Context, string) bool { return true },
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessAlive
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.settleBackgroundWorkAfterTeardown(ctx, sessionID, executionID)

	require.Empty(t, observer.settledSessions())
}

// TestOwnedTeardownKeepsWorkWhenLivenessProbeFails keeps an uncertain probe on
// the safe side: only verified process death settles a workload.
func TestOwnedTeardownKeepsWorkWhenLivenessProbeFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-probe-failure"
		sessionID   = "session-probe-failure"
		executionID = "execution-probe-failure"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

	agentManager := &failingAgentLivenessProbe{mockAgentManager: &mockAgentManager{}}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.settleBackgroundWorkAfterTeardown(ctx, sessionID, executionID)

	require.Empty(t, observer.settledSessions())
}

// TestIdleReclaimSettlesOpenBackgroundWork covers the idle-reclaim path: the
// released runtime can no longer report completion for its workloads.
func TestIdleReclaimSettlesOpenBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	const (
		taskID      = "task-reclaim-settle"
		sessionID   = "session-reclaim-settle"
		executionID = "exec-reclaim-settle"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: executionID,
		ResumeToken:      "resume-keep",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		LocalPID:         4242,
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	agentManager := &mockAgentManager{
		isAgentRunning: false,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.turnService = &inactiveTurnService{}
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	require.NoError(t, svc.reclaimIdleSession(ctx, sessionID))

	require.Equal(t, []string{sessionID}, observer.settledSessions())
}

func TestDeferredBackgroundWorkSettlementStopsWithService(t *testing.T) {
	repo := setupTestRepo(t)
	const (
		taskID    = "task-deferred-settle-stop"
		sessionID = "session-deferred-settle-stop"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), &mockAgentManager{})
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	release := svc.acquireSessionLifecycleLock(sessionID)
	svc.settleBackgroundWorkWithoutWaiting(context.Background(), sessionID, "execution-ended")
	svc.stopBackgroundWorkSettleWorkers()
	release()

	require.Eventually(t, func() bool {
		svc.backgroundWorkSettleMu.Lock()
		defer svc.backgroundWorkSettleMu.Unlock()
		if svc.backgroundWorkSettleDrain == nil {
			return false
		}
		select {
		case <-svc.backgroundWorkSettleDrain:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Empty(t, observer.settledSessions(), "shutdown must cancel deferred settlement before it acquires the lifecycle lock")
}

// TestStartupSettlementSettlesReconciledDeadProcess covers the real startup
// shape recorded in the incident: reconciliation terminalizes the row and
// clears its local PID before settlement runs, so the real liveness classifier
// answers Unknown. The ended row status is the surviving stop evidence.
func TestStartupSettlementSettlesReconciledDeadProcess(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	const (
		taskID      = "task-startup-settle"
		sessionID   = "session-startup-settle"
		executionID = "exec-startup-settle"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: executionID,
		ResumeToken:      "resume-keep",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusFailed,
		LocalPID:         0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	agentManager := &mockAgentManager{
		isAgentRunning: false,
		// Production has no in-memory execution after the restart; only the
		// persisted row remains.
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "", lifecycle.ErrNoExecutionForSession
		},
		// The real classifier: Unknown for a standalone row whose PID was cleared.
		rowLivenessFn: lifecycle.RowProcessLiveness,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.turnService = &inactiveTurnService{}
	observer := &settlingBackgroundWorkObserver{open: []string{sessionID}}
	svc.SetBackgroundWorkObserver(observer)

	svc.settleOrphanedBackgroundWorkOnStartup(ctx)

	require.Equal(t, []string{sessionID}, observer.settledSessions())
}

// TestStartupSettlementKeepsLiveRowOpen uses the real classifier for a row
// whose recorded process is alive, and asserts startup leaves its work open.
func TestStartupSettlementKeepsLiveRowOpen(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	const (
		taskID      = "task-startup-live"
		sessionID   = "session-startup-live"
		executionID = "exec-startup-live"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		AgentExecutionID: executionID,
		ResumeToken:      "resume-keep",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		LocalPID:         os.Getpid(),
		CreatedAt:        now,
		UpdatedAt:        now,
	}))

	agentManager := &mockAgentManager{
		isAgentRunningFn: func(context.Context, string) bool { return true },
		rowLivenessFn:    lifecycle.RowProcessLiveness,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.turnService = &inactiveTurnService{}
	observer := &settlingBackgroundWorkObserver{open: []string{sessionID}}
	svc.SetBackgroundWorkObserver(observer)

	svc.settleOrphanedBackgroundWorkOnStartup(ctx)

	require.Empty(t, observer.settledSessions())
}
