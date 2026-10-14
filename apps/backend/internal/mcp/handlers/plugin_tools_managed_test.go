package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestManagedToolProvenance(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedMCPHandlerSession(t, repo, "task-managed", "session-managed", models.TaskSessionStateRunning)
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "ws-state-event",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	managedTask, err := svc.GetTask(context.Background(), "task-managed")
	require.NoError(t, err)
	managedTask.Metadata = map[string]interface{}{
		"kandev.managed_retained":      true,
		"kandev.managed_by_plugin":     policy.PluginID,
		"kandev.installation_id":       policy.InstallationID,
		"kandev.workspace_id":          policy.WorkspaceID,
		"kandev.instance_key":          policy.InstanceKey,
		"kandev.conversation_revision": "3",
		"kandev.desired_paused":        false,
		"kandev.detached":              false,
		"kandev.approval_revision":     "2",
		"kandev.manifest_digest":       policy.ManifestDigest,
		"kandev.agent_tool_names":      []string{"read_task"},
	}
	require.NoError(t, repo.UpdateTask(context.Background(), managedTask))
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		TaskID: "task-managed", SessionID: "session-managed", AgentExecutionID: "execution-current", Status: "running",
	}))

	registry := plugins.NewRegistry()
	registry.Add(&store.Record{Manifest: manifest.Manifest{
		ID: "coordinator.v1", AgentTools: []manifest.AgentTool{
			{Name: "read_task", Description: "Read task.", Surfaces: []string{manifest.AgentToolSurfaceManaged}, InputSchema: map[string]any{"type": "object"}},
			{Name: "update_task", Description: "Update task.", Surfaces: []string{manifest.AgentToolSurfaceManaged}, InputSchema: map[string]any{"type": "object"}},
		},
	}, Status: store.StatusActive})
	pluginService := plugins.NewService(nil, registry, nil, testLogger(t))
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(pluginService)
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-current", TaskID: "task-managed", SessionID: "session-managed",
		ManagedToolPolicy: &policy,
	})

	listResponse, err := h.handleListPluginTools(ctx, makeWSMessage(t, ws.ActionMCPListPluginTools, map[string]any{
		"surface": manifest.AgentToolSurfaceManaged,
	}))
	require.NoError(t, err)
	var snapshot plugintools.Snapshot
	require.NoError(t, json.Unmarshal(listResponse.Payload, &snapshot))
	require.Len(t, snapshot.Tools, 1, "only the per-instance selected tool is exposed")
	require.Equal(t, "read_task", snapshot.Tools[0].LocalName)

	staleContext := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-replaced", TaskID: "task-managed", SessionID: "session-managed",
		ManagedToolPolicy: &policy,
	})
	staleResponse, err := h.handleInvokePluginTool(staleContext, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": policy.PluginID, "local_name": "read_task", "surface": manifest.AgentToolSurfaceManaged,
		"task_id": "task-managed", "session_id": "session-managed", "workspace_id": policy.WorkspaceID,
	}))
	require.NoError(t, err)
	require.NotNil(t, staleResponse)
	require.Equal(t, "error", string(staleResponse.Type))
	require.Contains(t, string(staleResponse.Payload), "execution generation")

	forgedSurface, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": policy.PluginID, "local_name": "update_task", "surface": manifest.AgentToolSurfaceKanban,
		"task_id": "task-managed", "session_id": "session-managed", "workspace_id": policy.WorkspaceID,
	}))
	require.NoError(t, err)
	require.NotNil(t, forgedSurface)
	require.Contains(t, string(forgedSurface.Payload), "managed tool policy")

	managedTask, err = svc.GetTask(context.Background(), "task-managed")
	require.NoError(t, err)
	managedTask.Metadata["kandev.desired_paused"] = true
	require.NoError(t, repo.UpdateTask(context.Background(), managedTask))
	pausedResponse, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": policy.PluginID, "local_name": "read_task", "surface": manifest.AgentToolSurfaceManaged,
		"task_id": "task-managed", "session_id": "session-managed", "workspace_id": policy.WorkspaceID,
	}))
	require.NoError(t, err)
	require.NotNil(t, pausedResponse)
	require.Contains(t, string(pausedResponse.Payload), "managed tool policy")

}
