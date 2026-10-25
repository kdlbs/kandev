//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@augmentcode/auggie

func TestAuggie_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "auggie",
		Command:       acpCommand(t, agents.NewAuggie()),
		Protocol:      agent.ProtocolACP,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("auggie completed in %s: %v", result.Duration, counts)
}
