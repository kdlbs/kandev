package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

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
	matched, err := c.findMatchedAvailability(req.Name, discoveryResults)
	if err != nil {
		return nil, err
	}
	agentConfig, agOk := c.agentRegistry.Get(req.Name)
	if !agOk {
		return nil, fmt.Errorf("unknown agent: %s", req.Name)
	}
	displayName, err := c.resolveDisplayName(agentConfig, req.Name)
	if err != nil {
		return nil, err
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
	profiles, err := c.createAgentProfiles(ctx, agent.ID, displayName, req.Profiles)
	if err != nil {
		return nil, err
	}
	result := toAgentDTO(agent, profiles)
	return &result, nil
}

func (c *Controller) findMatchedAvailability(name string, results []discovery.Availability) (*discovery.Availability, error) {
	for _, result := range results {
		if result.Name == name {
			if !result.Available {
				return nil, fmt.Errorf("agent not installed: %s", name)
			}
			r := result
			return &r, nil
		}
	}
	return nil, fmt.Errorf("unknown agent: %s", name)
}

func (c *Controller) createAgentProfiles(ctx context.Context, agentID, displayName string, profileReqs []CreateAgentProfileRequest) ([]*models.AgentProfile, error) {
	profiles := make([]*models.AgentProfile, 0, len(profileReqs))
	for _, profileReq := range profileReqs {
		profile := &models.AgentProfile{
			AgentID:                    agentID,
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
	return profiles, nil
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
	// If the agent has a tui_config, unregister from the in-memory registry
	agent, err := c.repo.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAgentNotFound
		}
		return err
	}
	if agent.TUIConfig != nil {
		_ = c.agentRegistry.Unregister(agent.Name)
	}

	if err := c.repo.DeleteAgent(ctx, id); err != nil {
		if strings.Contains(err.Error(), "agent not found") {
			return ErrAgentNotFound
		}
		return err
	}
	return nil
}
