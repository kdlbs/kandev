package controller

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

func newCustomTUIController(t *testing.T, st *fakeStore) *Controller {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return &Controller{agentRegistry: registry.NewRegistry(log), repo: st, logger: log}
}

// TestCreateCustomTUIAgent_CommandArgsReachArgv is the test that distinguishes
// the fix from the workaround. Smuggling flags into the space-separated
// Command string cannot express an argument that itself contains a space —
// strings.Fields would split it. Passing it through CommandArgs must deliver
// it to argv as a single element.
func TestCreateCustomTUIAgent_CommandArgsReachArgv(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	_, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Spaced Args",
		Command:     "my-cli",
		CommandArgs: []string{"--system-prompt", "you are a helpful agent"},
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("spaced-args")
	if !ok {
		t.Fatal("agent not registered")
	}
	want := []string{"my-cli", "--system-prompt", "you are a helpful agent"}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("argv = %#v, want %#v", got, want)
	}
}

// TestCreateCustomTUIAgent_CommandArgsPersisted pins that the args survive a
// restart: they must be written to the stored TUI config, not just handed to
// the in-memory registry.
func TestCreateCustomTUIAgent_CommandArgsPersisted(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	args := []string{"--flag", "value with space"}
	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Persisted Args",
		Command:     "my-cli",
		CommandArgs: args,
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	stored, ok := st.byName["persisted-args"]
	if !ok {
		t.Fatal("agent not persisted")
	}
	if stored.TUIConfig == nil {
		t.Fatal("stored agent has no TUI config")
	}
	if got := stored.TUIConfig.CommandArgs; !slices.Equal(got, args) {
		t.Errorf("stored CommandArgs = %#v, want %#v", got, args)
	}
}

// TestCreateCustomTUIAgent_NoCommandArgs keeps the existing behaviour intact
// when the caller omits the field.
func TestCreateCustomTUIAgent_NoCommandArgs(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Plain",
		Command:     "my-cli --verbose",
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("plain")
	if !ok {
		t.Fatal("agent not registered")
	}
	want := []string{"my-cli", "--verbose"}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("argv = %#v, want %#v", got, want)
	}
	if got := st.byName["plain"].TUIConfig.CommandArgs; len(got) != 0 {
		t.Errorf("stored CommandArgs = %#v, want empty", got)
	}
}

// --- MCP strategy ------------------------------------------------------------

// TestCreateCustomTUIAgent_MCPStrategyEnablesInjection is the test that
// distinguishes the fix from the bug in issue #2474. Before the fix a custom
// TUI agent was registered with no MCP strategy and supports_mcp=0, so
// applyPassthroughMCP was a no-op and the mcpconfig service refused the profile
// — kandev's own session tools were unreachable with no way to turn them on.
func TestCreateCustomTUIAgent_MCPStrategyEnablesInjection(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Fuel Claude",
		Command:     "fuelclaude",
		MCPStrategy: mcpconfig.StrategyKeyClaude,
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("fuel-claude")
	if !ok {
		t.Fatal("agent not registered")
	}
	pt, ok := ag.(agents.PassthroughAgent)
	if !ok {
		t.Fatal("agent does not implement PassthroughAgent")
	}
	// The strategy is what applyPassthroughMCP keys off; nil means the CLI is
	// never told the per-session MCP server exists.
	if pt.PassthroughConfig().MCPStrategy == nil {
		t.Error("PassthroughConfig.MCPStrategy = nil, want the Claude strategy")
	}
	// supports_mcp gates the profile MCP editor and the mcpconfig service.
	if stored := st.byName["fuel-claude"]; !stored.SupportsMCP {
		t.Error("stored SupportsMCP = false, want true")
	}
	if got := st.byName["fuel-claude"].TUIConfig.MCPStrategy; got != mcpconfig.StrategyKeyClaude {
		t.Errorf("stored MCPStrategy = %q, want %q", got, mcpconfig.StrategyKeyClaude)
	}
}

// TestCreateCustomTUIAgent_NoStrategyLeavesMCPOff pins that the default is
// unchanged: a plain TUI tool that never speaks MCP must not advertise support.
func TestCreateCustomTUIAgent_NoStrategyLeavesMCPOff(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Plain Tool",
		Command:     "k9s",
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, _ := c.agentRegistry.Get("plain-tool")
	if pt, ok := ag.(agents.PassthroughAgent); ok && pt.PassthroughConfig().MCPStrategy != nil {
		t.Error("MCPStrategy set, want nil for an agent created without one")
	}
	if st.byName["plain-tool"].SupportsMCP {
		t.Error("stored SupportsMCP = true, want false")
	}
}

// TestCreateCustomTUIAgent_RejectsUnknownStrategy pins that a bad key fails at
// creation. Accepting it would persist an agent whose MCP is silently off
// forever, with no error surfaced anywhere.
func TestCreateCustomTUIAgent_RejectsUnknownStrategy(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	_, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Typo Agent",
		Command:     "some-cli",
		MCPStrategy: "cluade",
	})
	if !errors.Is(err, ErrUnknownMCPStrategy) {
		t.Fatalf("err = %v, want ErrUnknownMCPStrategy", err)
	}
	if _, registered := c.agentRegistry.Get("typo-agent"); registered {
		t.Error("agent was registered despite the rejected strategy")
	}
	if _, stored := st.byName["typo-agent"]; stored {
		t.Error("agent was persisted despite the rejected strategy")
	}
}

// TestSetCustomTUIAgentMCPStrategy_OptsInExistingAgent covers the upgrade path:
// every custom TUI agent created before this feature has an empty strategy, and
// there is no route to edit a tui_config. Without this they could only get MCP
// by being deleted and rebuilt, losing their profiles.
func TestSetCustomTUIAgentMCPStrategy_OptsInExistingAgent(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	created, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Legacy Agent",
		Command:     "legacy-cli --flag",
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	got, err := c.SetCustomTUIAgentMCPStrategy(context.Background(), created.ID, mcpconfig.StrategyKeyCodex)
	if err != nil {
		t.Fatalf("SetCustomTUIAgentMCPStrategy: %v", err)
	}
	if !got.SupportsMCP {
		t.Error("returned SupportsMCP = false, want true")
	}

	ag, ok := c.agentRegistry.Get("legacy-agent")
	if !ok {
		t.Fatal("agent missing from registry after re-register")
	}
	pt, ok := ag.(agents.PassthroughAgent)
	if !ok {
		t.Fatal("agent does not implement PassthroughAgent")
	}
	if pt.PassthroughConfig().MCPStrategy == nil {
		t.Error("MCPStrategy = nil after opting in")
	}
	// The rest of the definition must survive the rebuild — the re-register
	// reconstructs the agent from stored config, so a dropped field here would
	// silently change the command the user launches.
	want := []string{"legacy-cli", "--flag"}
	if argv := ag.Runtime().Cmd.Args(); !slices.Equal(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
	if stored := st.byName["legacy-agent"]; stored.TUIConfig.MCPStrategy != mcpconfig.StrategyKeyCodex {
		t.Errorf("stored strategy = %q, want %q", stored.TUIConfig.MCPStrategy, mcpconfig.StrategyKeyCodex)
	}
}

// TestSetCustomTUIAgentMCPStrategy_TurnsOff pins the reverse direction: clearing
// the strategy must also clear supports_mcp, or the profile MCP editor would
// stay visible for an agent that can no longer receive any servers.
func TestSetCustomTUIAgentMCPStrategy_TurnsOff(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	created, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Toggle Agent",
		Command:     "toggle-cli",
		MCPStrategy: mcpconfig.StrategyKeyClaude,
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	got, err := c.SetCustomTUIAgentMCPStrategy(context.Background(), created.ID, mcpconfig.StrategyKeyNone)
	if err != nil {
		t.Fatalf("SetCustomTUIAgentMCPStrategy: %v", err)
	}
	if got.SupportsMCP {
		t.Error("returned SupportsMCP = true, want false after clearing the strategy")
	}
	ag, _ := c.agentRegistry.Get("toggle-agent")
	if pt, ok := ag.(agents.PassthroughAgent); ok && pt.PassthroughConfig().MCPStrategy != nil {
		t.Error("MCPStrategy still set after clearing")
	}
}

func TestSetCustomTUIAgentMCPStrategy_RejectsNonTUIAgent(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	builtin := &models.Agent{Name: "claude-acp"}
	if err := st.CreateAgent(context.Background(), builtin); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	_, err := c.SetCustomTUIAgentMCPStrategy(context.Background(), builtin.ID, mcpconfig.StrategyKeyClaude)
	if !errors.Is(err, ErrNotCustomTUIAgent) {
		t.Fatalf("err = %v, want ErrNotCustomTUIAgent", err)
	}
}

func TestSetCustomTUIAgentMCPStrategy_RejectsUnknownStrategy(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	created, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Guard Agent",
		Command:     "guard-cli",
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	if _, err := c.SetCustomTUIAgentMCPStrategy(context.Background(), created.ID, "nope"); !errors.Is(err, ErrUnknownMCPStrategy) {
		t.Fatalf("err = %v, want ErrUnknownMCPStrategy", err)
	}
	if stored := st.byName["guard-agent"]; stored.TUIConfig.MCPStrategy != mcpconfig.StrategyKeyNone {
		t.Errorf("stored strategy = %q, want unchanged", stored.TUIConfig.MCPStrategy)
	}
}
