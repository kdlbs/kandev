package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskChangeRequestReadRecorder struct {
	called int
	result TaskChangeRequestReadResponse
}

func (r *taskChangeRequestReadRecorder) GetTaskChangeRequests(_ context.Context, taskID string) (TaskChangeRequestReadResponse, error) {
	r.called++
	r.result.TaskID = taskID
	return r.result, nil
}

func TestGetTaskChangeRequestsUsesBoundPrincipal(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{
		ID: "task-current", WorkspaceID: "workspace-1", Title: "Current", State: v1.TaskStateTODO,
		CreatedAt: now, UpdatedAt: now,
	}))
	recorder := &taskChangeRequestReadRecorder{result: TaskChangeRequestReadResponse{Complete: true}}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeRequestReadService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-current", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})

	response, err := h.handleGetTaskChangeRequests(ctx, makeWSMessage(t, ws.ActionMCPGetTaskChangeRequests, map[string]interface{}{
		"task_id": "task-current",
	}))
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, 1, recorder.called)
	var payload TaskChangeRequestReadResponse
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	assert.Equal(t, "task-current", payload.TaskID)
	assert.True(t, payload.Complete)
}

func TestGetTaskChangeRequestsRejectsInjectedExecutionIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{
		ID: "task-current", WorkspaceID: "workspace-1", Title: "Current", State: v1.TaskStateTODO,
		CreatedAt: now, UpdatedAt: now,
	}))
	recorder := &taskChangeRequestReadRecorder{}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeRequestReadService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-current", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})

	response, err := h.handleGetTaskChangeRequests(ctx, makeWSMessage(t, ws.ActionMCPGetTaskChangeRequests, map[string]interface{}{
		"task_id": "task-current", "session_id": "spoofed",
	}))
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, ws.MessageTypeError, response.Type)
	assert.Equal(t, 0, recorder.called)
}
