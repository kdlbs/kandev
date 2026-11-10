package mcp

import (
	"encoding/json"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestStepCompleteHandlerSerializesOutcome(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"accepted":    false,
		"reason":      "already_signaled",
		"step_id":     "step-build",
		"signal_id":   "signal-1",
		"signaled_at": "2026-11-10T10:00:00Z",
		"source": map[string]interface{}{
			"session_id": "session-1", "turn_id": "turn-1", "turn_known": true, "step_id": "step-build", "entry_id": 4,
		},
		"outcome": map[string]interface{}{"status": "transition_applied"},
		"transition": map[string]interface{}{
			"ledger_id": 9, "workflow_id": "wf", "step_id": "step-review",
		},
		"current": map[string]interface{}{
			"state": "REVIEW", "workflow_id": "wf", "step_id": "step-review", "entry_id": 9, "primary_session_id": "review-session",
		},
	}}
	s := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)

	result := callTool(t, s, "step_complete_kandev", map[string]interface{}{"summary": "done"})
	require.False(t, result.IsError)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	require.Equal(t, false, payload["accepted"])
	require.Equal(t, "already_signaled", payload["reason"])
	require.Equal(t, "signal-1", payload["signal_id"])
	require.Equal(t, "turn-1", payload["source"].(map[string]interface{})["turn_id"])
	require.Equal(t, "transition_applied", payload["outcome"].(map[string]interface{})["status"])
	require.EqualValues(t, 9, payload["transition"].(map[string]interface{})["ledger_id"])
	require.Equal(t, "step-review", payload["current"].(map[string]interface{})["step_id"])
}

func TestStepCompleteToolDescriptionExplainsAsyncOutcome(t *testing.T) {
	s := New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
	tools := s.mcpServer.ListTools()
	tool, ok := tools["step_complete_kandev"]
	require.True(t, ok)
	require.Contains(t, tool.Tool.Description, "outcome")
	require.Contains(t, tool.Tool.Description, "pending")
	require.Contains(t, tool.Tool.Description, "transition proof")
	require.Contains(t, tool.Tool.Description, "reviewer")
}
