package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// activityValues returns the foreground_activity payloads of every
// task_session.activity_changed event recorded on the bus, in publish order.
// It is the operator-facing WS1 signal the composer and status indicator read.
func activityValues(eb *recordingEventBus) []string {
	var vals []string
	for _, rec := range eb.events {
		if rec.subject != events.TaskSessionActivityChanged {
			continue
		}
		if data, ok := rec.event.Data.(map[string]interface{}); ok {
			if v, ok := data["foreground_activity"].(string); ok {
				vals = append(vals, v)
			}
		}
	}
	return vals
}

// TestForegroundActivitySignal_PublishesOnFlips proves the WS1 producer emits
// the fine-grained busy signal exactly when the foreground/background substate
// flips — background when the agent yields to a spawned task, generating again
// when it streams foreground output — so the web composer/status can distinguish
// the three conditions without a coarse session-state transition.
func TestForegroundActivitySignal_PublishesOnFlips(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-activity"
	)

	// A top-level subagent tool_call: the foreground yields to background work.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
		},
	})

	// A streamed foreground message: the agent is generating again even though
	// the subagent is still outstanding.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:      "message_streaming",
			MessageID: "m1",
			Text:      "still working on it",
		},
	})

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground), string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) {
		t.Fatalf("expected activity signal on each flip %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity flip %d: expected %q, got %q (all: %v)", i, want[i], got[i], got)
		}
	}
}

// TestForegroundActivitySignal_NoPublishWithoutFlip proves the signal is emitted
// only on a real substate transition, never per background frame: a second
// concurrent background task, and completing all-but-the-last, must NOT publish.
func TestForegroundActivitySignal_NoPublishWithoutFlip(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-activity-2"
	)

	subagent := func(id string) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    taskID,
			SessionID: sessionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       agentEventToolCall,
				ToolCallID: id,
				ToolStatus: "running",
				Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
			},
		}
	}
	terminal := func(id string) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    taskID,
			SessionID: sessionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       "tool_update",
				ToolCallID: id,
				ToolStatus: agentEventComplete,
				Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
			},
		}
	}

	// First background task flips to background (publish #1). The second does not
	// flip anything (already yielded) — no publish.
	svc.handleAgentStreamEvent(context.Background(), subagent("subagent-1"))
	svc.handleAgentStreamEvent(context.Background(), subagent("subagent-2"))

	// Completing the first while the second is still outstanding does not flip —
	// no publish. Completing the last flips back to generating (publish #2).
	svc.handleAgentStreamEvent(context.Background(), terminal("subagent-1"))
	svc.handleAgentStreamEvent(context.Background(), terminal("subagent-2"))

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground), string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) {
		t.Fatalf("expected exactly one publish per real flip %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flip %d: expected %q, got %q (all: %v)", i, want[i], got[i], got)
		}
	}
}
