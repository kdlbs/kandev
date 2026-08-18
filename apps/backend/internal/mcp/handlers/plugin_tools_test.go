package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleInvokePluginToolRejectsForeignExecutionContext(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedMCPHandlerSession(t, repo, "task-owned", "session-owned", models.TaskSessionStateRunning)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{
		ID: "task-foreign", WorkspaceID: "ws-state-event", Title: "Foreign",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "session-foreign", TaskID: "task-foreign", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetPluginService(plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger(t)))
	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-owned", TaskID: "task-owned", SessionID: "session-owned",
	})
	resp, err := h.handleInvokePluginTool(ctx, makeWSMessage(t, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "echo", "local_name": "echo", "surface": "kanban-task",
		"task_id": "task-foreign", "session_id": "session-foreign", "workspace_id": "ws-state-event",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Contains(t, string(resp.Payload), "does not match the running execution")
}
