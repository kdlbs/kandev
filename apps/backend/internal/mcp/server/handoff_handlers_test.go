package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relatedTasksResponse is a related-task graph shaped like the backend's, with
// one description long enough that dropping it is visible in the payload size.
func relatedTasksResponse(description string) map[string]interface{} {
	return map[string]interface{}{
		"task": map[string]interface{}{
			"id": "task-A", "title": "Parent work", "state": "RUNNING", "description": description,
		},
		"parent": map[string]interface{}{
			"id": "task-P", "title": "Epic", "state": "RUNNING", "description": description,
		},
		"children": []interface{}{
			map[string]interface{}{
				"id": "task-C1", "title": "Child one", "state": "CREATED",
				"description": description, "document_keys": []interface{}{"spec"},
			},
			map[string]interface{}{
				"id": "task-C2", "title": "Child two", "state": "DONE", "description": description,
			},
		},
		"siblings":   []interface{}{},
		"blockers":   []interface{}{},
		"blocked_by": []interface{}{},
	}
}

func relatedTasksText(t *testing.T, args map[string]interface{}, description string) string {
	t.Helper()
	backend := &testBackend{response: relatedTasksResponse(description)}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_related_tasks_kandev", args)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestListRelatedTasks_OmitsDescriptionsByDefault(t *testing.T) {
	description := "a long task description that dominates the response size"

	text := relatedTasksText(t, map[string]interface{}{}, description)

	assert.NotContains(t, text, description)
	assert.NotContains(t, text, `"description"`)
	// The compact projection must still carry everything the caller navigates by.
	for _, want := range []string{"task-A", "task-P", "task-C1", "task-C2", "Child one", "CREATED", "spec"} {
		assert.Contains(t, text, want)
	}
}

func TestListRelatedTasks_VerboseKeepsDescriptions(t *testing.T) {
	description := "Depends on: task-C1"

	text := relatedTasksText(t, map[string]interface{}{"verbose": true}, description)

	assert.Contains(t, text, description)
	assert.Contains(t, text, `"description"`)
}

func TestListRelatedTasks_ToolSchemaExposesVerbose(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-A")

	tool, ok := s.mcpServer.ListTools()["list_related_tasks_kandev"]
	require.True(t, ok)

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	verbose, ok := props["verbose"].(map[string]interface{})
	require.True(t, ok, "verbose must be advertised so callers can opt into descriptions")
	assert.Equal(t, "boolean", verbose["type"])

	required, _ := parsed["required"].([]interface{})
	assert.NotContains(t, required, "verbose")
	assert.Contains(t, tool.Tool.Description, "verbose=true")
}
