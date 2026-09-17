package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentToolConversationSurfaceRequiresExplicitVersionedOptIn(t *testing.T) {
	tool := AgentTool{Name: "inspect", Description: "Inspect an example", Surfaces: []string{"conversation"}, InputSchema: map[string]any{"type": "object"}}
	m := &Manifest{APIVersion: 2, Runtime: Runtime{Type: "binary"}, AgentTools: []AgentTool{tool}}
	require.Empty(t, m.validateAgentTools())
	m.APIVersion = 1
	require.NotEmpty(t, m.validateAgentTools())
	m.AgentTools[0].Surfaces = []string{AgentToolSurfaceKanban}
	require.Empty(t, m.validateAgentTools())
	require.Equal(t, []string{AgentToolSurfaceKanban}, m.AgentTools[0].Surfaces)
}
