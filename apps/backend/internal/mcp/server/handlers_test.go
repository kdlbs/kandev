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
	tool, ok := toolsMap["create_task_kandev"]
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

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Write tests",
		"parent_id": "self",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"], "self should resolve to current task ID")
	assert.Equal(t, "Write tests", payload["title"])
	assert.Equal(t, "task-current", payload["source_task_id"], "source_task_id should be set to current task")
	assert.Equal(t, true, payload["start_agent"], "start_agent should default to true")
}

func TestCreateTask_SelfWithNoTaskContext_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
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

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Fix bug",
		"parent_id": "task-abc",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-abc", payload["parent_id"])
}

func TestCreateTask_NoParentID_WithIDs_CreatesTopLevelTask(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Standalone"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Standalone",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "", payload["parent_id"])
	assert.Equal(t, "ws-1", payload["workspace_id"])
	assert.Equal(t, "wf-1", payload["workflow_id"])
	assert.Equal(t, "task-current", payload["source_task_id"])
}

func TestCreateTask_SourceTaskID_AlwaysSet(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new"},
	}
	s := newTaskModeServer(t, backend, "my-task-123")

	callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "my-task-123", payload["source_task_id"])
}

func TestCreateTask_SourceTaskID_EmptyWhenNoTaskContext(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new"},
	}
	s := newTaskModeServer(t, backend, "")

	callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "", payload["source_task_id"])
}

func TestCreateTask_StartAgentFalse_DoesNotAutoStart(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Plan task"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Plan task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
		"start_agent":  false,
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, payload["start_agent"], "start_agent should be false when explicitly set")
}

func TestCreateTask_WithRepositoryID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with repo"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":         "Task with repo",
		"workspace_id":  "ws-1",
		"workflow_id":   "wf-1",
		"repository_id": "repo-123",
		"base_branch":   "main",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "repo-123", repos[0]["repository_id"])
	assert.Equal(t, "main", repos[0]["base_branch"])
}

func TestCreateTask_WithLocalPath(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with local path"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Task with local path",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
		"local_path":   "/Users/me/projects/myrepo",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "/Users/me/projects/myrepo", repos[0]["local_path"])
}

func TestCreateTask_WithRepositoryURL(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with URL"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":          "Task with URL",
		"workspace_id":   "ws-1",
		"workflow_id":    "wf-1",
		"repository_url": "https://github.com/acme/widgets",
		"base_branch":    "main",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://github.com/acme/widgets", repos[0]["github_url"])
	assert.Equal(t, "main", repos[0]["base_branch"])
}

func TestCreateTask_RepositoryURL_RejectedForSubtasks(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":          "Subtask with URL",
		"parent_id":      "self",
		"repository_url": "https://github.com/acme/widgets",
	})

	assert.True(t, result.IsError, "repository_url should be rejected for subtasks")
}

func TestMessageTask_ForwardsToBackend(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"task_id":    "task-target",
			"session_id": "sess-1",
			"status":     "queued",
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"task_id": "task-target",
		"prompt":  "follow up",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPMessageTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "follow up", payload["prompt"])
}

func TestMessageTask_MissingTaskID_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"prompt": "follow up",
	})

	assert.True(t, result.IsError)
}

func TestMessageTask_MissingPrompt_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"task_id": "task-target",
	})

	assert.True(t, result.IsError)
}
