package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// update_task_kandev must forward deferred_launch_prompt, and only when the
// caller sent one: an unconditional key would make every ordinary title edit
// look like a launch-prompt rewrite and be rejected on started tasks.
func TestUpdateTask_ForwardsDeferredLaunchPromptOnlyWhenSupplied(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "task-1"}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_kandev", map[string]interface{}{
		"task_id":                "task-1",
		"deferred_launch_prompt": "the refreshed brief",
	})
	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "the refreshed brief", payload["deferred_launch_prompt"])

	result = callTool(t, s, "update_task_kandev", map[string]interface{}{
		"task_id": "task-1",
		"title":   "Renamed",
	})
	require.False(t, result.IsError)
	payload, ok = backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, payload, "deferred_launch_prompt")
}

// The argument has to be advertised, or no agent can reach the fix.
func TestUpdateTask_ToolSchemaExposesDeferredLaunchPrompt(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	tool, ok := s.mcpServer.ListTools()["update_task_kandev"]
	require.True(t, ok)
	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))
	properties, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	prop, ok := properties["deferred_launch_prompt"].(map[string]interface{})
	require.True(t, ok, "the launch-prompt argument must be advertised or no agent can reach it")

	description, _ := prop["description"].(string)
	assert.Contains(t, description, "blocked_by",
		"the description must say which tasks this applies to")
	assert.Contains(t, description, "message_task_kandev",
		"a started task's caller needs to be told where to send context instead")
}
