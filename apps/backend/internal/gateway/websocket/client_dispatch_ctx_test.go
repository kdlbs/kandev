package websocket

import (
	"context"
	"errors"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHub_DispatchContextFallsBackBeforeRun guards the test-setup path:
// DispatchContext must never return a nil context, even when Run was never
// started (e.g. tests that just exercise handlers directly).
func TestHub_DispatchContextFallsBackBeforeRun(t *testing.T) {
	h := newTestHub(t)

	got := h.DispatchContext()
	if got == nil {
		t.Fatal("DispatchContext returned nil before Run; handlers would NPE")
	}
	if err := got.Err(); err != nil {
		t.Fatalf("fallback ctx should not be done, got err=%v", err)
	}
}

// TestHub_DispatchContextTracksRunCtx checks that DispatchContext returns the
// ctx Run was called with, so it cancels on server shutdown (the only
// cancellation reason that should still kill dispatched handlers).
func TestHub_DispatchContextTracksRunCtx(t *testing.T) {
	h := newTestHub(t)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Run(runCtx)

	// Wait briefly for Run to install the ctx. Polling beats a fixed sleep
	// because Run starts and stores the ctx synchronously after the lock,
	// but the goroutine still needs to be scheduled.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.DispatchContext().Err() == nil && h.DispatchContext() != context.Background() {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	dispatchCtx := h.DispatchContext()
	if dispatchCtx.Err() != nil {
		t.Fatalf("dispatchCtx prematurely done: %v", dispatchCtx.Err())
	}

	cancel()

	// After Run's ctx is cancelled, DispatchContext should reflect that —
	// otherwise shutdown can't interrupt in-flight handlers.
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if dispatchCtx.Err() != nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if dispatchCtx.Err() == nil {
		t.Fatal("dispatchCtx did not cancel when Run ctx was cancelled")
	}
}

// TestClient_HandleMessageUsesHubCtxNotConnCtx is the regression test for the
// SIGKILL cascade: a dispatched handler must NOT see a cancelled context when
// the originating client disconnects. Before the fix, the dispatcher was
// called with the connection ctx, so any in-flight exec.CommandContext
// subprocess (gh, git, agentctl HTTP) got SIGKILL'd the moment the user
// navigated away from the page.
func TestClient_HandleMessageUsesHubCtxNotConnCtx(t *testing.T) {
	h := newTestHub(t)
	dispatcher := ws.NewDispatcher()
	h.dispatcher = dispatcher

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go h.Run(hubCtx)

	// Let Run install its ctx.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.DispatchContext() != context.Background() {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Handler that records what ctx it received and waits long enough for
	// the test to cancel a fake connection ctx before returning. If the
	// dispatcher were still wired to the connection ctx, gotCtx.Err()
	// below would be non-nil.
	handlerEntered := make(chan struct{})
	releaseHandler := make(chan struct{})
	var gotCtx context.Context
	dispatcher.RegisterFunc("test.write", func(ctx context.Context, _ *ws.Message) (*ws.Message, error) {
		gotCtx = ctx
		close(handlerEntered)
		<-releaseHandler
		return nil, nil
	})

	c := newTestClient("c-disconnect")
	c.hub = h

	// Run handleMessage in a goroutine so we can cancel the "connection"
	// while the handler is mid-flight.
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.handleMessage(&ws.Message{Action: "test.write", Type: ws.MessageTypeRequest})
	}()

	select {
	case <-handlerEntered:
	case <-time.After(time.Second):
		t.Fatal("handler never entered; dispatch wiring is broken")
	}

	// The handler is inside Dispatch. In production this is the moment the
	// WS client disconnects. Verify the handler's ctx is not derived from
	// any connection lifetime: we don't even have a connection ctx to
	// cancel here, which is exactly the point — handleMessage no longer
	// takes one.
	if gotCtx == nil {
		t.Fatal("handler did not receive a context")
	}
	if err := gotCtx.Err(); err != nil {
		t.Fatalf("handler ctx already done at entry: %v", err)
	}

	// Sanity: the handler ctx must be the hub's lifetime ctx (or derived
	// from it). Cancel the hub and observe.
	hubCancel()
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if gotCtx.Err() != nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !errors.Is(gotCtx.Err(), context.Canceled) {
		t.Fatalf("handler ctx should cancel with hub shutdown, got err=%v", gotCtx.Err())
	}

	close(releaseHandler)
	<-done
}

// TestClient_HandleMessageSurvivesConnectionTeardown is the higher-level
// regression: even though ReadPump may exit and the client may be torn down,
// already-dispatched handlers continue running. Models the real bug — a
// session.launch already inside the handler when the WS closes.
func TestClient_HandleMessageSurvivesConnectionTeardown(t *testing.T) {
	h := newTestHub(t)
	dispatcher := ws.NewDispatcher()
	h.dispatcher = dispatcher

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go h.Run(hubCtx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.DispatchContext() != context.Background() {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	handlerCompleted := make(chan error, 1)
	releaseHandler := make(chan struct{})
	dispatcher.RegisterFunc("session.launch.fake", func(ctx context.Context, _ *ws.Message) (*ws.Message, error) {
		// Mimic an exec.CommandContext-style subroutine that watches ctx.
		select {
		case <-releaseHandler:
			handlerCompleted <- ctx.Err()
		case <-ctx.Done():
			handlerCompleted <- ctx.Err()
		}
		return nil, nil
	})

	c := newTestClient("c-teardown")
	c.hub = h

	go c.handleMessage(&ws.Message{Action: "session.launch.fake", Type: ws.MessageTypeRequest})

	// Give the handler a moment to enter.
	time.Sleep(20 * time.Millisecond)

	// Simulate connection teardown: close the client's send channel and
	// drop our hub reference, the way removeClient would. Crucially, this
	// does NOT cancel the hub ctx — only client-scoped state goes away.
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	// Handler must still be running. Release it explicitly.
	close(releaseHandler)

	select {
	case err := <-handlerCompleted:
		if err != nil {
			t.Fatalf("handler ctx was cancelled by connection teardown; want nil, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler never completed; deadlock or wrong ctx wiring")
	}
}
