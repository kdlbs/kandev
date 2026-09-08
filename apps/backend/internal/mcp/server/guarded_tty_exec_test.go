package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpsrv "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedTTYExecToolRequiresProfileAndLiveBridge(t *testing.T) {
	backend := NewChannelBackendClient(newTestLogger(t))
	t.Cleanup(backend.Close)
	profile := mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	)
	s := NewWithProfile(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, profile)

	assert.NotContains(t, getRegisteredToolNames(s), guardedTTYExecToolName)

	s.SetGuardedTTYAvailable(true)
	assert.Contains(t, getRegisteredToolNames(s), guardedTTYExecToolName)

	s.SetProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	assert.NotContains(t, getRegisteredToolNames(s), guardedTTYExecToolName)

	s.SetProfile(profile)
	assert.Contains(t, getRegisteredToolNames(s), guardedTTYExecToolName)

	s.SetGuardedTTYAvailable(false)
	assert.NotContains(t, getRegisteredToolNames(s), guardedTTYExecToolName)
}

func TestGuardedTTYExecHidesBackendErrorDetails(t *testing.T) {
	backend := &testBackend{err: errors.New("SECRET_PROVIDER_DETAIL")}
	profile := mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	)
	s := NewWithProfile(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, profile)
	s.SetGuardedTTYAvailable(true)

	result := callTool(t, s, guardedTTYExecToolName, map[string]interface{}{
		"argv": []interface{}{"pwd"},
	})

	require.True(t, result.IsError)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "SECRET_PROVIDER_DETAIL")
	assert.Contains(t, string(encoded), "Guarded TTY execution failed")
}

func TestGuardedTTYExecAvailabilityNotifiesOnceWithBoundedSchema(t *testing.T) {
	backend := NewChannelBackendClient(newTestLogger(t))
	t.Cleanup(backend.Close)
	profile := mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	)
	s := NewWithProfile(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, profile)
	session := &providerRefreshTestSession{
		id:            "guarded-tty-refresh",
		notifications: make(chan mcplib.JSONRPCNotification, 8),
	}
	require.NoError(t, s.mcpServer.RegisterSession(context.Background(), session))

	s.SetGuardedTTYAvailable(true)

	notification := <-session.notifications
	assert.Equal(t, mcplib.MethodNotificationToolsListChanged, notification.Method)
	select {
	case duplicate := <-session.notifications:
		t.Fatalf("availability update emitted duplicate notification %q", duplicate.Method)
	default:
	}

	tool, ok := s.mcpServer.ListTools()[guardedTTYExecToolName]
	require.True(t, ok)
	raw := tool.Tool.RawInputSchema
	require.NotEmpty(t, raw)
	assert.JSONEq(t, `{
		"type":"object",
		"properties":{
			"argv":{
				"type":"array",
				"description":"Command argv. The guarded execution derives cwd, sandbox, process identity, TTY mode, and limits.",
				"items":{"type":"string","minLength":1,"maxLength":4096},
				"minItems":1,
				"maxItems":64
			}
		},
		"required":["argv"],
		"additionalProperties":false
	}`, string(raw))
	forbidden := []string{"cwd", "env", "sandbox", "permission", "process_id", "tty", "stdin", "resize", "attach"}
	for _, field := range forbidden {
		assert.NotContains(t, string(raw), `"`+field+`"`)
	}
}

func TestGuardedTTYExecForwardsOnlyBoundSessionAndArgv(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"attestation_id": "attestation-1",
		"exit_code":      0,
	}}
	profile := mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	)
	s := NewWithProfile(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, profile)
	s.SetGuardedTTYAvailable(true)

	result := callTool(t, s, guardedTTYExecToolName, map[string]interface{}{
		"argv": []interface{}{"sh", "-lc", "test -t 0 && test -t 1"},
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPGuardedTTYExec, backend.lastAction)
	assert.Equal(t, map[string]interface{}{
		"task_id":    "task-1",
		"session_id": "session-1",
		"argv":       []interface{}{"sh", "-lc", "test -t 0 && test -t 1"},
	}, backend.lastPayload)
}

type cyclicGuardedTTYBackend struct{}

func (cyclicGuardedTTYBackend) RequestPayload(_ context.Context, _ string, _ interface{}, result interface{}) error {
	cycle := map[string]interface{}{}
	cycle["self"] = cycle
	*result.(*map[string]interface{}) = cycle
	return nil
}

func TestGuardedTTYExecReturnsToolErrorWhenReceiptCannotBeEncoded(t *testing.T) {
	profile := mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	)
	s := NewWithProfile(cyclicGuardedTTYBackend{}, "session-1", "task-1", 10005, newTestLogger(t), "", false, profile)
	s.SetGuardedTTYAvailable(true)

	result := callTool(t, s, guardedTTYExecToolName, map[string]interface{}{
		"argv": []interface{}{"pwd"},
	})

	require.True(t, result.IsError)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "failed to encode guarded TTY result")
}

var _ mcpsrv.ClientSession = (*providerRefreshTestSession)(nil)
