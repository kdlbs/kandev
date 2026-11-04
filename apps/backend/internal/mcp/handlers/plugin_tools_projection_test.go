package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleListPluginToolsProjectsAutomationSurfaceForOlderClients(t *testing.T) {
	registry := plugins.NewRegistry()
	registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "mixed-plugin",
			AgentTools: []manifest.AgentTool{
				{Name: "mixed", Description: "Mixed", Surfaces: []string{manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceAutomation}, InputSchema: map[string]any{"type": "object"}},
				{Name: "automation_only", Description: "Automation only", Surfaces: []string{manifest.AgentToolSurfaceAutomation}, InputSchema: map[string]any{"type": "object"}},
			},
		},
		Status: store.StatusActive, InstallationID: "installation-1", AgentToolPolicyVersion: "policy-1",
	})
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(plugins.NewService(nil, registry, nil, testLogger(t)))

	response, err := h.handleListPluginTools(context.Background(), makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceKanban,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(response.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)
	require.Equal(t, "mixed", snapshot.Tools[0].LocalName)
	require.Equal(t, []string{plugintools.SurfaceKanban}, snapshot.Tools[0].Surfaces)

	response, err = h.handleListPluginTools(context.Background(), makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{}))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(response.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1, "an unscoped ordinary pull must omit automation-only tools")
	require.Equal(t, "mixed", snapshot.Tools[0].LocalName)
	require.Equal(t, []string{plugintools.SurfaceKanban}, snapshot.Tools[0].Surfaces)
}
