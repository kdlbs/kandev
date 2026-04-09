package controller

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

type CreateProfileRequest struct {
	AgentID        string
	Name           string
	Model          string
	Mode           string
	AllowIndexing  bool
	CLIPassthrough bool
}

func (c *Controller) CreateProfile(ctx context.Context, req CreateProfileRequest) (*dto.AgentProfileDTO, error) {
	// Model is optional — the profile reconciler fills it from the host
	// utility probe cache on boot, and session start applies it via
	// session/set_model. An empty model means "use the agent's default".
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
		AgentID:          req.AgentID,
		Name:             req.Name,
		AgentDisplayName: displayName,
		Model:            req.Model,
		Mode:             req.Mode,
		AllowIndexing:    req.AllowIndexing,
		CLIPassthrough:   req.CLIPassthrough,
		UserModified:     true,
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

type UpdateProfileRequest struct {
	ID             string
	Name           *string
	Model          *string
	Mode           *string
	AllowIndexing  *bool
	CLIPassthrough *bool
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
		if req.Name == nil {
			if newName := c.resolveProfileNameForModel(ctx, profile.AgentID, *req.Model); newName != "" {
				profile.Name = newName
			}
		}
	}
	if req.Mode != nil {
		profile.Mode = *req.Mode
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

func (c *Controller) DeleteProfile(ctx context.Context, id string, force bool) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "agent profile not found") {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	if err := c.prepareProfileDeletion(ctx, id, force); err != nil {
		return nil, err
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

// prepareProfileDeletion checks for active sessions and cleans up ephemeral tasks before deletion.
func (c *Controller) prepareProfileDeletion(ctx context.Context, profileID string, force bool) error {
	if c.sessionChecker == nil {
		return nil
	}
	// Check for active non-ephemeral sessions (unless force is true)
	// This allows the UI to show a confirmation dialog with affected tasks
	if !force {
		activeTasks, err := c.sessionChecker.GetActiveTaskInfoByAgentProfile(ctx, profileID)
		if err != nil {
			return err
		}
		if len(activeTasks) > 0 {
			return &ErrProfileInUseDetail{ActiveSessions: activeTasks}
		}
	}
	// Clean up ephemeral tasks (quick chat, config chat) using this profile
	// Done after the force check since these don't need user confirmation
	c.cleanupEphemeralTasks(ctx, profileID)
	return nil
}

// cleanupEphemeralTasks removes ephemeral tasks (quick chat, config chat) associated with a profile.
func (c *Controller) cleanupEphemeralTasks(ctx context.Context, profileID string) {
	if c.sessionChecker == nil {
		return
	}
	deleted, err := c.sessionChecker.DeleteEphemeralTasksByAgentProfile(ctx, profileID)
	if err != nil {
		c.logger.Warn("failed to delete ephemeral tasks for profile",
			zap.String("profile_id", profileID), zap.Error(err))
		return
	}
	if deleted > 0 {
		c.logger.Info("deleted ephemeral tasks for profile deletion",
			zap.String("profile_id", profileID), zap.Int64("count", deleted))
	}
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
		ID:               profile.ID,
		AgentID:          profile.AgentID,
		Name:             profile.Name,
		AgentDisplayName: profile.AgentDisplayName,
		Model:            profile.Model,
		Mode:             profile.Mode,
		AllowIndexing:    profile.AllowIndexing,
		CLIPassthrough:   profile.CLIPassthrough,
		UserModified:     profile.UserModified,
		CreatedAt:        profile.CreatedAt,
		UpdatedAt:        profile.UpdatedAt,
	}
}

// resolveProfileNameForModel looks up the agent by ID, fetches its model list (using cache),
// and returns the display name for the given model ID. Returns empty string on failure.
func (c *Controller) resolveProfileNameForModel(ctx context.Context, agentID, modelID string) string {
	agent, err := c.repo.GetAgent(ctx, agentID)
	if err != nil {
		return ""
	}
	if _, ok := c.agentRegistry.Get(agent.Name); !ok {
		return ""
	}

	// Look up the model's display name from the host utility capability
	// cache. If the cache isn't populated yet (probes not finished, agent
	// not probed) we fall through to the raw model ID — better than
	// blocking the save.
	if c.hostUtility != nil {
		if caps, ok := c.hostUtility.Get(agent.Name); ok {
			for _, m := range caps.Models {
				if m.ID == modelID {
					return m.Name
				}
			}
		}
	}
	return modelID
}
