package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyRouteActionSerializesHandlersPerSession(t *testing.T) {
	svc := &Service{}
	const sessionID = "session1"
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32

	svc.SetRouteActionHandler(func(_ context.Context, request RouteActionRequest) (*RouteActionResult, error) {
		if calls.Add(1) == 1 {
			close(firstEntered)
			<-releaseFirst
		} else {
			close(secondEntered)
		}
		return &RouteActionResult{SessionID: request.SessionID}, nil
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.ApplyRouteAction(context.Background(), RouteActionRequest{
			SessionID: sessionID,
			Action:    RouteActionRetry,
		})
		firstDone <- err
	}()

	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first route action did not enter its handler")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.ApplyRouteAction(context.Background(), RouteActionRequest{
			SessionID: sessionID,
			Action:    RouteActionTryNext,
		})
		secondDone <- err
	}()

	select {
	case <-secondEntered:
		t.Fatal("second route action entered before the first completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	for _, done := range []<-chan error{firstDone, secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ApplyRouteAction: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("route action did not complete")
		}
	}
}

func TestApplyRouteActionAllowsDifferentSessionsConcurrently(t *testing.T) {
	svc := &Service{}
	entered := make(chan string, 2)
	release := make(chan struct{})
	var calls atomic.Int32

	svc.SetRouteActionHandler(func(_ context.Context, request RouteActionRequest) (*RouteActionResult, error) {
		calls.Add(1)
		entered <- request.SessionID
		<-release
		return &RouteActionResult{SessionID: request.SessionID}, nil
	})

	var wg sync.WaitGroup
	for _, sessionID := range []string{"session1", "session2"} {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			_, _ = svc.ApplyRouteAction(context.Background(), RouteActionRequest{
				SessionID: sessionID,
				Action:    RouteActionRetry,
			})
		}(sessionID)
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case sessionID := <-entered:
			seen[sessionID] = true
		case <-time.After(time.Second):
			t.Fatal("route actions for different sessions did not run concurrently")
		}
	}
	close(release)
	wg.Wait()
	if calls.Load() != 2 || len(seen) != 2 {
		t.Fatalf("calls = %d, sessions = %#v, want one call per session", calls.Load(), seen)
	}
}

func TestRouteActionClaimBlocksPromptAdmission(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	const sessionID = "session1"
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session promptable: %v", err)
	}

	handlerEntered := make(chan struct{})
	releaseHandler := make(chan struct{})
	svc.SetRouteActionHandler(func(_ context.Context, _ RouteActionRequest) (*RouteActionResult, error) {
		close(handlerEntered)
		<-releaseHandler
		return &RouteActionResult{SessionID: sessionID}, nil
	})

	routeDone := make(chan error, 1)
	go func() {
		_, err := svc.ApplyRouteAction(ctx, RouteActionRequest{
			SessionID: sessionID,
			Action:    RouteActionRetry,
		})
		routeDone <- err
	}()
	select {
	case <-handlerEntered:
	case <-time.After(time.Second):
		t.Fatal("route action handler did not start")
	}

	_, _, _, _, _, err := svc.claimSessionRunningForPrompt(
		ctx, "task1", sessionID, "", false, nil, nil, "", false,
	)
	close(releaseHandler)
	if routeErr := <-routeDone; routeErr != nil {
		t.Fatalf("route action: %v", routeErr)
	}
	if !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("prompt admission error = %v, want route-action busy error", err)
	}
}
