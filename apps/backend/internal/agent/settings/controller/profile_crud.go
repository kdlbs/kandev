package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

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
	displayName, err := c.resolveDisplayName(agentConfig, agent.Name)
	if err != nil {
		return nil, err
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
	result := dto.AgentDTO{
		ID:            agent.ID,
		Name:          agent.Name,
		WorkspaceID:   agent.WorkspaceID,
		SupportsMCP:   agent.SupportsMCP,
		MCPConfigPath: agent.MCPConfigPath,
		Profiles:      profileDTOs,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
	if agent.TUIConfig != nil {
		result.TUIConfig = &dto.TUIConfigDTO{
			Command:         agent.TUIConfig.Command,
			DisplayName:     agent.TUIConfig.DisplayName,
			Model:           agent.TUIConfig.Model,
			Description:     agent.TUIConfig.Description,
			CommandArgs:     agent.TUIConfig.CommandArgs,
			WaitForTerminal: agent.TUIConfig.WaitForTerminal,
		}
	}
	return result
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
