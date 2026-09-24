package controller

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
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

func newCustomTUIControllerWithDiscovery(t *testing.T, st *fakeStore) *Controller {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	agentRegistry := registry.NewRegistry(log)
	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		t.Fatalf("load discovery registry: %v", err)
	}
	return &Controller{
		agentRegistry: agentRegistry,
		discovery:     discoveryRegistry,
		repo:          st,
		logger:        log,
	}
}

func discoveryLists(t *testing.T, c *Controller, name string) bool {
	t.Helper()
	resp, err := c.ListDiscovery(context.Background())
	if err != nil {
		t.Fatalf("ListDiscovery: %v", err)
	}
	return slices.ContainsFunc(resp.Agents, func(a dto.AgentDiscoveryDTO) bool {
		return a.Name == name
	})
}

// The Installed Agents list is rendered from the discovery sweep, so a custom
// agent has to enter it on creation and leave it on deletion without a restart.
// Both directions were broken while discovery kept its own copy of the agent
// list: a deleted agent stayed listed (Rescan re-detected its binary from the
// stale list) and a new one stayed missing.
func TestCustomTUIAgentEntersAndLeavesDiscovery(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIControllerWithDiscovery(t, st)
	ctx := context.Background()

	if discoveryLists(t, c, "ghost-cli") {
		t.Fatal("ghost-cli reported by discovery before it was created")
	}

	created, err := c.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Ghost CLI",
		Command:     "ghost-cli",
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}
	if !discoveryLists(t, c, "ghost-cli") {
		t.Error("ghost-cli missing from discovery after it was created")
	}

	if err := c.DeleteAgent(ctx, created.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if discoveryLists(t, c, "ghost-cli") {
		t.Error("ghost-cli still reported by discovery after it was deleted")
	}
}

// Discovery writes SupportsMCP back over the agent row on every sweep, so a
// strategy change that discovery cannot see is reverted by the next sweep.
func TestCustomTUIAgentMCPStrategyChangeReachesDiscovery(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIControllerWithDiscovery(t, st)
	ctx := context.Background()

	created, err := c.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Strategy CLI",
		Command:     "strategy-cli",
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}
	if discoverySupportsMCP(t, c, "strategy-cli") {
		t.Fatal("SupportsMCP = true before a strategy was selected")
	}

	if _, err := c.SetCustomTUIAgentMCPStrategy(ctx, created.ID, mcpconfig.StrategyKeyClaude); err != nil {
		t.Fatalf("SetCustomTUIAgentMCPStrategy: %v", err)
	}
	if !discoverySupportsMCP(t, c, "strategy-cli") {
		t.Error("SupportsMCP = false after selecting an MCP strategy")
	}
}

func discoverySupportsMCP(t *testing.T, c *Controller, name string) bool {
	t.Helper()
	resp, err := c.ListDiscovery(context.Background())
	if err != nil {
		t.Fatalf("ListDiscovery: %v", err)
	}
	for _, a := range resp.Agents {
		if a.Name == name {
			return a.SupportsMCP
		}
	}
	t.Fatalf("%s missing from discovery", name)
	return false
}

// A custom agent created as ACP must be seeded as a structured profile. A
// passthrough profile launches the command under a PTY, which is what the
// terminal protocol is for — with an ACP command that shows JSON-RPC frames.
func TestCreateCustomACPAgent_SeedsStructuredProfile(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	created, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "My Agent",
		Command:     "my-agent --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("my-agent")
	if !ok {
		t.Fatal("agent not registered")
	}
	if agents.IsPassthroughOnly(ag) {
		t.Error("ACP custom agent registered as passthrough-only")
	}

	stored, ok := st.byName["my-agent"]
	if !ok || stored.TUIConfig == nil {
		t.Fatal("agent not persisted with a TUI config")
	}
	if stored.TUIConfig.Protocol != string(registry.CustomAgentProtocolACP) {
		t.Errorf("stored protocol = %q, want %q", stored.TUIConfig.Protocol, registry.CustomAgentProtocolACP)
	}
	if !stored.SupportsMCP {
		t.Error("stored SupportsMCP = false; ACP agents receive servers in session/new")
	}

	profiles := st.profiles[created.ID]
	if len(profiles) != 1 {
		t.Fatalf("seeded %d profiles, want 1", len(profiles))
	}
	if profiles[0].CLIPassthrough {
		t.Error("seeded profile is CLI passthrough")
	}
	if profiles[0].Model == "passthrough" {
		t.Error(`seeded profile model is "passthrough"; the capability probe fills it`)
	}
}

// The terminal protocol is the default, and its seeding must not drift.
func TestCreateCustomTUIAgent_DefaultsToTerminalProfile(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	created, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Terminal CLI",
		Command:     "terminal-cli",
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	stored, ok := st.byName["terminal-cli"]
	if !ok || stored.TUIConfig == nil {
		t.Fatal("agent not persisted with a TUI config")
	}
	if stored.TUIConfig.Protocol != "" {
		t.Errorf("stored protocol = %q, want empty", stored.TUIConfig.Protocol)
	}

	profiles := st.profiles[created.ID]
	if len(profiles) != 1 {
		t.Fatalf("seeded %d profiles, want 1", len(profiles))
	}
	if !profiles[0].CLIPassthrough {
		t.Error("seeded profile is not CLI passthrough")
	}
}

func TestCreateCustomTUIAgent_RejectsUnknownProtocol(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	_, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Bad Protocol",
		Command:     "bad-protocol",
		Protocol:    "websocket",
	})
	if !errors.Is(err, ErrUnknownCustomAgentProtocol) {
		t.Fatalf("error = %v, want ErrUnknownCustomAgentProtocol", err)
	}
	if c.agentRegistry.Exists("bad-protocol") {
		t.Error("agent registered despite an unknown protocol")
	}
}

func TestCreateCustomACPAgent_RejectsMCPStrategy(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	_, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "ACP With Strategy",
		Command:     "acp-with-strategy --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
		MCPStrategy: mcpconfig.StrategyKeyClaude,
	})
	if !errors.Is(err, ErrMCPStrategyNotApplicable) {
		t.Fatalf("error = %v, want ErrMCPStrategyNotApplicable", err)
	}
	if c.agentRegistry.Exists("acp-with-strategy") {
		t.Error("agent registered despite an inapplicable MCP strategy")
	}
}

// A stored ACP definition has to replay as an ACP agent after a restart;
// creation, strategy changes, and the boot replay all build their spec here.
func TestCustomAgentSpecFromStored_CarriesProtocol(t *testing.T) {
	spec := CustomAgentSpecFromStored("my-agent", &models.TUIConfigJSON{
		Command:     "my-agent --acp",
		DisplayName: "My Agent",
		Protocol:    string(registry.CustomAgentProtocolACP),
	})

	if spec.Protocol != registry.CustomAgentProtocolACP {
		t.Errorf("spec.Protocol = %q, want %q", spec.Protocol, registry.CustomAgentProtocolACP)
	}
	if spec.Slug != "my-agent" {
		t.Errorf("spec.Slug = %q, want %q", spec.Slug, "my-agent")
	}
}

type probeRecordingHostUtility struct {
	refreshed chan string
	caps      hostutility.AgentCapabilities
}

func (h *probeRecordingHostUtility) Get(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{}, false
}

func (h *probeRecordingHostUtility) Refresh(
	_ context.Context,
	agentType string,
) (hostutility.AgentCapabilities, error) {
	select {
	case h.refreshed <- agentType:
	default:
	}
	return h.caps, nil
}

func (h *probeRecordingHostUtility) ResolveModelConfig(
	context.Context,
	string,
	hostutility.ModelConfigResolutionRequest,
) (hostutility.ModelConfigResolution, error) {
	return hostutility.ModelConfigResolution{}, nil
}

// An ACP agent's models and modes come from the capability probe, and the
// probe only runs at boot. Without one kicked at creation, the profile editor
// reports "not_configured" until the user finds the manual refresh.
func TestCreateCustomACPAgent_ProbesCapabilities(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)
	probe := &probeRecordingHostUtility{refreshed: make(chan string, 1)}
	c.hostUtility = probe

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Probed Acp",
		Command:     "probed-acp --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	select {
	case agentType := <-probe.refreshed:
		if agentType != "probed-acp" {
			t.Errorf("probed %q, want %q", agentType, "probed-acp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no capability probe was kicked for a new ACP agent")
	}
}

// A terminal agent is never probed, so kicking one would spawn the user's CLI
// for nothing and park a failed capability status on the agent.
func TestCreateCustomTUIAgent_DoesNotProbeCapabilities(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)
	probe := &probeRecordingHostUtility{refreshed: make(chan string, 1)}
	c.hostUtility = probe

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Unprobed Terminal",
		Command:     "unprobed-terminal",
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	select {
	case agentType := <-probe.refreshed:
		t.Errorf("probed %q; terminal agents have no ACP server to probe", agentType)
	case <-time.After(200 * time.Millisecond):
	}
}

// The MCP-strategy route is the one place a stored definition's protocol can be
// paired with a new strategy. An ACP agent has no passthrough config file to
// write, so the pair has to be refused before the row is touched rather than
// written and rolled back.
func TestSetCustomTUIAgentMCPStrategy_RejectedForACPAgent(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)
	ctx := context.Background()

	created, err := c.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Strategy Acp",
		Command:     "strategy-acp --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	if _, err := c.SetCustomTUIAgentMCPStrategy(ctx, created.ID, mcpconfig.StrategyKeyClaude); !errors.Is(err, ErrMCPStrategyNotApplicable) {
		t.Fatalf("error = %v, want ErrMCPStrategyNotApplicable", err)
	}

	stored, ok := st.byName["strategy-acp"]
	if !ok || stored.TUIConfig == nil {
		t.Fatal("agent not persisted with a TUI config")
	}
	if stored.TUIConfig.MCPStrategy != "" {
		t.Errorf("stored strategy = %q, want empty", stored.TUIConfig.MCPStrategy)
	}
	if !stored.SupportsMCP {
		t.Error("SupportsMCP flipped off by a rejected strategy write")
	}
}

// profileUpdateSignalStore publishes each profile model write. The probe runs
// on its own goroutine, and fakeStore is not safe to read while it writes, so
// tests synchronise on the write instead of polling the store.
type profileUpdateSignalStore struct {
	*fakeStore
	models chan string
}

// profileAdoptionRaceStore simulates a user changing the profile after a
// background probe read it but before the probe can write. The conditional
// model update sees that edit and must leave it untouched. The old full-row
// read/modify/write path overwrote it.
type profileAdoptionRaceStore struct {
	*fakeStore
	userEditInjected bool
	fullRowWrites    int
}

func (s *profileAdoptionRaceStore) GetAgentProfile(
	ctx context.Context,
	profileID string,
) (*models.AgentProfile, error) {
	profile, err := s.fakeStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil || s.userEditInjected {
		return profile, err
	}
	s.userEditInjected = true
	userEdit := copyProfile(profile)
	userEdit.Model = "operator-choice"
	if err := s.fakeStore.UpdateAgentProfile(ctx, userEdit); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *profileAdoptionRaceStore) UpdateAgentProfile(
	ctx context.Context,
	profile *models.AgentProfile,
) error {
	s.fullRowWrites++
	return s.fakeStore.UpdateAgentProfile(ctx, profile)
}

func (s *profileAdoptionRaceStore) UpdateAgentProfileModelIfEmpty(
	ctx context.Context,
	profileID, model string,
) (bool, error) {
	if !s.userEditInjected {
		s.userEditInjected = true
		current, err := s.fakeStore.GetAgentProfile(ctx, profileID)
		if err != nil {
			return false, err
		}
		current.Model = "operator-choice"
		if err := s.fakeStore.UpdateAgentProfile(ctx, current); err != nil {
			return false, err
		}
	}
	current, err := s.fakeStore.GetAgentProfile(ctx, profileID)
	if err != nil || current == nil || current.Model != "" {
		return false, err
	}
	current.Model = model
	if err := s.fakeStore.UpdateAgentProfile(ctx, current); err != nil {
		return false, err
	}
	return true, nil
}

func (s *profileUpdateSignalStore) UpdateAgentProfile(ctx context.Context, p *models.AgentProfile) error {
	err := s.fakeStore.UpdateAgentProfile(ctx, p)
	select {
	case s.models <- p.Model:
	default:
	}
	return err
}

func (s *profileUpdateSignalStore) UpdateAgentProfileModelIfEmpty(
	ctx context.Context,
	profileID, model string,
) (bool, error) {
	profile, err := s.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil || profile.Model != "" {
		return false, err
	}
	profile.Model = model
	if err := s.UpdateAgentProfile(ctx, profile); err != nil {
		return false, err
	}
	return true, nil
}

func newProbedController(
	t *testing.T,
	caps hostutility.AgentCapabilities,
) (*Controller, *profileUpdateSignalStore) {
	t.Helper()
	st := &profileUpdateSignalStore{fakeStore: newFakeStore(), models: make(chan string, 4)}
	c := newCustomTUIController(t, st.fakeStore)
	c.repo = st
	c.hostUtility = &probeRecordingHostUtility{refreshed: make(chan string, 1), caps: caps}
	return c, st
}

// The seeded ACP profile carries no model on purpose: the probe is what learns
// one. ProfileReconciler copies a probed default into an empty profile, but it
// runs once during startup, so an agent registered afterwards would keep an
// empty model until the next restart and its sessions would silently take
// whatever the agent defaults to.
func TestCreateCustomACPAgent_AdoptsProbedModel(t *testing.T) {
	c, st := newProbedController(t, hostutility.AgentCapabilities{CurrentModelID: "probed-model"})

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Adopting Acp",
		Command:     "adopting-acp --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	select {
	case model := <-st.models:
		if model != "probed-model" {
			t.Errorf("wrote model %q, want the probed default", model)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the probed model was never written to the seeded profile")
	}
}

// A model the operator already chose is theirs. Overwriting it from the probe
// is exactly the silent fallback the reconciler refuses to make.
func TestCreateCustomACPAgent_KeepsAnOperatorModel(t *testing.T) {
	c, st := newProbedController(t, hostutility.AgentCapabilities{CurrentModelID: "probed-model"})

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Chosen Acp",
		Command:     "chosen-acp --acp",
		Protocol:    string(registry.CustomAgentProtocolACP),
		Model:       "operator-choice",
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	select {
	case model := <-st.models:
		t.Errorf("overwrote the operator's model with %q", model)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestAdoptProbedModelDoesNotOverwriteConcurrentProfileEdit(t *testing.T) {
	base := newFakeStore()
	profile := &models.AgentProfile{AgentID: "agent-1", Name: "Default"}
	if err := base.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	st := &profileAdoptionRaceStore{fakeStore: base}
	c := newCustomTUIController(t, base)
	c.repo = st

	c.adoptProbedModel(context.Background(), profile.ID, hostutility.AgentCapabilities{
		CurrentModelID: "probed-model",
	})

	got, err := base.GetAgentProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.Model != "operator-choice" {
		t.Fatalf("model = %q, want concurrent operator edit to win", got.Model)
	}
	if st.fullRowWrites != 0 {
		t.Fatalf("full-row writes = %d, want no full-row adoption write", st.fullRowWrites)
	}
}
