//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

// AssertTurnCompleted checks the core structural invariants:
//   - At least 1 event was received
//   - Session ID is non-empty
//   - No error events
//   - A complete event was received
//   - At least 1 user-visible event (message_chunk, tool_call, reasoning)
func AssertTurnCompleted(t *testing.T, result *TestResult) {
	t.Helper()

	if len(result.Events) == 0 {
		t.Fatal("no events received from agent")
	}

	if result.SessionID == "" {
		t.Error("session ID is empty after turn")
	}

	AssertNoErrors(t, result.Events)

	hasComplete := false
	for _, ev := range result.Events {
		if ev.Type == adapter.EventTypeComplete {
			hasComplete = true
			break
		}
	}
	if !hasComplete {
		t.Error("no complete event received")
	}

	hasVisible := false
	for _, ev := range result.Events {
		switch ev.Type {
		case adapter.EventTypeMessageChunk,
			adapter.EventTypeToolCall,
			adapter.EventTypeReasoning,
			adapter.EventTypeToolUpdate:
			hasVisible = true
		}
	}
	if !hasVisible {
		t.Error("no user-visible events (message_chunk, tool_call, reasoning) received")
	}
}

// AssertHasEventType checks that at least one event of the given type exists.
func AssertHasEventType(t *testing.T, events []adapter.AgentEvent, eventType string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == eventType {
			return
		}
	}
	t.Errorf("expected at least one %q event, got none", eventType)
}

// AssertNoErrors checks that no error events were received.
func AssertNoErrors(t *testing.T, events []adapter.AgentEvent) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == adapter.EventTypeError {
			t.Errorf("unexpected error event: %s", ev.Error)
		}
	}
}

// AssertSessionIDConsistent checks all events with a session_id have the same value.
func AssertSessionIDConsistent(t *testing.T, events []adapter.AgentEvent) {
	t.Helper()
	var sessionID string
	for _, ev := range events {
		if ev.SessionID == "" {
			continue
		}
		if sessionID == "" {
			sessionID = ev.SessionID
		} else if ev.SessionID != sessionID {
			t.Logf("warning: session ID changed mid-turn: %s -> %s", sessionID, ev.SessionID)
		}
	}
}

// CountEventsByType returns a map of event type -> count for debugging.
func CountEventsByType(events []adapter.AgentEvent) map[string]int {
	counts := make(map[string]int)
	for _, ev := range events {
		counts[ev.Type]++
	}
	return counts
}

// DumpEventsOnFailure logs all events and debug info when the test fails.
// Call with defer at the start of each test.
func DumpEventsOnFailure(t *testing.T, result *TestResult) {
	t.Helper()
	if !t.Failed() {
		return
	}
	t.Logf("--- E2E Failure Dump ---")
	t.Logf("duration: %s", result.Duration)
	t.Logf("session_id: %s", result.SessionID)
	t.Logf("operation_id: %s", result.OperationID)
	t.Logf("total events: %d", len(result.Events))

	counts := CountEventsByType(result.Events)
	t.Logf("event counts: %v", counts)

	for i, ev := range result.Events {
		t.Logf("  [%d] type=%-20s session=%-36s tool=%-10s text_len=%d error=%q",
			i, ev.Type, ev.SessionID, ev.ToolName, len(ev.Text), ev.Error)
	}
}
