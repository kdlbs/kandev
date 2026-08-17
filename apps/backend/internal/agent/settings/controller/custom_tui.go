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

// CreateCustomTUIAgentRequest is the request to create a custom TUI agent.
type CreateCustomTUIAgentRequest struct {
	DisplayName string
	Model       string
	Command     string
	Description string
	CommandArgs []string
	// MCPStrategy selects how kandev injects its per-session MCP server into
	// the wrapped CLI. Empty means no MCP tools, which is the default.
	MCPStrategy string
}

// CreateCustomTUIAgent registers a new custom TUI agent and persists it to the database.
func (c *Controller) CreateCustomTUIAgent(ctx context.Context, req CreateCustomTUIAgentRequest) (*dto.AgentDTO, error) {
	slug := slugify(req.DisplayName)
	if slug == "" {
		return nil, ErrInvalidSlug
	}
	if req.Command == "" {
		return nil, ErrCommandRequired
	}
	if _, ok := mcpconfig.StrategyByKey(req.MCPStrategy); !ok {
		return nil, ErrUnknownMCPStrategy
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
		Slug:           slug,
		DisplayName:    req.DisplayName,
		Command:        req.Command,
		Description:    req.Description,
		Model:          req.Model,
		CommandArgs:    req.CommandArgs,
		MCPStrategyKey: req.MCPStrategy,
	}); regErr != nil {
		return nil, fmt.Errorf("failed to register agent: %w", regErr)
	}

	// Persist to DB
	tuiConfig := &models.TUIConfigJSON{
		Command:         req.Command,
		DisplayName:     req.DisplayName,
		Model:           req.Model,
		Description:     req.Description,
		CommandArgs:     req.CommandArgs,
		WaitForTerminal: true,
		MCPStrategy:     req.MCPStrategy,
	}
	agent := &models.Agent{
		Name: slug,
		// Mirrors what TUIAgent.IsInstalled derives, so the profile MCP editor
		// works immediately instead of only after the next discovery sweep.
		SupportsMCP: req.MCPStrategy != mcpconfig.StrategyKeyNone,
		TUIConfig:   tuiConfig,
	}
	if err := c.repo.CreateAgent(ctx, agent); err != nil {
		// Rollback registry on DB failure
		_ = c.agentRegistry.Unregister(slug)
		return nil, err
	}

	// Create default passthrough profile — use model as profile name for distinct dropdown labels
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
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}

	profiles := []*models.AgentProfile{profile}
	result := toAgentDTO(agent, profiles)
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

	return c.customTUIAgentDTO(ctx, agent)
}

// reregisterCustomTUIAgent rebuilds the registry entry from the agent's stored
// tui_config. The strategy is baked into the TUIAgent at construction, so a
// changed strategy needs a new instance rather than a mutation. The swap is
// atomic: Unregister + Register would leave a window in which a launching
// session sees no entry for this agent ID.
func (c *Controller) reregisterCustomTUIAgent(agent *models.Agent) error {
	cfg := agent.TUIConfig
	return c.agentRegistry.ReplaceCustomTUIAgent(registry.CustomTUIAgentSpec{
		Slug:           agent.Name,
		DisplayName:    cfg.DisplayName,
		Command:        cfg.Command,
		Description:    cfg.Description,
		Model:          cfg.Model,
		CommandArgs:    cfg.CommandArgs,
		MCPStrategyKey: cfg.MCPStrategy,
	})
}

func (c *Controller) customTUIAgentDTO(ctx context.Context, agent *models.Agent) (*dto.AgentDTO, error) {
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	result := toAgentDTO(agent, filterGlobalProfiles(profiles))
	return &result, nil
}
