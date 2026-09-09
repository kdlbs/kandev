package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.5
// TestGetTaskPlan_EmptyObjectPayloadStillRendersNoPlan guards the transport
// change against the deliberately unchanged "no plan" convention: a `{}`
// backend response must keep decoding successfully so this rendering is
// unaffected by the new empty-payload rule.
func TestGetTaskPlan_EmptyObjectPayloadStillRendersNoPlan(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{})

	assert.False(t, result.IsError)
	assert.Equal(t, "No plan exists for this task yet.", resultText(t, result))
}
