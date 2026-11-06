package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const workspaceReaderPluginID = "kandev-workspace-reader"
const workspaceReadToolName = "read_workspace_data"

func workspaceReadToolDeclaration(surfaces ...string) manifest.AgentTool {
	readOnly := true
	return manifest.AgentTool{
		Name:            workspaceReadToolName,
		Description:     "Read workspace-owned data.",
		Surfaces:        surfaces,
		InvocationScope: manifest.AgentToolInvocationScopeWorkspace,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
			"required":   []any{"key"},
		},
		Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
	}
}

func workspaceReadHandler(t *testing.T) (*Handlers, *plugins.Registry, *sqliterepo.Repository) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-readable", Name: "Readable", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-foreign", Name: "Foreign", OwnerID: "someone-else", CreatedAt: now, UpdatedAt: now,
	}))
	registry := plugins.NewRegistry()
	registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID:          workspaceReaderPluginID,
			DisplayName: "Workspace Reader",
			AgentTools: []manifest.AgentTool{
				workspaceReadToolDeclaration(manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceOffice,
					manifest.AgentToolSurfaceExternal),
			},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	})
	pluginService := plugins.NewService(nil, registry, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(pluginService)
	h.SetPluginToolAuthEnabled(func() bool { return false })
	return h, registry, repo
}

func externalWorkspaceList(t *testing.T, h *Handlers, ctx context.Context) *ws.Message {
	t.Helper()
	resp, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceExternal,
	}))
	require.NoError(t, err)
	return resp
}

func externalWorkspaceCall(t *testing.T, h *Handlers, ctx context.Context, arguments map[string]any) *ws.Message {
	t.Helper()
	resp, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": workspaceReaderPluginID, "local_name": workspaceReadToolName,
		"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceExternal,
		"arguments": arguments,
	}))
	require.NoError(t, err)
	return resp
}

func pluginToolErrorPayload(t *testing.T, resp *ws.Message) ws.ErrorPayload {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	return payload
}

func TestExternalCatalogInjectsWorkspaceArgument(t *testing.T) {
	h, _, _ := workspaceReadHandler(t)
	ctx := mcporigin.WithTrustedExternalTransport(context.Background())

	resp := externalWorkspaceList(t, h, ctx)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(resp.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(snapshot.Tools[0].InputSchema, &schema))
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	argument, ok := properties[manifest.AgentToolWorkspaceArgument].(map[string]any)
	require.True(t, ok, "external catalog must advertise %s", manifest.AgentToolWorkspaceArgument)
	require.Equal(t, "string", argument["type"])
	require.Equal(t, float64(1), argument["minLength"])
	require.Contains(t, argument["description"], "list_workspaces_kandev")
	require.Contains(t, properties, "key", "plugin-declared inputs must be preserved")
	required, ok := schema["required"].([]any)
	require.True(t, ok)
	require.Contains(t, required, manifest.AgentToolWorkspaceArgument)
	require.Contains(t, required, "key")

	catalog, err := h.pluginSvc.AgentToolCatalog()
	require.NoError(t, err)
	require.NotContains(t, string(catalog.Tools[0].InputSchema), manifest.AgentToolWorkspaceArgument,
		"the plugin catalog schema must stay unchanged")
}

func TestExternalWorkspaceToolAuthorization(t *testing.T) {
	t.Run("readable workspace reaches plugin dispatch", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		ctx := mcporigin.WithTrustedExternalTransport(context.Background())
		payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-readable",
		}))
		require.Equal(t, ws.ErrorCodeInternalError, payload.Code,
			"authorization should pass; the fixture has no running plugin")
		require.Contains(t, payload.Message, "plugin tool invocation failed")
	})

	for name, workspaceID := range map[string]string{
		"unknown workspace": "ws-missing",
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := workspaceReadHandler(t)
			ctx := mcporigin.WithTrustedExternalTransport(context.Background())
			payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
				manifest.AgentToolWorkspaceArgument: workspaceID,
			}))
			require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
			require.Equal(t, "workspace is not available", payload.Message)
		})
	}

	t.Run("unreadable workspace", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		h.SetPluginToolAuthEnabled(func() bool { return true })
		ctx := authn.WithIdentity(mcporigin.WithTrustedExternalTransport(context.Background()),
			authn.Identity{UserID: "user-1"})
		payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-foreign",
		}))
		require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
		require.Equal(t, "workspace is not available", payload.Message)
	})

	t.Run("unknown and unreadable workspaces share one error", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		h.SetPluginToolAuthEnabled(func() bool { return true })
		ctx := authn.WithIdentity(mcporigin.WithTrustedExternalTransport(context.Background()),
			authn.Identity{UserID: "user-1"})
		unknown := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-missing",
		}))
		foreign := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-foreign",
		}))
		require.Equal(t, unknown.Code, foreign.Code)
		require.Equal(t, unknown.Message, foreign.Message)
	})

	for name, arguments := range map[string]map[string]any{
		"missing argument":    {},
		"blank argument":      {manifest.AgentToolWorkspaceArgument: "   "},
		"non-string argument": {manifest.AgentToolWorkspaceArgument: 42},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := workspaceReadHandler(t)
			ctx := mcporigin.WithTrustedExternalTransport(context.Background())
			payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, arguments))
			require.Equal(t, ws.ErrorCodeBadRequest, payload.Code)
			require.Contains(t, payload.Message, manifest.AgentToolWorkspaceArgument)
		})
	}

	t.Run("authentication enabled without an identity fails closed", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		h.SetPluginToolAuthEnabled(func() bool { return true })
		ctx := mcporigin.WithTrustedExternalTransport(context.Background())
		payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-readable",
		}))
		require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
		require.Equal(t, "workspace is not available", payload.Message)
	})

	t.Run("authenticated caller can read an existing workspace", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		h.SetPluginToolAuthEnabled(func() bool { return true })
		ctx := authn.WithIdentity(mcporigin.WithTrustedExternalTransport(context.Background()),
			authn.Identity{UserID: "user-1"})
		payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-readable",
		}))
		require.Equal(t, ws.ErrorCodeInternalError, payload.Code,
			"authorization should pass for a readable workspace")
	})

	t.Run("membership revocation fails the next call", func(t *testing.T) {
		h, _, repo := workspaceReadHandler(t)
		h.SetPluginToolAuthEnabled(func() bool { return true })
		now := time.Now().UTC()
		require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
			ID: "ws-member", Name: "Member", OwnerID: "owner-1", CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
			WorkspaceID: "ws-member", UserID: "user-1", Role: "viewer", AddedBy: "owner-1", CreatedAt: now,
		}))
		ctx := authn.WithIdentity(mcporigin.WithTrustedExternalTransport(context.Background()),
			authn.Identity{UserID: "user-1"})
		arguments := map[string]any{manifest.AgentToolWorkspaceArgument: "ws-member"}

		payload := pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, arguments))
		require.Equal(t, ws.ErrorCodeInternalError, payload.Code)

		require.NoError(t, repo.DeleteWorkspaceMember(context.Background(), "ws-member", "user-1"))
		payload = pluginToolErrorPayload(t, externalWorkspaceCall(t, h, ctx, arguments))
		require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
		require.Equal(t, "workspace is not available", payload.Message)
	})

	t.Run("forged payload identity is rejected", func(t *testing.T) {
		h, _, _ := workspaceReadHandler(t)
		ctx := mcporigin.WithTrustedExternalTransport(context.Background())
		resp, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
			"plugin_id": workspaceReaderPluginID, "local_name": workspaceReadToolName,
			"invocation_id": "invocation-1", "surface": manifest.AgentToolSurfaceExternal,
			"workspace_id": "ws-readable",
		}))
		require.NoError(t, err)
		payload := pluginToolErrorPayload(t, resp)
		require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
		require.Contains(t, payload.Message, "carry no task identity")
	})
}

func TestWorkspaceToolTaskSurfaceUsesExecutionWorkspace(t *testing.T) {
	h, _, repo := workspaceReadHandler(t)
	seedMCPHandlerSession(t, repo, "workspace-task", "workspace-session", models.TaskSessionStateRunning)
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "workspace-task", SessionID: "workspace-session",
	})
	execution, errorResponse, err := h.resolvePluginToolInvocationContext(ctx, makeWSMessage(t,
		ws.ActionMCPInvokePluginTool, map[string]any{
			"plugin_id": workspaceReaderPluginID, "local_name": workspaceReadToolName,
			"surface": manifest.AgentToolSurfaceKanban,
		}), pluginToolInvocationRequest{
		PluginID: workspaceReaderPluginID, LocalName: workspaceReadToolName,
		Surface: manifest.AgentToolSurfaceKanban,
	})
	require.NoError(t, err)
	require.Nil(t, errorResponse)
	require.Equal(t, "workspace-task", execution.TaskID)
	require.Equal(t, "workspace-session", execution.SessionID)
	require.Equal(t, "ws-state-event", execution.WorkspaceID)
}

func TestExtractExternalWorkspaceArgument(t *testing.T) {
	arguments := map[string]any{
		manifest.AgentToolWorkspaceArgument: "ws-1",
		"key":                               "value",
	}
	workspaceID, stripped, err := extractExternalWorkspaceArgument(arguments)
	require.NoError(t, err)
	require.Equal(t, "ws-1", workspaceID)
	require.Equal(t, map[string]any{"key": "value"}, stripped)
	require.Contains(t, arguments, manifest.AgentToolWorkspaceArgument,
		"the plugin arguments map must not be mutated in place")

	for name, value := range map[string]map[string]any{
		"absent":     {},
		"blank":      {manifest.AgentToolWorkspaceArgument: " "},
		"not string": {manifest.AgentToolWorkspaceArgument: 42},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := extractExternalWorkspaceArgument(value)
			require.Error(t, err)
		})
	}
}
