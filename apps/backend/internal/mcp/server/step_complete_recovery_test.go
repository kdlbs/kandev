package mcp

import (
	"errors"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStepCompleteToolDescription_ExplainsStaleTurnRecovery covers AC-003.4:
// the tool distinguishes a rejected stale turn from an accepted duplicate and
// does not turn a stale error into move authority.
func TestStepCompleteToolDescription_ExplainsStaleTurnRecovery(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	tool, ok := s.mcpServer.ListTools()["step_complete_kandev"]
	require.True(t, ok)

	description := tool.Tool.Description
	assert.Contains(t, description, "workflow step changed")
	assert.Contains(t, description, "stale")
	assert.Contains(t, description, "retries cannot recover")
	assert.Contains(t, description, "resume the session for the current step")
	assert.Contains(t, description, "already_signaled")
	assert.Contains(t, description, "Do not move the task solely to bypass a stale-turn error")
}

// TestStepCompleteTool_ForwardsStaleTurnRecoveryGuidance covers AC-003.4 at
// the MCP boundary: the backend diagnostic reaches the agent as tool-error
// text instead of being hidden in a transport-only field.
func TestStepCompleteTool_ForwardsStaleTurnRecoveryGuidance(t *testing.T) {
	const guidance = "workflow step changed before signal was recorded. This turn started in step step-review. The current step is step-work. No signal was recorded. Retrying in this turn cannot recover. End this turn and ask the user to resume this session for the current step."
	backend := &testBackend{err: errors.New(guidance)}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "step_complete_kandev", map[string]interface{}{
		"summary": "finished",
	})

	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	content, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.Equal(t, guidance, content.Text)
}
