package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-ACP-V2-002.1
func TestCanonicalMessageReplacementPreservesPromptGeneration(t *testing.T) {
	manager, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	require.NoError(t, manager.executionStore.Add(execution))
	generation, err := manager.executionStore.BeginPrompt(execution.ID)
	require.NoError(t, err)
	manager.executionStore.MarkPromptDispatched(execution.ID, generation)

	manager.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                  streams.EventTypeMessageChunk,
		Text:                  "replacement answer",
		CanonicalProjection:   true,
		CanonicalMessageID:    "answer",
		ReplaceMessageContent: true,
	})
	const reportedGeneration = 31
	manager.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                  streams.EventTypeReasoning,
		CanonicalProjection:   true,
		CanonicalMessageID:    "reasoning",
		ReplaceMessageContent: true,
		PromptGeneration:      reportedGeneration,
	})

	answers := streamEventsOfType(eventBus, "message_streaming")
	require.Len(t, answers, 1)
	require.Equal(t, "answer", answers[0].Data.MessageID)
	require.Equal(t, "replacement answer", answers[0].Data.Text)
	require.Equal(t, generation, answers[0].Data.PromptGeneration)

	thoughts := streamEventsOfType(eventBus, thinkingStreamingEventType)
	require.Len(t, thoughts, 1)
	require.Equal(t, "reasoning", thoughts[0].Data.MessageID)
	require.Empty(t, thoughts[0].Data.Text)
	require.EqualValues(t, reportedGeneration, thoughts[0].Data.PromptGeneration)
}
