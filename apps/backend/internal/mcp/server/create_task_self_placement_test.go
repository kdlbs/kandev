package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTaskSelfPlacementIntent(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "sibling-1", "parent_id": "parent-1"},
	}
	s := newTaskModeServer(t, backend, "child-1")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Sibling task",
		"parent_id": "self",
	})

	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "child-1", payload["parent_id"])
	assert.Equal(t, true, payload["parent_is_self"])
	assert.Equal(t, "child-1", payload["source_task_id"])
	assert.Equal(t, "test-session", payload["source_session_id"])

	backend.lastPayload = nil
	result = callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Explicit child",
		"parent_id": "child-1",
	})
	require.False(t, result.IsError)
	payload, ok = backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, payload, "parent_is_self")

	result = callTool(t, s, "create_task_kandev", map[string]interface{}{"title": "Top-level task"})
	require.False(t, result.IsError)
	payload = backend.lastPayload.(map[string]interface{})
	assert.Empty(t, payload["parent_id"])
	assert.NotContains(t, payload, "parent_is_self")
}

// @covers AC-TASKS-SELF-SIBLING-002.4
func TestCreateTaskSelfPlacementRejectsPublicMarker(t *testing.T) {
	for _, parentID := range []string{"self", "child-1", ""} {
		t.Run(parentID, func(t *testing.T) {
			backend := &testBackend{}
			s := newTaskModeServer(t, backend, "child-1")
			result := callTool(t, s, "create_task_kandev", map[string]interface{}{
				"title": "Forged intent", "parent_id": parentID, "parent_is_self": true,
			})
			require.True(t, result.IsError)
			require.Contains(t, firstText(t, result), "unknown arguments")
			require.Empty(t, backend.lastAction)
		})
	}
}

// @covers AC-TASKS-SELF-SIBLING-002.4 AC-TASKS-SELF-SIBLING-002.5
func TestCreateTaskSelfPlacementExternalBoundary(t *testing.T) {
	backend := &testBackend{}
	s := NewExternal(backend, newTestLogger(t), "")
	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title": "No current task", "parent_id": "self",
	})
	require.True(t, result.IsError)
	require.Contains(t, firstText(t, result), "no current task context")
	require.Empty(t, backend.lastAction)
	require.NotContains(t, s.mcpServer.ListTools()["create_task_kandev"].Tool.Description, "sibling")
}

func TestCreateTaskSelfPlacementResult(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"id":        "sibling-1",
			"parent_id": "parent-1",
			"parent_resolution": map[string]interface{}{
				"requested_parent_id": "child-1",
				"resolved_parent_id":  "parent-1",
				"reason":              "kanban_depth_limit",
				"message":             "Created a sibling task under parent-1 because the Kanban subtask depth limit was reached.",
			},
			"description": "prompt should be removed from the tool result",
		},
	}
	s := newTaskModeServer(t, backend, "child-1")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Sibling task",
		"parent_id": "self",
	})

	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	assert.Equal(t, backend.response["parent_resolution"], payload["parent_resolution"])
	assert.NotContains(t, payload, "description")
}

// @covers AC-TASKS-SELF-SIBLING-001.4
func TestCreateTaskSelfPlacementExistingResult(t *testing.T) {
	for _, complete := range []bool{false, true} {
		backend := &testBackend{response: map[string]interface{}{
			"id": "existing", "parent_id": "actual-parent", "deduplicated": true, "creation_complete": complete,
			"parent_resolution": map[string]interface{}{
				"requested_parent_id": "child-1", "resolved_parent_id": "parent-1", "reason": "kanban_depth_limit",
				"message": "Existing task returned without creation or reparenting.",
			},
		}}
		s := newTaskModeServer(t, backend, "child-1")
		result := callTool(t, s, "create_task_kandev", map[string]interface{}{"title": "Retry", "parent_id": "self"})
		require.False(t, result.IsError)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(firstText(t, result)), &payload))
		require.Equal(t, backend.response, payload)
	}
}

func TestCreateTaskSelfPlacementToolDescription(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "child-1")
	tool := s.mcpServer.ListTools()["create_task_kandev"]
	require.NotNil(t, tool)
	assert.Contains(t, tool.Tool.Description, "sibling")
	assert.Contains(t, tool.Tool.Description, "common parent")
	assert.Contains(t, tool.Tool.Description, "ordinary messaging")
	assert.NotContains(t, tool.Tool.Description, "parent_is_self")
}
