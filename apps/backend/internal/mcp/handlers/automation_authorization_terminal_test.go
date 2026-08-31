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
	require.NotNil(t, guarded)
	assert.Equal(t, ws.MessageTypeError, guarded.Type)
	assert.Nil(t, replacement)

	guarded, _, err = h.authorizeAutomationRequest(coordinatorCtx, message("task-foreign", "wf-foreign"))
	require.NoError(t, err)
	require.NotNil(t, guarded)
	assert.Equal(t, ws.MessageTypeError, guarded.Type)
}

func TestMoveTaskAuthorizationAllowsOnlyCurrentCoordinatorGrant(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-route", Name: "Route", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-route", WorkspaceID: "ws-route", Name: "Route", CreatedAt: now, UpdatedAt: now}))
	for _, task := range []*models.Task{
		{ID: "task-coordinator", WorkspaceID: "ws-route", WorkflowID: "wf-route", Title: "Coordinator", CreatedAt: now, UpdatedAt: now},
		{ID: "task-sibling", WorkspaceID: "ws-route", WorkflowID: "wf-route", Title: "Sibling", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-coordinator", TaskID: "task-coordinator", State: models.TaskSessionStateRunning, IsPrimary: true, StartedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "execution-coordinator", SessionID: "session-coordinator", TaskID: "task-coordinator", ExecutorID: "executor", Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "execution-coordinator"}))
	_, err := repo.DB().ExecContext(ctx, `INSERT INTO workspace_coordinator_grants (workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "ws-route", "task-coordinator", "owner", now, now)
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	payload, err := json.Marshal(map[string]interface{}{"task_id": "task-sibling", "workflow_id": "wf-route", "workflow_step_id": "step-done"})
	require.NoError(t, err)
	msg := &ws.Message{ID: "current-grant", Action: ws.ActionMCPMoveTask, Payload: payload}
	principal := mcpscope.Principal{AutomationID: "automation", WorkspaceID: "ws-route", CallerTaskID: "task-coordinator", CallerSessionID: "session-coordinator", CallerExecutionID: "execution-coordinator", Surface: mcpprofile.SurfaceAutomation}

	guarded, replacement, err := h.authorizeAutomationRequest(mcpscope.WithPrincipal(ctx, principal), msg)
	require.NoError(t, err)
	assert.Nil(t, guarded)
	require.NotNil(t, replacement)

	principal.CallerExecutionID = "expired-execution"
	guarded, _, err = h.authorizeAutomationRequest(mcpscope.WithPrincipal(ctx, principal), msg)
	require.NoError(t, err)
	require.NotNil(t, guarded)
}

func TestMoveTaskAuthorizationAllowsWorkspaceScopedConfigurationSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-config", Name: "Config", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-config", WorkspaceID: "ws-config", Name: "Config", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "config-task", WorkspaceID: "ws-config", WorkflowID: "wf-config", Title: "Config", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "target-task", WorkspaceID: "ws-config", WorkflowID: "wf-config", Title: "Target", CreatedAt: now, UpdatedAt: now}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	payload, err := json.Marshal(map[string]interface{}{"task_id": "target-task", "workflow_id": "wf-config", "workflow_step_id": "step-done"})
	require.NoError(t, err)
	principal := mcpscope.Principal{WorkspaceID: "ws-config", CallerTaskID: "config-task", CallerSessionID: "config-session", Surface: mcpprofile.SurfaceConfiguration}
	guarded, replacement, err := h.authorizeAutomationRequest(mcpscope.WithPrincipal(ctx, principal), &ws.Message{ID: "config-route", Action: ws.ActionMCPMoveTask, Payload: payload})
	require.NoError(t, err)
	assert.Nil(t, guarded)
	assert.NotNil(t, replacement)
}
