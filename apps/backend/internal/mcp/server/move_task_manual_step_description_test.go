package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-TASKS-WORKFLOW-STAGE-MOVES-004.4: both move_task_kandev registrations
// tell the agent to omit hand-off options for a step that starts no agent.
func TestMoveTaskDescriptionsStateManualStepRule(t *testing.T) {
	for name, server := range map[string]*Server{
		"task":   newTaskModeServer(t, &testBackend{}, "task-current"),
		"config": newTestServer(t, &testBackend{}),
	} {
		tool := server.mcpServer.ListTools()["move_task_kandev"]
		assert.Contains(t, tool.Tool.Description, "does not start an agent on entry", "%s tool description", name)
		props := toolInputProperties(t, server, "move_task_kandev")
		prompt, ok := props["prompt"].(map[string]interface{})
		require.True(t, ok, "%s prompt schema", name)
		assert.Contains(t, prompt["description"], "does not start an agent on entry", "%s prompt description", name)
		entryOptions, ok := props["entry_options"].(map[string]interface{})
		require.True(t, ok, "%s entry_options schema", name)
		assert.Contains(t, entryOptions["description"], "does not start an agent on entry", "%s entry_options description", name)
	}
}
