//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// https://www.npmjs.com/package/@augmentcode/auggie

// auggieCommand is the CLI command for Auggie in ACP mode.
// Derived from internal/agent/agents/auggie.go.
const auggieCommand = "npx -y @augmentcode/auggie@0.16.2 --acp"

func TestAuggie_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "auggie",
		Command:       auggieCommand,
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
