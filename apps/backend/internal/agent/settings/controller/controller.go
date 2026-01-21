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
	"github.com/kandev/kandev/internal/agent/settings/dto"
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
)

const defaultAgentProfileName = "default"

type Controller struct {
	repo           store.Repository
	discovery      *discovery.Registry
	sessionChecker SessionChecker
	mcpService     *mcpconfig.Service
	logger         *logger.Logger
}

type SessionChecker interface {
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
}

func NewController(repo store.Repository, discoveryRegistry *discovery.Registry, sessionChecker SessionChecker, log *logger.Logger) *Controller {
	return &Controller{
		repo:           repo,
		discovery:      discoveryRegistry,
		sessionChecker: sessionChecker,
		mcpService:     mcpconfig.NewService(repo),
		logger:         log.WithFields(zap.String("component", "agent-settings-controller")),
	}
}

func (c *Controller) ListDiscovery(ctx context.Context) (*dto.ListDiscoveryResponse, error) {
	results, err := c.discovery.Detect(ctx)
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
	results, err := c.discovery.Detect(ctx)
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
				DefaultModel:    def.ModelConfig.DefaultModel,
				AvailableModels: modelEntries,
			},
			UpdatedAt: now,
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
	results, err := c.discovery.Detect(ctx)
	if err != nil {
		return err
	}
	displayNameByAgent := c.displayNameByAgent()
	for _, result := range results {
		if !result.Available {
			continue
		}
		displayName, ok := displayNameByAgent[result.Name]
		if !ok || displayName == "" {
			return fmt.Errorf("unknown agent display name: %s", result.Name)
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
		defaultProfile := &models.AgentProfile{
			AgentID:          agent.ID,
			Name:             defaultAgentProfileName,
			Model:            "",
			Plan:             "",
			AgentDisplayName: displayName,
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
	Plan                       string
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
	discoveryResults, err := c.discovery.Detect(ctx)
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
			Plan:                       profileReq.Plan,
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
	Plan                       string
}

func (c *Controller) CreateProfile(ctx context.Context, req CreateProfileRequest) (*dto.AgentProfileDTO, error) {
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
		Plan:                       req.Plan,
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
	Plan                       *string
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
		profile.Model = *req.Model
	}
	if req.AutoApprove != nil {
		profile.AutoApprove = *req.AutoApprove
	}
	if req.DangerouslySkipPermissions != nil {
		profile.DangerouslySkipPermissions = *req.DangerouslySkipPermissions
	}
	if req.Plan != nil {
		profile.Plan = *req.Plan
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
		Plan:                       profile.Plan,
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
