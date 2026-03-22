package mcp

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTaskModeServer(t *testing.T, backend BackendClient, taskID string) *Server {
	t.Helper()
	log := newTestLogger(t)
	return New(backend, "test-session", taskID, 10005, log, "", false, ModeTask)
}

func TestCreateTask_ToolSchema_HasParentID(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	toolsMap := s.mcpServer.ListTools()
	tool, ok := toolsMap["create_task"]
	require.True(t, ok, "create_task tool not registered")

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "parent_id", "create_task schema must expose parent_id")
	assert.Contains(t, props, "title")
	assert.Contains(t, props, "workspace_id")
	assert.Contains(t, props, "workflow_id")

	// parent_id, workspace_id, workflow_id, workflow_step_id should NOT be required
	required, _ := parsed["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	assert.True(t, requiredSet["title"], "title should be required")
	assert.False(t, requiredSet["parent_id"], "parent_id should not be required")
	assert.False(t, requiredSet["workspace_id"], "workspace_id should not be required")
	assert.False(t, requiredSet["workflow_id"], "workflow_id should not be required")
}

func TestCreateTask_SelfResolvesToTaskID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task", map[string]interface{}{
		"title":     "Write tests",
		"parent_id": "self",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"], "self should resolve to current task ID")
	assert.Equal(t, "Write tests", payload["title"])
}

func TestCreateTask_SelfWithNoTaskContext_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "")

	result := callTool(t, s, "create_task", map[string]interface{}{
		"title":     "Write tests",
		"parent_id": "self",
	})

	assert.True(t, result.IsError)
}

func TestCreateTask_ExplicitParentID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-abc"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task", map[string]interface{}{
		"title":     "Fix bug",
		"parent_id": "task-abc",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "task-abc", payload["parent_id"])
}

func TestCreateTask_NoParentID_RequiresWorkspaceAndWorkflow(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	// No parent_id and no workspace/workflow -> error
	result := callTool(t, s, "create_task", map[string]interface{}{
		"title": "Standalone task",
	})

	assert.True(t, result.IsError)
}

func TestCreateTask_NoParentID_WithIDs_CreatesTopLevelTask(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Standalone"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task", map[string]interface{}{
		"title":        "Standalone",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "", payload["parent_id"])
	assert.Equal(t, "ws-1", payload["workspace_id"])
	assert.Equal(t, "wf-1", payload["workflow_id"])
}
