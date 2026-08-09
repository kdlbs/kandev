package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestProbeBackgroundWorkloads_EmptySessionID verifies AC-46's ninth
// condition: an empty Kandev task-session id resolves to unknown with no
// lookup attempted.
func TestProbeBackgroundWorkloads_EmptySessionID(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_CancelledContext verifies AC-46's tenth
// condition: a context already done on entry resolves to unknown with no
// lookup attempted, even for a session that exists.
func TestProbeBackgroundWorkloads_CancelledContext(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-1", SessionID: "sess-1", ACPSessionID: "acp-1",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := mgr.ProbeBackgroundWorkloads(ctx, "sess-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_NoExecution verifies that a Kandev
// task-session id with no execution — untranslatable — resolves to unknown.
func TestProbeBackgroundWorkloads_NoExecution(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "no-such-session")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_EmptyACPSessionID verifies that an execution
// with no ACP session id yet (not fully initialized) resolves to unknown
// rather than probing with an empty id.
func TestProbeBackgroundWorkloads_EmptyACPSessionID(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-2", SessionID: "sess-2", ACPSessionID: "",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "sess-2")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_NilClient verifies that an execution with no
// agentctl client attached yet resolves to unknown.
func TestProbeBackgroundWorkloads_NilClient(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-3", SessionID: "sess-3", ACPSessionID: "acp-3",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "sess-3")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_TransportErrorResolvesToUnknown verifies that
// a client transport error (here: the client is unconnected, so the stream
// request fails immediately) resolves to unknown per the port's contract —
// the error is swallowed, never surfaced to the caller.
func TestProbeBackgroundWorkloads_TransportErrorResolvesToUnknown(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	execution := &AgentExecution{
		ID: "ex-4", SessionID: "sess-4", ACPSessionID: "acp-4",
		agentctl: &agentctl.Client{},
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "sess-4")
	if err != nil {
		t.Fatalf("expected nil error (port contract swallows transport errors), got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}
