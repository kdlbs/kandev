package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanReadReturnsMetadataAndExactContentBlocks(t *testing.T) {
	content := "# Plan\r\n\n- [ ] keep exact spacing\r\n"
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A", "title": "Plan", "content": content,
		"version": "write-v1", "updated_at": "2026-09-17T10:00:00Z",
	}}
	s := newTaskModeServer(t, backend, "task-A")
	result := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{})
	require.False(t, result.IsError)
	require.Len(t, result.Content, 2)
	metadata, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, metadata.Text, "write-v1")
	assert.NotContains(t, metadata.Text, content)
	body, ok := result.Content[1].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, content, body.Text)
}

func TestPlanRevisionToolsBridgeAllArguments(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A", "revisions": []interface{}{},
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_task_plan_revisions_kandev", map[string]interface{}{
		"before_revision_number": 7, "limit": 4,
	})
	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-A", payload["task_id"])
	assert.Equal(t, 7, payload["before_revision_number"])
	assert.Equal(t, 4, payload["limit"])

	backend.response = map[string]interface{}{
		"task_id": "task-A", "revision_id": "rev-1", "content": "exact",
		"revision_version": "rev-v1",
	}
	result = callTool(t, s, "get_task_plan_revision_kandev", map[string]interface{}{
		"revision_id": "rev-1",
	})
	require.False(t, result.IsError)
	require.Len(t, result.Content, 2)
	body, ok := result.Content[1].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "exact", body.Text)

	backend.response = map[string]interface{}{
		"task_id": "task-A", "status": "restored", "revision_id": "rev-2",
		"revision_number": 2, "version": "write-v2", "content_bytes": 5,
	}
	result = callTool(t, s, "restore_task_plan_revision_kandev", map[string]interface{}{
		"revision_id": "rev-1", "expected_version": "write-v1",
		"expected_revision_version": "rev-v1",
	})
	require.False(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "write-v2")
	assert.Contains(t, text, "rev-2")
	assert.NotContains(t, text, "exact")
	assert.Equal(t, "mcp.restore_task_plan_revision", backend.lastAction)
	encoded, err := json.Marshal(backend.lastPayload)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "expected_revision_version")
}
