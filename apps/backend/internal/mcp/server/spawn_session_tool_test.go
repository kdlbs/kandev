package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnSessionToolDescribesEffectiveAgentProfile(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	tool, ok := s.mcpServer.ListTools()["spawn_session_kandev"]
	require.True(t, ok, "spawn_session_kandev must be registered in task mode")

	description := tool.Tool.Description
	assert.Contains(t, description, "not a native subagent")
	assert.Contains(t, description, "only when the user explicitly requests another Kandev session")
	assert.Contains(t, description, "does not create a task")
	assert.NotContains(t, description, "pair of hands")
	assert.NotContains(t, description, "parallelizable piece")
	assert.Contains(t, description, "agent_profile_id")
	assert.Contains(t, description, "workflow step may override")
	assert.Contains(t, description, "Returns {task_id, session_id, state, agent_profile_id}")

	profileProperty, ok := toolInputProperties(t, s, "spawn_session_kandev")["agent_profile_id"].(map[string]interface{})
	require.True(t, ok, "agent_profile_id input must be described")
	assert.Contains(t, profileProperty["description"], "workflow launch profile may override it")
}
