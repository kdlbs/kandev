package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestNotifyCanonicalAgentEventBatchCarriesLaterCompactionMetadata(t *testing.T) {
	execution := createTestExecution("exec-native-batch", "task-native-batch", "session-native-batch")
	var notified agentctl.AgentEvent
	streamsManager := &StreamManager{
		callbacks: StreamCallbacks{
			OnAgentEventWithGeneration: func(_ *AgentExecution, event agentctl.AgentEvent, _ uint64) {
				notified = event
			},
		},
	}
	count := int64(2)
	reporting := &streams.ContextCompactionReporting{
		Source:       streams.ContextCompactionSourceCopilotNative,
		Availability: streams.ContextCompactionAvailabilityAvailable,
		Coverage:     streams.ContextCompactionCoverageObserved,
		Revision:     7,
	}
	prepared := []preparedAgentEvent{
		{
			event:        agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, SessionID: execution.SessionID, Text: "hello"},
			durableEvent: &models.AgentDeliveryEvent{StreamID: "stream-native-batch", Sequence: 1},
		},
		{
			event:        agentctl.AgentEvent{Type: streams.EventTypeContextCompactionObservation, SessionID: execution.SessionID},
			durableEvent: &models.AgentDeliveryEvent{StreamID: "stream-native-batch", Sequence: 2},
		},
	}
	results := []models.CanonicalAgentDeliveryProjectionResult{
		{CanonicalMessageID: "message-1"},
		{ContextCompactionCount: &count, ContextCompactionReporting: reporting},
	}

	streamsManager.notifyCanonicalAgentEventBatch(execution, prepared, results, nil, 0)

	if notified.Type != streams.EventTypeMessageChunk {
		t.Fatalf("notified type = %q, want the first notifying message chunk", notified.Type)
	}
	if got := notified.Data[models.SessionMetaKeyContextCompactionCount]; got != int64(2) {
		t.Fatalf("batch notification count = %v, want the later committed count", got)
	}
	stored, ok := notified.Data[models.SessionMetaKeyContextCompactionReporting].(*streams.ContextCompactionReporting)
	if !ok || stored.Revision != 7 {
		t.Fatalf("batch notification reporting = %#v, want the later committed revision", notified.Data)
	}
}
