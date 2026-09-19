package mcp

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-OFFICE-CONFIG-AUTOMATION-001.1
func TestConfigAutomationCatalog(t *testing.T) {
	for _, mode := range []string{ModeConfig, ModeExternal, ModeTask, ModeTaskTitlePending, ModeOffice, ModeAutomation} {
		t.Run(mode, func(t *testing.T) {
			s := New(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, mode)
			_, found := s.mcpServer.ListTools()["create_automation_kandev"]
			assert.Equal(t, mode == ModeConfig, found)
			s.SetMode(ModeConfig)
			assert.Contains(t, s.mcpServer.ListTools(), "create_automation_kandev")
			s.SetMode(mode)
			_, found = s.mcpServer.ListTools()["create_automation_kandev"]
			assert.Equal(t, mode == ModeConfig, found)
		})
	}
}

// @covers AC-OFFICE-CONFIG-AUTOMATION-001.2
func TestConfigAutomationArguments(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "saved"}}
	s := newTestServer(t, backend)
	args := map[string]interface{}{
		"workspace_id": "ws", "name": "Daily report", "prompt": "Report progress",
		"description": "Progress summary", "workflow_id": "workflow", "workflow_step_id": "step",
		"agent_profile_id": "agent", "executor_profile_id": "executor", "task_title_template": "Daily {{trigger.type}}",
		"max_concurrent_runs": float64(1), "continuation_policy": "reuse_thread", "task_mode": "normal_task", "repository_mode": "selected",
		"repository_ids": []interface{}{"repo"},
		"repositories":   []interface{}{map[string]interface{}{"repository_id": "repo", "base_branch": "main"}},
		"triggers":       []interface{}{map[string]interface{}{"type": "scheduled", "config": map[string]interface{}{"cron_expression": "0 9 * * *"}, "enabled": true}},
	}
	result := callTool(t, s, "create_automation_kandev", args)
	require.False(t, result.IsError)
	assert.Equal(t, "mcp.create_automation", backend.lastAction)
	assert.Equal(t, args, backend.lastPayload)
}

func TestConfigAutomationInvalidArguments(t *testing.T) {
	for _, args := range []map[string]interface{}{
		{"workspace_id": "ws"},
		{"workspace_id": "ws", "name": "Daily", "triggers": "invalid"},
		{"workspace_id": "ws", "name": "Daily", "triggers": []interface{}{map[string]interface{}{"type": "scheduled", "config": "invalid"}}},
	} {
		backend := &testBackend{}
		result := callTool(t, newTestServer(t, backend), "create_automation_kandev", args)
		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	}
}
