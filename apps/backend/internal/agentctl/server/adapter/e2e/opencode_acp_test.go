//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// openCodeACPCommand is the CLI command for OpenCode in ACP mode (JSON-RPC over stdin/stdout).
// Derived from internal/agent/agents/opencode_acp.go.
const openCodeACPCommand = "npx -y opencode-ai acp"

func TestOpenCodeACP_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "opencode-acp",
		Command:       openCodeACPCommand,
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("opencode-acp completed in %s: %v", result.Duration, counts)
}
