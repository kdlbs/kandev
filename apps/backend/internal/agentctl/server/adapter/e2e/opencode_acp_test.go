//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

func TestOpenCodeACP_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "opencode-acp",
		Command:       acpCommand(t, agents.NewOpenCodeACP()),
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
