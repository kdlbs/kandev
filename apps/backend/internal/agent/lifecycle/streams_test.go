package lifecycle

import (
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
)

// TestConnectWorkspaceStream_IdempotentWhenAlreadyAttached is the regression
// test for the workspace-stream double-connect race. Two paths can call
// connectWorkspaceStream for the same execution (e.g. workspace-only ensure
// followed by full-launch promotion). The second call previously hit
// "workspace stream already connected" and burned 5 retries before logging
// a terminal ERROR. The fix short-circuits when a stream is already attached.
func TestConnectWorkspaceStream_IdempotentWhenAlreadyAttached(t *testing.T) {
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil)

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
