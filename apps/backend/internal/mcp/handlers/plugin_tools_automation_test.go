package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type fakeAutomationPluginTools struct {
	definitions   []plugintools.Definition
	authorization *plugintools.AutomationAuthorization
	err           error
}

func (f *fakeAutomationPluginTools) PluginToolOptions(context.Context, string) ([]plugintools.OptionGroup, error) {
	return nil, f.err
}

func (f *fakeAutomationPluginTools) ApprovedPluginTools(context.Context, string, string) ([]plugintools.Definition, error) {
	return f.definitions, f.err
}

func (f *fakeAutomationPluginTools) AutomationToolAuthorization(context.Context, string, string, string) (*plugintools.AutomationAuthorization, error) {
	return f.authorization, f.err
}

func automationHandlerFixture(t *testing.T) (*Handlers, context.Context, *plugins.Service, string) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	seedMCPHandlerSession(t, repo, "task-owned", "session-owned", models.TaskSessionStateRunning)
	registry := plugins.NewRegistry()
	registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-security",
			AgentTools: []manifest.AgentTool{
				{Name: "list_findings", Description: "List findings", Surfaces: []string{manifest.AgentToolSurfaceAutomation}, InputSchema: map[string]any{"type": "object"}},
				{Name: "complete_report", Description: "Complete report", Surfaces: []string{manifest.AgentToolSurfaceAutomation}, InputSchema: map[string]any{"type": "object"}},
			},
		},
		Status: store.StatusActive, InstallationID: "installation-1", AgentToolPolicyVersion: "policy-1",
	})
	pluginService := plugins.NewService(nil, registry, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(pluginService)
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "task-owned", SessionID: "session-owned",
	})
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "automation-1", WorkspaceID: "ws-state-event",
		CallerTaskID: "task-owned", CallerSessionID: "session-owned", Surface: mcpprofile.SurfaceAutomation,
	})
	return h, ctx, pluginService, "ws-state-event"
}

func TestAutomationPluginToolsDiscoveryIsFiltered(t *testing.T) {
	h, ctx, _, workspaceID := automationHandlerFixture(t)
	h.SetAutomationPluginTools(&fakeAutomationPluginTools{definitions: []plugintools.Definition{{
		PluginID: "kandev-security", LocalName: "list_findings",
		ExposedName: plugintools.ExposedName("kandev-security", "list_findings"),
		Description: "List findings", Surfaces: []string{plugintools.SurfaceAutomation},
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}})
	response, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(response.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)
	require.Equal(t, "list_findings", snapshot.Tools[0].LocalName)

	forged, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceKanban,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forged.Type))

	plain := context.Background()
	plain = streams.WithMCPExecutionContext(plain, streams.MCPExecutionContext{
		ExecutionID: "execution-plain", TaskID: "task-owned", SessionID: "session-owned",
	})
	plain = mcpscope.WithPrincipal(plain, mcpscope.Principal{
		WorkspaceID: workspaceID, CallerTaskID: "task-owned", CallerSessionID: "session-owned",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	denied, err := h.handleListPluginTools(plain, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceAutomation,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(denied.Type))
}

func TestAutomationPluginToolsInvocationScope(t *testing.T) {
	h, ctx, _, workspaceID := automationHandlerFixture(t)
	h.SetAutomationPluginTools(&fakeAutomationPluginTools{})

	unapproved, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-security", "local_name": "list_findings", "invocation_id": "invocation-1",
		"surface": manifest.AgentToolSurfaceAutomation, "workspace_id": workspaceID,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(unapproved.Type))
	require.Contains(t, string(unapproved.Payload), "not approved")

	h.SetAutomationPluginTools(&fakeAutomationPluginTools{authorization: &plugintools.AutomationAuthorization{
		AutomationID: "automation-1", WorkspaceID: workspaceID, PluginID: "kandev-security",
		InstallationID: "installation-1", PolicyVersion: "policy-1",
		ToolNames: []string{"list_findings"},
	}})
	forgedSurface, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-security", "local_name": "list_findings", "invocation_id": "invocation-3",
		"surface": manifest.AgentToolSurfaceKanban, "workspace_id": workspaceID,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forgedSurface.Type))
	require.Contains(t, string(forgedSurface.Payload), "automation surface")

	unselected, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-security", "local_name": "complete_report", "invocation_id": "invocation-2",
		"surface": manifest.AgentToolSurfaceAutomation, "workspace_id": workspaceID,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(unselected.Type))
	require.Contains(t, string(unselected.Payload), "not approved")

	approved, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-security", "local_name": "list_findings", "invocation_id": "invocation-1",
		"surface": manifest.AgentToolSurfaceAutomation, "workspace_id": workspaceID,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(approved.Type))
	require.Contains(t, string(approved.Payload), "not running")
}
