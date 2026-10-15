package lifecycle

import (
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.12
func TestNativeAsyncDeliveryDoesNotInheritCompletedPromptTurn(t *testing.T) {
	execution := &AgentExecution{SessionID: "session", promptTurnID: "completed-turn", promptGeneration: 10}
	for _, kind := range []string{"message_chunk", "tool_call", "complete"} {
		t.Run(kind, func(t *testing.T) {
			event := agentctl.AgentEvent{Type: kind, DeliveryStreamID: "stream", DeliverySequence: 100}
			stored := durableAgentEvent(execution, event)
			var payload agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(stored.Payload, &payload))
			require.Empty(t, payload.TurnID, "unowned native activity must not acquire an old dispatched turn")
		})
	}
}

func TestNativeAsyncCompletionDoesNotPublishCompletedPromptOwner(t *testing.T) {
	manager, bus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptTurnID = "completed-turn"
	execution.promptGeneration = 10
	execution.promptCompletionGeneration = 10
	require.NoError(t, manager.executionStore.Add(execution))
	manager.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete", SessionID: "session",
		Data: map[string]any{"synthetic": true, "synthetic_reason": "async_turn_idle"}})
	ready, complete := false, false
	for _, recorded := range bus.PublishedEvents {
		if recorded.Event == nil {
			continue
		}
		if recorded.Event.Type == events.AgentReady {
			payload := recorded.Event.Data.(AgentEventPayload)
			require.Empty(t, payload.TurnID)
			require.Zero(t, payload.PromptGeneration)
			ready = true
		}
		if payload, ok := recorded.Event.Data.(AgentStreamEventPayload); ok && payload.Data.Type == "complete" {
			require.Empty(t, payload.Data.TurnID)
			require.Zero(t, payload.Data.PromptGeneration)
			complete = true
		}
	}
	require.True(t, ready)
	require.True(t, complete)
}

func TestDurableEventKeepsExplicitDispatchOwnership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		event   agentctl.AgentEvent
		pending bool
		want    string
	}{
		{name: "explicit", event: agentctl.AgentEvent{TurnID: "native-turn"}, want: "native-turn"},
		{name: "current generation", event: agentctl.AgentEvent{PromptGeneration: 10}, want: "current-turn"},
		{name: "older generation", event: agentctl.AgentEvent{PromptGeneration: 9}, want: "previous-turn"},
		{name: "unknown generation", event: agentctl.AgentEvent{PromptGeneration: 8}},
		{name: "submission", event: agentctl.AgentEvent{DeliverySubmissionID: "submission"}, want: "current-turn"},
		{name: "pending legacy dispatch", pending: true, want: "current-turn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execution := &AgentExecution{SessionID: "session", promptTurnID: "current-turn", promptGeneration: 10,
				promptTurnIDs: map[uint64]string{9: "previous-turn"}}
			execution.dispatchedPromptPending.Store(tc.pending)
			stored := durableAgentEvent(execution, tc.event)
			var payload agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(stored.Payload, &payload))
			require.Equal(t, tc.want, payload.TurnID)
		})
	}
}
