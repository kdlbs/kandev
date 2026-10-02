package lifecycle

import (
	"encoding/json"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
func TestContinuationSafetySnapshotTerminalEventRoundTrip(t *testing.T) {
	mgr, bus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.promptGeneration = 7
	require.NoError(t, mgr.executionStore.Add(execution))
	snapshot := &streams.ContinuationSafetySnapshot{Support: streams.ContinuationNativeSavedHistoryV1, PromptGeneration: 7, Known: true, CompletedReads: 1}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "error", Error: "interrupted", PromptGeneration: 7, ContinuationSafety: snapshot})
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for _, published := range bus.PublishedEvents {
		if published.Subject != events.AgentFailed {
			continue
		}
		raw, err := json.Marshal(published.Event.Data)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(raw, &wire))
		require.Contains(t, wire, "continuation_safety", "terminal snapshot must reach lifecycle bus")
		return
	}
	t.Fatal("no agent.failed event")
}
