package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	orchestratorexec "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestClaimForcedExecutionCleanup_UsesExactExecutionClaim(t *testing.T) {
	svc := newCoordinatorStopTestService(
		setupTestRepo(t),
		newMockTaskRepo(),
		&mockAgentManager{},
	)
	guard, release := svc.acquireCancelInFlightGuard("session-claim")
	guard.Lock()
	require.True(t, svc.claimExecutionTeardown(
		"session-claim",
		"execution-old",
		executionTeardownIntentGraceful,
	))
	guard.Unlock()
	release()

	require.True(t, svc.claimForcedExecutionCleanup("session-claim", "execution-new"))
	require.False(t, svc.claimForcedExecutionCleanup("session-claim", "execution-new"))
	require.False(t, svc.claimForcedExecutionCleanup("session-claim", "execution-old"))
}

func TestRegisterExecutionStopOwner_SuppressesOrphanCleanupAndRecordsForceEscalation(t *testing.T) {
	svc := newCoordinatorStopTestService(
		setupTestRepo(t),
		newMockTaskRepo(),
		&mockAgentManager{},
	)

	svc.RegisterExecutionStopOwner("session-owner", "execution-owner", false)
	require.False(t, svc.claimForcedExecutionCleanup("session-owner", "execution-owner"))

	svc.RegisterExecutionStopOwner("session-owner", "execution-owner", true)
	value, ok := svc.executionTeardownClaims.Load(
		terminalExecutionKey("session-owner", "execution-owner"),
	)
	require.True(t, ok)
	claim, ok := value.(executionTeardownClaim)
	require.True(t, ok)
	require.Equal(t, executionTeardownIntentForce, claim.intent)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.1
func TestRegisterExecutionStopOwner_ContendedGuardDoesNotBlock(t *testing.T) {
	svc := newCoordinatorStopTestService(
		setupTestRepo(t),
		newMockTaskRepo(),
		&mockAgentManager{},
	)
	const (
		sessionID   = "session-contended"
		executionID = "execution-contended"
	)
	guard, release := svc.acquireCancelInFlightGuard(sessionID)
	guardLocked := false
	t.Cleanup(func() {
		if guardLocked {
			guard.Unlock()
		}
		release()
	})
	guard.Lock()
	guardLocked = true

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		svc.RegisterExecutionStopOwner(sessionID, executionID, false)
		close(done)
	}()
	<-started

	select {
	case <-done:
		_, claimed := svc.executionTeardownClaims.Load(terminalExecutionKey(sessionID, executionID))
		require.False(t, claimed)
	case <-time.After(time.Second):
		guard.Unlock()
		guardLocked = false
		<-done
		t.Fatal("advisory stop-owner registration blocked on the session guard")
	}
}

func TestHandleAgentStopped_OwnedPredecessorDoesNotCancelStartingReplacement(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID         = "task-stale-stop"
		sessionID      = "session-stale-stop"
		oldExecutionID = "execution-stale-stop"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)

	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), &mockAgentManager{})
	svc.RegisterExecutionStopOwner(sessionID, oldExecutionID, true)

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: oldExecutionID,
	})

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, session.State)
}

func TestCleanupAgentExecution_CancelledSessionUsesExactExecutionOwnership(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-cleanup-claim", "session-cleanup-claim", models.TaskSessionStateRunning)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"session-cleanup-claim",
		models.TaskSessionStateCancelled,
		"task archived",
	))

	stopCalls := make(chan stopAgentCall, 2)
	manager := &mockAgentManager{
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopCalls <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.RegisterExecutionStopOwner("session-cleanup-claim", "execution-snapshot", true)

	svc.cleanupAgentExecution("execution-snapshot", "task-cleanup-claim", "session-cleanup-claim")
	svc.cleanupAgentExecution("execution-late", "task-cleanup-claim", "session-cleanup-claim")

	select {
	case call := <-stopCalls:
		require.Equal(t, "execution-late", call.ExecutionID)
		require.True(t, call.Force)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for unclaimed execution cleanup")
	}
	select {
	case duplicate := <-stopCalls:
		t.Fatalf("claimed snapshot execution was stopped twice: %#v", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestReapPromptUnreadyExecution_StopsOnlyWhenRecoveryOwnsExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-recovery-owner", "session-recovery-owner", models.TaskSessionStateWaitingForInput)

	var stopCalls atomic.Int32
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-recovery-owner", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls.Add(1)
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.RegisterExecutionStopOwner("session-recovery-owner", "execution-recovery-owner", true)

	err := svc.reapPromptUnreadyExecution(
		ctx,
		"session-recovery-owner",
		errors.New("agent never became prompt-ready"),
	)

	require.Error(t, err)
	require.Zero(t, stopCalls.Load(), "recovery must not duplicate coordinator-owned teardown")
}

func TestReapPromptUnreadyExecution_RetiresActivityAfterForcedStop(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID      = "task-recovery-activity"
		sessionID   = "session-recovery-activity"
		executionID = "execution-recovery-activity"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)

	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	svc.registerBackgroundWork(sessionID, "orphaned-work", executionID, "work")
	svc.markForegroundIdle(sessionID)

	require.NoError(t, svc.reapPromptUnreadyExecution(
		ctx,
		sessionID,
		errors.New("agent never became prompt-ready"),
	))
	_, ok := turnActivityRecord(t, svc, sessionID)
	require.False(t, ok, "forced prompt-readiness teardown retained activity")
}

func TestReapPromptUnreadyExecution_DoesNotResumeAfterConcurrentCancellation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-recovery-race", "session-recovery-race", models.TaskSessionStateWaitingForInput)

	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-recovery-race", nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			close(stopEntered)
			<-releaseStop
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
	reapDone := make(chan error, 1)
	go func() {
		reapDone <- svc.reapPromptUnreadyExecution(
			ctx,
			"session-recovery-race",
			errors.New("agent never became prompt-ready"),
		)
	}()

	coordinatorStopAwaitSignal(t, stopEntered, "prompt recovery stop")
	svc.RegisterExecutionStopOwner("session-recovery-race", "execution-recovery-race", true)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"session-recovery-race",
		models.TaskSessionStateCancelled,
		"stopped by parent task via MCP",
	))
	close(releaseStop)

	err := <-reapDone
	require.ErrorIs(t, err, orchestratorexec.ErrSessionStateSuperseded)
}

func TestStopSession_GracefulTeardownClaimSuppressesLateForceCleanup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-legacy-stop", "session-legacy-stop", models.TaskSessionStateRunning)

	stopCalls := make(chan stopAgentCall, 2)
	allowGracefulStop := make(chan struct{})
	gracefulStopDone := make(chan struct{})
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-legacy-stop", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, reason string, force bool) error {
			stopCalls <- stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
			if !force {
				<-allowGracefulStop
				close(gracefulStopDone)
			}
			return nil
		},
	}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)

	require.NoError(t, svc.StopSession(ctx, "session-legacy-stop", "legacy graceful stop", false))
	first := <-stopCalls
	require.Equal(t, "execution-legacy-stop", first.ExecutionID)
	require.False(t, first.Force)

	svc.cleanupAgentExecution("execution-legacy-stop", "task-legacy-stop", "session-legacy-stop")
	close(allowGracefulStop)
	coordinatorStopAwaitSignal(t, gracefulStopDone, "legacy graceful teardown")
	select {
	case duplicate := <-stopCalls:
		t.Fatalf("late terminal cleanup duplicated legacy teardown: %#v", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
}
