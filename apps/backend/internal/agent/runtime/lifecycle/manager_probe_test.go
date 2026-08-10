package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	ws "github.com/kandev/kandev/pkg/websocket"
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

// TestProbeBackgroundWorkloads_DeniedSessionAccessResolvesToUnknown verifies
// F4 (Review round 2): ProbeBackgroundWorkloads resolves the execution via a
// bare *BySessionID lookup, which per apps/backend/CLAUDE.md's documented
// convention must call CheckSessionAccess itself since it skips the
// GetOrEnsure* chokepoint. A denial must resolve to "unknown" — matching
// every other failure path (AC-46) — and, mirroring
// TestExecutionAccessChecksGateBeforeCache's "before cache" pattern, the
// guard must run BEFORE the execution-store lookup so a cached execution for
// a session the caller does not own is never reached.
func TestProbeBackgroundWorkloads_DeniedSessionAccessResolvesToUnknown(t *testing.T) {
	denied := errors.New("denied")
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-5", SessionID: "sess-5", ACPSessionID: "acp-5",
	}); err != nil {
		t.Fatal(err)
	}
	mgr.SetSessionAccessChecker(func(_ context.Context, sessionID string) error {
		if sessionID == "sess-5" {
			return denied
		}
		return nil
	})

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "sess-5")
	if err != nil {
		t.Fatalf("expected nil error (denial maps to unknown, not surfaced), got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown", result)
	}
}

// TestProbeBackgroundWorkloads_AllowedSessionAccessProceeds verifies the
// guard's positive path: a checker that allows the session does not block
// the probe from reaching its normal unknown-mapping logic (here: no
// agentctl client attached yet).
func TestProbeBackgroundWorkloads_AllowedSessionAccessProceeds(t *testing.T) {
	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-6", SessionID: "sess-6", ACPSessionID: "acp-6",
	}); err != nil {
		t.Fatal(err)
	}
	mgr.SetSessionAccessChecker(func(context.Context, string) error { return nil })

	result, err := mgr.ProbeBackgroundWorkloads(context.Background(), "sess-6")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "unknown" {
		t.Fatalf("result = %q, want unknown (no agentctl client attached)", result)
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

// TestProbeBackgroundWorkloads_TranslatesKandevIDToACPIDOnTheWire closes
// AC-45's port-level translation gap: the AC names, as its distinguishing
// assertion, driving the port with a Kandev id and reading the ACP id off
// the emitted frame — "which fails for an implementation that passes the
// Kandev id straight through". Every prior test in this file short-circuits
// to "unknown" before any frame is emitted (no agentctl client attached), so
// none of them exercise this. Drives a real *agentctl.Client connected to a
// mock agentctl WebSocket server (mirroring session_test.go's
// newMockAgentServer/createTestClient pattern already used elsewhere in this
// package) and asserts the wire payload carries the ACP session id, not the
// Kandev session id the port was called with.
func TestProbeBackgroundWorkloads_TranslatesKandevIDToACPIDOnTheWire(t *testing.T) {
	mock := newMockAgentServer(t)
	defer mock.Close()

	var gotSessionID string
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.background.probe" {
			return nil
		}
		var payload struct {
			SessionID string `json:"session_id"`
		}
		_ = msg.ParsePayload(&payload)
		gotSessionID = payload.SessionID
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"result": "live"})
		return resp
	}

	client := createTestClient(t, mock.server.URL)
	defer client.Close()

	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("failed to connect stream: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let the stream goroutine start

	mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "ex-7", SessionID: "kandev-sess-7", ACPSessionID: "acp-sess-7",
		agentctl: client,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.ProbeBackgroundWorkloads(ctx, "kandev-sess-7")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "live" {
		t.Fatalf("result = %q, want live", result)
	}
	if gotSessionID != "acp-sess-7" {
		t.Fatalf("wire session_id = %q, want acp-sess-7 (the translated ACP id) — got the Kandev id straight through", gotSessionID)
	}
}
