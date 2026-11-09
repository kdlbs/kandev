package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func instanceReadToolDeclaration(name string, surfaces ...string) manifest.AgentTool {
	readOnly := true
	destructive := false
	return manifest.AgentTool{
		Name:            name,
		Description:     "Read cached instance usage.",
		Surfaces:        surfaces,
		InvocationScope: manifest.AgentToolInvocationScopeInstance,
		InputSchema:     map[string]any{"type": "object", "additionalProperties": false},
		Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly, DestructiveHint: &destructive},
	}
}

func instanceReadRecord(status string) *store.Record {
	return &store.Record{
		Manifest: manifest.Manifest{
			ID:          "kandev-provider-usage",
			DisplayName: "Provider Usage",
			AgentTools: []manifest.AgentTool{
				instanceReadToolDeclaration("get_provider_usage",
					manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceCoordinator, manifest.AgentToolSurfaceExternal),
			},
		},
		Status:                 status,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
}

func instanceReadHandler(t *testing.T) (*Handlers, *plugins.Registry) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	seedMCPHandlerSession(t, repo, "conversation-task", "conversation-session", "running")
	registry := plugins.NewRegistry()
	registry.Add(instanceReadRecord(store.StatusActive))
	pluginService := plugins.NewService(nil, registry, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(pluginService)
	h.coordinatorPhase2ForTest = func() bool { return true }
	h.coordinatorConversationForTest = func(context.Context, string) (string, bool, error) {
		return "coordinator-1", true, nil
	}
	return h, registry
}

func TestInstancePluginToolTrustedExternalContext(t *testing.T) {
	h, _ := instanceReadHandler(t)
	ctx := mcporigin.WithTrustedExternalTransport(context.Background())

	list, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceExternal,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(list.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)
	require.Equal(t, "get_provider_usage", snapshot.Tools[0].LocalName)
	require.True(t, snapshot.Tools[0].IsInstanceRead())

	call, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceExternal,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(call.Type))
	require.Contains(t, string(call.Payload), "plugin tool invocation failed")
}

func TestInstancePluginToolRejectsForgedIdentity(t *testing.T) {
	h, _ := instanceReadHandler(t)
	plain := context.Background()

	untrusted, err := h.handleListPluginTools(plain, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceExternal,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(untrusted.Type))
	require.Contains(t, string(untrusted.Payload), "trusted external transport")

	execution := streams.WithMCPExecutionContext(plain, streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "conversation-task", SessionID: "conversation-session",
	})
	forgedTransport := mcporigin.WithTrustedExternalTransport(execution)
	forged, err := h.handleListPluginTools(forgedTransport, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceExternal,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forged.Type))
	require.Contains(t, string(forged.Payload), "execution context")

	forgedIdentity, err := h.handleInvokePluginTool(mcporigin.WithTrustedExternalTransport(plain), makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-2", "surface": manifest.AgentToolSurfaceExternal,
		"task_id": "conversation-task",
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forgedIdentity.Type))
	require.Contains(t, string(forgedIdentity.Payload), "carry no task identity")
}

func coordinatorInstanceReadContext(binding *mcpprofile.CoordinatorToolPolicy) context.Context {
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "conversation-task", SessionID: "conversation-session",
		CoordinatorToolPolicy: binding, CoordinatorToolPolicyRequired: true,
	})
	return mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		CoordinatorID: "coordinator-1", WorkspaceID: "ws-state-event",
		CallerTaskID: "conversation-task", CallerSessionID: "conversation-session",
		Surface: mcpprofile.SurfaceCoordinator,
	})
}

func coordinatorBindingV2(installationID, policyVersion string) *mcpprofile.CoordinatorToolPolicy {
	return &mcpprofile.CoordinatorToolPolicy{
		Version:            mcpprofile.CoordinatorToolPolicyVersionV2,
		CoordinatorID:      "coordinator-1",
		WorkspaceID:        "ws-state-event",
		ConversationTaskID: "conversation-task",
		PolicyRevision:     1,
		ToolNames:          []string{"list_tasks_kandev"},
		PluginTools: []mcpprofile.CoordinatorPluginTool{{
			PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
			InstallationID: installationID, PolicyVersion: policyVersion,
		}},
	}
}

func TestCoordinatorPluginToolAuthorization(t *testing.T) {
	h, _ := instanceReadHandler(t)
	ctx := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))

	list, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(list.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)
	require.Equal(t, "get_provider_usage", snapshot.Tools[0].LocalName)

	call, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(call.Type))
	require.Contains(t, string(call.Payload), "plugin tool invocation failed")

	stale := coordinatorInstanceReadContext(coordinatorBindingV2("installation-old", "policy-1"))
	staleCall, err := h.handleInvokePluginTool(stale, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-2", "surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(staleCall.Type))
	require.Contains(t, string(staleCall.Payload), "no longer eligible")

	forgedSurface, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-forged", "surface": manifest.AgentToolSurfaceKanban,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forgedSurface.Type))
	require.Contains(t, string(forgedSurface.Payload), "require the coordinator surface")

	forgedWorkspace := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))
	forged, err := h.handleInvokePluginTool(forgedWorkspace, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-3", "surface": manifest.AgentToolSurfaceCoordinator,
		"workspace_id": "ws-other",
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(forged.Type))
	require.Contains(t, string(forged.Payload), "request context does not match")
}

func TestCoordinatorPluginToolLegacyBindingGrantsNothing(t *testing.T) {
	h, _ := instanceReadHandler(t)
	legacy := coordinatorBindingV2("installation-1", "policy-1")
	legacy.Version = mcpprofile.CoordinatorToolPolicyVersionV1
	legacy.PluginTools = nil
	ctx := coordinatorInstanceReadContext(legacy)

	list, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(list.Payload, &snapshot))
	require.Empty(t, snapshot.Tools)

	call, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(call.Type))
	require.Contains(t, string(call.Payload), "not bound")
}

func TestCoordinatorPluginActionPolicyGuard(t *testing.T) {
	h, _ := instanceReadHandler(t)
	ctx := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	require.True(t, ok)
	refused, response, err := h.authorizeCoordinatorPolicy(ctx, principal, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.False(t, refused)
	require.Nil(t, response)
	require.NoError(t, err)
}

func TestInstancePluginToolRevokedAfterDiscovery(t *testing.T) {
	h, registry := instanceReadHandler(t)
	coordinatorCtx := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))

	list, err := h.handleListPluginTools(coordinatorCtx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(list.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)

	registry.Add(instanceReadRecord(store.StatusDisabled))
	call, err := h.handleInvokePluginTool(coordinatorCtx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(call.Type))
	require.Contains(t, string(call.Payload), "no longer eligible")
}

func TestTaskPrincipalCannotInvokeAnotherSurface(t *testing.T) {
	h, _ := instanceReadHandler(t)
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "conversation-task", SessionID: "conversation-session",
	})
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID: "ws-state-event", CallerTaskID: "conversation-task",
		CallerSessionID: "conversation-session", Surface: mcpprofile.SurfaceConfiguration,
	})
	response, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceKanban,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(response.Type))
	require.Contains(t, string(response.Payload), "does not match the caller principal")
}

func TestCoordinatorPluginToolsRequirePhase2(t *testing.T) {
	h, _ := instanceReadHandler(t)
	h.coordinatorPhase2ForTest = func() bool { return false }
	ctx := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))
	for _, action := range []string{ws.ActionMCPListPluginTools, ws.ActionMCPInvokePluginTool} {
		payload := map[string]any{"surface": manifest.AgentToolSurfaceCoordinator}
		if action == ws.ActionMCPInvokePluginTool {
			payload["plugin_id"] = "kandev-provider-usage"
			payload["local_name"] = "get_provider_usage"
			payload["invocation_id"] = "invocation-1"
		}
		var response *ws.Message
		var err error
		if action == ws.ActionMCPListPluginTools {
			response, err = h.handleListPluginTools(ctx, makeWSMessage(t, action, payload))
		} else {
			response, err = h.handleInvokePluginTool(ctx, makeWSMessage(t, action, payload))
		}
		require.NoError(t, err)
		require.Equal(t, string(ws.MessageTypeError), string(response.Type))
		require.Contains(t, string(response.Payload), "require phase 2")
	}
}

func TestCoordinatorPluginToolsRequireCurrentConversation(t *testing.T) {
	h, _ := instanceReadHandler(t)
	h.coordinatorConversationForTest = func(context.Context, string) (string, bool, error) {
		return "other-coordinator", true, nil
	}
	ctx := coordinatorInstanceReadContext(coordinatorBindingV2("installation-1", "policy-1"))
	list, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(list.Payload, &snapshot))
	require.Empty(t, snapshot.Tools)

	call, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "kandev-provider-usage", "local_name": "get_provider_usage",
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceCoordinator,
	}))
	require.NoError(t, err)
	require.Equal(t, string(ws.MessageTypeError), string(call.Type))
	require.Contains(t, string(call.Payload), "not bound")
}
