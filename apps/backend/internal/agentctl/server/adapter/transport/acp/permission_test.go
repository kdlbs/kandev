package acp

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestHandlePermissionRequest_ToolAlreadyTerminal_AutoCancels exercises the
// stale-permission guard: when a session/request_permission arrives for a
// tool_call whose terminal status was already streamed (the OpenCode bug from
// issue #717), the adapter must auto-cancel upstream and not emit any kandev
// event. Otherwise the orchestrator creates a permission_request message that
// has nothing to resolve it — pending forever.
func TestHandlePermissionRequest_ToolAlreadyTerminal_AutoCancels(t *testing.T) {
	a := newTestAdapter()

	// Seed an execute tool_call (status=pending) and stream a terminal update.
	seedExecuteToolCall(t, a, "tc-stale")
	completed := acp.ToolCallStatus("completed")
	if ev := a.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "tc-stale",
		Status:     &completed,
	}); ev == nil {
		t.Fatalf("seed: terminal tool_call_update returned nil")
	}

	// Drain seed events so we can assert nothing new is emitted by the guard.
	_ = drainEvents(a)

	// A handler that, if invoked, fails the test — the guard must short-circuit
	// before forwarding. Tracking whether it was called gives a clear failure
	// when the guard regresses.
	handlerCalled := false
	a.SetPermissionHandler(func(_ context.Context, _ *types.PermissionRequest) (*types.PermissionResponse, error) {
		handlerCalled = true
		return &types.PermissionResponse{OptionID: "allow"}, nil
	})

	resp, err := a.handlePermissionRequest(context.Background(), &types.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: "tc-stale",
		Title:      "Bash",
		Options: []streams.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
			{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if handlerCalled {
		t.Fatal("permission handler was invoked for an already-terminal tool_call; guard regressed")
	}
	if resp == nil || !resp.Cancelled {
		t.Fatalf("expected cancelled response, got %+v", resp)
	}

	events := drainEvents(a)
	for _, ev := range events {
		if ev.Type == streams.EventTypeToolCall && ev.ToolStatus == "pending_permission" {
			t.Fatalf("guard must not emit synthetic pending_permission tool_call event, got %+v", ev)
		}
	}
}

// TestHandlePermissionRequest_ToolStillActive_ForwardsToHandler is the
// happy-path counterpart: when the tool_call is still in-flight (no terminal
// update yet), the request must reach the handler unchanged.
func TestHandlePermissionRequest_ToolStillActive_ForwardsToHandler(t *testing.T) {
	a := newTestAdapter()
	seedExecuteToolCall(t, a, "tc-active")
	_ = drainEvents(a)

	handlerCalled := false
	a.SetPermissionHandler(func(_ context.Context, _ *types.PermissionRequest) (*types.PermissionResponse, error) {
		handlerCalled = true
		return &types.PermissionResponse{OptionID: "allow"}, nil
	})

	resp, err := a.handlePermissionRequest(context.Background(), &types.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: "tc-active",
		Title:      "Bash",
		Options: []streams.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if !handlerCalled {
		t.Fatal("permission handler must be invoked for an active tool_call")
	}
	if resp == nil || resp.OptionID != "allow" {
		t.Fatalf("expected allow response, got %+v", resp)
	}
}
