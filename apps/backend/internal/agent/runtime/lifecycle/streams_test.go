package lifecycle

import (
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestConnectWorkspaceStream_IdempotentWhenAlreadyAttached is the regression
// test for the workspace-stream double-connect race. Two paths can call
// connectWorkspaceStream for the same execution (e.g. workspace-only ensure
// followed by full-launch promotion). The second call previously hit
// "workspace stream already connected" and burned 5 retries before logging
// a terminal ERROR. The fix short-circuits when a stream is already attached.
func TestConnectWorkspaceStream_IdempotentWhenAlreadyAttached(t *testing.T) {
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)

	execution := &AgentExecution{ID: "exec-1", SessionID: "sess-1"}
	// Pre-attach a non-nil workspace stream — simulates another goroutine
	// having already connected before this call.
	execution.SetWorkspaceStream(&agentctl.WorkspaceStream{})

	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		sm.connectWorkspaceStream(execution, ready)
		close(done)
	}()

	// Should return effectively immediately (well under the 1s first-retry
	// backoff). 500ms gives ample headroom on slow CI without masking a
	// regression that would burn through the full 5-retry loop.
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connectWorkspaceStream did not exit early when stream was already attached")
	}

	// ready must be closed (deferred signalReady runs even on early exit).
	select {
	case <-ready:
	default:
		t.Error("ready channel was not closed on early-exit path")
	}
}

// TestConnectWorkspaceStream_BackoffDrainsOnStop is the regression test for
// the workspace-stream retry-backoff leak. Before the fix, connectWorkspaceStream
// slept the full backoff in time.Sleep between retries; if the test (or
// production Manager) tore down while a retry was in flight, the goroutine
// stranded until the backoff fired, surviving the Manager's Stop(). The fix
// selects on stopCh so the backoff drains immediately on shutdown.
//
// We trigger the failing path by pointing the client at a closed port —
// StreamWorkspace returns "connection refused" on every attempt, sending the
// loop into its retry backoff. Closing stopCh must release the goroutine in
// well under the (uncapped) backoff window.
func TestConnectWorkspaceStream_BackoffDrainsOnStop(t *testing.T) {
	stopCh := make(chan struct{})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, stopCh)

	log := newTestLogger()
	badClient := agentctl.NewClient("127.0.0.1", 1, log) // port 1 is reserved
	defer badClient.Close()

	execution := &AgentExecution{
		ID:        "exec-backoff",
		SessionID: "sess-backoff",
		agentctl:  badClient,
	}

	done := make(chan struct{})
	go func() {
		sm.connectWorkspaceStream(execution, nil)
		close(done)
	}()

	close(stopCh)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connectWorkspaceStream did not drain on stopCh close — backoff is leaking")
	}
}

// TestConnectWorkspaceStream_ClosesWSOnStop is the regression test for the
// connected-stream shutdown leak. Once StreamWorkspace returns successfully
// the goroutine parks on `<-ws.Done()`. If the manager shuts down while the
// remote WS is still alive, simply observing stopCh isn't enough — the
// underlying WS connection (and the agentctl-side read/write loops it owns)
// keep running. The fix calls ws.Close() on the stopCh path so the WS
// actually tears down, and ws.Done() fires for any other observer.
//
// We exercise this by pointing a real agentctl client at a mock workspace
// stream server that stays open until the client closes the WS. After the
// goroutine connects, we close stopCh and assert ws.Done() fires within
// a short window — which is only true if connectWorkspaceStream called
// ws.Close() on its way out.
func TestConnectWorkspaceStream_ClosesWSOnStop(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	stopCh := make(chan struct{})
	sm := NewStreamManager(newSessionTestLogger(), StreamCallbacks{}, nil, stopCh)

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	execution := &AgentExecution{
		ID:        "exec-close",
		SessionID: "sess-close",
		agentctl:  client,
	}

	done := make(chan struct{})
	go func() {
		sm.connectWorkspaceStream(execution, nil)
		close(done)
	}()

	// Wait for the stream to attach so we know we've reached the
	// post-connect select. 500ms is generous on slow CI but short
	// enough to fail loudly if the connect never lands.
	deadline := time.Now().Add(500 * time.Millisecond)
	var ws *agentctl.WorkspaceStream
	for time.Now().Before(deadline) {
		ws = execution.GetWorkspaceStream()
		if ws != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ws == nil {
		t.Fatal("workspace stream never attached")
	}

	close(stopCh)

	select {
	case <-ws.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ws.Done() did not fire on stopCh close — ws.Close() was not called")
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connectWorkspaceStream goroutine did not drain on stopCh close")
	}
}
