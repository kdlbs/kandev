package acp

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// drainAdapterEventsWithoutComplete returns every event delivered within one
// idle window and reports whether a synthetic completion arrived.
func drainAdapterEventsWithoutComplete(
	t *testing.T,
	a *Adapter,
	window time.Duration,
) (events []AgentEvent, completed int) {
	t.Helper()
	time.Sleep(window)
	synctest.Wait()
	for {
		select {
		case event := <-a.updatesCh:
			if event.Type == streams.EventTypeComplete {
				completed++
				continue
			}
			events = append(events, event)
		default:
			return events, completed
		}
	}
}

// TestAsyncTurnComplete_StatuslessToolUpdateDoesNotHoldTurnOpen reproduces the
// recovered seq 1173 frame shape: a metadata-only tool_call_update for a tool
// the adapter already retired. A following foreground tool cycle must still
// produce exactly one synthetic completion.
func TestAsyncTurnComplete_StatuslessToolUpdateDoesNotHoldTurnOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeMessageChunk, SessionID: "session", Text: "follow-up output",
		})
		// PostToolUse metadata-only update: no status, no content, no title.
		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "launch",
		})
		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolCall, SessionID: "session", ToolCallID: "check",
			ToolStatus: toolStatusInProgress,
		})
		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "check",
			ToolStatus: toolStatusComplete,
		})

		_, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond)
		if completed != 1 {
			t.Fatalf("synthetic completions = %d, want exactly 1", completed)
		}
	})
}

// TestAsyncTurnComplete_StatuslessToolUpdateAloneStartsNoTurn pins that a
// status-less update is not turn content: with no other agent-started activity
// it must not synthesize a completion.
func TestAsyncTurnComplete_StatuslessToolUpdateAloneStartsNoTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "launch",
		})

		if _, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond); completed != 0 {
			t.Fatalf("synthetic completions = %d, want 0", completed)
		}
	})
}

// TestAsyncTurnComplete_StatuslessUpdateKeepsForegroundToolActive pins that a
// status-less update cannot retire a tool that still holds the turn open.
func TestAsyncTurnComplete_StatuslessUpdateKeepsForegroundToolActive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolCall, SessionID: "session", ToolCallID: "foreground",
			ToolStatus: toolStatusInProgress,
		})
		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "foreground",
		})

		if _, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond); completed != 0 {
			t.Fatalf("synthetic completions = %d, want 0 while the tool has no terminal status", completed)
		}
	})
}

// TestAsyncTurnComplete_ProviderBoundaryClosesTurnWithUnterminatedTool covers
// the provider's explicit end-of-turn boundary: even a tool that never reported
// a terminal status must not keep an agent-started turn open past it.
func TestAsyncTurnComplete_ProviderBoundaryClosesTurnWithUnterminatedTool(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolCall, SessionID: "session", ToolCallID: "foreground",
			ToolStatus: toolStatusInProgress,
		})
		a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
			UsageUpdate: &acp.SessionUsageUpdate{
				Size: 1_000_000,
				Used: 25_000,
				Meta: map[string]any{
					"_claude/origin": map[string]any{"kind": "task-notification"},
				},
			},
		}), 0)

		if _, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond); completed != 1 {
			t.Fatalf("synthetic completions = %d, want exactly 1 after the provider boundary", completed)
		}
	})
}

// TestAsyncTurnFinalizer_StatuslessToolUpdateDoesNotScheduleTurn pins that a
// status-less update never creates or extends an agent-started turn, even
// though it still counts as prompt content for turn attribution.
func TestAsyncTurnFinalizer_StatuslessToolUpdateDoesNotScheduleTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "launch",
		})

		a.asyncTurnMu.Lock()
		finalizers := len(a.asyncTurnFinalizers)
		a.asyncTurnMu.Unlock()
		if finalizers != 0 {
			t.Fatalf("status-less update created %d async turn finalizer(s)", finalizers)
		}
		if _, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond); completed != 0 {
			t.Fatalf("synthetic completions = %d, want 0", completed)
		}
	})
}

// TestHandleACPUpdate_StatuslessToolUpdateKeepsPromptGeneration pins that the
// finalizer's exclusion does not strip prompt ownership from the frame: the
// same status-less update still belongs to the live prompt turn.
func TestHandleACPUpdate_StatuslessToolUpdateKeepsPromptGeneration(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	_, turn := a.registerPromptTurn(context.Background(), 42)
	a.mu.Unlock()
	t.Cleanup(func() { a.clearPromptTurn(turn) })

	a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{ToolCallId: "launch"},
	}), 42)

	var update *AgentEvent
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeToolUpdate {
			eventCopy := event
			update = &eventCopy
			break
		}
	}
	if update == nil {
		t.Fatal("expected the status-less tool update to be delivered")
	}
	if update.PromptGeneration != 42 {
		t.Fatalf("prompt generation = %d, want 42", update.PromptGeneration)
	}
}

func TestHandleACPUpdate_StatuslessMetadataDoesNotReactivateCompletedTool(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool",
				Title:      "Read",
				Kind:       acp.ToolKind("read"),
				Status:     acp.ToolCallStatus("in_progress"),
			},
		}), 0)
		completed := acp.ToolCallStatus("completed")
		a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{ToolCallId: "tool", Status: &completed},
		}), 0)
		title := "Read package metadata"
		a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{ToolCallId: "tool", Title: &title},
		}), 0)

		a.asyncTurnMu.Lock()
		active := len(a.asyncTurnFinalizers["session"].activeTools)
		a.asyncTurnMu.Unlock()
		if active != 0 {
			t.Fatalf("status-less metadata reactivated %d completed tool(s)", active)
		}
		if _, completed := drainAdapterEventsWithoutComplete(t, a, 50*time.Millisecond); completed != 1 {
			t.Fatalf("synthetic completions = %d, want exactly 1", completed)
		}
	})
}
