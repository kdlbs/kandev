package mcp

import (
	"encoding/json"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportWorkflowTool_AdvertisesReadOnlyRisk(t *testing.T) {
	s := newTestServer(t, &testBackend{})
	tool, ok := s.mcpServer.ListTools()["export_workflow_kandev"]
	require.True(t, ok)
	annotations := tool.Tool.Annotations
	require.NotNil(t, annotations.ReadOnlyHint)
	require.NotNil(t, annotations.DestructiveHint)
	require.NotNil(t, annotations.IdempotentHint)
	require.NotNil(t, annotations.OpenWorldHint)
	assert.True(t, *annotations.ReadOnlyHint)
	assert.False(t, *annotations.DestructiveHint)
	assert.True(t, *annotations.IdempotentHint)
	assert.False(t, *annotations.OpenWorldHint)
	assert.Contains(t, tool.Tool.Description, "1 MiB")
}

func TestExportWorkflowHandler_ForwardsWorkflowIDAndReturnsJSONText(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"version": 1,
			"type":    "kandev_workflow",
			"workflows": []interface{}{
				map[string]interface{}{"name": "Sprint Board", "steps": []interface{}{}},
			},
		},
	}
	s := newTestServer(t, backend)

	result := callTool(t, s, "export_workflow_kandev", map[string]interface{}{
		"workflow_id": "wf-123",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, "mcp.export_workflow", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "wf-123", payload["workflow_id"])
	require.Len(t, result.Content, 1)
	content, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var exported map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(content.Text), &exported))
	assert.Equal(t, float64(1), exported["version"])
	assert.Equal(t, "kandev_workflow", exported["type"])
}

func TestExportWorkflowHandler_MissingWorkflowID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "export_workflow_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
}

func TestExportWorkflowHandler_RejectsWhitespaceWorkflowID(t *testing.T) {
	backend := &testBackend{}
	s := newTestServer(t, backend)

	result := callTool(t, s, "export_workflow_kandev", map[string]interface{}{
		"workflow_id": " \t",
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
}
