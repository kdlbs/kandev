package mcp

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManageTaskChangeRequestSchemaAndDispatch(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-target",
		"links": []interface{}{map[string]interface{}{
			"provider": "gitlab", "repository_id": "repo-gl", "number": 42,
		}},
	}}
	s := newTaskModeServer(t, backend, "task-current")

	tool, ok := s.mcpServer.ListTools()["manage_task_change_request_kandev"]
	require.True(t, ok, "neutral task change request tool must be registered")

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tool.Tool.RawInputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)
	for _, field := range []string{
		"operation", "task_id", "provider", "repository_id", "number",
		"old_provider", "old_repository_id", "old_number",
	} {
		assert.Contains(t, properties, field)
	}

	result := callTool(t, s, "manage_task_change_request_kandev", map[string]interface{}{
		"operation": "link", "task_id": "task-target", "provider": "gitlab",
		"repository_id": "repo-gl", "number": 42,
	})
	require.False(t, result.IsError)
	assert.Equal(t, "mcp.manage_task_change_request", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "link", payload["operation"])
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "task-current", payload["caller_task_id"])
	assert.Equal(t, "gitlab", payload["provider"])
	assert.Equal(t, "repo-gl", payload["repository_id"])
	assert.Equal(t, float64(42), payload["number"])
}

func TestManageTaskChangeRequestSchemaRejectsInvalidBranches(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "unknown field",
			args: map[string]interface{}{
				"operation": "link", "task_id": "task-target", "provider": "github",
				"repository_id": "repo-gh", "number": 1, "unexpected": true,
			},
		},
		{
			name: "old identity on link",
			args: map[string]interface{}{
				"operation": "link", "task_id": "task-target", "provider": "github",
				"repository_id": "repo-gh", "number": 1, "old_provider": "github",
			},
		},
		{
			name: "incomplete replacement identity",
			args: map[string]interface{}{
				"operation": "replace", "task_id": "task-target", "provider": "github",
				"repository_id": "repo-gh", "number": 1, "old_provider": "github",
			},
		},
		{
			name: "fractional number",
			args: map[string]interface{}{
				"operation": "link", "task_id": "task-target", "provider": "github",
				"repository_id": "repo-gh", "number": 1.5,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{}
			s := newTaskModeServer(t, backend, "task-current")
			result := callTool(t, s, "manage_task_change_request_kandev", tt.args)
			assert.True(t, result.IsError)
			assert.Empty(t, backend.lastAction, "invalid input must not reach the backend")
		})
	}
}

func TestManageTaskChangeRequestToolPreservesSerializedFailureState(t *testing.T) {
	details := map[string]interface{}{
		"task_id": "task-current",
		"links": []interface{}{
			map[string]interface{}{"provider": "gitlab", "repository_id": "repo-old", "number": 7},
			map[string]interface{}{"provider": "gitlab", "repository_id": "repo-new", "number": 42},
		},
		"operation_error": "old unlink failed",
		"rollback_error":  "rollback failed",
		"state_known":     false,
	}
	response, err := ws.NewError("ignored", ws.ActionMCPManageTaskChangeRequest, "validation", "failed to manage task change request", details)
	require.NoError(t, err)
	dispatcher := &fakeDispatcher{resp: response}
	client := NewDispatcherBackendClient(dispatcher, newTestLogger(t))
	s := newTaskModeServer(t, client, "task-current")

	result := callTool(t, s, "manage_task_change_request_kandev", map[string]interface{}{
		"operation": "replace", "task_id": "task-current", "provider": "gitlab",
		"repository_id": "repo-new", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-old", "old_number": 7,
	})

	require.True(t, result.IsError)
	structured, ok := result.StructuredContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "old unlink failed", structured["operation_error"])
	assert.Equal(t, "rollback failed", structured["rollback_error"])
	assert.Equal(t, false, structured["state_known"])
	assert.Len(t, structured["links"], 2)
	content, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	wantText, err := json.MarshalIndent(details, "", "  ")
	require.NoError(t, err)
	assert.JSONEq(t, string(wantText), content.Text)
}

func TestGetTaskChangeRequestsToolIsBoundAndClosed(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-current",
	}}
	s := newTaskModeServer(t, backend, "task-current")

	tool, ok := s.mcpServer.ListTools()["get_task_change_requests_kandev"]
	require.True(t, ok, "neutral task change request read tool must be registered")

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tool.Tool.RawInputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.Empty(t, schema["required"])
	assert.Empty(t, schema["properties"])

	result := callTool(t, s, "get_task_change_requests_kandev", map[string]interface{}{})
	require.False(t, result.IsError)
	assert.Equal(t, "mcp.get_task_change_requests", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"])
}

func TestChangeRequestAutomationToolSchemaAndDispatch(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-current", "status": "applied", "state_known": true,
	}}
	s := newTaskModeServer(t, backend, "task-current")

	tool, ok := s.mcpServer.ListTools()["update_task_change_request_automation_kandev"]
	require.True(t, ok, "neutral task change request automation tool must be registered")
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tool.Tool.RawInputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.Equal(t, []interface{}{"target", "patch"}, schema["required"])

	result := callTool(t, s, "update_task_change_request_automation_kandev", map[string]interface{}{
		"target": map[string]interface{}{"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 8},
		"patch":  map[string]interface{}{"auto_fix_enabled": false},
	})
	require.False(t, result.IsError)
	assert.Equal(t, "mcp.update_task_change_request_automation", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, payload, "task_id")
	assert.Equal(t, false, payload["patch"].(map[string]interface{})["auto_fix_enabled"])
}
