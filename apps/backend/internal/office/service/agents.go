package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"

	"go.uber.org/zap"
)

// Office event types (local to service; events/types.go is not modified).
const (
	EventAgentCreated       = "office.agent.created"
	EventAgentUpdated       = "office.agent.updated"
	EventAgentStatusChanged = "office.agent.status_changed"
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
	case models.AgentRoleWorker:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": false,
			"max_subtask_depth":     1,
		}
	default: // specialist and any unknown roles
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

// CreateAgentInstance validates and creates a new agent instance in the DB.
func (s *Service) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentCreate(ctx, agent); err != nil {
		return err
	}
	if agent.ID == "" {
		agent.ID = uuid.New().String()
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
	if agent.Status == "" {
		agent.Status = models.AgentStatusIdle
	}
	// Apply role-based default: CEO defaults to false (proactive coordination),
	// all other roles default to true (skip idle heartbeats).
	// Always set so new agents get the correct default regardless of caller input.
	agent.SkipIdleRuns = agent.Role != models.AgentRoleCEO
	if err := s.repo.CreateAgentInstance(ctx, agent); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	if err := s.CreateDefaultInstructions(ctx, agent.ID, string(agent.Role)); err != nil {
		s.logger.Warn("failed to create default instructions", zap.Error(err))
	}
	return nil
}

// GetAgentInstance returns an agent instance by ID from the ConfigLoader.
func (s *Service) GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error) {
	return s.GetAgentFromConfig(ctx, id)
}

// ListAgentInstances returns all agent instances for a workspace from the ConfigLoader.
func (s *Service) ListAgentInstances(ctx context.Context, wsID string) ([]*models.AgentInstance, error) {
	return s.ListAgentsFromConfig(ctx, wsID)
}

// ListAgentInstancesByIDs returns agent instances whose ids are in `ids`.
func (s *Service) ListAgentInstancesByIDs(ctx context.Context, ids []string) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstancesByIDs(ctx, ids)
}

// AgentListFilter specifies optional filters for listing agents.
type AgentListFilter struct {
	Role      string
	Status    string
	ReportsTo string
}

// ListAgentInstancesFiltered returns agents matching the given filters from the DB.
func (s *Service) ListAgentInstancesFiltered(
	ctx context.Context, workspaceID string, filter AgentListFilter,
) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstancesFiltered(ctx, workspaceID, sqlite.AgentListFilter{
		Role:      filter.Role,
		Status:    filter.Status,
		ReportsTo: filter.ReportsTo,
	})
}

// UpdateAgentInstance validates and updates an existing agent instance in the DB.
func (s *Service) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentUpdate(ctx, agent); err != nil {
		return err
	}
	if err := s.repo.UpdateAgentInstance(ctx, agent); err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

// UpdateAgentStatusFields persists a status + pause reason without
// running the transition validator. Satisfies shared.AgentWriter so
// the office service can stand in for agents.AgentService in tests
// that exercise budget-driven agent pause paths.
func (s *Service) UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error {
	return s.repo.UpdateAgentStatusFields(ctx, agentID, status, pauseReason)
}

// UpdateAgentStatus validates a status transition and persists the new state to the DB.
func (s *Service) UpdateAgentStatus(
	ctx context.Context, id string, newStatus models.AgentStatus, pauseReason string,
) (*models.AgentInstance, error) {
	agent, err := s.GetAgentFromConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateStatusTransition(agent.Status, newStatus); err != nil {
		return nil, err
	}
	if dbErr := s.repo.UpdateAgentStatusFields(ctx, agent.ID, string(newStatus), pauseReason); dbErr != nil {
		return nil, fmt.Errorf("persist agent status: %w", dbErr)
	}
	agent.Status = newStatus
	agent.PauseReason = pauseReason
	return agent, nil
}

// DeleteAgentInstance deletes an agent instance from the DB.
func (s *Service) DeleteAgentInstance(ctx context.Context, id string) error {
	agent, err := s.GetAgentFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteAgentInstance(ctx, agent.ID); err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
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
		if s.countAgentsByRole(ctx, models.AgentRoleCEO, agent.WorkspaceID, "") > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if err := s.validateAgentNameUnique(ctx, agent.Name, agent.WorkspaceID, ""); err != nil {
		return err
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.ReportsTo, "")
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
		if s.countAgentsByRole(ctx, models.AgentRoleCEO, agent.WorkspaceID, agent.ID) > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.ReportsTo, agent.ID)
	}
	return nil
}

// countAgentsByRole counts agents with a role in a workspace, optionally excluding one ID.
func (s *Service) countAgentsByRole(ctx context.Context, role models.AgentRole, workspaceID, excludeID string) int {
	count, err := s.repo.CountAgentInstancesByRole(ctx, workspaceID, string(role), excludeID)
	if err != nil {
		return 0
	}
	return count
}

// validateAgentNameUnique ensures no other agent in the workspace has the same name.
func (s *Service) validateAgentNameUnique(ctx context.Context, name, workspaceID, excludeID string) error {
	exists, err := s.repo.AgentInstanceExistsByName(ctx, workspaceID, name, excludeID)
	if err != nil {
		return nil
	}
	if exists {
		return fmt.Errorf("agent name %q already exists", name)
	}
	return nil
}

// validateReportsTo ensures the target agent exists.
func (s *Service) validateReportsTo(ctx context.Context, reportsTo, selfID string) error {
	if selfID != "" && reportsTo == selfID {
		return ErrAgentReportsToSelf
	}
	_, err := s.GetAgentFromConfig(ctx, reportsTo)
	if err != nil {
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
