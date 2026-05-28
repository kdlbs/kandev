package acp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestHandlePermissionRequest_NoDuplicateWhenToolCallNotificationLanded verifies
// the activeToolCalls check suppresses the synthetic tool_call emit when a
// SessionUpdate.ToolCall has already populated the map. Pre-existing guard;
// covered here so the race fix below has a baseline counterpart.
func TestHandlePermissionRequest_NoDuplicateWhenToolCallNotificationLanded(t *testing.T) {
	a := newTestAdapter()
	a.activeToolCalls["tool-1"] = &streams.NormalizedPayload{}

	_, _ = a.handlePermissionRequest(context.Background(), &PermissionRequest{
		SessionID:  "sess-1",
		ToolCallID: "tool-1",
		Title:      "Kandev: List Workspaces",
		ActionType: "other",
	})

	for _, ev := range drainEvents(a) {
		if ev.Type == streams.EventTypeToolCall {
			t.Fatalf("did not expect synthetic tool_call when notification already tracked, got %#v", ev)
		}
	}
}

// TestHandlePermissionRequest_WaitsForRacingToolCallNotification covers the
// race window: the SDK delivers a ToolCall SessionUpdate and a
// request_permission for the same toolCallID on separate goroutines, and the
// permission handler may run before the notification handler has populated
// activeToolCalls. Without the bounded wait, the handler would emit a
// synthetic tool_call that becomes a duplicate message in the chat.
func TestHandlePermissionRequest_WaitsForRacingToolCallNotification(t *testing.T) {
	a := newTestAdapter()

	go func() {
		time.Sleep(20 * time.Millisecond)
		a.mu.Lock()
		a.activeToolCalls["tool-1"] = &streams.NormalizedPayload{}
		a.mu.Unlock()
	}()

	_, _ = a.handlePermissionRequest(context.Background(), &PermissionRequest{
		SessionID:  "sess-1",
		ToolCallID: "tool-1",
		Title:      "Kandev: List Workspaces",
		ActionType: "other",
	})

	for _, ev := range drainEvents(a) {
		if ev.Type == streams.EventTypeToolCall {
			t.Fatalf("waited for racing tool_call notification but still emitted synthetic %#v", ev)
		}
	}
}

// TestHandlePermissionRequest_EmitsSyntheticAfterTimeout exercises the
// fallback path: when no ToolCall notification ever arrives, the bounded wait
// must give up and emit the synthetic tool_call so the UI still gets a row to
// attach the approval to. Agents that skip the ToolCall notification and go
// straight to request_permission rely on this path.
func TestHandlePermissionRequest_EmitsSyntheticAfterTimeout(t *testing.T) {
	a := newTestAdapter()

	start := time.Now()
	_, _ = a.handlePermissionRequest(context.Background(), &PermissionRequest{
		SessionID:  "sess-1",
		ToolCallID: "tool-1",
		Title:      "Kandev: List Workspaces",
		ActionType: "other",
	})
	elapsed := time.Since(start)

	if elapsed < syntheticToolCallRaceWindow {
		t.Errorf("handler returned before the race window expired: elapsed=%v window=%v", elapsed, syntheticToolCallRaceWindow)
	}

	var synthetic *AgentEvent
	for _, ev := range drainEvents(a) {
		if ev.Type == streams.EventTypeToolCall && ev.ToolCallID == "tool-1" {
			ev := ev
			synthetic = &ev
		}
	}
	if synthetic == nil {
		t.Fatalf("expected synthetic tool_call after timeout, got none")
	}
	if synthetic.ToolStatus != "pending_permission" {
		t.Errorf("synthetic ToolStatus = %q, want %q", synthetic.ToolStatus, "pending_permission")
	}
	if synthetic.ToolTitle != "Kandev: List Workspaces" {
		t.Errorf("synthetic ToolTitle = %q, want %q", synthetic.ToolTitle, "Kandev: List Workspaces")
	}
}
