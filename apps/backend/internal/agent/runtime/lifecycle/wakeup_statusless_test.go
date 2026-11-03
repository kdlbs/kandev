package lifecycle

import (
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestWakeup_StatuslessToolUpdateDoesNotFlipToRunning pins the agentctl
// finalizer boundary: a tool_update with no status carries metadata only, so
// the session must stay Ready and must not publish agent.running. The finalizer
// ignores that update, and a backend flip here would leave the session busy for
// a turn nothing will ever close.
func TestWakeup_StatuslessToolUpdateDoesNotFlipToRunning(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	_ = mgr.executionStore.Add(execution)

	execution.firstActivityOnce.Do(func() {})
	mgr.executionStore.UpdateStatus(execution.ID, v1.AgentStatusReady)

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_update",
		ToolCallID: "tool-1",
	})

	for _, published := range eventBus.PublishedEvents {
		if published.Event != nil && published.Event.Type == events.AgentRunning {
			t.Fatal("status-less tool update published agent.running")
		}
	}
	if execution.Status != v1.AgentStatusReady {
		t.Fatalf("execution.Status = %q, want %q", execution.Status, v1.AgentStatusReady)
	}

	execution.lastActivityAtMu.Lock()
	lastActivityAt := execution.lastActivityAt
	execution.lastActivityAtMu.Unlock()
	if lastActivityAt.IsZero() {
		t.Fatal("status-less tool update must still refresh the activity timestamp")
	}
}

// TestWakeup_MissingWireStatusToolUpdateDoesNotFlipToRunning covers the
// adapter's synthesized status: a metadata-only ACP update with no wire status
// can carry in_progress after shell-output recognition. The finalizer still
// ignores it, so the lifecycle boundary must too.
func TestWakeup_MissingWireStatusToolUpdateDoesNotFlipToRunning(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	_ = mgr.executionStore.Add(execution)

	execution.firstActivityOnce.Do(func() {})
	mgr.executionStore.UpdateStatus(execution.ID, v1.AgentStatusReady)

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "tool_update",
		ToolCallID:        "tool-1",
		ToolStatus:        "in_progress",
		ToolStatusMissing: true,
	})

	for _, published := range eventBus.PublishedEvents {
		if published.Event != nil && published.Event.Type == events.AgentRunning {
			t.Fatal("missing-wire-status tool update published agent.running")
		}
	}
	if execution.Status != v1.AgentStatusReady {
		t.Fatalf("execution.Status = %q, want %q", execution.Status, v1.AgentStatusReady)
	}
}

// TestWakeup_TurnContentStillFlipsToRunning keeps the positive side of the
// same boundary: real turn content and an active tool update still start the
// agent-started turn.
func TestWakeup_TurnContentStillFlipsToRunning(t *testing.T) {
	tests := []struct {
		name  string
		event agentctl.AgentEvent
	}{
		{name: "message chunk", event: agentctl.AgentEvent{Type: "message_chunk", Text: "hello"}},
		{name: "tool call", event: agentctl.AgentEvent{Type: "tool_call", ToolCallID: "tool-1", ToolName: "bash"}},
		{name: "active tool update", event: agentctl.AgentEvent{Type: "tool_update", ToolCallID: "tool-1", ToolStatus: "in_progress"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("exec-1", "task-1", "session-1")
			_ = mgr.executionStore.Add(execution)

			execution.firstActivityOnce.Do(func() {})
			mgr.executionStore.UpdateStatus(execution.ID, v1.AgentStatusReady)

			// Clear the timestamp so the activity assertion is independent of
			// when the execution was created.
			execution.lastActivityAtMu.Lock()
			execution.lastActivityAt = time.Time{}
			execution.lastLiveActivityAt = time.Time{}
			execution.lastActivityAtMu.Unlock()

			mgr.handleAgentEvent(execution, tt.event)

			published := false
			for _, event := range eventBus.PublishedEvents {
				if event.Event != nil && event.Event.Type == events.AgentRunning {
					published = true
					break
				}
			}
			if !published {
				t.Fatal("turn content did not publish agent.running")
			}
			if execution.Status != v1.AgentStatusRunning {
				t.Fatalf("execution.Status = %q, want %q", execution.Status, v1.AgentStatusRunning)
			}
		})
	}
}
