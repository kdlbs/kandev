package agents

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

func newTestCustomACPAgent() *CustomACPAgent {
	return NewCustomACPAgent(CustomACPAgentConfig{
		AgentID:     "my-agent",
		AgentName:   "my-agent",
		Display:     "My Agent",
		Command:     "my-agent",
		CommandArgs: []string{"--acp"},
	})
}

// A custom agent registered as ACP must not be classified as passthrough-only:
// that predicate decides whether the agent can only run as a raw CLI under a
// PTY, and it seeds the default profile as terminal passthrough.
func TestCustomACPAgentRunsStructured(t *testing.T) {
	a := newTestCustomACPAgent()

	if IsPassthroughOnly(a) {
		t.Error("IsPassthroughOnly = true for a custom ACP agent")
	}
	if !SupportsInteractiveMCPTools(a) {
		t.Error("SupportsInteractiveMCPTools = false for a custom ACP agent")
	}
	if _, ok := any(a).(PassthroughAgent); ok {
		t.Error("custom ACP agent advertises passthrough; its command is an ACP server, not a TUI")
	}
}

func TestCustomACPAgentRuntimeSpeaksACP(t *testing.T) {
	a := newTestCustomACPAgent()

	rt := a.Runtime()
	if rt.Protocol != agent.ProtocolACP {
		t.Errorf("Runtime().Protocol = %q, want %q", rt.Protocol, agent.ProtocolACP)
	}
	want := []string{"my-agent", "--acp"}
	if got := rt.Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("Runtime().Cmd = %#v, want %#v", got, want)
	}
	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Errorf("BuildCommand = %#v, want %#v", got, want)
	}
}

// Models and modes come from the host-utility probe, which only runs for
// agents that advertise inference. Without it the profile has no model to
// select and session start fails the no-silent-model-fallback policy.
func TestCustomACPAgentIsProbedForCapabilities(t *testing.T) {
	a := newTestCustomACPAgent()

	inference, ok := any(a).(InferenceAgent)
	if !ok {
		t.Fatal("custom ACP agent does not implement InferenceAgent")
	}
	cfg := inference.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		t.Fatal("InferenceConfig is not supported")
	}
	want := []string{"my-agent", "--acp"}
	if got := cfg.Command.Args(); !slices.Equal(got, want) {
		t.Errorf("InferenceConfig().Command = %#v, want %#v", got, want)
	}
}

// Resolved MCP servers travel in ACP session/new, so a custom ACP agent needs
// no per-CLI config-file strategy to get kandev's tools.
func TestCustomACPAgentSupportsMCP(t *testing.T) {
	a := newTestCustomACPAgent()

	result, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if !result.SupportsMCP {
		t.Error("SupportsMCP = false for a custom ACP agent")
	}
}

// The host-utility probe spawns from an allow-list of compiled-in literals. A
// command the operator typed can never be on it, so the agent has to carry the
// marker that lets the probe accept it.
func TestCustomACPAgentMarksItsCommandOperatorDefined(t *testing.T) {
	cfg := newTestCustomACPAgent().InferenceConfig()

	if !cfg.OperatorDefined {
		t.Error("OperatorDefined = false; the probe would refuse this command")
	}
}

// A built-in agent must not claim the marker: its command is compiled in and
// belongs on the allow-list, where a drift between the two is a real bug.
func TestBuiltinACPAgentDoesNotClaimOperatorDefined(t *testing.T) {
	cfg := NewGooseACP().InferenceConfig()

	if cfg.OperatorDefined {
		t.Error("OperatorDefined = true for a built-in agent")
	}
}
