package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"go.uber.org/zap"
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
	return &dto.ListAvailableAgentsResponse{Agents: payload, Total: len(payload)}, nil
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

	modelEntries, supportsDynamic := c.fetchModelsWithCache(ctx, ag)

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
		Name:              ag.ID(),
		DisplayName:       displayName,
		SupportsMCP:       availability.SupportsMCP,
		MCPConfigPath:     availability.MCPConfigPath,
		InstallationPaths: availability.InstallationPaths,
		Available:         availability.Available,
		MatchedPath:       availability.MatchedPath,
		Capabilities:      capabilities,
		ModelConfig: dto.ModelConfigDTO{
			DefaultModel:          ag.DefaultModel(),
			AvailableModels:       modelEntries,
			SupportsDynamicModels: supportsDynamic,
		},
		PermissionSettings: permissionSettings,
		PassthroughConfig:  passthroughConfig,
		UpdatedAt:          now,
	}
}

// fetchModelsWithCache returns model entries for an agent, using the model cache
// to avoid expensive subprocess calls (e.g. `opencode models`) on every request.
func (c *Controller) fetchModelsWithCache(ctx context.Context, ag agents.Agent) ([]dto.ModelEntryDTO, bool) {
	agentName := ag.ID()

	// Check cache first
	if entry, exists := c.modelCache.Get(agentName); exists && (entry.IsValid() || entry.IsStale()) {
		if entry.Error == nil && entry.Models != nil {
			return modelsToDTO(entry.Models), true
		}
	}

	// Cache miss — call ListModels and cache the result
	modelList, err := ag.ListModels(ctx)
	if err != nil {
		c.logger.Warn("failed to list models for available agent",
			zap.String("agent", agentName), zap.Error(err))
		c.modelCache.Set(agentName, nil, err)
		return nil, false
	}
	if modelList == nil {
		return nil, false
	}

	if modelList.SupportsDynamic {
		c.modelCache.Set(agentName, modelList.Models, nil)
	}

	return modelsToDTO(modelList.Models), modelList.SupportsDynamic
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
	modelList       *agents.ModelList
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
		resolvedName := resolveModelDisplayName(p.modelList, profile.Model)
		if p.isPassthrough {
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
	ctx context.Context,
	agentConfig agents.Agent,
	agentName, displayName, defaultModel string,
	isPassthroughOnly bool,
) profileSyncParams {
	autoApprove, allowIndexing, skipPermissions := resolvePermissionDefaults(agentConfig.PermissionSettings())
	modelList, listErr := agentConfig.ListModels(ctx)
	if listErr != nil {
		c.logger.Warn("failed to list models during profile sync, using model ID as name",
			zap.String("agent", agentName), zap.Error(listErr))
	}
	return profileSyncParams{
		displayName:     displayName,
		defaultModel:    defaultModel,
		isPassthrough:   isPassthroughOnly,
		autoApprove:     autoApprove,
		allowIndexing:   allowIndexing,
		skipPermissions: skipPermissions,
		modelList:       modelList,
	}
}

// createDefaultProfile creates the initial agent profile when none exist for an agent.
func (c *Controller) createDefaultProfile(ctx context.Context, agentID string, p profileSyncParams) error {
	profileName := resolveModelDisplayName(p.modelList, p.defaultModel)
	if p.isPassthrough {
		profileName = p.displayName
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

func resolveDefaultModel(agentConfig agents.Agent, name string) (string, bool, error) {
	defaultModel := agentConfig.DefaultModel()
	if defaultModel != "" {
		return defaultModel, false, nil
	}
	if ptAgent, ok := agentConfig.(agents.PassthroughAgent); ok && ptAgent.PassthroughConfig().Supported {
		return "passthrough", true, nil
	}
	return "", false, fmt.Errorf("unknown agent default model: %s", name)
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
