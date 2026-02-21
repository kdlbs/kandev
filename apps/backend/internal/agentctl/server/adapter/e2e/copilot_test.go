//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@github/copilot

// copilotCommand is the CLI command for GitHub Copilot.
// Derived from internal/agent/agents/copilot.go.
const copilotCommand = "npx -y @github/copilot@0.0.414 --server --log-level error"

func TestCopilot_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "copilot",
		Command:       copilotCommand,
		Protocol:      agent.ProtocolCopilot,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("copilot completed in %s: %v", result.Duration, counts)
}
