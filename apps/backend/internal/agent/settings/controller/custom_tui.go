package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a display name into a URL-friendly slug.
func slugify(displayName string) string {
	s := strings.ToLower(strings.TrimSpace(displayName))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// CreateCustomTUIAgentRequest is the request to create a custom agent.
type CreateCustomTUIAgentRequest struct {
	DisplayName string
	Model       string
	Command     string
	Description string
	CommandArgs []string
	// MCPStrategy selects how kandev injects its per-session MCP server into
	// the wrapped CLI. Empty means no MCP tools, which is the default. It
	// applies to the terminal protocol only.
	MCPStrategy string
	// Protocol is the runtime kandev drives the command with
	// (registry.CustomAgentProtocol*). Empty means terminal passthrough.
	Protocol string
	// DisableBracketedPaste selects paced unframed delivery for terminal TUIs
	// that do not accept bracketed-paste delimiters.
	DisableBracketedPaste bool
}

// validateCustomAgentProtocol rejects a protocol/strategy pair the registry
// cannot build, so the HTTP layer answers 400 instead of surfacing a
// registration failure as a server error.
func validateCustomAgentProtocol(protocol registry.CustomAgentProtocol, mcpStrategy string) error {
	switch protocol {
	case registry.CustomAgentProtocolTerminal:
		if _, ok := mcpconfig.StrategyByKey(mcpStrategy); !ok {
			return ErrUnknownMCPStrategy
		}
		return nil
	case registry.CustomAgentProtocolACP:
		// An ACP agent receives resolved MCP servers in session/new, so a
		// passthrough config-file strategy has nothing to write into.
		if mcpStrategy != mcpconfig.StrategyKeyNone {
			return ErrMCPStrategyNotApplicable
		}
		return nil
	default:
		return ErrUnknownCustomAgentProtocol
	}
}

// CreateCustomTUIAgent registers a new custom agent and persists it to the database.
func (c *Controller) CreateCustomTUIAgent(ctx context.Context, req CreateCustomTUIAgentRequest) (*dto.AgentDTO, error) {
	slug := slugify(req.DisplayName)
	if slug == "" {
		return nil, ErrInvalidSlug
	}
	if req.Command == "" {
		return nil, ErrCommandRequired
	}
	protocol := registry.CustomAgentProtocol(req.Protocol)
	if err := validateCustomAgentProtocol(protocol, req.MCPStrategy); err != nil {
		return nil, err
	}

	// Check for conflict with existing registry entry
	if c.agentRegistry.Exists(slug) {
		return nil, ErrAgentAlreadyExists
	}

	// Check for conflict with existing DB entry
	existing, err := c.repo.GetAgentByName(ctx, slug)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAgentAlreadyExists
	}

	// Register in the in-memory registry
	if regErr := c.agentRegistry.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{
		Slug:                  slug,
		DisplayName:           req.DisplayName,
		Command:               req.Command,
		Description:           req.Description,
		Model:                 req.Model,
		CommandArgs:           req.CommandArgs,
		MCPStrategyKey:        req.MCPStrategy,
		Protocol:              protocol,
		DisableBracketedPaste: req.DisableBracketedPaste,
	}); regErr != nil {
		return nil, fmt.Errorf("failed to register agent: %w", regErr)
	}

	// Persist to DB
	acp := protocol == registry.CustomAgentProtocolACP
	tuiConfig := &models.TUIConfigJSON{
		Command:               req.Command,
		DisplayName:           req.DisplayName,
		Model:                 req.Model,
		Description:           req.Description,
		CommandArgs:           req.CommandArgs,
		WaitForTerminal:       true,
		MCPStrategy:           req.MCPStrategy,
		Protocol:              req.Protocol,
		DisableBracketedPaste: req.DisableBracketedPaste,
	}
	agent := &models.Agent{
		Name: slug,
		// Mirrors what the registered agent's IsInstalled derives, so the
		// profile MCP editor works immediately instead of only after the next
		// discovery sweep. An ACP agent always supports MCP: resolved servers
		// travel in session/new rather than through a config-file strategy.
		SupportsMCP: acp || req.MCPStrategy != mcpconfig.StrategyKeyNone,
		TUIConfig:   tuiConfig,
	}
	if err := c.repo.CreateAgent(ctx, agent); err != nil {
		// Rollback registry on DB failure
		_ = c.agentRegistry.Unregister(slug)
		return nil, err
	}

	// Seed the default profile in the mode the protocol actually runs in. An
	// ACP profile carries no model: the host-utility capability probe learns
	// the agent's models and the reconciler fills one in.
	profileName := req.Model
	if profileName == "" {
		profileName = req.DisplayName
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             profileName,
		AgentDisplayName: req.DisplayName,
		Model:            "passthrough",
		CLIPassthrough:   true,
	}
	if acp {
		// The probe supplies a default when the operator named no model; it
		// must not be the literal "passthrough" a terminal profile carries.
		profile.Model = req.Model
		profile.CLIPassthrough = false
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}

	// A discovery sweep reports whatever the agent registry holds, and its
	// results are cached, so a membership change has to drop that cache or the
	// new agent is absent from Installed Agents until the TTL expires.
	c.InvalidateDiscoveryCache()
	if acp {
		// An ACP agent's models and modes come from the capability probe, and
		// the boot sweep is long past. A terminal agent has no ACP server to
		// probe, so kicking one there would spawn the user's CLI for nothing.
		c.probeAndAdoptModel(slug, profile.ID)
	}

	profiles := []*models.AgentProfile{profile}
	result := c.toAgentDTO(agent, profiles)
	return &result, nil
}

// SetCustomTUIAgentMCPStrategy changes an existing custom TUI agent's MCP
// injection strategy.
//
// Everything else about a tui_config is immutable — there is no edit route for
// the command — but the strategy has to be mutable or every agent created
// before this feature existed could never get MCP tools without being deleted
// and rebuilt, losing its profiles and any session history pointing at it.
//
// The in-memory registry entry is rebuilt because the strategy is baked into
// the TUIAgent at construction; the DB row is updated to match. Sessions
// already running keep the strategy they launched with until they restart,
// since the passthrough command is built once at launch.
func (c *Controller) SetCustomTUIAgentMCPStrategy(ctx context.Context, agentID, strategyKey string) (*dto.AgentDTO, error) {
	if _, ok := mcpconfig.StrategyByKey(strategyKey); !ok {
		return nil, ErrUnknownMCPStrategy
	}

	agent, err := c.repo.GetAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.TUIConfig == nil {
		// Built-in agents declare their strategy in Go; only custom TUI agents
		// carry a user-selected one.
		return nil, ErrNotCustomTUIAgent
	}
	protocol := registry.CustomAgentProtocol(agent.TUIConfig.Protocol)
	if err := validateCustomAgentProtocol(protocol, strategyKey); err != nil {
		return nil, err
	}
	if agent.TUIConfig.MCPStrategy == strategyKey {
		return c.customTUIAgentDTO(ctx, agent)
	}

	previous := agent.TUIConfig.MCPStrategy
	agent.TUIConfig.MCPStrategy = strategyKey
	agent.SupportsMCP = strategyKey != mcpconfig.StrategyKeyNone
	if err := c.repo.UpdateAgent(ctx, agent); err != nil {
		return nil, err
	}

	if err := c.reregisterCustomTUIAgent(agent); err != nil {
		// Roll the row back so the DB cannot claim a strategy the running
		// registry does not have — that mismatch would persist until restart.
		agent.TUIConfig.MCPStrategy = previous
		agent.SupportsMCP = previous != mcpconfig.StrategyKeyNone
		if rollbackErr := c.repo.UpdateAgent(ctx, agent); rollbackErr != nil {
			return nil, fmt.Errorf("re-register agent: %w (rollback also failed: %v)", err, rollbackErr)
		}
		return nil, fmt.Errorf("failed to re-register agent: %w", err)
	}
	// The replacement instance reports a different SupportsMCP, and the sweep
	// writes that flag back over the agent row: a cached sweep would revert the
	// strategy change that just succeeded.
	c.InvalidateDiscoveryCache()

	return c.customTUIAgentDTO(ctx, agent)
}

// reregisterCustomTUIAgent rebuilds the registry entry from the agent's stored
// tui_config. The strategy is baked into the TUIAgent at construction, so a
// changed strategy needs a new instance rather than a mutation. The swap is
// atomic: Unregister + Register would leave a window in which a launching
// session sees no entry for this agent ID.
func (c *Controller) reregisterCustomTUIAgent(agent *models.Agent) error {
	return c.agentRegistry.ReplaceCustomTUIAgent(CustomAgentSpecFromStored(agent.Name, agent.TUIConfig))
}

// CustomAgentSpecFromStored builds the registry spec a stored custom-agent
// definition replays into. Strategy changes and the boot replay both go
// through it, so a field added to the stored config cannot reach one path and
// silently miss the other.
func CustomAgentSpecFromStored(name string, cfg *models.TUIConfigJSON) registry.CustomTUIAgentSpec {
	return registry.CustomTUIAgentSpec{
		Slug:                  name,
		DisplayName:           cfg.DisplayName,
		Command:               cfg.Command,
		Description:           cfg.Description,
		Model:                 cfg.Model,
		CommandArgs:           cfg.CommandArgs,
		MCPStrategyKey:        cfg.MCPStrategy,
		Protocol:              registry.CustomAgentProtocol(cfg.Protocol),
		DisableBracketedPaste: cfg.DisableBracketedPaste,
	}
}

func (c *Controller) customTUIAgentDTO(ctx context.Context, agent *models.Agent) (*dto.AgentDTO, error) {
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	result := c.toAgentDTO(agent, filterGlobalProfiles(profiles))
	return &result, nil
}
