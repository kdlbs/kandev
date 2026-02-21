//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/opencode-ai

// openCodeCommand is the CLI command for OpenCode in serve mode.
// Derived from internal/agent/agents/opencode.go.
const openCodeCommand = "npx -y opencode-ai@1.2.10 serve --hostname 127.0.0.1 --port 0"

func TestOpenCode_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "opencode",
		Command:       openCodeCommand,
		Protocol:      agent.ProtocolOpenCode,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("opencode completed in %s: %v", result.Duration, counts)
}
