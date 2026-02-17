package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// buildCommandString builds a display-friendly command string with proper quoting.
func buildCommandString(cmd []string) string {
	var parts []string
	for _, arg := range cmd {
		if strings.ContainsAny(arg, " \t\n\"'`$\\") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			parts = append(parts, "\""+escaped+"\"")
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentProfileNotFound = errors.New("agent profile not found")
	ErrAgentProfileInUse    = errors.New("agent profile is used by an active agent session")
	ErrAgentMcpUnsupported  = errors.New("mcp not supported by agent")
	ErrModelRequired        = errors.New("model is required for agent profiles")
	ErrLogoNotAvailable     = errors.New("logo not available for agent")
)

type Controller struct {
	repo           store.Repository
	discovery      *discovery.Registry
	agentRegistry  *registry.Registry
	sessionChecker SessionChecker
	mcpService     *mcpconfig.Service
	modelCache     *modelfetcher.Cache
	logger         *logger.Logger
}

type SessionChecker interface {
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
}

func NewController(repo store.Repository, discoveryRegistry *discovery.Registry, agentRegistry *registry.Registry, sessionChecker SessionChecker, log *logger.Logger,
) *Controller {
	return &Controller{
		repo:           repo,
		discovery:      discoveryRegistry,
		agentRegistry:  agentRegistry,
		sessionChecker: sessionChecker,
		mcpService:     mcpconfig.NewService(repo),
		modelCache:     modelfetcher.NewCache(),
		logger:         log.WithFields(zap.String("component", "agent-settings-controller")),
	}
}

// EnsureDefaultMcpConfig ensures all agent profiles that support MCP have
// MCP enabled by default. The kandev MCP server is automatically injected
// by agentctl at session creation time, so profiles don't need to explicitly
// configure it.
func (c *Controller) EnsureDefaultMcpConfig(ctx context.Context) error {
	agents, err := c.repo.ListAgents(ctx)
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if !agent.SupportsMCP {
			continue
		}

		profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return err
		}

		for _, profile := range profiles {
			// Check if MCP config already exists
			existingConfig, err := c.repo.GetAgentProfileMcpConfig(ctx, profile.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			// Skip if config already exists (don't overwrite user settings)
			if existingConfig != nil {
				continue
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
				continue
			}

			c.logger.Info("created default MCP config for profile",
				zap.String("profile_id", profile.ID),
				zap.String("profile_name", profile.Name))
		}
	}

	return nil
}

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
			availability = discovery.Availability{
				Name:      ag.ID(),
				Available: false,
			}
		}

		displayName := ag.DisplayName()
		if displayName == "" {
			displayName = ag.Name()
		}

		// Get models from the agent
		var modelEntries []dto.ModelEntryDTO
		var supportsDynamic bool
		modelList, err := ag.ListModels(ctx)
		if err == nil && modelList != nil {
			supportsDynamic = modelList.SupportsDynamic
			modelEntries = make([]dto.ModelEntryDTO, 0, len(modelList.Models))
			for _, model := range modelList.Models {
				modelEntries = append(modelEntries, dto.ModelEntryDTO{
					ID:            model.ID,
					Name:          model.Name,
					Provider:      model.Provider,
					ContextWindow: model.ContextWindow,
					IsDefault:     model.IsDefault,
				})
			}
		}

		// Get discovery result for capabilities
		disc, discErr := ag.IsInstalled(ctx)
		var capabilities dto.AgentCapabilitiesDTO
		if discErr == nil && disc != nil {
			capabilities = dto.AgentCapabilitiesDTO{
				SupportsSessionResume: disc.Capabilities.SupportsSessionResume,
				SupportsShell:         disc.Capabilities.SupportsShell,
				SupportsWorkspaceOnly: disc.Capabilities.SupportsWorkspaceOnly,
			}
		}

		// Convert permission settings
		var permissionSettings map[string]dto.PermissionSettingDTO
		permSettings := ag.PermissionSettings()
		if permSettings != nil {
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

		// Convert passthrough config
		var passthroughConfig *dto.PassthroughConfigDTO
		if ptAgent, ok := ag.(agents.PassthroughAgent); ok {
			pt := ptAgent.PassthroughConfig()
			passthroughConfig = &dto.PassthroughConfigDTO{
				Supported:   pt.Supported,
				Label:       pt.Label,
				Description: pt.Description,
			}
		}

		payload = append(payload, dto.AvailableAgentDTO{
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
		})
	}
	return &dto.ListAvailableAgentsResponse{Agents: payload, Total: len(payload)}, nil
}

func (c *Controller) ListAgents(ctx context.Context) (*dto.ListAgentsResponse, error) {
	agents, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	payload := make([]dto.AgentDTO, 0, len(agents))
	for _, agent := range agents {
		profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		payload = append(payload, toAgentDTO(agent, profiles))
	}
	return &dto.ListAgentsResponse{Agents: payload, Total: len(payload)}, nil
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
		agentConfig, ok := c.agentRegistry.Get(result.Name)
		if !ok {
			return fmt.Errorf("unknown agent: %s", result.Name)
		}
		displayName := agentConfig.DisplayName()
		if displayName == "" {
			displayName = agentConfig.Name()
		}
		if displayName == "" {
			return fmt.Errorf("unknown agent display name: %s", result.Name)
		}
		defaultModel := agentConfig.DefaultModel()
		isPassthroughOnly := false
		if defaultModel == "" {
			if ptAgent, ok := agentConfig.(agents.PassthroughAgent); ok && ptAgent.PassthroughConfig().Supported {
				isPassthroughOnly = true
				defaultModel = "passthrough"
			} else {
				return fmt.Errorf("unknown agent default model: %s", result.Name)
			}
		}
		agent, err := c.repo.GetAgentByName(ctx, result.Name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) || agent == nil {
			agent = &models.Agent{
				Name:          result.Name,
				SupportsMCP:   result.SupportsMCP,
				MCPConfigPath: result.MCPConfigPath,
			}
			if err := c.repo.CreateAgent(ctx, agent); err != nil {
				return err
			}
		} else {
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
					return err
				}
			}
		}
		profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return err
		}
		autoApprove, allowIndexing, skipPermissions := resolvePermissionDefaults(agentConfig.PermissionSettings())

		// Fetch model list once per agent (not per profile) to resolve display names
		modelList, listErr := agentConfig.ListModels(ctx)
		if listErr != nil {
			c.logger.Warn("failed to list models during profile sync, using model ID as name",
				zap.String("agent", result.Name), zap.Error(listErr))
		}

		if len(profiles) > 0 {
			for _, profile := range profiles {
				if profile.UserModified {
					continue
				}
				updated := false

				if profile.AgentDisplayName != displayName {
					profile.AgentDisplayName = displayName
					updated = true
				}
				if profile.Model != defaultModel {
					profile.Model = defaultModel
					updated = true
				}

				// Re-resolve profile name from model display name
				resolvedName := resolveModelDisplayName(modelList, profile.Model)
				if isPassthroughOnly {
					resolvedName = displayName
				}
				if profile.Name != resolvedName {
					profile.Name = resolvedName
					updated = true
				}

				if profile.AutoApprove != autoApprove {
					profile.AutoApprove = autoApprove
					updated = true
				}
				if profile.AllowIndexing != allowIndexing {
					profile.AllowIndexing = allowIndexing
					updated = true
				}
				if profile.DangerouslySkipPermissions != skipPermissions {
					profile.DangerouslySkipPermissions = skipPermissions
					updated = true
				}
				if isPassthroughOnly && !profile.CLIPassthrough {
					profile.CLIPassthrough = true
					updated = true
				}

				if updated {
					if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
						return err
					}
				}
			}
			continue
		}

		profileName := resolveModelDisplayName(modelList, defaultModel)
		if isPassthroughOnly {
			profileName = displayName
		}
		defaultProfile := &models.AgentProfile{
			AgentID:                    agent.ID,
			Name:                       profileName,
			Model:                      defaultModel,
			AgentDisplayName:           displayName,
			AutoApprove:                autoApprove,
			AllowIndexing:              allowIndexing,
			DangerouslySkipPermissions: skipPermissions,
			CLIPassthrough:             isPassthroughOnly,
		}
		if err := c.repo.CreateAgentProfile(ctx, defaultProfile); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) GetAgent(ctx context.Context, id string) (*dto.AgentDTO, error) {
	agent, err := c.repo.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	result := toAgentDTO(agent, profiles)
	return &result, nil
}

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

type CreateAgentRequest struct {
	Name        string
	WorkspaceID *string
	Profiles    []CreateAgentProfileRequest
}

type CreateAgentProfileRequest struct {
	Name                       string
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
}

func (c *Controller) CreateAgent(ctx context.Context, req CreateAgentRequest) (*dto.AgentDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	existing, err := c.repo.GetAgentByName(ctx, req.Name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && existing != nil {
		return nil, fmt.Errorf("agent already configured: %s", req.Name)
	}
	discoveryResults, err := c.detectAgents(ctx)
	if err != nil {
		return nil, err
	}
	var matched *discovery.Availability
	for _, result := range discoveryResults {
		if result.Name == req.Name {
			matched = &result
			break
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("unknown agent: %s", req.Name)
	}
	if !matched.Available {
		return nil, fmt.Errorf("agent not installed: %s", req.Name)
	}
	agentConfig, agOk := c.agentRegistry.Get(req.Name)
	if !agOk {
		return nil, fmt.Errorf("unknown agent: %s", req.Name)
	}
	displayName := agentConfig.DisplayName()
	if displayName == "" {
		displayName = agentConfig.Name()
	}
	if displayName == "" {
		return nil, fmt.Errorf("unknown agent display name: %s", req.Name)
	}
	agent := &models.Agent{
		Name:          matched.Name,
		WorkspaceID:   req.WorkspaceID,
		SupportsMCP:   matched.SupportsMCP,
		MCPConfigPath: matched.MCPConfigPath,
	}
	if err := c.repo.CreateAgent(ctx, agent); err != nil {
		return nil, err
	}
	profiles := make([]*models.AgentProfile, 0, len(req.Profiles))
	for _, profileReq := range req.Profiles {
		profile := &models.AgentProfile{
			AgentID:                    agent.ID,
			Name:                       profileReq.Name,
			AgentDisplayName:           displayName,
			Model:                      profileReq.Model,
			AutoApprove:                profileReq.AutoApprove,
			DangerouslySkipPermissions: profileReq.DangerouslySkipPermissions,
		}
		if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	result := toAgentDTO(agent, profiles)
	return &result, nil
}

type UpdateAgentRequest struct {
	ID            string
	WorkspaceID   *string
	SupportsMCP   *bool
	MCPConfigPath *string
}

func (c *Controller) UpdateAgent(ctx context.Context, req UpdateAgentRequest) (*dto.AgentDTO, error) {
	agent, err := c.repo.GetAgent(ctx, req.ID)
	if err != nil {
		return nil, ErrAgentNotFound
	}
	if req.WorkspaceID != nil {
		agent.WorkspaceID = req.WorkspaceID
	}
	if req.SupportsMCP != nil {
		agent.SupportsMCP = *req.SupportsMCP
	}
	if req.MCPConfigPath != nil {
		agent.MCPConfigPath = *req.MCPConfigPath
	}
	if err := c.repo.UpdateAgent(ctx, agent); err != nil {
		return nil, err
	}
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	result := toAgentDTO(agent, profiles)
	return &result, nil
}

func (c *Controller) DeleteAgent(ctx context.Context, id string) error {
	if err := c.repo.DeleteAgent(ctx, id); err != nil {
		if strings.Contains(err.Error(), "agent not found") {
			return ErrAgentNotFound
		}
		return err
	}
	return nil
}

type CreateProfileRequest struct {
	AgentID                    string
	Name                       string
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	AllowIndexing              bool
	CLIPassthrough             bool
}

func (c *Controller) CreateProfile(ctx context.Context, req CreateProfileRequest) (*dto.AgentProfileDTO, error) {
	// Validate that model is provided
	if strings.TrimSpace(req.Model) == "" {
		return nil, ErrModelRequired
	}
	agent, err := c.repo.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}
	agentConfig, agOk := c.agentRegistry.Get(agent.Name)
	if !agOk {
		return nil, fmt.Errorf("unknown agent: %s", agent.Name)
	}
	displayName := agentConfig.DisplayName()
	if displayName == "" {
		displayName = agentConfig.Name()
	}
	if displayName == "" {
		return nil, fmt.Errorf("unknown agent display name: %s", agent.Name)
	}
	profile := &models.AgentProfile{
		AgentID:                    req.AgentID,
		Name:                       req.Name,
		AgentDisplayName:           displayName,
		Model:                      req.Model,
		AutoApprove:                req.AutoApprove,
		DangerouslySkipPermissions: req.DangerouslySkipPermissions,
		AllowIndexing:              req.AllowIndexing,
		CLIPassthrough:             req.CLIPassthrough,
		UserModified:               true,
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

type UpdateProfileRequest struct {
	ID                         string
	Name                       *string
	Model                      *string
	AutoApprove                *bool
	DangerouslySkipPermissions *bool
	AllowIndexing              *bool
	CLIPassthrough             *bool
}

func (c *Controller) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, req.ID)
	if err != nil {
		return nil, ErrAgentProfileNotFound
	}
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Model != nil {
		// Validate that model is not empty
		if strings.TrimSpace(*req.Model) == "" {
			return nil, ErrModelRequired
		}
		profile.Model = *req.Model
	}
	if req.AutoApprove != nil {
		profile.AutoApprove = *req.AutoApprove
	}
	if req.DangerouslySkipPermissions != nil {
		profile.DangerouslySkipPermissions = *req.DangerouslySkipPermissions
	}
	if req.AllowIndexing != nil {
		profile.AllowIndexing = *req.AllowIndexing
	}
	if req.CLIPassthrough != nil {
		profile.CLIPassthrough = *req.CLIPassthrough
	}
	profile.UserModified = true
	if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

func (c *Controller) DeleteProfile(ctx context.Context, id string) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "agent profile not found") {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	if c.sessionChecker != nil {
		active, err := c.sessionChecker.HasActiveTaskSessionsByAgentProfile(ctx, id)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, ErrAgentProfileInUse
		}
	}
	if err := c.repo.DeleteAgentProfile(ctx, id); err != nil {
		if strings.Contains(err.Error(), "agent profile not found") {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

func toAgentDTO(agent *models.Agent, profiles []*models.AgentProfile) dto.AgentDTO {
	profileDTOs := make([]dto.AgentProfileDTO, 0, len(profiles))
	for _, profile := range profiles {
		profileDTOs = append(profileDTOs, toProfileDTO(profile))
	}
	return dto.AgentDTO{
		ID:            agent.ID,
		Name:          agent.Name,
		WorkspaceID:   agent.WorkspaceID,
		SupportsMCP:   agent.SupportsMCP,
		MCPConfigPath: agent.MCPConfigPath,
		Profiles:      profileDTOs,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
}

func toProfileDTO(profile *models.AgentProfile) dto.AgentProfileDTO {
	return dto.AgentProfileDTO{
		ID:                         profile.ID,
		AgentID:                    profile.AgentID,
		Name:                       profile.Name,
		AgentDisplayName:           profile.AgentDisplayName,
		Model:                      profile.Model,
		AutoApprove:                profile.AutoApprove,
		DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
		AllowIndexing:              profile.AllowIndexing,
		CLIPassthrough:             profile.CLIPassthrough,
		UserModified:               profile.UserModified,
		CreatedAt:                  profile.CreatedAt,
		UpdatedAt:                  profile.UpdatedAt,
	}
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
			Prompt:           "{prompt}",
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
		if entry, exists := c.modelCache.Get(agentName); exists && entry.IsValid() {
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
			}, nil
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
