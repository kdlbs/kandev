package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/store"
	"github.com/kandev/kandev/internal/utility/template"
)

var (
	ErrAgentNotFound = errors.New("utility agent not found")
	ErrInvalidAgent  = errors.New("invalid utility agent")
	ErrCallNotFound  = errors.New("utility agent call not found")
	ErrBuiltinAgent  = errors.New("cannot modify built-in agent")
)

// Service provides business logic for utility agents.
type Service struct {
	repo           store.Repository
	templateEngine *template.Engine
}

// NewService creates a new utility agents service.
func NewService(repo store.Repository) *Service {
	return &Service{
		repo:           repo,
		templateEngine: template.NewEngine(),
	}
}

// ListAgents returns all utility agents.
func (s *Service) ListAgents(ctx context.Context) ([]*models.UtilityAgent, error) {
	return s.repo.ListAgents(ctx)
}

// GetAgentByID returns a utility agent by ID.
func (s *Service) GetAgentByID(ctx context.Context, id string) (*models.UtilityAgent, error) {
	agent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return agent, nil
}

// GetAgentByName returns a utility agent by name.
func (s *Service) GetAgentByName(ctx context.Context, name string) (*models.UtilityAgent, error) {
	agent, err := s.repo.GetAgentByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return agent, nil
}

// CreateAgent creates a new utility agent.
func (s *Service) CreateAgent(ctx context.Context, name, description, prompt, agentID, model string) (*models.UtilityAgent, error) {
	name = strings.TrimSpace(name)
	prompt = strings.TrimSpace(prompt)
	agentID = strings.TrimSpace(agentID)
	model = strings.TrimSpace(model)

	if name == "" || prompt == "" || agentID == "" || model == "" {
		return nil, ErrInvalidAgent
	}

	agent := &models.UtilityAgent{
		Name:        name,
		Description: description,
		Prompt:      prompt,
		AgentID:     agentID,
		Model:       model,
		Builtin:     false,
	}

	if err := s.repo.CreateAgent(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// UpdateAgent updates an existing utility agent.
func (s *Service) UpdateAgent(ctx context.Context, id string, name, description, prompt, agentID, model *string, enabled *bool) (*models.UtilityAgent, error) {
	agent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		return nil, ErrAgentNotFound
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, ErrInvalidAgent
		}
		agent.Name = trimmed
	}
	if description != nil {
		agent.Description = strings.TrimSpace(*description)
	}
	if prompt != nil {
		trimmed := strings.TrimSpace(*prompt)
		if trimmed == "" {
			return nil, ErrInvalidAgent
		}
		agent.Prompt = trimmed
	}
	if agentID != nil {
		agent.AgentID = strings.TrimSpace(*agentID)
	}
	if model != nil {
		agent.Model = strings.TrimSpace(*model)
	}
	if enabled != nil {
		// Allow enabling even with empty agent_id/model - defaults can be used at execution time.
		// For custom (non-builtin) agents, we still require agent_id and model.
		if *enabled && !agent.Builtin && (agent.AgentID == "" || agent.Model == "") {
			return nil, ErrInvalidAgent
		}
		agent.Enabled = *enabled
	}

	if err := s.repo.UpdateAgent(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// DeleteAgent deletes a utility agent.
func (s *Service) DeleteAgent(ctx context.Context, id string) error {
	agent, err := s.repo.GetAgentByID(ctx, id)
	if err != nil {
		return ErrAgentNotFound
	}
	if agent.Builtin {
		return ErrBuiltinAgent
	}
	return s.repo.DeleteAgent(ctx, id)
}

// ResolvePrompt resolves template variables in the agent's prompt.
func (s *Service) ResolvePrompt(ctx context.Context, utilityID string, tmplCtx *template.Context) (string, error) {
	agent, err := s.repo.GetAgentByID(ctx, utilityID)
	if err != nil {
		return "", ErrAgentNotFound
	}
	return s.templateEngine.Resolve(agent.Prompt, tmplCtx)
}

// GetAvailableVariables returns the list of available template variables.
func (s *Service) GetAvailableVariables() []template.VariableInfo {
	return s.templateEngine.AvailableVariables()
}

// DefaultUtilitySettings contains the user's default utility agent/model settings.
type DefaultUtilitySettings struct {
	AgentID string
	Model   string
}

// PreparePromptRequest prepares a prompt request by resolving the template.
// If the utility agent has empty AgentID/Model, the defaults are used.
// When sessionless is true, missing template variables are substituted with
// empty strings instead of being left as {{Var}}.
func (s *Service) PreparePromptRequest(ctx context.Context, utilityID string, tmplCtx *template.Context, defaults *DefaultUtilitySettings, sessionless bool) (*PromptRequest, error) {
	agent, err := s.repo.GetAgentByID(ctx, utilityID)
	if err != nil {
		return nil, ErrAgentNotFound
	}

	// Resolve template
	resolvedPrompt, err := s.templateEngine.ResolveWithOptions(agent.Prompt, tmplCtx, template.ResolveOptions{
		MissingAsEmpty: sessionless,
	})
	if err != nil {
		return nil, err
	}

	// Use agent's configured values, fall back to defaults if empty
	agentCLI := agent.AgentID
	model := agent.Model
	if agentCLI == "" && defaults != nil {
		agentCLI = defaults.AgentID
	}
	if model == "" && defaults != nil {
		model = defaults.Model
	}

	return &PromptRequest{
		UtilityID:      utilityID,
		ResolvedPrompt: resolvedPrompt,
		AgentCLI:       agentCLI,
		Model:          model,
	}, nil
}

// PromptRequest contains the prepared request for executing a utility prompt.
type PromptRequest struct {
	UtilityID      string
	ResolvedPrompt string
	AgentCLI       string // The inference agent ID (e.g., "claude-code", "amp")
	Model          string // The model to use
}

// CreateCall creates a new call record (for tracking history).
func (s *Service) CreateCall(ctx context.Context, utilityID, sessionID, resolvedPrompt, model string) (*models.UtilityAgentCall, error) {
	call := &models.UtilityAgentCall{
		UtilityID:      utilityID,
		SessionID:      sessionID,
		ResolvedPrompt: resolvedPrompt,
		Model:          model,
		Status:         "pending",
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.repo.CreateCall(ctx, call); err != nil {
		return nil, err
	}
	return call, nil
}

// CompleteCall marks a call as completed with the response.
func (s *Service) CompleteCall(ctx context.Context, callID, response string, promptTokens, responseTokens, durationMs int) error {
	call, err := s.repo.GetCallByID(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	now := time.Now().UTC()
	call.Response = response
	call.PromptTokens = promptTokens
	call.ResponseTokens = responseTokens
	call.DurationMs = durationMs
	call.Status = "completed"
	call.CompletedAt = &now
	return s.repo.UpdateCall(ctx, call)
}

// FailCall marks a call as failed with an error message.
func (s *Service) FailCall(ctx context.Context, callID, errorMessage string, durationMs int) error {
	call, err := s.repo.GetCallByID(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	now := time.Now().UTC()
	call.ErrorMessage = errorMessage
	call.DurationMs = durationMs
	call.Status = "failed"
	call.CompletedAt = &now
	return s.repo.UpdateCall(ctx, call)
}

// ListCalls returns the call history for a utility agent.
func (s *Service) ListCalls(ctx context.Context, utilityID string, limit int) ([]*models.UtilityAgentCall, error) {
	return s.repo.ListCalls(ctx, utilityID, limit)
}
