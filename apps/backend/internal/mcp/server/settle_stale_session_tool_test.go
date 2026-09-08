package mcp

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettleStaleSessionToolInjectsTrustedCallerIdentity(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"status": "settled"}}
	s := newTaskModeServer(t, backend, "task-current")
	_, ok := s.mcpServer.ListTools()["settle_stale_session_kandev"]
	require.True(t, ok)

	result := callTool(t, s, "settle_stale_session_kandev", map[string]interface{}{
		"session_id": "session-target", "turn_id": "turn-target",
	})
	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPSettleStaleSession, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["sender_task_id"])
	assert.Equal(t, "test-session", payload["sender_session_id"])
	assert.Equal(t, "session-target", payload["session_id"])
	assert.Equal(t, "turn-target", payload["turn_id"])
}

func TestSettleStaleSessionToolIsTaskModeOnly(t *testing.T) {
	for _, mode := range []string{ModeOffice, ModeConfig} {
		s := New(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, mode)
		assert.NotContains(t, s.mcpServer.ListTools(), "settle_stale_session_kandev")
	}
}
