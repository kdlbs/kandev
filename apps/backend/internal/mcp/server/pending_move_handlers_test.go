package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCancelPendingMoveInvalidArgumentsReachBackendAudit(t *testing.T) {
	valid := map[string]interface{}{
		"pending_move_id":                   "11111111-1111-4111-8111-111111111111",
		"session_id":                        "22222222-2222-4222-8222-222222222222",
		"task_id":                           "33333333-3333-4333-8333-333333333333",
		"move_id":                           "44444444-4444-4444-8444-444444444444",
		"workflow_id":                       "55555555-5555-4555-8555-555555555555",
		"expected_current_workflow_step_id": "66666666-6666-4666-8666-666666666666",
		"expected_target_workflow_step_id":  "77777777-7777-4777-8777-777777777777",
	}
	cases := map[string]map[string]interface{}{
		"missing required field": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			delete(args, "task_id")
		}),
		"wrong field type": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			args["task_id"] = float64(42)
		}),
		"unknown field": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			args["force"] = true
		}),
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"audited": true}}
			server := NewWithProfile(backend, "session", "task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())

			result := callTool(t, server, "cancel_pending_move_kandev", arguments)

			assert.False(t, result.IsError)
			assert.Equal(t, ws.ActionMCPCancelPendingMove, backend.lastAction)
			assert.Equal(t, arguments, backend.lastPayload)
		})
	}
}

func TestCancelPendingMoveDoesNotLogArguments(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	backend := &testBackend{response: map[string]interface{}{"cancelled": true}}
	server := NewWithProfile(backend, "session", "task", 10005, log, "", false, mcpprofile.NewAutomation())

	callTool(t, server, "cancel_pending_move_kandev", clonePendingMoveArguments(map[string]interface{}{
		"pending_move_id":                   "11111111-1111-4111-8111-111111111111",
		"session_id":                        "22222222-2222-4222-8222-222222222222",
		"task_id":                           "33333333-3333-4333-8333-333333333333",
		"move_id":                           "44444444-4444-4444-8444-444444444444",
		"workflow_id":                       "55555555-5555-4555-8555-555555555555",
		"expected_current_workflow_step_id": "66666666-6666-4666-8666-666666666666",
		"expected_target_workflow_step_id":  "77777777-7777-4777-8777-777777777777",
	}, func(map[string]interface{}) {}))

	entries := observed.FilterMessage("MCP tool call").All()
	require.Len(t, entries, 1)
	_, loggedArguments := entries[0].ContextMap()["args"]
	require.False(t, loggedArguments)
}

func TestReadPendingMoveInvalidArgumentsReachBackendAudit(t *testing.T) {
	valid := map[string]interface{}{
		"task_id": "33333333-3333-4333-8333-333333333333",
	}
	cases := map[string]map[string]interface{}{
		"missing required field": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			delete(args, "task_id")
		}),
		"wrong field type": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			args["task_id"] = float64(42)
		}),
		"unknown field": clonePendingMoveArguments(valid, func(args map[string]interface{}) {
			args["workspace_id"] = "forged-workspace"
		}),
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"audited": true}}
			server := NewWithProfile(backend, "session", "task", 10005, newTestLogger(t), "", false, mcpprofile.NewAutomation())

			result := callTool(t, server, "read_pending_move_kandev", arguments)

			assert.False(t, result.IsError)
			assert.Equal(t, ws.ActionMCPReadPendingMove, backend.lastAction)
			assert.Equal(t, arguments, backend.lastPayload)
		})
	}
}

func TestReadPendingMoveDoesNotLogArguments(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	backend := &testBackend{response: map[string]interface{}{"found": false}}
	server := NewWithProfile(backend, "session", "task", 10005, log, "", false, mcpprofile.NewAutomation())

	callTool(t, server, "read_pending_move_kandev", map[string]interface{}{
		"task_id": "33333333-3333-4333-8333-333333333333",
	})

	entries := observed.FilterMessage("MCP tool call").All()
	require.Len(t, entries, 1)
	_, loggedArguments := entries[0].ContextMap()["args"]
	require.False(t, loggedArguments)
}

func clonePendingMoveArguments(
	source map[string]interface{},
	mutate func(map[string]interface{}),
) map[string]interface{} {
	clone := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		clone[key] = value
	}
	mutate(clone)
	return clone
}
