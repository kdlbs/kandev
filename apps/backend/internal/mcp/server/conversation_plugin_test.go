package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/stretchr/testify/require"
)

func TestPluginToolConversationExplicitOptIn(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := New(backend, "session", "task", 10005, log, "", false, ModeConversation)
	tool := plugintools.Definition{PluginID: "example", LocalName: "inspect", ExposedName: plugintools.ExposedName("example", "inspect"), Description: "Inspect example", InputSchema: []byte(`{"type":"object"}`), Surfaces: []string{plugintools.SurfaceKanban}}
	require.NoError(t, server.SetPluginTools(plugintools.Snapshot{Generation: "one", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.NotContains(t, server.mcpServer.ListTools(), tool.ExposedName)
	tool.Surfaces = []string{"conversation"}
	require.NoError(t, server.SetPluginTools(plugintools.Snapshot{Generation: "one", Revision: 2, Tools: []plugintools.Definition{tool}}))
	require.Contains(t, server.mcpServer.ListTools(), "kandev_example_inspect")
	require.NoError(t, server.SetPluginTools(plugintools.Snapshot{Generation: "one", Revision: 3}))
	require.NotContains(t, server.mcpServer.ListTools(), tool.ExposedName)
}
