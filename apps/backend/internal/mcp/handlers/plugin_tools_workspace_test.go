package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
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
	"github.com/kdlbs/kandev/pluginsdk"
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

// workspaceRecordingPlugin records the exact request the host sends over the
// real plugin RPC boundary.
type workspaceRecordingPlugin struct {
	pluginsdk.UnimplementedPlugin

	mu      sync.Mutex
	request *pluginsdk.AgentToolRequest
}

func (p *workspaceRecordingPlugin) InvokeAgentTool(
	_ context.Context, req *pluginsdk.AgentToolRequest,
) (*pluginsdk.AgentToolResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.request = req
	return &pluginsdk.AgentToolResult{Text: "workspace data"}, nil
}

func (p *workspaceRecordingPlugin) recorded() *pluginsdk.AgentToolRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request
}

type workspacePluginRuntime struct {
	mu      sync.Mutex
	remote  *pluginsdk.RemotePlugin
	running bool
}

func (*workspacePluginRuntime) Start(context.Context, *store.Record, func(string) pluginsdk.Host) error {
	return nil
}
func (*workspacePluginRuntime) Stop(string) {}
func (r *workspacePluginRuntime) Get(string) (*pluginsdk.RemotePlugin, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return nil, false
	}
	return r.remote, true
}
func (*workspacePluginRuntime) Ping(string) error { return nil }
func (r *workspacePluginRuntime) Running(string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}
func (*workspacePluginRuntime) RestartCount(string) int { return 0 }
func (*workspacePluginRuntime) StopAll()                {}

func workspaceReadHandlerWithPlugin(t *testing.T) (*Handlers, *workspaceRecordingPlugin) {
	t.Helper()
	h, _, _ := workspaceReadHandler(t)
	impl := &workspaceRecordingPlugin{}
	client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{
			Impl: impl, HostDialTimeout: 50 * time.Millisecond,
		},
	})
	t.Cleanup(func() { _ = client.Close() })
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	require.NoError(t, err)
	remote, ok := raw.(*pluginsdk.RemotePlugin)
	require.True(t, ok, "Dispense returned %T", raw)
	h.pluginSvc.SetRuntime(&workspacePluginRuntime{remote: remote, running: true})
	return h, impl
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

func TestWithExternalWorkspaceArgumentAdjustsPropertyCounts(t *testing.T) {
	adjusted, err := addExternalWorkspaceArgument([]byte(
		`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"minProperties":1,"maxProperties":1}`))
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(adjusted, &schema))
	require.Equal(t, float64(2), schema["minProperties"])
	require.Equal(t, float64(2), schema["maxProperties"])
	require.Contains(t, schema["properties"], manifest.AgentToolWorkspaceArgument)
	require.Contains(t, schema["required"], manifest.AgentToolWorkspaceArgument)

	unbounded, err := addExternalWorkspaceArgument([]byte(`{"type":"object"}`))
	require.NoError(t, err)
	var unboundedSchema map[string]any
	require.NoError(t, json.Unmarshal(unbounded, &unboundedSchema))
	require.NotContains(t, unboundedSchema, "minProperties")
	require.NotContains(t, unboundedSchema, "maxProperties")
}

func TestExternalWorkspaceToolAuthorization(t *testing.T) {
	t.Run("readable workspace reaches plugin dispatch with stripped arguments", func(t *testing.T) {
		h, recording := workspaceReadHandlerWithPlugin(t)
		ctx := mcporigin.WithTrustedExternalTransport(context.Background())
		resp := externalWorkspaceCall(t, h, ctx, map[string]any{
			manifest.AgentToolWorkspaceArgument: "ws-readable",
			"key":                               "value",
		})
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		var result struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(resp.Payload, &result))
		require.Equal(t, "workspace data", result.Text)

		request := recording.recorded()
		require.NotNil(t, request)
		require.Equal(t, "ws-readable", request.Context.WorkspaceID)
		require.Equal(t, manifest.AgentToolSurfaceExternal, request.Context.Surface)
		require.Empty(t, request.Context.TaskID)
		require.Empty(t, request.Context.SessionID)
		require.Empty(t, request.Context.ExecutionID)
		require.Equal(t, map[string]any{"key": "value"}, request.Arguments)
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
