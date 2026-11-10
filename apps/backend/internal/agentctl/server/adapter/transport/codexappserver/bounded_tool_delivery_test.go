package codexappserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestBoundedToolNativeEmission(t *testing.T) {
	payload := streams.NewGeneric("arbitrary-tool", nil)
	original := strings.Repeat("x", 1048605)
	payload.Generic().Output = original
	a := &Adapter{updates: make(chan streams.AgentEvent, 1), updatesDone: make(chan struct{})}
	a.emit(streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool", ToolStatus: "complete", NormalizedPayload: payload})
	retained := <-a.updates
	encoded, err := json.Marshal(retained)
	require.NoError(t, err)
	require.Less(t, len(encoded), 1<<20)
	require.Equal(t, true, retained.NormalizedPayload.Generic().Output.(map[string]any)["truncated"])
	require.Equal(t, original, payload.Generic().Output)
}
