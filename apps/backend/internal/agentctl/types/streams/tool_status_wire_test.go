package streams

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentEventRoundTripsToolStatusMissing(t *testing.T) {
	encoded, err := json.Marshal(AgentEvent{
		Type:              EventTypeToolUpdate,
		ToolStatus:        "in_progress",
		ToolStatusMissing: true,
	})
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"tool_status_missing":true`)

	var decoded AgentEvent
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.True(t, decoded.ToolStatusMissing,
		"the backend lifecycle boundary needs the adapter's wire provenance")

	omitted, err := json.Marshal(AgentEvent{Type: EventTypeToolUpdate, ToolStatus: "in_progress"})
	require.NoError(t, err)
	require.NotContains(t, string(omitted), "tool_status_missing")
}
