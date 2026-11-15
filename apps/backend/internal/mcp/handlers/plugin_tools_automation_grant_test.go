package handlers

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func providerUsageAutomationDefinition() plugintools.Definition {
	return plugintools.Definition{
		PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
		ExposedName:     plugintools.ExposedName("kandev-provider-usage", "get_provider_usage"),
		Description:     "Read cached instance usage.",
		Surfaces:        []string{plugintools.SurfaceAutomation},
		InvocationScope: plugintools.InvocationScopeInstance,
		InstallationID:  "installation-1",
		PolicyVersion:   "policy-1",
		InputSchema:     json.RawMessage(`{"type":"object","additionalProperties":false}`),
		ReadOnlyHint:    true,
	}
}

func TestProviderUsageRequiresAutomationGrant(t *testing.T) {
	h, ctx, _, _ := automationHandlerFixture(t)
	provider := &fakeAutomationPluginTools{}
	h.SetAutomationPluginTools(provider)

	unapprovedList, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(unapprovedList.Payload, &snapshot))
	require.Empty(t, snapshot.Tools, "adding a declaration must not approve it")

	provider.definitions = []plugintools.Definition{providerUsageAutomationDefinition()}
	approvedList, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(approvedList.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)

	unapprovedCall, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-unapproved", "surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(unapprovedCall.Type))
	require.Contains(t, string(unapprovedCall.Payload), "not approved")

	provider.authorization = &plugintools.AutomationAuthorization{
		AutomationID: "automation-1", WorkspaceID: "ws-state-event", PluginID: "kandev-provider-usage",
		InstallationID: "installation-1", PolicyVersion: "policy-1", ToolNames: []string{"get_provider_usage"},
	}
	approvedCall, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-approved", "surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(approvedCall.Type))
	require.Contains(t, string(approvedCall.Payload), "plugin tool invocation failed")
}
