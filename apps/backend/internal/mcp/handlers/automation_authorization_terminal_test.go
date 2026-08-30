package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveTaskAuthorizationSeparatesOrdinaryAndCoordinatorAuthority(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-route", Name: "Route", CreatedAt: now, UpdatedAt: now},
		{ID: "ws-foreign", Name: "Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateWorkspace(ctx, workspace))
	}
	for _, workflow := range []*models.Workflow{
		{ID: "wf-route", WorkspaceID: "ws-route", Name: "Route", CreatedAt: now, UpdatedAt: now},
		{ID: "wf-foreign", WorkspaceID: "ws-foreign", Name: "Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	}
	for _, task := range []*models.Task{
		{ID: "task-self", WorkspaceID: "ws-route", WorkflowID: "wf-route", Title: "Self", CreatedAt: now, UpdatedAt: now},
		{ID: "task-sibling", WorkspaceID: "ws-route", WorkflowID: "wf-route", Title: "Sibling", CreatedAt: now, UpdatedAt: now},
		{ID: "task-foreign", WorkspaceID: "ws-foreign", WorkflowID: "wf-foreign", Title: "Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	message := func(taskID, workflowID string) *ws.Message {
		payload, err := json.Marshal(map[string]interface{}{
			"task_id": taskID, "workflow_id": workflowID, "workflow_step_id": "step-done",
			"sender_session_id": "spoofed-session",
		})
		require.NoError(t, err)
		return &ws.Message{ID: "route", Action: ws.ActionMCPMoveTask, Payload: payload}
	}

	ordinaryCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID: "ws-route", CallerTaskID: "task-self", CallerSessionID: "session-self",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	guarded, replacement, err := h.authorizeAutomationRequest(ordinaryCtx, message("task-self", "wf-route"))
	require.NoError(t, err)
	assert.Nil(t, guarded)
	require.NotNil(t, replacement)
	var rewritten map[string]interface{}
	require.NoError(t, json.Unmarshal(replacement.Payload, &rewritten))
	assert.Equal(t, "session-self", rewritten["sender_session_id"])
	assert.Equal(t, "task-self", rewritten["caller_task_id"])

	guarded, _, err = h.authorizeAutomationRequest(ordinaryCtx, message("task-sibling", "wf-route"))
	require.NoError(t, err)
	require.NotNil(t, guarded)
	assert.Equal(t, ws.MessageTypeError, guarded.Type)

	coordinatorCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "coordinator-grant", WorkspaceID: "ws-route",
		CallerTaskID: "task-self", CallerSessionID: "session-coordinator",
		Surface: mcpprofile.SurfaceAutomation,
	})
	guarded, replacement, err = h.authorizeAutomationRequest(coordinatorCtx, message("task-sibling", "wf-route"))
	require.NoError(t, err)
	assert.Nil(t, guarded)
	require.NotNil(t, replacement)

	guarded, _, err = h.authorizeAutomationRequest(coordinatorCtx, message("task-foreign", "wf-foreign"))
	require.NoError(t, err)
	require.NotNil(t, guarded)
	assert.Equal(t, ws.MessageTypeError, guarded.Type)
}
