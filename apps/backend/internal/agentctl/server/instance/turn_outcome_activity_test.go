package instance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestRetainedOutcomeActivityIsObservedOnceAndSurvivesWire(t *testing.T) {
	var state turnOutcomeState
	before := time.Now().UTC()
	state.Retain(9, streams.AgentEvent{Type: "complete", PromptGeneration: 4})
	after := time.Now().UTC()
	first, ok := state.Peek()
	require.True(t, ok)
	require.NotNil(t, first.Event.ActivityAt)
	require.False(t, first.Event.ActivityAt.Before(before))
	require.False(t, first.Event.ActivityAt.After(after))
	second, ok := state.Peek()
	require.True(t, ok)
	require.Equal(t, first.Event.ActivityAt, second.Event.ActivityAt)
	encoded, err := json.Marshal(first)
	require.NoError(t, err)
	var decoded TurnOutcome
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.True(t, decoded.Event.ActivityAt.Equal(*first.Event.ActivityAt))
	state.Clear(5)
	state.Retain(10, streams.AgentEvent{Type: "complete", PromptGeneration: 4})
	_, ok = state.Peek()
	require.False(t, ok, "a stale terminal cannot restore retired activity")
}
