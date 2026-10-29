package lifecycle

import (
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

// @covers AC-PLATFORM-TURN-CONTINUITY-003.6
func TestCapacityContinuationRetainsOpenCodeRuntime(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-opencode"
	execution.AgentID = openCodeACPAgentID
	execution.setSessionInitialized(true)

	accepted := fixture.manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type:                     "complete",
		SessionID:                execution.SessionID,
		PromptGeneration:         1,
		Error:                    "Internal error: Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		CapacityContinuation: &streams.CapacityContinuationSnapshot{
			Support: streams.CapacityContinuationOpenCodeLiveSessionV1, PromptGeneration: 1,
			EvidenceComplete: true, CompletedTools: 1,
		},
		Data: map[string]any{"is_error": true},
	})
	require.True(t, accepted)
	require.Equal(t, v1.AgentStatusReady, execution.Status)
	require.Nil(t, execution.FinishedAt)

	var turnFailed, failed bool
	for _, event := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
		switch event.Type {
		case "agent.turn_failed":
			turnFailed = true
			raw, err := json.Marshal(event.Data)
			require.NoError(t, err)
			var payload AgentEventPayload
			require.NoError(t, json.Unmarshal(raw, &payload))
			require.Equal(t, streams.PromptFailureDispositionRetainRuntime, payload.PromptFailureDisposition)
			require.NotNil(t, payload.CapacityContinuation)
			require.Equal(t, streams.CapacityContinuationOpenCodeLiveSessionV1, payload.CapacityContinuation.Support)
		case "agent.failed":
			failed = true
		}
	}
	require.True(t, turnFailed, "the OpenCode retained turn failure must publish agent.turn_failed")
	require.False(t, failed, "an OpenCode retained turn failure must not publish a terminal execution failure")
}

func TestRetainableACPAgentCoversOpenCode(t *testing.T) {
	require.True(t, isRetainableACPAgent(openCodeACPAgentID))
	require.False(t, isRetainableACPAgent("unproven-acp"))
}
