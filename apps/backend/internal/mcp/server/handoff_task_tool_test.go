package mcp

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers handoff_task_kandev's tool-registration/dispatch half of
// the MCP transport (registerHandoffTaskTool / handoffTaskHandler). The
// WS-handler half (handleHandoffTask) is covered separately in
// internal/mcp/handlers/handoff_task_handlers_test.go.

func newHandoffCapableServer(t *testing.T, backend BackendClient, sessionID, taskID string) *Server {
	t.Helper()
	profileContext := mcpprofile.New(mcpprofile.SurfaceOfficeTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityHandoffTask}, nil)
	return NewWithProfile(backend, sessionID, taskID, 10005, newTestLogger(t), "", false, profileContext)
}

func validHandoffToolArgs() map[string]interface{} {
	return map[string]interface{}{
		"target_workspace_id": "ws-target",
		"workflow_id":         "wf-target",
		"title":               "Deliver",
		"prompt":              "Do it",
		"agent_profile_id":    "agent-profile",
		"executor_profile_id": "executor-profile",
	}
}

// AC-5: the tool accepts exactly its documented arguments and rejects any
// call carrying an argument outside that set. task_id/session_id are not in
// that set — they are the trusted envelope the wrapper injects itself — so a
// client that supplies either must be refused, not silently overwritten.

func TestHandoffTaskHandler_RejectsClientSuppliedTaskID(t *testing.T) {
	backend := &testBackend{}
	s := newHandoffCapableServer(t, backend, "test-session", "task-current")

	args := validHandoffToolArgs()
	args["task_id"] = "attacker-task"

	result := callTool(t, s, "handoff_task_kandev", args)

	require.True(t, result.IsError, "a client-supplied task_id must be rejected, not silently overwritten")
	assert.Empty(t, backend.lastAction, "backend must not be called when an out-of-set argument is supplied")
}

func TestHandoffTaskHandler_RejectsClientSuppliedSessionID(t *testing.T) {
	backend := &testBackend{}
	s := newHandoffCapableServer(t, backend, "test-session", "task-current")

	args := validHandoffToolArgs()
	args["session_id"] = "attacker-session"

	result := callTool(t, s, "handoff_task_kandev", args)

	require.True(t, result.IsError, "a client-supplied session_id must be rejected, not silently overwritten")
	assert.Empty(t, backend.lastAction, "backend must not be called when an out-of-set argument is supplied")
}

func TestHandoffTaskHandler_SendsTrustedTaskAndSessionID(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"task_id": "delivered"}}
	s := newHandoffCapableServer(t, backend, "test-session", "task-current")

	result := callTool(t, s, "handoff_task_kandev", validHandoffToolArgs())

	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"])
	assert.Equal(t, "test-session", payload["session_id"])
}

func TestHandoffTaskHandler_RequiresBoundTaskAndSession(t *testing.T) {
	backend := &testBackend{}
	s := newHandoffCapableServer(t, backend, "", "")

	result := callTool(t, s, "handoff_task_kandev", validHandoffToolArgs())

	require.True(t, result.IsError, "unbound task/session must be rejected")
	assert.Empty(t, backend.lastAction, "backend must not be called without a bound session")
}
