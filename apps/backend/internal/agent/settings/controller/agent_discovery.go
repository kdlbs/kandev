package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"os/exec"
	"runtime"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func (c *Controller) ListDiscovery(ctx context.Context) (*dto.ListDiscoveryResponse, error) {
	results, err := c.detectAgents(ctx)
	if err != nil {
		return nil, err
	}
	payload := make([]dto.AgentDiscoveryDTO, 0, len(results))
	for _, result := range results {
		payload = append(payload, dto.AgentDiscoveryDTO{
			Name:              result.Name,
			SupportsMCP:       result.SupportsMCP,
			MCPConfigPath:     result.MCPConfigPath,
			InstallationPaths: result.InstallationPaths,
			Available:         result.Available,
			MatchedPath:       result.MatchedPath,
		})
	}
	return &dto.ListDiscoveryResponse{Agents: payload, Total: len(payload)}, nil
}

func (c *Controller) ListAvailableAgents(ctx context.Context) (*dto.ListAvailableAgentsResponse, error) {
	results, err := c.detectAgents(ctx)
	if err != nil {
		return nil, err
	}
	availabilityByName := make(map[string]discovery.Availability, len(results))
	for _, result := range results {
		availabilityByName[result.Name] = result
	}

	enabled := c.agentRegistry.ListEnabled()
	now := time.Now().UTC()
	payload := make([]dto.AvailableAgentDTO, 0, len(enabled))
	for _, ag := range enabled {
		availability, ok := availabilityByName[ag.ID()]
		if !ok {
			availability = discovery.Availability{Name: ag.ID(), Available: false}
		}
		payload = append(payload, c.buildAvailableAgentDTO(ctx, ag, availability, now))
	}
	tools := c.detectTools()
	return &dto.ListAvailableAgentsResponse{Agents: payload, Tools: tools, Total: len(payload)}, nil
}

// HasAvailableAgents returns true if at least one agent is detected as installed.
func (c *Controller) HasAvailableAgents(ctx context.Context) (bool, error) {
	results, err := c.detectAgents(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range results {
		if r.Available {
			return true, nil
		}
	}
	return false, nil
}

func (c *Controller) InvalidateDiscoveryCache() {
	if c.discovery != nil {
		c.discovery.InvalidateCache()
	}
}

func (c *Controller) buildAvailableAgentDTO(ctx context.Context, ag agents.Agent, availability discovery.Availability, now time.Time) dto.AvailableAgentDTO {
	displayName := ag.DisplayName()
	if displayName == "" {
		displayName = ag.Name()
	}

	modelConfig := c.buildModelConfigFromHostUtility(ag.ID())
	_ = ctx

	capabilities := dto.AgentCapabilitiesDTO{
		SupportsSessionResume: availability.Capabilities.SupportsSessionResume,
		SupportsShell:         availability.Capabilities.SupportsShell,
		SupportsWorkspaceOnly: availability.Capabilities.SupportsWorkspaceOnly,
	}

	var permissionSettings map[string]dto.PermissionSettingDTO
	if permSettings := ag.PermissionSettings(); permSettings != nil {
		permissionSettings = make(map[string]dto.PermissionSettingDTO, len(permSettings))
		for key, setting := range permSettings {
			permissionSettings[key] = dto.PermissionSettingDTO{
				Supported:    setting.Supported,
				Default:      setting.Default,
				Label:        setting.Label,
				Description:  setting.Description,
				ApplyMethod:  setting.ApplyMethod,
				CLIFlag:      setting.CLIFlag,
				CLIFlagValue: setting.CLIFlagValue,
			}
		}
	}

	var passthroughConfig *dto.PassthroughConfigDTO
	if ptAgent, ok := ag.(agents.PassthroughAgent); ok {
		pt := ptAgent.PassthroughConfig()
		passthroughConfig = &dto.PassthroughConfigDTO{
			Supported:   pt.Supported,
			Label:       pt.Label,
			Description: pt.Description,
		}
	}

	return dto.AvailableAgentDTO{
		Name:               ag.ID(),
		DisplayName:        displayName,
		Description:        ag.Description(),
		InstallScript:      ag.InstallScript(),
		SupportsMCP:        availability.SupportsMCP,
		MCPConfigPath:      availability.MCPConfigPath,
		InstallationPaths:  availability.InstallationPaths,
		Available:          availability.Available,
		MatchedPath:        availability.MatchedPath,
		Capabilities:       capabilities,
		ModelConfig:        modelConfig,
		PermissionSettings: permissionSettings,
		PassthroughConfig:  passthroughConfig,
		UpdatedAt:          now,
	}
}

// buildModelConfigFromHostUtility reads cached ACP probe data for the agent
// type and produces a ModelConfigDTO with models, modes, and status. Agents
// not in the probe cache (e.g. the mock agent used in E2E tests, which
// doesn't speak ACP through its binary) fall back to `SupportsDynamicModels:
// false` with an empty model list — callers render the profile's stored
// model as a plain string rather than offering a dropdown.
func (c *Controller) buildModelConfigFromHostUtility(agentID string) dto.ModelConfigDTO {
	// Always initialize slices so JSON marshals as [] not null — the
	// frontend uses .some()/.find() on these without null checks.
	cfg := dto.ModelConfigDTO{
		SupportsDynamicModels: false,
		AvailableModels:       []dto.ModelEntryDTO{},
		AvailableModes:        []dto.ModeEntryDTO{},
	}
	if c.hostUtility == nil {
		cfg.Status = "not_configured"
		return cfg
	}
	caps, ok := c.hostUtility.Get(agentID)
	if !ok {
		cfg.Status = "not_configured"
		return cfg
	}
	cfg.SupportsDynamicModels = true
	cfg.Status = string(caps.Status)
	cfg.Error = caps.Error
	cfg.DefaultModel = caps.CurrentModelID
	cfg.CurrentModelID = caps.CurrentModelID
	cfg.CurrentModeID = caps.CurrentModeID
	for _, m := range caps.Models {
		cfg.AvailableModels = append(cfg.AvailableModels, dto.ModelEntryDTO{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			IsDefault:   m.ID == caps.CurrentModelID,
			Meta:        m.Meta,
		})
	}
	for _, m := range caps.Modes {
		cfg.AvailableModes = append(cfg.AvailableModes, dto.ModeEntryDTO{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Meta:        m.Meta,
		})
	}
	for _, c := range caps.Commands {
		cfg.AvailableCommands = append(cfg.AvailableCommands, dto.CommandEntryDTO{
			Name:        c.Name,
			Description: c.Description,
		})
	}
	return cfg
}

func (c *Controller) EnsureInitialAgentProfiles(ctx context.Context) error {
	results, err := c.detectAgents(ctx)
	if err != nil {
		return err
	}
	for _, result := range results {
		if !result.Available {
			continue
		}
		if err := c.syncAgentFromDiscovery(ctx, result); err != nil {
			return err
		}
	}
	return nil
}

// profileSyncParams holds resolved parameters used when syncing agent profiles.
type profileSyncParams struct {
	displayName     string
	defaultModel    string
	isPassthrough   bool
	autoApprove     bool
	allowIndexing   bool
	skipPermissions bool
}

// updateExistingProfiles syncs non-user-modified profiles with current agent defaults.
func (c *Controller) updateExistingProfiles(ctx context.Context, profiles []*models.AgentProfile, p profileSyncParams) error {
	for _, profile := range profiles {
		if profile.UserModified {
			continue
		}
		updated := false
		if profile.AgentDisplayName != p.displayName {
			profile.AgentDisplayName = p.displayName
			updated = true
		}
		if profile.Model != p.defaultModel {
			profile.Model = p.defaultModel
			updated = true
		}
		resolvedName := profile.Model
		if p.isPassthrough || resolvedName == "" {
			resolvedName = p.displayName
		}
		if profile.Name != resolvedName {
			profile.Name = resolvedName
			updated = true
		}
		if profile.AutoApprove != p.autoApprove {
			profile.AutoApprove = p.autoApprove
			updated = true
		}
		if profile.AllowIndexing != p.allowIndexing {
			profile.AllowIndexing = p.allowIndexing
			updated = true
		}
		if profile.DangerouslySkipPermissions != p.skipPermissions {
			profile.DangerouslySkipPermissions = p.skipPermissions
			updated = true
		}
		if p.isPassthrough && !profile.CLIPassthrough {
			profile.CLIPassthrough = true
			updated = true
		}
		if updated {
			if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Controller) syncAgentFromDiscovery(ctx context.Context, result discovery.Availability) error {
	agentConfig, ok := c.agentRegistry.Get(result.Name)
	if !ok {
		return fmt.Errorf("unknown agent: %s", result.Name)
	}
	displayName, err := c.resolveDisplayName(agentConfig, result.Name)
	if err != nil {
		return err
	}
	defaultModel, isPassthroughOnly, err := resolveDefaultModel(agentConfig, result.Name)
	if err != nil {
		return err
	}
	agent, err := c.upsertAgent(ctx, result)
	if err != nil {
		return err
	}
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		return err
	}
	p := c.buildProfileSyncParams(ctx, agentConfig, result.Name, displayName, defaultModel, isPassthroughOnly)

	if len(profiles) > 0 {
		return c.updateExistingProfiles(ctx, profiles, p)
	}
	return c.createDefaultProfile(ctx, agent.ID, p)
}

// resolveDisplayName returns the display name for an agent config, falling back to its
// internal name, and returns an error if no name can be determined.
func (c *Controller) resolveDisplayName(agentConfig agents.Agent, agentName string) (string, error) {
	displayName := agentConfig.DisplayName()
	if displayName == "" {
		displayName = agentConfig.Name()
	}
	if displayName == "" {
		return "", fmt.Errorf("unknown agent display name: %s", agentName)
	}
	return displayName, nil
}

// buildProfileSyncParams assembles the parameters needed to create or update agent profiles.
func (c *Controller) buildProfileSyncParams(
	_ context.Context,
	agentConfig agents.Agent,
	_, displayName, defaultModel string,
	isPassthroughOnly bool,
) profileSyncParams {
	autoApprove, allowIndexing, skipPermissions := resolvePermissionDefaults(agentConfig.PermissionSettings())
	return profileSyncParams{
		displayName:     displayName,
		defaultModel:    defaultModel,
		isPassthrough:   isPassthroughOnly,
		autoApprove:     autoApprove,
		allowIndexing:   allowIndexing,
		skipPermissions: skipPermissions,
	}
}

// createDefaultProfile creates the initial agent profile when none exist for an agent.
// The model may be empty — the profile reconciler fills it from the host
// utility capability cache on boot.
func (c *Controller) createDefaultProfile(ctx context.Context, agentID string, p profileSyncParams) error {
	profileName := p.displayName
	if !p.isPassthrough && p.defaultModel != "" {
		profileName = p.defaultModel
	}
	defaultProfile := &models.AgentProfile{
		AgentID:                    agentID,
		Name:                       profileName,
		Model:                      p.defaultModel,
		AgentDisplayName:           p.displayName,
		AutoApprove:                p.autoApprove,
		AllowIndexing:              p.allowIndexing,
		DangerouslySkipPermissions: p.skipPermissions,
		CLIPassthrough:             p.isPassthrough,
	}
	return c.repo.CreateAgentProfile(ctx, defaultProfile)
}

func resolveDefaultModel(agentConfig agents.Agent, _ string) (string, bool, error) {
	// Default models come from the host utility capability cache after probing.
	// The legacy bootstrap path leaves the model empty and lets the reconciler
	// heal it against the cache. TUI-only agents still seed as passthrough so
	// the terminal UI is enabled by default.
	if agents.IsPassthroughOnly(agentConfig) {
		return "passthrough", true, nil
	}
	// Mock agent is not probed (not an InferenceAgent) but needs a concrete
	// model for E2E tests that exercise the ModelSelector UI.
	if agentConfig.ID() == "mock-agent" {
		return "mock-default", false, nil
	}
	return "", false, nil
}

func (c *Controller) upsertAgent(ctx context.Context, result discovery.Availability) (*models.Agent, error) {
	agent, err := c.repo.GetAgentByName(ctx, result.Name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) || agent == nil {
		agent = &models.Agent{
			Name:          result.Name,
			SupportsMCP:   result.SupportsMCP,
			MCPConfigPath: result.MCPConfigPath,
		}
		if err := c.repo.CreateAgent(ctx, agent); err != nil {
			return nil, err
		}
		return agent, nil
	}
	updated := false
	if agent.SupportsMCP != result.SupportsMCP {
		agent.SupportsMCP = result.SupportsMCP
		updated = true
	}
	if agent.MCPConfigPath != result.MCPConfigPath {
		agent.MCPConfigPath = result.MCPConfigPath
		updated = true
	}
	if updated {
		if err := c.repo.UpdateAgent(ctx, agent); err != nil {
			return nil, err
		}
	}
	return agent, nil
}

// detectTools checks for recommended CLI tools (e.g. gh) and returns their status.
func (c *Controller) detectTools() []dto.ToolStatusDTO {
	ghTool := dto.ToolStatusDTO{
		Name:        "gh",
		DisplayName: "GitHub CLI",
		Description: "Required for GitHub integration features (PRs, reviews, webhooks).",
		InfoURL:     "https://cli.github.com",
	}
	switch runtime.GOOS {
	case "darwin":
		ghTool.InstallScript = "brew install gh"
	case "windows":
		ghTool.InstallScript = "winget install --id GitHub.cli"
	default:
		// No single command works across all Linux distros; rely on info_url.
		ghTool.InstallScript = ""
	}
	if path, err := exec.LookPath("gh"); err == nil {
		ghTool.Available = true
		ghTool.MatchedPath = path
	}
	return []dto.ToolStatusDTO{ghTool}
}

// detectAgents runs discovery and forces mock-agent available when enabled.
func (c *Controller) detectAgents(ctx context.Context) ([]discovery.Availability, error) {
	results, err := c.discovery.Detect(ctx)
	if err != nil {
		return nil, err
	}
	// Force mock-agent as available when enabled (skip file-presence discovery)
	agentConfig, ok := c.agentRegistry.Get("mock-agent")
	if ok && agentConfig.Enabled() {
		for i := range results {
			if results[i].Name == "mock-agent" {
				results[i].Available = true
				break
			}
		}
	}
	return results, nil
}
