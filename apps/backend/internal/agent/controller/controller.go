// Package controller provides the business logic coordination layer for the agent module.
package controller

import (
	"context"

	"github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
)

// Controller coordinates agent business logic
type Controller struct {
	lifecycle *lifecycle.Manager
	registry  *registry.Registry
}

// NewController creates a new agent controller
func NewController(lm *lifecycle.Manager, reg *registry.Registry) *Controller {
	return &Controller{
		lifecycle: lm,
		registry:  reg,
	}
}

// ListAgents returns all agent instances
func (c *Controller) ListAgents(ctx context.Context, req dto.ListAgentsRequest) (dto.ListAgentsResponse, error) {
	if c.lifecycle == nil {
		return dto.ListAgentsResponse{}, ErrLifecycleManagerNotAvailable
	}

	agents := c.lifecycle.ListInstances()
	resp := dto.ListAgentsResponse{
		Agents: make([]dto.AgentDTO, 0, len(agents)),
		Total:  len(agents),
	}

	for _, a := range agents {
		agent := dto.FromAgentInstance(&dto.AgentInstanceData{
			ID:           a.ID,
			TaskID:       a.TaskID,
			AgentType:    a.AgentType,
			ContainerID:  a.ContainerID,
			Status:       string(a.Status),
			Progress:     a.Progress,
			StartedAt:    a.StartedAt,
			FinishedAt:   a.FinishedAt,
			ExitCode:     a.ExitCode,
			ErrorMessage: a.ErrorMessage,
		})
		resp.Agents = append(resp.Agents, agent)
	}

	return resp, nil
}

// LaunchAgent launches a new agent instance
func (c *Controller) LaunchAgent(ctx context.Context, req dto.LaunchAgentRequest) (dto.LaunchAgentResponse, error) {
	if c.lifecycle == nil {
		return dto.LaunchAgentResponse{}, ErrLifecycleManagerNotAvailable
	}

	launchReq := &lifecycle.LaunchRequest{
		TaskID:        req.TaskID,
		AgentType:     req.AgentType,
		WorkspacePath: req.WorkspacePath,
		Env:           req.Env,
	}

	instance, err := c.lifecycle.Launch(ctx, launchReq)
	if err != nil {
		return dto.LaunchAgentResponse{}, err
	}

	return dto.LaunchAgentResponse{
		Success: true,
		AgentID: instance.ID,
		TaskID:  req.TaskID,
	}, nil
}

// GetAgentStatus returns the status of a specific agent
func (c *Controller) GetAgentStatus(ctx context.Context, req dto.GetAgentStatusRequest) (dto.AgentDTO, error) {
	if c.lifecycle == nil {
		return dto.AgentDTO{}, ErrLifecycleManagerNotAvailable
	}

	agent, found := c.lifecycle.GetInstance(req.AgentID)
	if !found {
		return dto.AgentDTO{}, ErrAgentNotFound
	}

	return dto.FromAgentInstance(&dto.AgentInstanceData{
		ID:           agent.ID,
		TaskID:       agent.TaskID,
		AgentType:    agent.AgentType,
		ContainerID:  agent.ContainerID,
		Status:       string(agent.Status),
		Progress:     agent.Progress,
		StartedAt:    agent.StartedAt,
		FinishedAt:   agent.FinishedAt,
		ExitCode:     agent.ExitCode,
		ErrorMessage: agent.ErrorMessage,
	}), nil
}

// GetAgentLogs returns logs for a specific agent
func (c *Controller) GetAgentLogs(ctx context.Context, req dto.GetAgentLogsRequest) (dto.GetAgentLogsResponse, error) {
	if c.lifecycle == nil {
		return dto.GetAgentLogsResponse{}, ErrLifecycleManagerNotAvailable
	}

	_, found := c.lifecycle.GetInstance(req.AgentID)
	if !found {
		return dto.GetAgentLogsResponse{}, ErrAgentNotFound
	}

	// Note: Log retrieval requires Docker client access which is not available here
	return dto.GetAgentLogsResponse{
		AgentID: req.AgentID,
		Logs:    []string{},
		Message: "Use HTTP API for full log retrieval",
	}, nil
}

// StopAgent stops a running agent
func (c *Controller) StopAgent(ctx context.Context, req dto.StopAgentRequest) (dto.SuccessResponse, error) {
	if c.lifecycle == nil {
		return dto.SuccessResponse{}, ErrLifecycleManagerNotAvailable
	}

	if err := c.lifecycle.StopAgent(ctx, req.AgentID, false); err != nil {
		return dto.SuccessResponse{}, err
	}

	return dto.SuccessResponse{Success: true}, nil
}

// ListAgentTypes returns all available agent types
func (c *Controller) ListAgentTypes(ctx context.Context, req dto.ListAgentTypesRequest) (dto.ListAgentTypesResponse, error) {
	if c.registry == nil {
		return dto.ListAgentTypesResponse{}, ErrRegistryNotAvailable
	}

	types := c.registry.List()
	resp := dto.ListAgentTypesResponse{
		Types: make([]dto.AgentTypeDTO, 0, len(types)),
		Total: len(types),
	}

	for _, t := range types {
		resp.Types = append(resp.Types, dto.FromAgentType(&dto.AgentTypeData{
			ID:           t.ID,
			Name:         t.Name,
			Description:  t.Description,
			Image:        t.Image,
			Capabilities: t.Capabilities,
			Enabled:      t.Enabled,
		}))
	}

	return resp, nil
}

