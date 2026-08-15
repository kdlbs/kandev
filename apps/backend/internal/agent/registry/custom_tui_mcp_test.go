package registry

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewRegistry(log)
}

func registeredStrategy(t *testing.T, reg *Registry, slug string) mcpconfig.PassthroughMCPStrategy {
	t.Helper()
	ag, ok := reg.Get(slug)
	if !ok {
		t.Fatalf("agent %q not registered", slug)
	}
	pt, ok := ag.(agents.PassthroughAgent)
	if !ok {
		t.Fatalf("agent %q does not implement PassthroughAgent", slug)
	}
	return pt.PassthroughConfig().MCPStrategy
}

// TestRegisterCustomTUIAgent_StrategyKeyReachesAgent covers the contract the
// registration tests previously left untested: every case passed the zero-value
// key, so a regression that dropped the strategy on the way to the agent would
// not have failed anything here.
func TestRegisterCustomTUIAgent_StrategyKeyReachesAgent(t *testing.T) {
	reg := newTestRegistry(t)

	if err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug: "with-strategy", DisplayName: "With", Command: "cli",
		MCPStrategyKey: mcpconfig.StrategyKeyClaude,
	}); err != nil {
		t.Fatalf("RegisterCustomTUIAgent: %v", err)
	}
	if registeredStrategy(t, reg, "with-strategy") == nil {
		t.Error("MCPStrategy = nil, want the Claude strategy")
	}
}

func TestRegisterCustomTUIAgent_NoKeyLeavesStrategyNil(t *testing.T) {
	reg := newTestRegistry(t)

	if err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug: "no-strategy", DisplayName: "None", Command: "k9s",
	}); err != nil {
		t.Fatalf("RegisterCustomTUIAgent: %v", err)
	}
	if registeredStrategy(t, reg, "no-strategy") != nil {
		t.Error("MCPStrategy set, want nil")
	}
}

// TestRegisterCustomTUIAgent_UnknownKeyRejectedWithoutRegistering pins that a
// bad key fails loudly instead of registering an agent whose MCP is silently off.
func TestRegisterCustomTUIAgent_UnknownKeyRejectedWithoutRegistering(t *testing.T) {
	reg := newTestRegistry(t)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug: "bad-key", DisplayName: "Bad", Command: "cli", MCPStrategyKey: "cluade",
	})
	if !errors.Is(err, ErrUnknownMCPStrategy) {
		t.Fatalf("err = %v, want ErrUnknownMCPStrategy", err)
	}
	if _, ok := reg.Get("bad-key"); ok {
		t.Error("agent was registered despite the rejected strategy key")
	}
}

// TestReplaceCustomTUIAgent_SwapsStrategyWithoutRemovingEntry is the reason
// Replace is used instead of Unregister + Register: the pair leaves a window in
// which the ID resolves to nothing, and a session launching then fails with
// "agent type not found". The entry must be continuously resolvable.
func TestReplaceCustomTUIAgent_SwapsStrategyWithoutRemovingEntry(t *testing.T) {
	reg := newTestRegistry(t)
	spec := CustomTUIAgentSpec{Slug: "swap-me", DisplayName: "Swap", Command: "cli --flag"}
	if err := reg.RegisterCustomTUIAgent(spec); err != nil {
		t.Fatalf("RegisterCustomTUIAgent: %v", err)
	}

	spec.MCPStrategyKey = mcpconfig.StrategyKeyCodex
	if err := reg.ReplaceCustomTUIAgent(spec); err != nil {
		t.Fatalf("ReplaceCustomTUIAgent: %v", err)
	}

	if registeredStrategy(t, reg, "swap-me") == nil {
		t.Error("MCPStrategy = nil after replace")
	}
	// Replace must not duplicate or drop the entry, and the rest of the
	// definition has to survive the rebuild.
	ag, _ := reg.Get("swap-me")
	want := []string{"cli", "--flag"}
	got := ag.Runtime().Cmd.Args()
	if len(got) != len(want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv = %#v, want %#v", got, want)
		}
	}
}

// TestReplaceCustomTUIAgent_RejectsUnknownKeyLeavingEntryIntact pins that a bad
// key on an edit does not take the existing agent down with it.
func TestReplaceCustomTUIAgent_RejectsUnknownKeyLeavingEntryIntact(t *testing.T) {
	reg := newTestRegistry(t)
	spec := CustomTUIAgentSpec{
		Slug: "keep-me", DisplayName: "Keep", Command: "cli",
		MCPStrategyKey: mcpconfig.StrategyKeyClaude,
	}
	if err := reg.RegisterCustomTUIAgent(spec); err != nil {
		t.Fatalf("RegisterCustomTUIAgent: %v", err)
	}

	bad := spec
	bad.MCPStrategyKey = "nope"
	if err := reg.ReplaceCustomTUIAgent(bad); !errors.Is(err, ErrUnknownMCPStrategy) {
		t.Fatalf("err = %v, want ErrUnknownMCPStrategy", err)
	}
	if registeredStrategy(t, reg, "keep-me") == nil {
		t.Error("existing agent lost its strategy after a rejected replace")
	}
}
