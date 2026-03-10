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
	return New(backend, "test-session", 10005, log, "", false, ModeConfig)
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
	assert.Equal(t, "mcp.create_workflow_step", ws.ActionMCPCreateWorkflowStep)
	assert.Equal(t, "mcp.update_workflow_step", ws.ActionMCPUpdateWorkflowStep)
	assert.Equal(t, "mcp.delete_workflow_step", ws.ActionMCPDeleteWorkflowStep)
	assert.Equal(t, "mcp.reorder_workflow_steps", ws.ActionMCPReorderWorkflowStep)
	assert.Equal(t, "mcp.list_agents", ws.ActionMCPListAgents)
	assert.Equal(t, "mcp.create_agent", ws.ActionMCPCreateAgent)
	assert.Equal(t, "mcp.update_agent", ws.ActionMCPUpdateAgent)
	assert.Equal(t, "mcp.delete_agent", ws.ActionMCPDeleteAgent)
	assert.Equal(t, "mcp.list_agent_profiles", ws.ActionMCPListAgentProfiles)
	assert.Equal(t, "mcp.update_agent_profile", ws.ActionMCPUpdateAgentProfile)
	assert.Equal(t, "mcp.get_mcp_config", ws.ActionMCPGetMcpConfig)
	assert.Equal(t, "mcp.update_mcp_config", ws.ActionMCPUpdateMcpConfig)
	assert.Equal(t, "mcp.move_task", ws.ActionMCPMoveTask)
	assert.Equal(t, "mcp.delete_task", ws.ActionMCPDeleteTask)
	assert.Equal(t, "mcp.archive_task", ws.ActionMCPArchiveTask)
	assert.Equal(t, "mcp.update_task_state", ws.ActionMCPUpdateTaskState)
}

// --- Workflow handler tests ---

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

func TestCreateAgentHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "agent-1", "name": "my-agent"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent", map[string]interface{}{
		"name": "my-agent",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateAgent, backend.lastAction)
}

func TestCreateAgentHandler_MissingName(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestCreateAgentHandler_WithWorkspaceID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "agent-1"},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "create_agent", map[string]interface{}{
		"name":         "my-agent",
		"workspace_id": "ws-123",
	})

	assert.False(t, result.IsError)
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

func TestDeleteAgentHandler_Success(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"success": true},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_agent", map[string]interface{}{
		"agent_id": "agent-1",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPDeleteAgent, backend.lastAction)
}

func TestDeleteAgentHandler_MissingAgentID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "delete_agent", map[string]interface{}{})

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
