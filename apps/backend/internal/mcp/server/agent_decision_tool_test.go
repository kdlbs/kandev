package mcp

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers the record_step_decision_kandev tool-registration/dispatch
// half of the MCP transport (registerRecordStepDecisionTool /
// recordStepDecisionHandler). Review round 1 flagged this file at zero
// coverage. The WS-handler half (handleRecordStepDecision,
// mapRecordStepDecisionError) is covered separately in
// internal/mcp/handlers/agent_decision_handlers_test.go; these tests instead
// exercise what's unique to this transport boundary: the bound-session guard,
// trimming/validation before dispatch, the exact payload shape sent to
// s.backend.RequestPayload, and surfacing a backend error as a tool error.

func newOfficeModeServer(t *testing.T, backend BackendClient, sessionID, taskID string) *Server {
	t.Helper()
	return New(backend, sessionID, taskID, 10005, newTestLogger(t), "", false, ModeOffice)
}

func TestRecordStepDecision_RequiresBoundTaskAndSession(t *testing.T) {
	backend := &testBackend{}
	s := newOfficeModeServer(t, backend, "", "")

	result := callTool(t, s, "record_step_decision_kandev", map[string]interface{}{
		"decision": "approved",
		"reason":   "looks good",
	})

	require.True(t, result.IsError, "unbound task/session must be rejected")
	assert.Empty(t, backend.lastAction, "backend must not be called without a bound session")
}

func TestRecordStepDecision_RejectsEmptyReason(t *testing.T) {
	backend := &testBackend{}
	s := newOfficeModeServer(t, backend, "test-session", "task-current")

	result := callTool(t, s, "record_step_decision_kandev", map[string]interface{}{
		"decision": "approved",
		"reason":   "   ",
	})

	require.True(t, result.IsError, "whitespace-only reason must be rejected")
	assert.Empty(t, backend.lastAction, "backend must not be called with an empty reason")
}

func TestRecordStepDecision_SendsExpectedPayload(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"decision": "approved",
			"role":     "reviewer",
			"step_id":  "step-1",
		},
	}
	s := newOfficeModeServer(t, backend, "test-session", "task-current")

	result := callTool(t, s, "record_step_decision_kandev", map[string]interface{}{
		"decision": "approved",
		"reason":   "  looks good  ",
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPRecordStepDecision, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"])
	assert.Equal(t, "test-session", payload["session_id"])
	assert.Equal(t, "approved", payload["decision"])
	assert.Equal(t, "looks good", payload["reason"], "reason should be trimmed before dispatch")
}

func TestRecordStepDecision_SurfacesBackendErrorAsToolError(t *testing.T) {
	backend := &testBackend{err: assert.AnError}
	s := newOfficeModeServer(t, backend, "test-session", "task-current")

	result := callTool(t, s, "record_step_decision_kandev", map[string]interface{}{
		"decision": "rejected",
		"reason":   "needs work",
	})

	require.True(t, result.IsError, "a backend error must surface as a tool error, not panic or succeed")
}
