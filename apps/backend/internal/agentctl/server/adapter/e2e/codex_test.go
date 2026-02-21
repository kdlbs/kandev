//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@openai/codex

// codexCommand is the CLI command for OpenAI Codex in app-server mode.
// Derived from internal/agent/agents/codex.go.
const codexCommand = "npx -y @openai/codex@0.104.0 app-server"

func TestCodex_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "codex",
		Command:       codexCommand,
		Protocol:      agent.ProtocolCodex,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("codex completed in %s: %v", result.Duration, counts)
}
