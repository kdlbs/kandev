package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStepCompleteToolDescription_DeliveryExtent covers REQ-004: the
// registered descriptions must state the actual delivered behavior (handoff
// is delivered once, to the immediately-next step, and not carried further;
// blockers is recorded on the transition record and never delivered to the
// next step's agent), state the byte ceiling, and must not claim a
// forwarding behavior the mechanism does not provide.
func TestStepCompleteToolDescription_DeliveryExtent(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	tool, ok := s.mcpServer.ListTools()["step_complete_kandev"]
	require.True(t, ok)

	var handoffDesc, blockersDesc string
	raw, ok := tool.Tool.InputSchema.Properties["handoff"].(map[string]interface{})
	require.True(t, ok, "expected handoff property schema")
	handoffDesc, _ = raw["description"].(string)
	raw, ok = tool.Tool.InputSchema.Properties["blockers"].(map[string]interface{})
	require.True(t, ok, "expected blockers property schema")
	blockersDesc, _ = raw["description"].(string)

	// AC-004.1: handoff's description states one-time, immediately-next-step
	// delivery and that it is not carried beyond that step.
	assert.Contains(t, handoffDesc, "once")
	assert.Contains(t, handoffDesc, "not carried beyond")

	// AC-004.2: blockers' description states it is recorded on the transition
	// record and NOT delivered to the next step's agent.
	assert.Contains(t, blockersDesc, "not delivered to the next step")

	// AC-004.3: both descriptions state the byte ceiling.
	assert.Contains(t, handoffDesc, "8,192")
	assert.Contains(t, blockersDesc, "8,192")

	// AC-004.4: the top-level description must not claim handoff/blockers are
	// forwarded to the next step (the mechanism does not deliver blockers at
	// all, and handoff only survives one hop).
	assert.NotContains(t, strings.ToLower(tool.Tool.Description), "forwarded to the next step")

	// AC-004.5: both arguments remain registered, optional, and string-typed.
	_, handoffRequired := requiredSet(tool.Tool.InputSchema.Required)["handoff"]
	_, blockersRequired := requiredSet(tool.Tool.InputSchema.Required)["blockers"]
	assert.False(t, handoffRequired)
	assert.False(t, blockersRequired)
}

func requiredSet(required []string) map[string]struct{} {
	set := make(map[string]struct{}, len(required))
	for _, name := range required {
		set[name] = struct{}{}
	}
	return set
}
