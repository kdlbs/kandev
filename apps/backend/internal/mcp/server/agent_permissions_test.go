package mcp

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPermissionToolsAreExternalOnly(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeTaskTitlePending, ModeConfig, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			tools := getRegisteredToolNames(New(&testBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, mode))
			assert.NotContains(t, tools, "list_pending_agent_permissions_kandev")
			assert.NotContains(t, tools, "resolve_agent_permission_kandev")
		})
	}

	tools := getRegisteredToolNames(NewExternal(&testBackend{}, newTestLogger(t), ""))
	assert.Contains(t, tools, "list_pending_agent_permissions_kandev")
	assert.Contains(t, tools, "resolve_agent_permission_kandev")
}

func TestAgentPermissionToolSchemasExposeOnlyExactInputs(t *testing.T) {
	s := NewExternal(&testBackend{}, newTestLogger(t), "")
	tests := []struct {
		name       string
		properties []string
		required   []interface{}
	}{
		{"list_pending_agent_permissions_kandev", []string{"task_id", "session_id"}, []interface{}{"task_id"}},
		{"resolve_agent_permission_kandev", []string{"task_id", "session_id", "request_id", "pending_id", "option_id"}, []interface{}{"task_id", "session_id", "request_id", "pending_id", "option_id"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, ok := s.mcpServer.ListTools()[tc.name]
			require.True(t, ok)
			raw, err := json.Marshal(tool.Tool.InputSchema)
			require.NoError(t, err)
			var schema struct {
				Properties map[string]any `json:"properties"`
				Required   []interface{}  `json:"required"`
			}
			require.NoError(t, json.Unmarshal(raw, &schema))
			assert.ElementsMatch(t, tc.properties, mapKeys(schema.Properties))
			assert.Equal(t, tc.required, schema.Required)
			assert.NotContains(t, schema.Properties, "command")
			assert.NotContains(t, schema.Properties, "cancelled")
			assert.NotContains(t, schema.Properties, "options")
		})
	}
}

func TestAgentPermissionToolsForwardExactPayloads(t *testing.T) {
	backend := &testBackend{response: map[string]any{"permissions": []any{}, "total": 0}}
	s := NewExternal(backend, newTestLogger(t), "")
	result := callTool(t, s, "list_pending_agent_permissions_kandev", map[string]any{"task_id": "task-1", "session_id": "session-1"})
	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListPendingAgentPermissions, backend.lastAction)
	assert.Equal(t, map[string]any{"task_id": "task-1", "session_id": "session-1"}, backend.lastPayload)

	backend.response = map[string]any{"status": "resolved"}
	args := map[string]any{"task_id": "task-1", "session_id": "session-1", "request_id": "request-1", "pending_id": "pending-1", "option_id": "allow-once"}
	result = callTool(t, s, "resolve_agent_permission_kandev", args)
	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPResolveAgentPermission, backend.lastAction)
	assert.Equal(t, args, backend.lastPayload)
}

func TestAgentPermissionToolsRejectUnknownArgumentsBeforeDispatch(t *testing.T) {
	backend := &testBackend{}
	s := NewExternal(backend, newTestLogger(t), "")
	result := callTool(t, s, "resolve_agent_permission_kandev", map[string]any{
		"task_id": "task-1", "session_id": "session-1", "request_id": "request-1",
		"pending_id": "pending-1", "option_id": "allow-once", "command": "rm -rf /",
	})
	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
