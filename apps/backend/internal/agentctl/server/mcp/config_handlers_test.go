package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBackend implements BackendClient for testing handlers.
type testBackend struct {
	lastAction  string
	lastPayload interface{}
	response    map[string]interface{}
	err         error
}

func (tb *testBackend) RequestPayload(_ context.Context, action string, payload, result interface{}) error {
	tb.lastAction = action
	tb.lastPayload = payload
	if tb.err != nil {
		return tb.err
	}
	if tb.response != nil && result != nil {
		data, _ := json.Marshal(tb.response)
		return json.Unmarshal(data, result)
	}
	return nil
}

func newTestServer(t *testing.T, backend BackendClient) *Server {
	t.Helper()
	log := newTestLogger(t)
	return New(backend, "test-session", "", 10005, log, "", false, ModeConfig)
}

func callTool(t *testing.T, s *Server, toolName string, args map[string]interface{}) *mcplib.CallToolResult {
	t.Helper()
	toolsMap := s.mcpServer.ListTools()
	st, ok := toolsMap[toolName]
	require.True(t, ok, "tool %q not registered", toolName)

	reqArgs, err := json.Marshal(args)
	require.NoError(t, err)

	req := mcplib.CallToolRequest{}
	req.Method = "tools/call"
	req.Params.Name = toolName
	req.Params.Arguments = make(map[string]interface{})
	if err := json.Unmarshal(reqArgs, &req.Params.Arguments); err != nil {
		t.Fatal(err)
	}

	result, err := st.Handler(context.Background(), req)
	require.NoError(t, err)
	return result
}

// --- Action constant tests ---

func TestActionConstants_MatchWebSocketActions(t *testing.T) {
	// Verify canonical constants in pkg/websocket match the expected WS action strings.
	assert.Equal(t, "mcp.create_workflow", ws.ActionMCPCreateWorkflow)
	assert.Equal(t, "mcp.update_workflow", ws.ActionMCPUpdateWorkflow)
	assert.Equal(t, "mcp.delete_workflow", ws.ActionMCPDeleteWorkflow)
	assert.Equal(t, "mcp.create_workflow_step", ws.ActionMCPCreateWorkflowStep)
	assert.Equal(t, "mcp.update_workflow_step", ws.ActionMCPUpdateWorkflowStep)
	assert.Equal(t, "mcp.delete_workflow_step", ws.ActionMCPDeleteWorkflowStep)
	assert.Equal(t, "mcp.reorder_workflow_steps", ws.ActionMCPReorderWorkflowStep)
	assert.Equal(t, "mcp.list_agents", ws.ActionMCPListAgents)
	assert.Equal(t, "mcp.update_agent", ws.ActionMCPUpdateAgent)
	assert.Equal(t, "mcp.list_agent_profiles", ws.ActionMCPListAgentProfiles)
	assert.Equal(t, "mcp.create_agent_profile", ws.ActionMCPCreateAgentProfile)
	assert.Equal(t, "mcp.update_agent_profile", ws.ActionMCPUpdateAgentProfile)
	assert.Equal(t, "mcp.delete_agent_profile", ws.ActionMCPDeleteAgentProfile)
	assert.Equal(t, "mcp.get_mcp_config", ws.ActionMCPGetMcpConfig)
	assert.Equal(t, "mcp.update_mcp_config", ws.ActionMCPUpdateMcpConfig)
	assert.Equal(t, "mcp.list_executors", ws.ActionMCPListExecutors)
	assert.Equal(t, "mcp.list_executor_profiles", ws.ActionMCPListExecutorProfiles)
	assert.Equal(t, "mcp.create_executor_profile", ws.ActionMCPCreateExecutorProfile)
	assert.Equal(t, "mcp.update_executor_profile", ws.ActionMCPUpdateExecutorProfile)
	assert.Equal(t, "mcp.delete_executor_profile", ws.ActionMCPDeleteExecutorProfile)
	assert.Equal(t, "mcp.move_task", ws.ActionMCPMoveTask)
	assert.Equal(t, "mcp.delete_task", ws.ActionMCPDeleteTask)
	assert.Equal(t, "mcp.archive_task", ws.ActionMCPArchiveTask)
	assert.Equal(t, "mcp.update_task_state", ws.ActionMCPUpdateTaskState)
}

// --- Workflow handler tests ---

func TestCreateWorkflowHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "wf-1", "name": "Sprint Board"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow", map[string]interface{}{
		"workspace_id": "ws-123",
		"name":         "Sprint Board",
		"description":  "A sprint workflow",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateWorkflow, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ws-123", payload["workspace_id"])
	assert.Equal(t, "Sprint Board", payload["name"])
	assert.Equal(t, "A sprint workflow", payload["description"])
}

func TestCreateWorkflowHandler_MissingWorkspaceID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow", map[string]interface{}{
		"name": "Sprint Board",
	})

	assert.True(t, result.IsError)
}

func TestCreateWorkflowHandler_MissingName(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow", map[string]interface{}{
		"workspace_id": "ws-123",
	})

	assert.True(t, result.IsError)
}

func TestUpdateWorkflowHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "wf-1", "name": "Updated"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_workflow", map[string]interface{}{
		"workflow_id": "wf-1",
		"name":        "Updated",
		"description": "New description",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateWorkflow, backend.lastAction)
}

func TestUpdateWorkflowHandler_MissingWorkflowID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_workflow", map[string]interface{}{
		"name": "Updated",
	})

	assert.True(t, result.IsError)
}

func TestDeleteWorkflowHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"success": true},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_workflow", map[string]interface{}{
		"workflow_id": "wf-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPDeleteWorkflow, backend.lastAction)
}

func TestDeleteWorkflowHandler_MissingWorkflowID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_workflow", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestCreateWorkflowStepHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"step": map[string]interface{}{"id": "step-1", "name": "Review"}},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow_step", map[string]interface{}{
		"workflow_id": "wf-123",
		"name":        "Review",
		"color":       "#3b82f6",
		"position":    float64(2),
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateWorkflowStep, backend.lastAction)
}

func TestCreateWorkflowStepHandler_AllFields(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"step": map[string]interface{}{"id": "step-1"}},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow_step", map[string]interface{}{
		"workflow_id":           "wf-123",
		"name":                  "Deploy",
		"position":              float64(0),
		"color":                 "#22c55e",
		"prompt":                "Deploy prompt",
		"is_start_step":         true,
		"allow_manual_move":     true,
		"show_in_command_panel": true,
		"events": map[string]interface{}{
			"on_enter": []interface{}{map[string]interface{}{"type": "auto_start_agent"}},
		},
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateWorkflowStep, backend.lastAction)
	// Verify optional fields are forwarded in the payload
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, payload["allow_manual_move"])
	assert.Equal(t, true, payload["show_in_command_panel"])
	assert.NotNil(t, payload["events"])
}

func TestCreateWorkflowStepHandler_MissingWorkflowID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow_step", map[string]interface{}{
		"name": "Review",
	})

	assert.True(t, result.IsError)
}

func TestCreateWorkflowStepHandler_MissingName(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_workflow_step", map[string]interface{}{
		"workflow_id": "wf-123",
	})

	assert.True(t, result.IsError)
}

func TestUpdateWorkflowStepHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"step": map[string]interface{}{"id": "step-1"}},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_workflow_step", map[string]interface{}{
		"step_id": "step-1",
		"name":    "Updated Name",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateWorkflowStep, backend.lastAction)
}

func TestUpdateWorkflowStepHandler_AllFields(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"step": map[string]interface{}{"id": "step-1"}},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_workflow_step", map[string]interface{}{
		"step_id":                  "step-1",
		"name":                     "In Review",
		"color":                    "#3b82f6",
		"allow_manual_move":        true,
		"show_in_command_panel":    true,
		"auto_archive_after_hours": float64(48),
		"events": map[string]interface{}{
			"on_enter": []interface{}{map[string]interface{}{"type": "enable_plan_mode"}},
		},
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateWorkflowStep, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, payload["allow_manual_move"])
	assert.Equal(t, true, payload["show_in_command_panel"])
	assert.Equal(t, float64(48), payload["auto_archive_after_hours"])
	assert.NotNil(t, payload["events"])
}

func TestUpdateWorkflowStepHandler_MissingStepID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_workflow_step", map[string]interface{}{
		"name": "Updated",
	})

	assert.True(t, result.IsError)
}

// --- Agent handler tests ---

func TestListAgentsHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"agents": []interface{}{}, "total": float64(0)},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_agents", map[string]interface{}{})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListAgents, backend.lastAction)
}

func TestCreateAgentProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "profile-1", "name": "My Profile"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent_profile", map[string]interface{}{
		"agent_id": "agent-1",
		"name":     "My Profile",
		"model":    "claude-sonnet-4-5-20250514",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateAgentProfile, backend.lastAction)
}

func TestCreateAgentProfileHandler_MissingAgentID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent_profile", map[string]interface{}{
		"name":  "My Profile",
		"model": "claude-sonnet-4-5-20250514",
	})

	assert.True(t, result.IsError)
}

func TestCreateAgentProfileHandler_MissingModel(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent_profile", map[string]interface{}{
		"agent_id": "agent-1",
		"name":     "My Profile",
	})

	assert.True(t, result.IsError)
}

func TestUpdateAgentHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "agent-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_agent", map[string]interface{}{
		"agent_id":     "agent-1",
		"supports_mcp": true,
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateAgent, backend.lastAction)
}

func TestUpdateAgentHandler_MissingAgentID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_agent", map[string]interface{}{
		"supports_mcp": true,
	})

	assert.True(t, result.IsError)
}

func TestDeleteAgentProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "profile-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_agent_profile", map[string]interface{}{
		"profile_id": "profile-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPDeleteAgentProfile, backend.lastAction)
}

func TestDeleteAgentProfileHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_agent_profile", map[string]interface{}{})

	assert.True(t, result.IsError)
}

// --- MCP config handler tests ---

func TestListAgentProfilesHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"profiles": []interface{}{}, "total": float64(0)},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_agent_profiles", map[string]interface{}{
		"agent_id": "agent-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListAgentProfiles, backend.lastAction)
}

func TestListAgentProfilesHandler_MissingAgentID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_agent_profiles", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestUpdateAgentProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "profile-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_agent_profile", map[string]interface{}{
		"profile_id": "profile-1",
		"name":       "Updated Profile",
		"model":      "claude-3.5-sonnet",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateAgentProfile, backend.lastAction)
}

func TestUpdateAgentProfileHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_agent_profile", map[string]interface{}{
		"name": "Updated",
	})

	assert.True(t, result.IsError)
}

func TestGetMcpConfigHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"profile_id": "p-1", "enabled": true},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "get_mcp_config", map[string]interface{}{
		"profile_id": "p-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPGetMcpConfig, backend.lastAction)
}

func TestGetMcpConfigHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "get_mcp_config", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestUpdateMcpConfigHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"profile_id": "p-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_mcp_config", map[string]interface{}{
		"profile_id": "p-1",
		"enabled":    true,
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateMcpConfig, backend.lastAction)
}

func TestUpdateMcpConfigHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_mcp_config", map[string]interface{}{})

	assert.True(t, result.IsError)
}

// --- Executor handler tests ---

func TestListExecutorsHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"executors": []interface{}{}, "total": float64(0)},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_executors", map[string]interface{}{})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListExecutors, backend.lastAction)
}

func TestListExecutorProfilesHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"profiles": []interface{}{}, "total": float64(0)},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_executor_profiles", map[string]interface{}{
		"executor_id": "exec-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListExecutorProfiles, backend.lastAction)
}

func TestListExecutorProfilesHandler_MissingExecutorID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_executor_profiles", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestCreateExecutorProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "prof-1", "name": "Default"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_executor_profile", map[string]interface{}{
		"executor_id": "exec-1",
		"name":        "Default",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateExecutorProfile, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "exec-1", payload["executor_id"])
	assert.Equal(t, "Default", payload["name"])
}

func TestCreateExecutorProfileHandler_MissingExecutorID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_executor_profile", map[string]interface{}{
		"name": "Default",
	})

	assert.True(t, result.IsError)
}

func TestCreateExecutorProfileHandler_MissingName(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_executor_profile", map[string]interface{}{
		"executor_id": "exec-1",
	})

	assert.True(t, result.IsError)
}

func TestUpdateExecutorProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "prof-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_executor_profile", map[string]interface{}{
		"profile_id": "prof-1",
		"name":       "Updated Profile",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPUpdateExecutorProfile, backend.lastAction)
}

func TestUpdateExecutorProfileHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "update_executor_profile", map[string]interface{}{
		"name": "Updated",
	})

	assert.True(t, result.IsError)
}

func TestDeleteExecutorProfileHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"success": true},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_executor_profile", map[string]interface{}{
		"profile_id": "prof-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPDeleteExecutorProfile, backend.lastAction)
}

func TestDeleteExecutorProfileHandler_MissingProfileID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_executor_profile", map[string]interface{}{})

	assert.True(t, result.IsError)
}

// --- ForwardToBackend tests ---

func TestForwardToBackend_BackendError(t *testing.T) {
	backend := &testBackend{
		err: fmt.Errorf("connection refused"),
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_agents", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestForwardToBackend_ResultContainsJSON(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"agents": []interface{}{
				map[string]interface{}{"id": "a1", "name": "claude-code"},
			},
			"total": float64(1),
		},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "list_agents", map[string]interface{}{})

	assert.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	// The result should be JSON text content
	tc, ok := result.Content[0].(mcplib.TextContent)
	assert.True(t, ok, "expected TextContent")
	assert.NotEmpty(t, tc.Text)
}
