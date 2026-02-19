package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"go.uber.org/zap"
)

type UpdateAgentProfileMcpConfigRequest struct {
	Enabled bool
	Servers map[string]mcpconfig.ServerDef
	Meta    map[string]any
}

func (c *Controller) GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*dto.AgentProfileMcpConfigDTO, error) {
	config, err := c.mcpService.GetConfigByProfileID(ctx, profileID)
	if err != nil {
		if errors.Is(err, mcpconfig.ErrAgentProfileNotFound) {
			return nil, ErrAgentProfileNotFound
		}
		if errors.Is(err, mcpconfig.ErrAgentMcpUnsupported) {
			return nil, ErrAgentMcpUnsupported
		}
		return nil, err
	}
	return &dto.AgentProfileMcpConfigDTO{
		ProfileID: config.ProfileID,
		Enabled:   config.Enabled,
		Servers:   config.Servers,
		Meta:      config.Meta,
	}, nil
}

func (c *Controller) UpdateAgentProfileMcpConfig(ctx context.Context, profileID string, req UpdateAgentProfileMcpConfigRequest) (*dto.AgentProfileMcpConfigDTO, error) {
	config, err := c.mcpService.UpsertConfigByProfileID(ctx, profileID, &mcpconfig.ProfileConfig{
		Enabled: req.Enabled,
		Servers: req.Servers,
		Meta:    req.Meta,
	})
	if err != nil {
		if errors.Is(err, mcpconfig.ErrAgentProfileNotFound) {
			return nil, ErrAgentProfileNotFound
		}
		if errors.Is(err, mcpconfig.ErrAgentMcpUnsupported) {
			return nil, ErrAgentMcpUnsupported
		}
		return nil, err
	}
	return &dto.AgentProfileMcpConfigDTO{
		ProfileID: config.ProfileID,
		Enabled:   config.Enabled,
		Servers:   config.Servers,
		Meta:      config.Meta,
	}, nil
}

// EnsureDefaultMcpConfig ensures all agent profiles that support MCP have
// MCP enabled by default. The kandev MCP server is automatically injected
// by agentctl at session creation time, so profiles don't need to explicitly
// configure it.
func (c *Controller) EnsureDefaultMcpConfig(ctx context.Context) error {
	agentList, err := c.repo.ListAgents(ctx)
	if err != nil {
		return err
	}

	for _, agent := range agentList {
		if !agent.SupportsMCP {
			continue
		}

		profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return err
		}

		for _, profile := range profiles {
			if err := c.ensureProfileMcpConfig(ctx, profile); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Controller) ensureProfileMcpConfig(ctx context.Context, profile *models.AgentProfile) error {
	// Check if MCP config already exists
	existingConfig, err := c.repo.GetAgentProfileMcpConfig(ctx, profile.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// Skip if config already exists (don't overwrite user settings)
	if existingConfig != nil {
		return nil
	}

	// Create default MCP config with MCP enabled but no servers configured.
	// The kandev MCP server is automatically injected by agentctl when
	// creating a new session. Users can add additional external MCP servers.
	config := &models.AgentProfileMcpConfig{
		ProfileID: profile.ID,
		Enabled:   true,
		Servers:   map[string]interface{}{},
		Meta:      map[string]interface{}{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := c.repo.UpsertAgentProfileMcpConfig(ctx, config); err != nil {
		c.logger.Warn("failed to create default MCP config for profile",
			zap.String("profile_id", profile.ID),
			zap.String("profile_name", profile.Name),
			zap.Error(err))
		return nil
	}

	c.logger.Info("created default MCP config for profile",
		zap.String("profile_id", profile.ID),
		zap.String("profile_name", profile.Name))
	return nil
}

// GetAgentLogo returns the SVG logo bytes for the given agent and variant.
func (c *Controller) GetAgentLogo(ctx context.Context, agentName string, variant agents.LogoVariant) ([]byte, error) {
	ag, ok := c.agentRegistry.Get(agentName)
	if !ok {
		return nil, ErrAgentNotFound
	}
	data := ag.Logo(variant)
	if len(data) == 0 {
		return nil, ErrLogoNotAvailable
	}
	return data, nil
}

// resolvePermissionDefaults extracts the default permission values from agent settings.
func resolvePermissionDefaults(permSettings map[string]agents.PermissionSetting) (autoApprove, allowIndexing, skipPermissions bool) {
	if permSettings == nil {
		return
	}
	if s, exists := permSettings["auto_approve"]; exists {
		autoApprove = s.Default
	}
	if s, exists := permSettings["allow_indexing"]; exists {
		allowIndexing = s.Default
	}
	if s, exists := permSettings["dangerously_skip_permissions"]; exists {
		skipPermissions = s.Default
	}
	return
}

// resolveModelDisplayName returns the display name for a model ID from the model list,
// falling back to the model ID itself if not found.
func resolveModelDisplayName(modelList *agents.ModelList, modelID string) string {
	if modelList != nil {
		for _, m := range modelList.Models {
			if m.ID == modelID {
				return m.Name
			}
		}
	}
	return modelID
}

// CommandPreviewRequest contains the draft settings for command preview
type CommandPreviewRequest struct {
	Model              string
	PermissionSettings map[string]bool
	CLIPassthrough     bool
}

// PreviewAgentCommand generates a preview of the CLI command that will be executed
func (c *Controller) PreviewAgentCommand(ctx context.Context, agentName string, req CommandPreviewRequest) (*dto.CommandPreviewResponse, error) {
	agentConfig, ok := c.agentRegistry.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", agentName)
	}

	var cmd agents.Command
	if ptAgent, ok := agentConfig.(agents.PassthroughAgent); ok && req.CLIPassthrough {
		cmd = ptAgent.BuildPassthroughCommand(agents.PassthroughOptions{
			Model:            req.Model,
			PermissionValues: req.PermissionSettings,
		})
	} else {
		cmd = agentConfig.BuildCommand(agents.CommandOptions{
			Model:            req.Model,
			PermissionValues: req.PermissionSettings,
		})
	}

	return &dto.CommandPreviewResponse{
		Supported:     true,
		Command:       cmd.Args(),
		CommandString: buildCommandString(cmd.Args()),
	}, nil
}

// FetchDynamicModels fetches models for an agent, optionally refreshing the cache
func (c *Controller) FetchDynamicModels(ctx context.Context, agentName string, refresh bool) (*dto.DynamicModelsResponse, error) {
	ag, ok := c.agentRegistry.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentName)
	}

	// Check cache unless refresh is requested
	if !refresh {
		if cached, ok := c.checkModelCache(agentName); ok {
			return cached, nil
		}
	}

	// Fetch models from the agent
	modelList, err := ag.ListModels(ctx)
	if err != nil {
		c.logger.Warn("model fetch failed",
			zap.String("agent", agentName),
			zap.Error(err))
		c.modelCache.Set(agentName, nil, err)
		s := err.Error()
		return &dto.DynamicModelsResponse{
			AgentName: agentName,
			Models:    nil,
			Cached:    false,
			Error:     &s,
		}, nil
	}

	models := modelList.Models

	// Cache the result if dynamic models are supported
	if modelList.SupportsDynamic {
		c.modelCache.Set(agentName, models, nil)
		cachedAt := time.Now()
		return &dto.DynamicModelsResponse{
			AgentName: agentName,
			Models:    modelsToDTO(models),
			Cached:    true,
			CachedAt:  &cachedAt,
		}, nil
	}

	return &dto.DynamicModelsResponse{
		AgentName: agentName,
		Models:    modelsToDTO(models),
		Cached:    false,
	}, nil
}

// checkModelCache returns cached model response if available and valid.
func (c *Controller) checkModelCache(agentName string) (*dto.DynamicModelsResponse, bool) {
	entry, exists := c.modelCache.Get(agentName)
	if !exists || !entry.IsValid() {
		return nil, false
	}
	cachedAt := entry.CachedAt
	modelDTOs := modelsToDTO(entry.Models)
	var errStr *string
	if entry.Error != nil {
		s := entry.Error.Error()
		errStr = &s
	}
	return &dto.DynamicModelsResponse{
		AgentName: agentName,
		Models:    modelDTOs,
		Cached:    true,
		CachedAt:  &cachedAt,
		Error:     errStr,
	}, true
}

// modelsToDTO converts agent models to DTOs.
func modelsToDTO(models []agents.Model) []dto.ModelEntryDTO {
	dtos := make([]dto.ModelEntryDTO, 0, len(models))
	for _, m := range models {
		dtos = append(dtos, dto.ModelEntryDTO{
			ID:            m.ID,
			Name:          m.Name,
			Provider:      m.Provider,
			ContextWindow: m.ContextWindow,
			IsDefault:     m.IsDefault,
			Source:        m.Source,
		})
	}
	return dtos
}
