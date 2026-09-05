package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"
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

	resetDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		releaseLifecycleLock := svc.acquireSessionLifecycleLock(sessionID)
		defer releaseLifecycleLock()
		resetGuard := svc.lockCancelInFlightGuard(sessionID)
		defer resetGuard.release()
		close(resetDone)
	}()

	// Give the reset goroutine a moment to queue up behind the lifecycle lock
	// (held by nobody yet) and then attempt the guard, before letting the
	// route action handler proceed to also request the lifecycle lock.
	time.Sleep(50 * time.Millisecond)
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
