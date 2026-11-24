package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestConfigAutomationAcceptsPerPRSettings(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "saved"}}
	s := newTestServer(t, backend)
	args := map[string]interface{}{
		"workspace_id": "ws", "name": "PR review", "task_mode": "normal_task", "workflow_id": "workflow",
		"continuation_policy": "reuse_per_pr", "follow_up_prompt": "Review only {{pr.previous_head_sha}}..{{pr.head_sha}}",
		"pr_continuation_step_id": "review", "pr_merged_action": "archive", "pr_closed_action": "delete",
		"triggers": []interface{}{map[string]interface{}{"type": "github_pr", "enabled": true, "config": map[string]interface{}{
			"events": []interface{}{"opened", "new_commits", "review_requested"}, "quiet_period_minutes": float64(10),
			"reviewers": []interface{}{"kandev-bot"}, "include_existing_prs": true,
		}}},
	}
	result := callTool(t, s, "create_automation_kandev", args)
	require.False(t, result.IsError)
	assert.Equal(t, args, backend.lastPayload)

	for _, bad := range []map[string]interface{}{
		{"workspace_id": "ws", "name": "x", "continuation_policy": "reuse_forever"},
		{"workspace_id": "ws", "name": "x", "pr_merged_action": "explode"},
	} {
		backend := &testBackend{}
		result := callTool(t, newTestServer(t, backend), "create_automation_kandev", bad)
		assert.True(t, result.IsError, "%v", bad)
		assert.Empty(t, backend.lastAction)
	}
}
