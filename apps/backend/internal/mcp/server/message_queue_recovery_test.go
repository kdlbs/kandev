package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestQueueRecoveryCatalogIsTaskBound(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task")
	for _, name := range []string{"get_terminal_session_message_queue_kandev", "get_child_queue_inventory_kandev", "get_child_session_message_queue_kandev"} {
		tool, ok := s.mcpServer.ListTools()[name]
		require.True(t, ok, name)
		require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		require.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Tool.Annotations.DestructiveHint)
	}
}

func TestQueueRecoveryNeverLogsUntrustedArguments(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task")
	core, logs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	s.logger = log
	result := callTool(t, s, "get_child_queue_inventory_kandev", map[string]interface{}{"task_id": "child", "attachment_data": "private-secret"})
	require.True(t, result.IsError)
	for _, entry := range logs.All() {
		require.NotContains(t, entry.ContextMap(), "args")
		require.NotContains(t, entry.ContextMap(), "result")
	}
}

func TestQueueRecoveryOwnTaskForwardingAndRestrictedCatalog(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"page": map[string]interface{}{"snapshot_sha256": "hash"}}}
	s := newTaskModeServer(t, backend, "bound-task")
	forged := callTool(t, s, "get_terminal_session_message_queue_kandev", map[string]interface{}{"session_id": "predecessor", "task_id": "forged"})
	require.True(t, forged.IsError)
	require.Empty(t, backend.lastAction)
	result := callTool(t, s, "get_terminal_session_message_queue_kandev", map[string]interface{}{"session_id": "predecessor", "max_body_bytes": float64(1024)})
	require.False(t, result.IsError, "%+v", result.Content)
	require.Equal(t, ws.ActionMCPGetTerminalSessionMessageQueue, backend.lastAction)
	payload := backend.lastPayload.(map[string]interface{})
	require.Equal(t, "bound-task", payload["task_id"])
	require.Equal(t, "predecessor", payload["session_id"])
	require.Equal(t, 1024, payload["max_body_bytes"])
	for _, surface := range []profile.Surface{profile.SurfaceExternal, profile.SurfaceConfiguration, profile.SurfaceAutomation, profile.SurfaceCoordinator, profile.SurfaceManagedConversation} {
		server := NewWithProfile(&testBackend{}, "caller", "task", 10005, newTestLogger(t), "", false, profile.Context{Surface: surface})
		_, present := server.mcpServer.ListTools()["get_terminal_session_message_queue_kandev"]
		require.False(t, present, string(surface))
	}
}
