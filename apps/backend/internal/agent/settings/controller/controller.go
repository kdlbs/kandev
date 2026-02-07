package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentProfileNotFound = errors.New("agent profile not found")
	ErrAgentProfileInUse    = errors.New("agent profile is used by an active agent session")
	ErrAgentMcpUnsupported  = errors.New("mcp not supported by agent")
	ErrModelRequired        = errors.New("model is required for agent profiles")
)

const defaultAgentProfileName = "default"

type Controller struct {
	repo           store.Repository
	discovery      *discovery.Registry
	agentRegistry  *registry.Registry
	sessionChecker SessionChecker
	mcpService     *mcpconfig.Service
	modelFetcher   *modelfetcher.Fetcher
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
		modelFetcher:   modelfetcher.NewFetcher(agentRegistry, log),
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
	definitions := c.discovery.Definitions()
	now := time.Now().UTC()
	payload := make([]dto.AvailableAgentDTO, 0, len(definitions))
	for _, def := range definitions {
		availability, ok := availabilityByName[def.Name]
		if !ok {
			availability = discovery.Availability{
				Name:          def.Name,
				SupportsMCP:   def.SupportsMCP,
				MCPConfigPath: "",
				Available:     false,
			}
		}

		displayName := def.DisplayName
		if displayName == "" {
			displayName = def.Name
		}
		// Convert model entries
		modelEntries := make([]dto.ModelEntryDTO, 0, len(def.ModelConfig.AvailableModels))
		for _, model := range def.ModelConfig.AvailableModels {
			modelEntries = append(modelEntries, dto.ModelEntryDTO{
				ID:            model.ID,
				Name:          model.Name,
				Provider:      model.Provider,
				ContextWindow: model.ContextWindow,
				IsDefault:     model.IsDefault,
			})
		}

		// Convert permission settings
		var permissionSettings map[string]dto.PermissionSettingDTO
		var passthroughConfig *dto.PassthroughConfigDTO
		if agentConfig, ok := c.agentRegistry.Get(def.Name); ok {
			if agentConfig.PermissionSettings != nil {
				permissionSettings = make(map[string]dto.PermissionSettingDTO, len(agentConfig.PermissionSettings))
				for key, setting := range agentConfig.PermissionSettings {
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
			if agentConfig.PassthroughConfig.Supported {
				passthroughConfig = &dto.PassthroughConfigDTO{
					Supported:   agentConfig.PassthroughConfig.Supported,
					Label:       agentConfig.PassthroughConfig.Label,
					Description: agentConfig.PassthroughConfig.Description,
				}
			}
		}

		payload = append(payload, dto.AvailableAgentDTO{
			Name:              availability.Name,
			DisplayName:       displayName,
			SupportsMCP:       availability.SupportsMCP,
			MCPConfigPath:     availability.MCPConfigPath,
			InstallationPaths: availability.InstallationPaths,
			Available:         availability.Available,
			MatchedPath:       availability.MatchedPath,
			Capabilities: dto.AgentCapabilitiesDTO{
				SupportsSessionResume: def.Capabilities.SupportsSessionResume,
				SupportsShell:         def.Capabilities.SupportsShell,
				SupportsWorkspaceOnly: def.Capabilities.SupportsWorkspaceOnly,
			},
			ModelConfig: dto.ModelConfigDTO{
				DefaultModel:          def.ModelConfig.DefaultModel,
				AvailableModels:       modelEntries,
				SupportsDynamicModels: len(def.ModelConfig.DynamicModelsCmd) > 0,
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
	displayNameByAgent := c.displayNameByAgent()
	defaultModelByAgent := c.defaultModelByAgent()
	for _, result := range results {
		if !result.Available {
			continue
		}
		displayName, ok := displayNameByAgent[result.Name]
		if !ok || displayName == "" {
			return fmt.Errorf("unknown agent display name: %s", result.Name)
		}
		defaultModel, ok := defaultModelByAgent[result.Name]
		if !ok || defaultModel == "" {
			return fmt.Errorf("unknown agent default model: %s", result.Name)
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
		if len(profiles) > 0 {
			continue
		}
		// Get agent config to read permission settings defaults
		var autoApprove, allowIndexing, skipPermissions bool
		if agentConfig, ok := c.agentRegistry.Get(agent.Name); ok && agentConfig.PermissionSettings != nil {
			if setting, exists := agentConfig.PermissionSettings["auto_approve"]; exists {
				autoApprove = setting.Default
			}
			if setting, exists := agentConfig.PermissionSettings["allow_indexing"]; exists {
				allowIndexing = setting.Default
			}
			if setting, exists := agentConfig.PermissionSettings["dangerously_skip_permissions"]; exists {
				skipPermissions = setting.Default
			}
		}

		defaultProfile := &models.AgentProfile{
			AgentID:                    agent.ID,
			Name:                       defaultAgentProfileName,
			Model:                      defaultModel,
			AgentDisplayName:           displayName,
			AutoApprove:                autoApprove,
			AllowIndexing:              allowIndexing,
			DangerouslySkipPermissions: skipPermissions,
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
	displayName := c.mustDisplayName(req.Name)
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
	displayName := c.mustDisplayName(agent.Name)
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
		CreatedAt:                  profile.CreatedAt,
		UpdatedAt:                  profile.UpdatedAt,
	}
}

func (c *Controller) displayNameByAgent() map[string]string {
	definitions := c.discovery.Definitions()
	mapped := make(map[string]string, len(definitions))
	for _, def := range definitions {
		if def.Name == "" || def.DisplayName == "" {
			continue
		}
		mapped[def.Name] = def.DisplayName
	}
	return mapped
}

func (c *Controller) mustDisplayName(agentName string) string {
	if agentName == "" {
		return ""
	}
	definitions := c.discovery.Definitions()
	for _, def := range definitions {
		if def.Name == agentName {
			return def.DisplayName
		}
	}
	return ""
}

func (c *Controller) defaultModelByAgent() map[string]string {
	definitions := c.discovery.Definitions()
	mapped := make(map[string]string, len(definitions))
	for _, def := range definitions {
		if def.Name == "" || def.ModelConfig.DefaultModel == "" {
			continue
		}
		mapped[def.Name] = def.ModelConfig.DefaultModel
	}
	return mapped
}

// detectAgents runs discovery and forces mock-agent available when enabled.
func (c *Controller) detectAgents(ctx context.Context) ([]discovery.Availability, error) {
	results, err := c.discovery.Detect(ctx)
	if err != nil {
		return nil, err
	}
	// Force mock-agent as available when enabled (skip file-presence discovery)
	agentConfig, ok := c.agentRegistry.Get("mock-agent")
	if ok && agentConfig.Enabled {
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
	// Get agent config from registry
	agentConfig, ok := c.agentRegistry.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", agentName)
	}

	// Build the command based on whether passthrough is supported AND enabled
	var cmd []string
	if agentConfig.PassthroughConfig.Supported && req.CLIPassthrough {
		cmd = c.buildPassthroughCommandPreview(agentConfig, req)
	} else {
		cmd = c.buildStandardCommandPreview(agentConfig, req)
	}

	// Build the command string with proper quoting for display
	cmdString := c.buildCommandString(cmd)

	return &dto.CommandPreviewResponse{
		Supported:     true,
		Command:       cmd,
		CommandString: cmdString,
	}, nil
}

// buildStandardCommandPreview builds the standard command (non-passthrough) for preview purposes
func (c *Controller) buildStandardCommandPreview(agentConfig *registry.AgentTypeConfig, req CommandPreviewRequest) []string {
	// Start with base command from config
	cmd := make([]string, len(agentConfig.Cmd))
	copy(cmd, agentConfig.Cmd)

	// Apply model flag if configured and model is set
	if req.Model != "" && agentConfig.ModelFlag != "" {
		expanded := strings.ReplaceAll(agentConfig.ModelFlag, "{model}", req.Model)
		parts := strings.SplitN(expanded, " ", 2)
		cmd = append(cmd, parts...)
	}

	// Apply permission settings that use CLI flags
	cmd = c.applyPermissionFlags(cmd, agentConfig, req.PermissionSettings)

	return cmd
}

// buildPassthroughCommandPreview builds the passthrough command for preview purposes
func (c *Controller) buildPassthroughCommandPreview(agentConfig *registry.AgentTypeConfig, req CommandPreviewRequest) []string {
	// Start with passthrough_cmd
	cmd := make([]string, len(agentConfig.PassthroughConfig.PassthroughCmd))
	copy(cmd, agentConfig.PassthroughConfig.PassthroughCmd)

	// Apply model flag if configured and model is set
	if req.Model != "" && agentConfig.PassthroughConfig.ModelFlag != "" {
		expanded := strings.ReplaceAll(agentConfig.PassthroughConfig.ModelFlag, "{model}", req.Model)
		// Split on first space to separate flag from value (if combined)
		parts := strings.SplitN(expanded, " ", 2)
		cmd = append(cmd, parts...)
	}

	// Apply permission settings that use CLI flags
	cmd = c.applyPermissionFlags(cmd, agentConfig, req.PermissionSettings)

	// Add prompt - use PromptFlag if configured, otherwise append directly
	cmd = c.appendPromptPlaceholder(cmd, agentConfig.PassthroughConfig.PromptFlag)

	return cmd
}

// appendPromptPlaceholder adds the {prompt} placeholder to the command
// If promptFlag is set (e.g., "--prompt {prompt}"), it uses that format
// Otherwise, it appends {prompt} directly at the end
func (c *Controller) appendPromptPlaceholder(cmd []string, promptFlag string) []string {
	if promptFlag != "" {
		// Use the configured prompt flag format, e.g., "--prompt {prompt}"
		parts := strings.SplitN(promptFlag, " ", 2)
		return append(cmd, parts...)
	}
	// Default: append prompt directly at the end
	return append(cmd, "{prompt}")
}

// applyPermissionFlags applies CLI flags for permission settings that are enabled
func (c *Controller) applyPermissionFlags(cmd []string, agentConfig *registry.AgentTypeConfig, permissionValues map[string]bool) []string {
	if agentConfig.PermissionSettings == nil || permissionValues == nil {
		return cmd
	}

	for settingName, setting := range agentConfig.PermissionSettings {
		// Skip if not supported or not a CLI flag setting
		if !setting.Supported || setting.ApplyMethod != "cli_flag" || setting.CLIFlag == "" {
			continue
		}

		// Get the value for this setting from the request
		value, exists := permissionValues[settingName]
		if !exists || !value {
			continue
		}

		// Apply the CLI flag
		if setting.CLIFlagValue != "" {
			// Flag with value: "--flag value"
			cmd = append(cmd, setting.CLIFlag, setting.CLIFlagValue)
		} else {
			// Boolean flag or multiple flags: "--flag" or "--flag1 --flag2 arg"
			// Split on spaces to handle multiple flags in one setting
			parts := strings.Fields(setting.CLIFlag)
			cmd = append(cmd, parts...)
		}
	}

	return cmd
}

// buildCommandString builds a display-friendly command string with proper quoting.
// Note: We intentionally don't use strconv.Quote here because it produces Go string
// syntax (e.g., "\t" becomes "\\t") which is not ideal for shell command display.
// The manual quoting approach produces more readable shell-style output.
func (c *Controller) buildCommandString(cmd []string) string {
	var parts []string
	for _, arg := range cmd {
		// Quote arguments that contain spaces or special characters
		if strings.ContainsAny(arg, " \t\n\"'`$\\") {
			// Use double quotes and escape internal double quotes
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			parts = append(parts, "\""+escaped+"\"")
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

// FetchDynamicModels fetches models for an agent, optionally refreshing the cache
func (c *Controller) FetchDynamicModels(ctx context.Context, agentName string, refresh bool) (*dto.DynamicModelsResponse, error) {
	result, err := c.modelFetcher.Fetch(ctx, agentName, refresh)
	if err != nil {
		return nil, err
	}

	// Convert to DTOs
	modelDTOs := make([]dto.ModelEntryDTO, 0, len(result.Models))
	for _, m := range result.Models {
		modelDTOs = append(modelDTOs, dto.ModelEntryDTO{
			ID:            m.ID,
			Name:          m.Name,
			Provider:      m.Provider,
			ContextWindow: m.ContextWindow,
			IsDefault:     m.IsDefault,
			Source:        m.Source,
		})
	}

	// Convert error to string pointer
	var errStr *string
	if result.Error != nil {
		s := result.Error.Error()
		errStr = &s
	}

	return &dto.DynamicModelsResponse{
		AgentName: result.AgentName,
		Models:    modelDTOs,
		Cached:    result.Cached,
		CachedAt:  result.CachedAt,
		Error:     errStr,
	}, nil
}
