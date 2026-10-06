package lifecycle

import (
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/stretchr/testify/require"
)

func TestCapacityContinuationSnapshotTerminalEventRoundTrip(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.promptGeneration = 7
	require.NoError(t, mgr.executionStore.Add(execution))
	snapshot := &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: 7,
		EvidenceComplete: true, CompletedTools: 2,
	}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: streams.EventTypeError, Error: "capacity", PromptGeneration: 7, CapacityContinuation: snapshot,
	})
	snapshot.CompletedTools = 99

	eventBus.mu.Lock()
	defer eventBus.mu.Unlock()
	for _, published := range eventBus.PublishedEvents {
		if published.Subject != events.AgentFailed {
			continue
		}
		raw, err := json.Marshal(published.Event.Data)
		require.NoError(t, err)
		var payload AgentEventPayload
		require.NoError(t, json.Unmarshal(raw, &payload))
		require.Equal(t, &streams.CapacityContinuationSnapshot{
			Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: 7,
			EvidenceComplete: true, CompletedTools: 2,
		}, payload.CapacityContinuation)
		return
	}
	t.Fatal("no agent.failed event")
}

func TestCapacityContinuationSnapshotRejectsStaleGeneration(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.promptGeneration = 7
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: streams.EventTypeError, Error: "capacity", PromptGeneration: 7,
		CapacityContinuation: &streams.CapacityContinuationSnapshot{
			Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: 6,
			EvidenceComplete: true, CompletedTools: 2,
		},
	})

	eventBus.mu.Lock()
	defer eventBus.mu.Unlock()
	for _, published := range eventBus.PublishedEvents {
		if published.Subject != events.AgentFailed {
			continue
		}
		raw, err := json.Marshal(published.Event.Data)
		require.NoError(t, err)
		var payload AgentEventPayload
		require.NoError(t, json.Unmarshal(raw, &payload))
		require.Nil(t, payload.CapacityContinuation)
		return
	}
	t.Fatal("no agent.failed event")
}
