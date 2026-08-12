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
	assert.Contains(t, description, "agent_profile_id")
	assert.Contains(t, description, "effective agent profile")
	assert.Contains(t, description, "Returns {task_id, session_id, state, agent_profile_id}")
}
