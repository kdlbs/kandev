package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestApplyRouteActionAndLifecycleResetDoNotDeadlock proves that
// ApplyRouteAction and a session-lifecycle operation (modeled here the same
// way resetAgentContext acquires its locks) can run concurrently on the same
// session without an ABBA deadlock between acquireCancelInFlightGuard and
// acquireSessionLifecycleLock.
//
// Before the fix, ApplyRouteAction held the cancel-in-flight guard across the
// whole routeActionHandler call, and the handler here reaches
// acquireSessionLifecycleLock (as the real dynamic-route launch path does via
// startCreatedSession) — guard-then-lifecycle. The concurrent goroutine takes
// lifecycle-then-guard, exactly like resetAgentContext. With both orders live
// on the same sessionID, each goroutine can block on the lock the other
// holds, and neither ever completes.
func TestApplyRouteActionAndLifecycleResetDoNotDeadlock(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	ctx := context.Background()
	sessionID := "session1"

	handlerEntered := make(chan struct{})
	handlerMayLockLifecycle := make(chan struct{})
	svc.SetRouteActionHandler(func(_ context.Context, request RouteActionRequest) (*RouteActionResult, error) {
		close(handlerEntered)
		<-handlerMayLockLifecycle
		release := svc.acquireSessionLifecycleLock(request.SessionID)
		defer release()
		return &RouteActionResult{SessionID: request.SessionID}, nil
	})

	var wg sync.WaitGroup
	routeActionDone := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := svc.ApplyRouteAction(ctx, RouteActionRequest{
			SessionID: sessionID,
			Action:    RouteActionRetry,
		})
		routeActionDone <- err
	}()

	select {
	case <-handlerEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("route action handler never started")
	}

	resetHasLifecycleLock := make(chan struct{})
	resetDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		releaseLifecycleLock := svc.acquireSessionLifecycleLock(sessionID)
		defer releaseLifecycleLock()
		close(resetHasLifecycleLock)
		resetGuard := svc.lockCancelInFlightGuard(sessionID)
		defer resetGuard.release()
		close(resetDone)
	}()

	// Barrier: wait for the reset goroutine to actually hold the lifecycle
	// lock before letting the route action handler also request it. A fixed
	// sleep here would only assume enough time passed; this proves it.
	select {
	case <-resetHasLifecycleLock:
	case <-time.After(3 * time.Second):
		t.Fatal("reset goroutine never acquired the session lifecycle lock")
	}
	close(handlerMayLockLifecycle)

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: ApplyRouteAction and lifecycle reset did not both complete within 3s")
	}

	select {
	case <-resetDone:
	default:
		t.Fatal("lifecycle reset goroutine did not complete")
	}
	if err := <-routeActionDone; err != nil {
		t.Fatalf("ApplyRouteAction: %v", err)
	}
}

// pausedSecondSessionLoadRepo blocks the *second* GetTaskSession call for a
// target session until released. handleAgentCompleted's first repo call
// (captureSessionCommitsSweep, via resolveArchiveBaseCommitAndBranch) runs
// before the cancel-in-flight guard is acquired; the second is
// handleAgentCompletedLocked's own session load, the first repo access made
// while that guard is held. Pausing there proves the guard is genuinely held
// at the moment of the pause, without relying on a real reachability hook
// production code doesn't have.
type pausedSecondSessionLoadRepo struct {
	sessionExecutorStore
	sessionID string
	calls     atomic.Int32
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (r *pausedSecondSessionLoadRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if sessionID == r.sessionID && r.calls.Add(1) == 2 {
		r.once.Do(func() { close(r.entered) })
		<-r.release
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

// TestHandleAgentCompletedAndResetAgentContextDoNotDeadlock exercises the
// real reachability F1a fixed: handleAgentCompletedLocked reaches
// reclaimIdleSession (directly, and via setSessionWaitingForInputIfRequested)
// while it can still be holding the cancel-in-flight guard, and
// reclaimIdleSession's first act is acquireSessionLifecycleLock — the same
// lock resetAgentContext already holds when it (unconditionally) reacquires
// the cancel guard via quiesceActiveResetTurn's resetGuard.relock. Two
// goroutines taking these locks in opposite order on the same session is a
// live ABBA deadlock unless the guard is released before the lifecycle
// lock is requested.
func TestHandleAgentCompletedAndResetAgentContextDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "")

	pausedRepo := &pausedSecondSessionLoadRepo{
		sessionExecutorStore: repo,
		sessionID:            "session1",
		entered:              make(chan struct{}),
		release:              make(chan struct{}),
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task1", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.repo = pausedRepo

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		svc.handleAgentCompleted(ctx, watcherAgentCompletedData("task1", "session1", "exec-1"))
	}()

	// Barrier: wait for handleAgentCompleted to reach its own guarded session
	// load. At this point it holds the cancel-in-flight guard and has not
	// touched the lifecycle lock.
	select {
	case <-pausedRepo.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handleAgentCompleted never reached its guarded session load")
	}

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	resetDone := make(chan bool, 1)
	go func() {
		resetDone <- svc.resetAgentContext(ctx, "task1", session, "Successor")
	}()

	// Barrier: prove resetAgentContext has genuinely acquired the lifecycle
	// lock (and is therefore blocked requesting the guard held above, exactly
	// like the real global lock order) before releasing the paused handler.
	// A losing TryLock on the same mutex is a positive signal, not a timing
	// guess.
	require.Eventually(t, func() bool {
		value, ok := svc.sessionLifecycleLocks.Load("session1")
		if !ok {
			return false
		}
		mu, ok := value.(*sync.Mutex)
		if !ok {
			return false
		}
		if mu.TryLock() {
			mu.Unlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond, "resetAgentContext never acquired the session lifecycle lock")

	close(pausedRepo.release)

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: handleAgentCompleted did not complete within 3s")
	}
	select {
	case resetOK := <-resetDone:
		if !resetOK {
			t.Fatal("resetAgentContext returned false")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: resetAgentContext did not complete within 3s")
	}
}
