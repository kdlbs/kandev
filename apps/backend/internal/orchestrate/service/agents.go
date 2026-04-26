package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

// Orchestrate event types (local to service; events/types.go is not modified).
const (
	EventAgentCreated       = "orchestrate.agent.created"
	EventAgentUpdated       = "orchestrate.agent.updated"
	EventAgentStatusChanged = "orchestrate.agent.status_changed"
)

// Sentinel errors for agent validation.
var (
	ErrAgentNameRequired     = errors.New("agent name is required")
	ErrAgentRoleInvalid      = errors.New("invalid agent role")
	ErrAgentCEOAlreadyExists = errors.New("workspace already has a CEO agent")
	ErrAgentReportsToInvalid = errors.New("reports_to agent does not exist in this workspace")
	ErrAgentReportsToSelf    = errors.New("agent cannot report to itself")
	ErrAgentStatusTransition = errors.New("invalid status transition")
)

// validRoles enumerates accepted roles.
var validRoles = map[models.AgentRole]bool{
	models.AgentRoleCEO:        true,
	models.AgentRoleWorker:     true,
	models.AgentRoleSpecialist: true,
	models.AgentRoleAssistant:  true,
}

// allowedTransitions defines which status transitions are valid.
var allowedTransitions = map[models.AgentStatus][]models.AgentStatus{
	models.AgentStatusIdle:            {models.AgentStatusWorking, models.AgentStatusPaused, models.AgentStatusStopped},
	models.AgentStatusWorking:         {models.AgentStatusIdle, models.AgentStatusPaused, models.AgentStatusStopped},
	models.AgentStatusPaused:          {models.AgentStatusIdle, models.AgentStatusStopped},
	models.AgentStatusStopped:         {models.AgentStatusIdle},
	models.AgentStatusPendingApproval: {models.AgentStatusIdle, models.AgentStatusStopped},
}

// DefaultPermissions returns the default permissions JSON for a role.
func DefaultPermissions(role models.AgentRole) string {
	perms := defaultPermsForRole(role)
	b, _ := json.Marshal(perms)
	return string(b)
}

func defaultPermsForRole(role models.AgentRole) map[string]interface{} {
	switch role {
	case models.AgentRoleCEO:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     true,
			"can_approve":           true,
			"can_manage_own_skills": true,
			"max_subtask_depth":     3,
		}
	case models.AgentRoleAssistant:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": true,
			"max_subtask_depth":     1,
		}
	default: // worker, specialist
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      false,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": false,
			"max_subtask_depth":     1,
		}
	}
}

// CreateAgentInstance validates and creates a new agent instance.
func (s *Service) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentCreate(ctx, agent); err != nil {
		return err
	}
	if agent.Permissions == "" || agent.Permissions == "{}" {
		agent.Permissions = DefaultPermissions(agent.Role)
	}
	if agent.MaxConcurrentSessions < 1 {
		agent.MaxConcurrentSessions = 1
	}
	if agent.CooldownSec <= 0 {
		agent.CooldownSec = 10
	}
	if agent.DesiredSkills == "" {
		agent.DesiredSkills = "[]"
	}
	if agent.ExecutorPreference == "" {
		agent.ExecutorPreference = "{}"
	}
	return s.repo.CreateAgentInstance(ctx, agent)
}

// GetAgentInstance returns an agent instance by ID.
func (s *Service) GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error) {
	return s.repo.GetAgentInstance(ctx, id)
}

// ListAgentInstances returns all agent instances for a workspace.
func (s *Service) ListAgentInstances(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstances(ctx, workspaceID)
}

// ListAgentInstancesFiltered returns filtered agents.
func (s *Service) ListAgentInstancesFiltered(
	ctx context.Context, workspaceID string, filter sqlite.AgentListFilter,
) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstancesFiltered(ctx, workspaceID, filter)
}

// UpdateAgentInstance validates and updates an existing agent instance.
func (s *Service) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentUpdate(ctx, agent); err != nil {
		return err
	}
	return s.repo.UpdateAgentInstance(ctx, agent)
}

// UpdateAgentStatus validates a status transition and updates the agent.
func (s *Service) UpdateAgentStatus(
	ctx context.Context, id string, newStatus models.AgentStatus, pauseReason string,
) (*models.AgentInstance, error) {
	agent, err := s.repo.GetAgentInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateStatusTransition(agent.Status, newStatus); err != nil {
		return nil, err
	}
	agent.Status = newStatus
	agent.PauseReason = pauseReason
	if err := s.repo.UpdateAgentInstance(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// DeleteAgentInstance deletes an agent instance.
func (s *Service) DeleteAgentInstance(ctx context.Context, id string) error {
	return s.repo.DeleteAgentInstance(ctx, id)
}

// validateAgentCreate checks all business rules for creating an agent.
func (s *Service) validateAgentCreate(ctx context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		count, err := s.repo.CountAgentInstancesByRole(ctx, agent.WorkspaceID, models.AgentRoleCEO)
		if err != nil {
			return fmt.Errorf("checking CEO count: %w", err)
		}
		if count > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.WorkspaceID, agent.ReportsTo, "")
	}
	return nil
}

// validateAgentUpdate checks business rules for updating an agent.
func (s *Service) validateAgentUpdate(ctx context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		count, err := s.repo.CountAgentInstancesByRoleExcluding(
			ctx, agent.WorkspaceID, models.AgentRoleCEO, agent.ID)
		if err != nil {
			return fmt.Errorf("checking CEO count: %w", err)
		}
		if count > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.WorkspaceID, agent.ReportsTo, agent.ID)
	}
	return nil
}

// validateReportsTo ensures the target agent exists in the same workspace.
func (s *Service) validateReportsTo(ctx context.Context, wsID, reportsTo, selfID string) error {
	if selfID != "" && reportsTo == selfID {
		return ErrAgentReportsToSelf
	}
	parent, err := s.repo.GetAgentInstance(ctx, reportsTo)
	if err != nil {
		return ErrAgentReportsToInvalid
	}
	if parent.WorkspaceID != wsID {
		return ErrAgentReportsToInvalid
	}
	return nil
}

// validateStatusTransition checks if a status transition is allowed.
func validateStatusTransition(from, to models.AgentStatus) error {
	if from == to {
		return nil
	}
	allowed, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrAgentStatusTransition, from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: cannot transition from %q to %q", ErrAgentStatusTransition, from, to)
}
