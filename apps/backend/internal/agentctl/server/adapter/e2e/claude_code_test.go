//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@anthropic-ai/claude-code

// claudeCodeCommand is the CLI command for Claude Code in stream-json mode.
// Derived from internal/agent/agents/claude_code.go.
const claudeCodeCommand = "npx -y @anthropic-ai/claude-code@2.1.50 -p --output-format=stream-json --input-format=stream-json --verbose"

func TestClaudeCode_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "claude-code",
		Command:       claudeCodeCommand,
		Protocol:      agent.ProtocolClaudeCode,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("claude-code completed in %s: %v", result.Duration, counts)
}

func TestClaudeCode_ToolUse(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "claude-code",
		Command:       claudeCodeCommand,
		Protocol:      agent.ProtocolClaudeCode,
		DefaultPrompt: "List the files in the current directory using ls",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertHasEventType(t, result.Events, adapter.EventTypeToolCall)

	counts := CountEventsByType(result.Events)
	t.Logf("claude-code tool use completed in %s: %v", result.Duration, counts)
}

func TestClaudeCode_SlashCost(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "claude-code",
		Command:       claudeCodeCommand,
		Protocol:      agent.ProtocolClaudeCode,
		DefaultPrompt: "/cost",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	// /cost should complete without errors and produce at least a message chunk.
	AssertNoErrors(t, result.Events)
	AssertHasEventType(t, result.Events, adapter.EventTypeComplete)

	counts := CountEventsByType(result.Events)
	t.Logf("claude-code /cost completed in %s: %v", result.Duration, counts)
}
