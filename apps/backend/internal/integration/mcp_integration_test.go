package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// setupMCPTestServer creates a test server with MCP handlers registered
// and returns the server along with a parent task's IDs for subtask tests.
func setupMCPTestServer(t *testing.T) (*TestServer, string, string, string, string) {
	t.Helper()

	ts := NewTestServer(t)

	// Register MCP handlers on the gateway dispatcher
	mcpH := mcphandlers.NewHandlers(ts.TaskSvc, nil, nil, nil, nil, nil, nil, nil, nil, ts.Logger)
	mcpH.RegisterHandlers(ts.Gateway.Dispatcher)

	client := NewWSClient(t, ts.Server.URL)
	t.Cleanup(func() { client.Close() })

	workspaceID := createWorkspace(t, client)

	// Create workflow
	workflowResp, err := client.SendRequest("wf-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "MCP Test Workflow",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)
	var wfPayload map[string]interface{}
	require.NoError(t, workflowResp.ParsePayload(&wfPayload))
	workflowID := wfPayload["id"].(string)

	// Get first workflow step
	stepResp, err := client.SendRequest("steps-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)
	var stepPayload map[string]interface{}
	require.NoError(t, stepResp.ParsePayload(&stepPayload))
	steps := stepPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// Create parent task
	taskResp, err := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": workflowStepID,
		"title":            "Parent Task",
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, taskResp.Type)
	var taskPayload map[string]interface{}
	require.NoError(t, taskResp.ParsePayload(&taskPayload))
	parentTaskID := taskPayload["id"].(string)

	return ts, parentTaskID, workspaceID, workflowID, workflowStepID
}

func TestMCPCreateTask_SubtaskInheritsFromParent(t *testing.T) {
	ts, parentTaskID, workspaceID, workflowID, _ := setupMCPTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	// Create subtask with title, description, and parent_id — workspace/workflow/step should be inherited
	resp, err := client.SendRequest("subtask-1", ws.ActionMCPCreateTask, map[string]interface{}{
		"parent_id":   parentTaskID,
		"title":       "Subtask via MCP",
		"description": "Implement the subtask feature",
	})
	require.NoError(t, err)

	if resp.Type == ws.MessageTypeError {
		var errPayload ws.ErrorPayload
		require.NoError(t, resp.ParsePayload(&errPayload))
		t.Fatalf("expected response but got error: %s", errPayload.Message)
	}

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.NotEmpty(t, payload["id"])
	assert.Equal(t, "Subtask via MCP", payload["title"])
	assert.Equal(t, parentTaskID, payload["parent_id"])
	assert.Equal(t, workspaceID, payload["workspace_id"])
	assert.Equal(t, workflowID, payload["workflow_id"])
}

func TestMCPCreateTask_SubtaskVisibleInListTasks(t *testing.T) {
	ts, parentTaskID, _, _, _ := setupMCPTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	// Get parent's workflow_id for listing
	getResp, err := client.SendRequest("get-parent", ws.ActionTaskGet, map[string]interface{}{
		"id": parentTaskID,
	})
	require.NoError(t, err)
	var parentPayload map[string]interface{}
	require.NoError(t, getResp.ParsePayload(&parentPayload))
	workflowID := parentPayload["workflow_id"].(string)

	// Create subtask
	resp, err := client.SendRequest("subtask-1", ws.ActionMCPCreateTask, map[string]interface{}{
		"parent_id":   parentTaskID,
		"title":       "Visible Subtask",
		"description": "Implement the visible subtask feature",
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	// List tasks — should contain both parent and subtask
	listResp, err := client.SendRequest("list-1", ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)
	var listPayload map[string]interface{}
	require.NoError(t, listResp.ParsePayload(&listPayload))

	tasks := listPayload["tasks"].([]interface{})
	assert.Len(t, tasks, 2, "should have parent + subtask")

	// Find the subtask and verify parent_id
	var found bool
	for _, raw := range tasks {
		task := raw.(map[string]interface{})
		if task["title"] == "Visible Subtask" {
			assert.Equal(t, parentTaskID, task["parent_id"])
			found = true
		}
	}
	assert.True(t, found, "subtask should be visible in list_tasks")
}

func TestMCPCreateTask_TopLevelWithoutParent(t *testing.T) {
	ts, _, workspaceID, workflowID, workflowStepID := setupMCPTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	// Create top-level task (no parent_id, all IDs provided)
	resp, err := client.SendRequest("top-1", ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": workflowStepID,
		"title":            "Top Level Task",
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.NotEmpty(t, payload["id"])
	assert.Equal(t, "Top Level Task", payload["title"])
	// parent_id should be empty (omitempty means absent from JSON)
	_, hasParent := payload["parent_id"]
	assert.False(t, hasParent, "top-level task should not have parent_id")
}

func TestMCPCreateTask_NoParentNoIDs_ReturnsError(t *testing.T) {
	ts, _, _, _, _ := setupMCPTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	// No parent_id and no workspace/workflow -> should fail
	resp, err := client.SendRequest("fail-1", ws.ActionMCPCreateTask, map[string]interface{}{
		"title": "Orphan Task",
	})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}

func TestMCPCreateTask_InvalidParentID_ReturnsError(t *testing.T) {
	ts, _, _, _, _ := setupMCPTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("bad-parent", ws.ActionMCPCreateTask, map[string]interface{}{
		"parent_id": "nonexistent-task-id",
		"title":     "Bad Parent",
	})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
}
