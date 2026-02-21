//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// ampCommand is the CLI command for Amp in stream-json mode.
// Derived from internal/agent/agents/amp.go.
const ampCommand = "amp --execute --stream-json --stream-json-input"

func TestAmp_BasicPrompt(t *testing.T) {
	result := RunAgent(t, AgentSpec{
		Name:          "amp",
		Command:       ampCommand,
		Protocol:      agent.ProtocolAmp,
		DefaultPrompt: "What is 2 + 2? Reply with just the number.",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	counts := CountEventsByType(result.Events)
	t.Logf("amp completed in %s: %v", result.Duration, counts)
}
