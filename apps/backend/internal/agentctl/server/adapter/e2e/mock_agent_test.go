//go:build e2e

package e2e

import (
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// TestMockAgent_BasicPrompt validates the harness works end-to-end using
// the mock agent. No API cost — can always run.
func TestMockAgent_BasicPrompt(t *testing.T) {
	binary := buildMockAgent(t)

	result := RunAgent(t, AgentSpec{
		Name:          "mock-agent",
		Command:       binary + " --model mock-fast",
		Protocol:      agent.ProtocolClaudeCode,
		DefaultPrompt: "hello",
		AutoApprove:   true,
	})
	defer DumpEventsOnFailure(t, result)

	AssertTurnCompleted(t, result)
	AssertSessionIDConsistent(t, result.Events)

	t.Logf("mock agent completed in %s with %d events", result.Duration, len(result.Events))
}
