package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/profilebinding"
	"github.com/kandev/kandev/internal/utility/store"
	"github.com/kandev/kandev/internal/utility/template"
)

var (
	ErrAgentNotFound       = errors.New("utility agent not found")
	ErrInvalidAgent        = errors.New("invalid utility agent")
	ErrCallNotFound        = errors.New("utility agent call not found")
	ErrBuiltinAgent        = errors.New("cannot modify built-in agent")
	ErrProfileRequired     = errors.New("utility agent profile is required")
	ErrProfileUnconfigured = errors.New("utility agent profile is not configured")
	ErrExecutionRouting    = errors.New("utility execution routing is not configured")
)

type ProfileResolver interface {
	Resolve(ctx context.Context, id string) (*agentsettingsmodels.AgentProfile, error)
	MatchLegacy(ctx context.Context, agentID, model string) (*agentsettingsmodels.AgentProfile, error)
}

// ExecutionProfileResolver is an optional shared-router extension. It keeps
// the logical profile ID on the call while returning the concrete profile used
// for this one utility invocation.
type ExecutionProfileResolver interface {
	ResolveExecution(context.Context, string) (*agentsettingsmodels.AgentProfile, string, error)
}

// SessionExecutionProfileResolver is the session-aware form used by dynamic
// routing. The legacy interface remains supported for small callers and test
// doubles that only need a concrete profile lookup.
type SessionExecutionProfileResolver interface {
	ResolveExecutionForSession(context.Context, string, string) (*agentsettingsmodels.AgentProfile, string, error)
}

// ExecutionDetailsResolver exposes route identity to callers that may need to
// apply a classified pre-result failure to the same dynamic route.
type ExecutionDetailsResolver interface {
	ResolveExecutionDetails(context.Context, string, string) (agentruntime.ProfileExecution, error)
}

// ExecutionFailureResolver advances a dynamic route after a classified
// failure that occurred before any user-visible result was produced.
type ExecutionFailureResolver interface {
	ResolveExecutionAfterFailure(context.Context, string, string, string, int64, *routingerr.Error) (agentruntime.ProfileExecution, error)
}

// Service provides business logic for utility agents.
type Service struct {
	repo              store.Repository
	templateEngine    *template.Engine
	profileResolver   ProfileResolver
	executionResolver ExecutionProfileResolver
}

// SetProfileResolver wires the operator-owned profile eligibility boundary.
func (s *Service) SetProfileResolver(resolver ProfileResolver) {
	s.profileResolver = resolver
}

func (s *Service) SetExecutionProfileResolver(resolver ExecutionProfileResolver) {
	s.executionResolver = resolver
}

// ResolveExecutionAfterFailure advances a shared dynamic route after a
// classified pre-result utility failure.
func (s *Service) ResolveExecutionAfterFailure(
	ctx context.Context,
	sessionID, profileID, currentExecutionProfileID string,
	expectedGeneration int64,
	failure *routingerr.Error,
) (agentruntime.ProfileExecution, error) {
	resolver, ok := s.executionResolver.(ExecutionFailureResolver)
	if !ok {
		return agentruntime.ProfileExecution{}, ErrExecutionRouting
	}
	return resolver.ResolveExecutionAfterFailure(ctx, sessionID, profileID, currentExecutionProfileID, expectedGeneration, failure)
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

// ClearAgentProfileBindings marks utility agents that reference a deleted
// profile as unconfigured. The stale profile ID is retained so forced profile
// deletion remains diagnosable and fail-closed.
func (s *Service) ClearAgentProfileBindings(ctx context.Context, profileID string) error {
	agents, err := s.repo.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent == nil || agent.AgentProfileID != profileID {
			continue
		}
		agent.ProfileBindingState = models.ProfileBindingUnconfigured
		if err := s.repo.UpdateAgent(ctx, agent); err != nil {
			return err
		}
	}
	return nil
}

// MigrateLegacyBindings upgrades old agent/model selections after profile
// reconciliation. The operation is idempotent and leaves explicit bindings
// untouched. An empty unconfigured built-in is normalized to inherit because
// no concrete override remains to preserve.
func (s *Service) MigrateLegacyBindings(ctx context.Context) (int, error) {
	if s.profileResolver == nil {
		return 0, nil
	}
	agents, err := s.repo.ListAgents(ctx)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, agent := range agents {
		// Skip already-inherited rows and any row that still carries a concrete
		// profile ID. The latter includes stale/unconfigured overrides whose
		// original intent must remain available for explicit user repair.
		if agent == nil || agent.AgentProfileID != "" ||
			agent.ProfileBindingState == models.ProfileBindingInherit {
			continue
		}
		if agent.ProfileBindingState == models.ProfileBindingUnconfigured {
			if !agent.Builtin {
				continue
			}
			changed, err := s.repo.NormalizeEmptyBuiltinBinding(ctx, agent.ID)
			if err != nil {
				return updated, err
			}
			if changed {
				updated++
			}
			continue
		}
		profile, matchErr := s.profileResolver.MatchLegacy(ctx, agent.AgentID, agent.Model)
		switch {
		case matchErr == nil && profile != nil:
			agent.AgentProfileID = profile.ID
			agent.ProfileBindingState = models.ProfileBindingExplicit
		case errors.Is(matchErr, profilebinding.ErrLegacyBindingAmbiguous):
			if agent.Builtin {
				agent.ProfileBindingState = models.ProfileBindingInherit
			} else {
				agent.ProfileBindingState = models.ProfileBindingUnconfigured
			}
		case matchErr != nil:
			return updated, matchErr
		default:
			if agent.Builtin {
				agent.ProfileBindingState = models.ProfileBindingInherit
			} else {
				agent.ProfileBindingState = models.ProfileBindingUnconfigured
			}
		}
		if err := s.repo.UpdateAgent(ctx, agent); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
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
func (s *Service) CreateAgent(ctx context.Context, name, description, prompt, agentID, model, profileID, bindingState string) (*models.UtilityAgent, error) {
	name = strings.TrimSpace(name)
	prompt = strings.TrimSpace(prompt)
	agentID = strings.TrimSpace(agentID)
	model = strings.TrimSpace(model)

	profileID = strings.TrimSpace(profileID)
	bindingState = strings.TrimSpace(bindingState)
	if name == "" || prompt == "" {
		return nil, ErrInvalidAgent
	}
	if s.profileResolver != nil {
		if profileID == "" {
			return nil, ErrProfileRequired
		}
		if _, err := s.profileResolver.Resolve(ctx, profileID); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidAgent, err)
		}
		bindingState = models.ProfileBindingExplicit
	} else if agentID == "" || model == "" {
		return nil, ErrInvalidAgent
	}

	agent := &models.UtilityAgent{
		Name:                name,
		Description:         description,
		Prompt:              prompt,
		AgentID:             agentID,
		Model:               model,
		AgentProfileID:      profileID,
		ProfileBindingState: bindingState,
		Builtin:             false,
		// Custom agents validate agent_id + model above, so they're always
		// configured and ready to run at creation time. The DB's NOT NULL
		// column persists the field value (not the schema default), so we
		// have to set it here — otherwise new custom agents land disabled
		// and never appear as runnable in the utility-agent pickers.
		Enabled: true,
	}

	if err := s.repo.CreateAgent(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// UpdateAgent updates an existing utility agent.
//
//nolint:cyclop // each optional update field has independent validation.
func (s *Service) UpdateAgent(ctx context.Context, id string, name, description, prompt, agentID, model, profileID, bindingState *string, enabled *bool) (*models.UtilityAgent, error) {
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
	if profileID != nil {
		agent.AgentProfileID = strings.TrimSpace(*profileID)
	}
	if bindingState != nil {
		agent.ProfileBindingState = strings.TrimSpace(*bindingState)
	}
	if s.profileResolver != nil {
		if agent.Builtin && agent.ProfileBindingState == models.ProfileBindingInherit {
			agent.AgentProfileID = ""
		} else if agent.AgentProfileID == "" {
			return nil, ErrProfileRequired
		} else if _, err := s.profileResolver.Resolve(ctx, agent.AgentProfileID); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidAgent, err)
		} else {
			agent.ProfileBindingState = models.ProfileBindingExplicit
		}
	}
	if enabled != nil {
		// Allow enabling even with empty agent_id/model - defaults can be used at execution time.
		// For custom (non-builtin) agents, we still require agent_id and model.
		if *enabled && !agent.Builtin && s.profileResolver == nil && (agent.AgentID == "" || agent.Model == "") {
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
	AgentID   string
	Model     string
	ProfileID string
}

// PreparePromptRequest prepares a prompt request by resolving the template.
// If the utility agent has no Model, the default AgentID/Model pair is used.
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

	if s.profileResolver != nil {
		var profileID string
		switch {
		case models.UsesDefaultProfile(agent):
			if defaults != nil {
				profileID = defaults.ProfileID
			}
		case agent.ProfileBindingState == models.ProfileBindingUnconfigured:
			return nil, ErrProfileUnconfigured
		case agent.ProfileBindingState == models.ProfileBindingInherit:
			// inherit with a non-empty profile ID is an inconsistent state that
			// UpdateAgent prevents; fail closed rather than silently resolving it.
			return nil, ErrProfileRequired
		default:
			profileID = agent.AgentProfileID
		}
		if profileID == "" {
			return nil, ErrProfileRequired
		}
		profile, err := s.profileResolver.Resolve(ctx, profileID)
		if err != nil {
			return nil, err
		}
		logicalProfileID := profile.ID
		executionProfileID := logicalProfileID
		routeSessionID := ""
		routeGeneration := int64(0)
		if s.executionResolver != nil {
			var resolved *agentsettingsmodels.AgentProfile
			var concreteID string
			var resolveErr error
			if detailsResolver, ok := s.executionResolver.(ExecutionDetailsResolver); ok {
				routeSessionID = "utility:" + uuid.NewString()
				details, detailsErr := detailsResolver.ResolveExecutionDetails(ctx, routeSessionID, profileID)
				if detailsErr != nil {
					return nil, detailsErr
				}
				resolved = details.Profile
				concreteID = details.ExecutionProfileID
				if details.RouteSessionID != "" {
					routeSessionID = details.RouteSessionID
				}
				routeGeneration = details.Generation
			} else if sessionResolver, ok := s.executionResolver.(SessionExecutionProfileResolver); ok {
				routeSessionID = "utility:" + uuid.NewString()
				resolved, concreteID, resolveErr = sessionResolver.ResolveExecutionForSession(ctx, routeSessionID, profileID)
			} else {
				resolved, concreteID, resolveErr = s.executionResolver.ResolveExecution(ctx, profileID)
			}
			if resolveErr != nil {
				return nil, resolveErr
			}
			if resolved != nil {
				profile = resolved
			}
			if concreteID != "" {
				executionProfileID = concreteID
			}
		}
		return &PromptRequest{
			UtilityID:          utilityID,
			ResolvedPrompt:     resolvedPrompt,
			AgentCLI:           profile.AgentID,
			Model:              profile.Model,
			AgentProfileID:     logicalProfileID,
			ExecutionProfileID: executionProfileID,
			RouteSessionID:     routeSessionID,
			RouteGeneration:    routeGeneration,
		}, nil
	}

	// Use agent-specific values when fully configured. If the model is empty,
	// treat the default agent/model as an inseparable pair so a default model
	// from one provider is never sent to another provider's ACP config.
	agentCLI := agent.AgentID
	model := agent.Model
	if defaults != nil {
		if model == "" {
			agentCLI = defaults.AgentID
			model = defaults.Model
		} else if agentCLI == "" {
			agentCLI = defaults.AgentID
		}
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
	UtilityID          string
	ResolvedPrompt     string
	AgentCLI           string // The inference agent ID (e.g., "claude-acp", "amp")
	Model              string // The model to use
	AgentProfileID     string
	ExecutionProfileID string
	RouteSessionID     string
	RouteGeneration    int64
}

// CreateCall creates a new call record (for tracking history).
func (s *Service) CreateCall(ctx context.Context, utilityID, sessionID, resolvedPrompt, model string, profileID ...string) (*models.UtilityAgentCall, error) {
	call := &models.UtilityAgentCall{
		UtilityID:      utilityID,
		SessionID:      sessionID,
		ResolvedPrompt: resolvedPrompt,
		Model:          model,
		Status:         "pending",
		CreatedAt:      time.Now().UTC(),
	}
	if len(profileID) > 0 {
		call.AgentProfileID = profileID[0]
	}
	if err := s.repo.CreateCall(ctx, call); err != nil {
		return nil, err
	}
	return call, nil
}

func (s *Service) CreateCallWithExecutionProfile(ctx context.Context, utilityID, sessionID, resolvedPrompt, model, logicalProfileID, executionProfileID string) (*models.UtilityAgentCall, error) {
	call, err := s.CreateCall(ctx, utilityID, sessionID, resolvedPrompt, model, logicalProfileID)
	if err != nil {
		return nil, err
	}
	call.ExecutionProfileID = executionProfileID
	if err := s.repo.UpdateCall(ctx, call); err != nil {
		return nil, err
	}
	return call, nil
}

// SetCallExecutionProfile updates the concrete profile attribution after a
// pre-result route fallback succeeds.
func (s *Service) SetCallExecutionProfile(ctx context.Context, callID, executionProfileID string) error {
	call, err := s.repo.GetCallByID(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	call.ExecutionProfileID = executionProfileID
	return s.repo.UpdateCall(ctx, call)
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
