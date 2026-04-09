package controller

import (
	"context"

	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/service"
	"github.com/kandev/kandev/internal/utility/template"
)

// Controller handles utility agent operations.
type Controller struct {
	svc *service.Service
}

// NewController creates a new utility agent controller.
func NewController(svc *service.Service) *Controller {
	return &Controller{svc: svc}
}

// ListAgents returns all utility agents.
func (c *Controller) ListAgents(ctx context.Context) (*dto.UtilityAgentsResponse, error) {
	agents, err := c.svc.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.UtilityAgentDTO, 0, len(agents))
	for _, agent := range agents {
		dtos = append(dtos, dto.FromUtilityAgent(agent))
	}

	return &dto.UtilityAgentsResponse{Agents: dtos}, nil
}

// GetAgent returns a utility agent by ID.
func (c *Controller) GetAgent(ctx context.Context, id string) (*dto.UtilityAgentDTO, error) {
	agent, err := c.svc.GetAgentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := dto.FromUtilityAgent(agent)
	return &result, nil
}

// CreateAgent creates a new utility agent.
func (c *Controller) CreateAgent(ctx context.Context, req dto.CreateUtilityAgentRequest) (*dto.UtilityAgentDTO, error) {
	agent, err := c.svc.CreateAgent(
		ctx,
		req.Name,
		req.Description,
		req.Prompt,
		req.AgentID,
		req.Model,
	)
	if err != nil {
		return nil, err
	}
	result := dto.FromUtilityAgent(agent)
	return &result, nil
}

// UpdateAgent updates an existing utility agent.
func (c *Controller) UpdateAgent(ctx context.Context, id string, req dto.UpdateUtilityAgentRequest) (*dto.UtilityAgentDTO, error) {
	agent, err := c.svc.UpdateAgent(
		ctx,
		id,
		req.Name,
		req.Description,
		req.Prompt,
		req.AgentID,
		req.Model,
		req.Enabled,
	)
	if err != nil {
		return nil, err
	}
	result := dto.FromUtilityAgent(agent)
	return &result, nil
}

// DeleteAgent deletes a utility agent.
func (c *Controller) DeleteAgent(ctx context.Context, id string) error {
	return c.svc.DeleteAgent(ctx, id)
}

// GetTemplateVariables returns the available template variables.
func (c *Controller) GetTemplateVariables(ctx context.Context) *dto.TemplateVariablesResponse {
	return &dto.TemplateVariablesResponse{
		Variables: c.svc.GetAvailableVariables(),
	}
}

// PreparePromptRequest prepares a prompt request by resolving the template.
// If defaults is provided and the utility agent has empty agent_id/model, the defaults are used.
func (c *Controller) PreparePromptRequest(ctx context.Context, req dto.ExecutePromptRequest, defaults *service.DefaultUtilitySettings, sessionless bool) (*service.PromptRequest, error) {
	tmplCtx := &template.Context{
		GitDiff:             req.GitDiff,
		CommitLog:           req.CommitLog,
		ChangedFiles:        req.ChangedFiles,
		DiffSummary:         req.DiffSummary,
		BranchName:          req.BranchName,
		BaseBranch:          req.BaseBranch,
		TaskTitle:           req.TaskTitle,
		TaskDescription:     req.TaskDescription,
		SessionID:           req.SessionID,
		UserPrompt:          req.UserPrompt,
		ConversationHistory: req.ConversationHistory,
	}
	return c.svc.PreparePromptRequest(ctx, req.UtilityAgentID, tmplCtx, defaults, sessionless)
}

// CreateCall creates a call record for tracking.
func (c *Controller) CreateCall(ctx context.Context, utilityID, sessionID, resolvedPrompt, model string) (string, error) {
	call, err := c.svc.CreateCall(ctx, utilityID, sessionID, resolvedPrompt, model)
	if err != nil {
		return "", err
	}
	return call.ID, nil
}

// CompleteCall marks a call as completed.
func (c *Controller) CompleteCall(ctx context.Context, callID, response string, promptTokens, responseTokens, durationMs int) error {
	return c.svc.CompleteCall(ctx, callID, response, promptTokens, responseTokens, durationMs)
}

// FailCall marks a call as failed.
func (c *Controller) FailCall(ctx context.Context, callID, errorMessage string, durationMs int) error {
	return c.svc.FailCall(ctx, callID, errorMessage, durationMs)
}

// ListCalls returns the call history for a utility agent.
func (c *Controller) ListCalls(ctx context.Context, utilityID string, limit int) (*dto.UtilityAgentCallsResponse, error) {
	calls, err := c.svc.ListCalls(ctx, utilityID, limit)
	if err != nil {
		return nil, err
	}

	dtos := make([]dto.UtilityAgentCallDTO, 0, len(calls))
	for _, call := range calls {
		dtos = append(dtos, dto.FromUtilityAgentCall(call))
	}

	return &dto.UtilityAgentCallsResponse{Calls: dtos}, nil
}
