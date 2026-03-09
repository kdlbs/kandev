package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Task Config E2E Tests
// =============================================================================

func TestE2E_Task_DeleteTask(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, workflowID := seedWorkspace(t, env)

	stepResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Backlog",
		"color":       "#000",
		"position":    0,
	})
	assertWSSuccess(t, stepResp)
	stepID := decodePayload(t, stepResp)["step"].(map[string]interface{})["id"].(string)

	ctx := context.Background()
	task, err := env.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Deletable Task",
	})
	require.NoError(t, err)

	deleteResp := callHandler(t, env.handlers, ws.ActionMCPDeleteTask, map[string]interface{}{
		"task_id": task.ID,
	})
	assertWSSuccess(t, deleteResp)
	assert.Equal(t, true, decodePayload(t, deleteResp)["success"])
}

func TestE2E_Task_DeleteNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPDeleteTask, map[string]interface{}{
		"task_id": "nonexistent",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_Task_ArchiveTask(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, workflowID := seedWorkspace(t, env)

	stepResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Done",
		"color":       "#22c55e",
		"position":    0,
	})
	assertWSSuccess(t, stepResp)
	stepID := decodePayload(t, stepResp)["step"].(map[string]interface{})["id"].(string)

	ctx := context.Background()
	task, err := env.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Archivable Task",
	})
	require.NoError(t, err)

	archiveResp := callHandler(t, env.handlers, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": task.ID,
	})
	assertWSSuccess(t, archiveResp)
	assert.Equal(t, true, decodePayload(t, archiveResp)["success"])
}

func TestE2E_Task_ArchiveNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": "nonexistent",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_Task_UpdateState(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, workflowID := seedWorkspace(t, env)

	stepResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "In Progress",
		"color":       "#f59e0b",
		"position":    0,
	})
	assertWSSuccess(t, stepResp)
	stepID := decodePayload(t, stepResp)["step"].(map[string]interface{})["id"].(string)

	ctx := context.Background()
	state := v1.TaskStateTODO
	task, err := env.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Stateful Task",
		State:          &state,
	})
	require.NoError(t, err)

	updateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateTaskState, map[string]interface{}{
		"task_id": task.ID,
		"state":   "in_progress",
	})
	assertWSSuccess(t, updateResp)

	payload := decodePayload(t, updateResp)
	assert.Equal(t, "in_progress", payload["state"])
}

func TestE2E_Task_UpdateStateNonexistent(t *testing.T) {
	env := setupTestEnv(t)

	resp := callHandler(t, env.handlers, ws.ActionMCPUpdateTaskState, map[string]interface{}{
		"task_id": "nonexistent",
		"state":   "in_progress",
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestE2E_Task_MoveTask(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, workflowID := seedWorkspace(t, env)

	step1Resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Backlog",
		"color":       "#6b7280",
		"position":    0,
	})
	assertWSSuccess(t, step1Resp)
	step1ID := decodePayload(t, step1Resp)["step"].(map[string]interface{})["id"].(string)

	step2Resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "In Progress",
		"color":       "#f59e0b",
		"position":    1,
	})
	assertWSSuccess(t, step2Resp)
	step2ID := decodePayload(t, step2Resp)["step"].(map[string]interface{})["id"].(string)

	ctx := context.Background()
	task, err := env.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: step1ID,
		Title:          "Movable Task",
	})
	require.NoError(t, err)

	moveResp := callHandler(t, env.handlers, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          task.ID,
		"workflow_id":      workflowID,
		"workflow_step_id": step2ID,
		"position":         0,
	})
	assertWSSuccess(t, moveResp)

	payload := decodePayload(t, moveResp)
	assert.Equal(t, step2ID, payload["workflow_step_id"])
}

func TestE2E_Task_MoveNonexistent(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	stepResp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Target",
		"color":       "#000",
		"position":    0,
	})
	assertWSSuccess(t, stepResp)
	stepID := decodePayload(t, stepResp)["step"].(map[string]interface{})["id"].(string)

	resp := callHandler(t, env.handlers, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "nonexistent",
		"workflow_id":      workflowID,
		"workflow_step_id": stepID,
	})
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

// =============================================================================
// Handler Registration E2E Tests
// =============================================================================

func TestE2E_RegisterHandlers_AllConfigHandlersRegistered(t *testing.T) {
	env := setupTestEnv(t)

	d := ws.NewDispatcher()
	env.handlers.RegisterHandlers(d)

	configActions := []string{
		ws.ActionMCPCreateWorkflowStep,
		ws.ActionMCPUpdateWorkflowStep,
		ws.ActionMCPDeleteWorkflowStep,
		ws.ActionMCPReorderWorkflowStep,
		ws.ActionMCPListAgents,
		ws.ActionMCPCreateAgent,
		ws.ActionMCPUpdateAgent,
		ws.ActionMCPDeleteAgent,
		ws.ActionMCPListAgentProfiles,
		ws.ActionMCPUpdateAgentProfile,
		ws.ActionMCPGetMcpConfig,
		ws.ActionMCPUpdateMcpConfig,
		ws.ActionMCPMoveTask,
		ws.ActionMCPDeleteTask,
		ws.ActionMCPArchiveTask,
		ws.ActionMCPUpdateTaskState,
	}
	for _, action := range configActions {
		msg := makeWSMessage(t, action, map[string]string{})
		_, err := d.Dispatch(context.Background(), msg)
		assert.NoError(t, err, "action %s should be registered", action)
	}
}

func TestE2E_RegisterHandlers_WithoutConfigDeps(t *testing.T) {
	log := testLogger(t)
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, log)

	d := ws.NewDispatcher()
	h.RegisterHandlers(d)

	configActions := []string{
		ws.ActionMCPCreateWorkflowStep,
		ws.ActionMCPListAgents,
		ws.ActionMCPGetMcpConfig,
	}
	for _, action := range configActions {
		msg := makeWSMessage(t, action, map[string]string{})
		resp, err := d.Dispatch(context.Background(), msg)
		if err == nil && resp != nil {
			assert.Equal(t, ws.MessageTypeError, resp.Type, "action %s should not be registered", action)
		}
	}
}

// =============================================================================
// Full Pipeline E2E Tests (multi-step scenarios)
// =============================================================================

func TestE2E_FullPipeline_CreateAgentConfigureMCPAndManageWorkflow(t *testing.T) {
	env := setupTestEnv(t)
	_, workflowID := seedWorkspace(t, env)

	// 1. Create an agent with a profile
	agentID, profileID := createAgentWithProfile(t, env)

	// 2. Verify agent exists via list
	listAgentsResp := callHandler(t, env.handlers, ws.ActionMCPListAgents, map[string]interface{}{})
	assertWSSuccess(t, listAgentsResp)
	agentsList := decodePayload(t, listAgentsResp)["agents"].([]interface{})
	assert.Len(t, agentsList, 1)
	_ = agentID

	// 3. Update profile model
	updateProfileResp := callHandler(t, env.handlers, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"profile_id": profileID,
		"model":      "claude-3.5-sonnet",
	})
	assertWSSuccess(t, updateProfileResp)

	// 4. Configure MCP servers
	mcpResp := callHandler(t, env.handlers, ws.ActionMCPUpdateMcpConfig, map[string]interface{}{
		"profile_id": profileID,
		"enabled":    true,
		"servers": map[string]interface{}{
			"github": map[string]interface{}{
				"command": "npx",
				"args":    []string{"-y", "@github/mcp-server"},
			},
		},
	})
	assertWSSuccess(t, mcpResp)

	// 5. Create workflow steps
	for _, step := range []struct {
		name  string
		color string
	}{
		{"Backlog", "#6b7280"},
		{"In Progress", "#f59e0b"},
		{"Review", "#3b82f6"},
		{"Done", "#22c55e"},
	} {
		resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
			"workflow_id": workflowID,
			"name":        step.name,
			"color":       step.color,
			"position":    0,
		})
		assertWSSuccess(t, resp)
	}

	// 6. Verify MCP config persisted
	getConfigResp := callHandler(t, env.handlers, ws.ActionMCPGetMcpConfig, map[string]interface{}{
		"profile_id": profileID,
	})
	assertWSSuccess(t, getConfigResp)
	configPayload := decodePayload(t, getConfigResp)
	assert.Equal(t, true, configPayload["enabled"])
}

func TestE2E_FullPipeline_TaskLifecycle(t *testing.T) {
	env := setupTestEnv(t)
	workspaceID, workflowID := seedWorkspace(t, env)

	step1Resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Todo",
		"color":       "#6b7280",
		"position":    0,
	})
	assertWSSuccess(t, step1Resp)
	step1ID := decodePayload(t, step1Resp)["step"].(map[string]interface{})["id"].(string)

	step2Resp := callHandler(t, env.handlers, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id": workflowID,
		"name":        "Done",
		"color":       "#22c55e",
		"position":    1,
	})
	assertWSSuccess(t, step2Resp)
	step2ID := decodePayload(t, step2Resp)["step"].(map[string]interface{})["id"].(string)

	ctx := context.Background()
	state := v1.TaskStateTODO
	task, err := env.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: step1ID,
		Title:          "Lifecycle Task",
		State:          &state,
	})
	require.NoError(t, err)

	// Update state
	stateResp := callHandler(t, env.handlers, ws.ActionMCPUpdateTaskState, map[string]interface{}{
		"task_id": task.ID,
		"state":   "in_progress",
	})
	assertWSSuccess(t, stateResp)
	assert.Equal(t, "in_progress", decodePayload(t, stateResp)["state"])

	// Move task
	moveResp := callHandler(t, env.handlers, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          task.ID,
		"workflow_id":      workflowID,
		"workflow_step_id": step2ID,
		"position":         0,
	})
	assertWSSuccess(t, moveResp)
	assert.Equal(t, step2ID, decodePayload(t, moveResp)["workflow_step_id"])

	// Archive
	archiveResp := callHandler(t, env.handlers, ws.ActionMCPArchiveTask, map[string]interface{}{
		"task_id": task.ID,
	})
	assertWSSuccess(t, archiveResp)
}
