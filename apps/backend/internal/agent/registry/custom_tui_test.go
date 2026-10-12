package registry

import (
	"errors"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/pkg/agent"
)

func TestRegisterCustomTUIAgent_Success(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "my-agent", DisplayName: "My Agent", Command: "my-agent --verbose", Description: "A test agent", Model: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("my-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if ag.ID() != "my-agent" {
		t.Errorf("expected ID %q, got %q", "my-agent", ag.ID())
	}
	if ag.DisplayName() != "My Agent" {
		t.Errorf("expected display name %q, got %q", "My Agent", ag.DisplayName())
	}
	if ag.Description() != "A test agent" {
		t.Errorf("expected description %q, got %q", "A test agent", ag.Description())
	}

	rt := ag.Runtime()
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	cmd := rt.Cmd.Args()
	if len(cmd) < 2 || cmd[0] != "my-agent" || cmd[1] != "--verbose" {
		t.Errorf("expected command [my-agent --verbose], got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_ModelTemplate(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "tmpl-agent", DisplayName: "Template", Command: "my-cli --model {{model}}", Description: "", Model: "best"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("tmpl-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	cmd := ag.Runtime().Cmd.Args()
	found := false
	for _, arg := range cmd {
		if arg == "best" {
			found = true
		}
		if arg == "{{model}}" {
			t.Error("{{model}} should have been replaced")
		}
	}
	if !found {
		t.Errorf("expected 'best' in command args, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_ModelTemplateNotReplacedWhenEmpty(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "no-model", DisplayName: "No Model", Command: "cli --model {{model}}", Description: "", Model: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, _ := reg.Get("no-model")
	cmd := ag.Runtime().Cmd.Args()
	found := false
	for _, arg := range cmd {
		if arg == "{{model}}" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected {{model}} to remain when model is empty, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_CommandArgs(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "extra-args", DisplayName: "Extra", Command: "my-cli", Description: "", Model: "", CommandArgs: []string{"--extra", "--flag"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, _ := reg.Get("extra-args")
	cmd := ag.Runtime().Cmd.Args()
	if len(cmd) < 3 || cmd[len(cmd)-2] != "--extra" || cmd[len(cmd)-1] != "--flag" {
		t.Errorf("expected command args to include --extra --flag, got %v", cmd)
	}
}

func TestRegisterCustomTUIAgent_DisableBracketedPaste(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug:                  "raw-tui",
		DisplayName:           "Raw TUI",
		Command:               "raw-tui",
		DisableBracketedPaste: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("raw-tui")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	pt, ok := ag.(agents.PassthroughAgent)
	if !ok {
		t.Fatal("custom terminal agent does not implement PassthroughAgent")
	}
	if !pt.PassthroughConfig().DisableBracketedPaste {
		t.Error("DisableBracketedPaste = false, want true")
	}
}

func TestRegisterCustomTUIAgent_EmptyCommand(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "empty-cmd", DisplayName: "Empty", Command: "", Description: "", Model: ""})
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRegisterCustomTUIAgent_DuplicateID(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	_ = reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "dup-agent", DisplayName: "First", Command: "first-cli", Description: "", Model: ""})
	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{Slug: "dup-agent", DisplayName: "Second", Command: "second-cli", Description: "", Model: ""})
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

// A spec that names the ACP protocol must produce an agent kandev drives over
// ACP, not a terminal passthrough one. The protocol is part of the stored
// definition, so this is what a restart replays.
func TestRegisterCustomTUIAgent_ACPProtocol(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug:        "my-agent",
		DisplayName: "My Agent",
		Command:     "my-agent --acp",
		Protocol:    CustomAgentProtocolACP,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("my-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if agents.IsPassthroughOnly(ag) {
		t.Error("ACP custom agent classified as passthrough-only")
	}
	if ag.Runtime().Protocol != agent.ProtocolACP {
		t.Errorf("Runtime().Protocol = %q, want %q", ag.Runtime().Protocol, agent.ProtocolACP)
	}
	want := []string{"my-agent", "--acp"}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("Runtime().Cmd = %#v, want %#v", got, want)
	}
}

// The empty protocol is what every definition stored before the field existed
// decodes to, so it has to keep meaning terminal passthrough.
func TestRegisterCustomTUIAgent_DefaultProtocolStaysTerminal(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	if err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug:        "legacy-agent",
		DisplayName: "Legacy Agent",
		Command:     "legacy-agent",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ag, ok := reg.Get("legacy-agent")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if !agents.IsPassthroughOnly(ag) {
		t.Error("a spec with no protocol is no longer terminal passthrough")
	}
}

// An unknown protocol is a typo from an older client or a value removed by an
// upgrade. Registering it as a terminal agent would silently ignore the user's
// choice, so it has to fail loudly — same stance as an unknown MCP strategy.
func TestRegisterCustomTUIAgent_UnknownProtocol(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug:        "bad-agent",
		DisplayName: "Bad Agent",
		Command:     "bad-agent",
		Protocol:    "websocket",
	})
	if !errors.Is(err, ErrUnknownCustomAgentProtocol) {
		t.Fatalf("error = %v, want ErrUnknownCustomAgentProtocol", err)
	}
	if reg.Exists("bad-agent") {
		t.Error("agent registered despite an unknown protocol")
	}
}

// The ACP path must not silently drop the MCP strategy field: an ACP agent
// gets kandev's tools through session/new, so a passthrough strategy is a
// contradiction rather than a no-op.
func TestRegisterCustomTUIAgent_ACPRejectsMCPStrategy(t *testing.T) {
	log := newTestLogger()
	reg := NewRegistry(log)

	err := reg.RegisterCustomTUIAgent(CustomTUIAgentSpec{
		Slug:           "strategy-acp",
		DisplayName:    "Strategy ACP",
		Command:        "strategy-acp --acp",
		Protocol:       CustomAgentProtocolACP,
		MCPStrategyKey: mcpconfig.StrategyKeyClaude,
	})
	if !errors.Is(err, ErrMCPStrategyNotApplicable) {
		t.Fatalf("error = %v, want ErrMCPStrategyNotApplicable", err)
	}
	if reg.Exists("strategy-acp") {
		t.Error("agent registered despite an inapplicable MCP strategy")
	}
}
