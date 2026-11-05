package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func automationToolDefinition(localName string) plugintools.Definition {
	return plugintools.Definition{
		PluginID: "kandev-security", LocalName: localName,
		ExposedName: plugintools.ExposedName("kandev-security", localName),
		Description: localName, InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceAutomation},
	}
}

func TestAutomationPluginToolsGlobalPushCannotWiden(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "automation-session", "automation-task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())
	definition := automationToolDefinition("list_findings")

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 1, Tools: []plugintools.Definition{definition}}))
	require.NotContains(t, s.mcpServer.ListTools(), definition.ExposedName, "a global push must not install automation tools")

	require.NoError(t, s.SetAutomationPluginTools(plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{definition}}))
	require.Contains(t, s.mcpServer.ListTools(), definition.ExposedName)

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 2}))
	require.NotContains(t, s.mcpServer.ListTools(), definition.ExposedName, "global invalidation clears the filtered catalog")
}

func TestAutomationPluginToolsSelectionRefreshAtSameCatalogRevision(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "automation-session", "automation-task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())
	read := automationToolDefinition("list_findings")
	write := automationToolDefinition("complete_report")

	require.NoError(t, s.SetAutomationPluginTools(plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{read}}))
	require.Contains(t, s.mcpServer.ListTools(), read.ExposedName)
	require.NotContains(t, s.mcpServer.ListTools(), write.ExposedName)

	// A selection change can alter membership without a declaration change or
	// a new catalog revision.
	require.NoError(t, s.SetAutomationPluginTools(plugintools.Snapshot{Generation: "automation", Revision: 1, Tools: []plugintools.Definition{write}}))
	require.NotContains(t, s.mcpServer.ListTools(), read.ExposedName)
	require.Contains(t, s.mcpServer.ListTools(), write.ExposedName)
}
