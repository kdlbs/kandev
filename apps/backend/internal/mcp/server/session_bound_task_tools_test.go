package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionBoundChangeRequestToolsRejectTaskID(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{
			name: "automation update",
			tool: "update_task_change_request_automation_kandev",
			args: map[string]interface{}{
				"task_id": "task-parent",
				"target":  map[string]interface{}{"scope": "task", "providers": []interface{}{"github"}},
				"patch":   map[string]interface{}{"auto_merge_enabled": true},
			},
		},
		{
			name: "read",
			tool: "get_task_change_requests_kandev",
			args: map[string]interface{}{"task_id": "task-parent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{}
			s := newTaskModeServer(t, backend, "task-current")

			result := callTool(t, s, tt.tool, tt.args)

			require.True(t, result.IsError)
			assert.Empty(t, backend.lastAction, "rejected arguments must not reach the backend")
			require.NotEmpty(t, result.Content)
			content, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			assert.Contains(t, content.Text, `unknown arguments: "task_id"`)
			assert.Contains(t, content.Text, "This tool is bound to the calling task; cross-task targeting is not supported")
		})
	}
}

func TestSessionBoundChangeRequestToolsRejectTaskIDInsidePatch(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_change_request_automation_kandev", map[string]interface{}{
		"target": map[string]interface{}{"scope": "task", "providers": []interface{}{"github"}},
		"patch": map[string]interface{}{
			"auto_merge_enabled": true,
			"task_id":            "task-parent",
		},
	})

	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "rejected arguments must not reach the backend")
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `/patch`)
	assert.Contains(t, content.Text, `unknown arguments: "task_id"`)
	assert.Contains(t, content.Text, "This tool is bound to the calling task; cross-task targeting is not supported")
}

func TestSessionBoundChangeRequestToolsNameTaskIDWithOtherValidationErrors(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_change_request_automation_kandev", map[string]interface{}{
		"task_id": "task-parent",
	})

	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "rejected arguments must not reach the backend")
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `unknown arguments: "task_id"`)
	assert.Contains(t, content.Text, "This tool is bound to the calling task; cross-task targeting is not supported")
	assert.Contains(t, content.Text, `missing: "patch", "target"`)
	assert.NotContains(t, content.Text, "task-parent")
}

func TestSessionBoundChangeRequestToolsNameNestedTaskIDWithOtherValidationErrors(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_change_request_automation_kandev", map[string]interface{}{
		"target": map[string]interface{}{"scope": "association"},
		"patch":  map[string]interface{}{"task_id": "task-parent"},
	})

	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "rejected arguments must not reach the backend")
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `unknown arguments: "task_id" at /patch`)
	assert.Contains(t, content.Text, "This tool is bound to the calling task; cross-task targeting is not supported")
	assert.NotContains(t, content.Text, "task-parent")
}

func TestSessionBoundChangeRequestToolsDeclareBinding(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	for _, name := range []string{
		"get_task_change_requests_kandev",
		"update_task_change_request_automation_kandev",
	} {
		t.Run(name, func(t *testing.T) {
			tool, ok := s.mcpServer.ListTools()[name]
			require.True(t, ok, "tool must be registered")
			assert.Contains(t, tool.Tool.Description,
				"The task is bound to the calling session and is not a tool argument")
		})
	}
}

func TestUnknownArgumentNamingStaysGenericOffSessionBoundTools(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_workspaces_kandev", map[string]interface{}{"task_id": "task-current"})

	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `unknown arguments: "task_id"`)
	assert.NotContains(t, content.Text, "bound to the calling task")
}
