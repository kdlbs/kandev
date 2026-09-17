package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyNativeMCPHasNoAmbientTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()
	profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, []string{"github"})
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, profile)
	tool := plugintools.Definition{PluginID: "example", LocalName: "inspect", ExposedName: "kandev_example_inspect", Description: "Advisory read-only", ReadOnlyHint: true, InputSchema: []byte(`{"type":"object"}`), Surfaces: []string{"conversation"}}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "one", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.Empty(t, s.mcpServer.ListTools())
}
