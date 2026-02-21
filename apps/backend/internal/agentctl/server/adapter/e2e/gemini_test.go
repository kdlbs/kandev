//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@google/gemini-cli

// geminiCommand is the CLI command for Google Gemini in ACP mode.
// Derived from internal/agent/agents/gemini.go.
const geminiCommand = "npx -y @google/gemini-cli --experimental-acp"

func TestGemini_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "gemini",
		Command:       geminiCommand,
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("gemini completed in %s: %v", result.Duration, counts)
}
