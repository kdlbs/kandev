package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestrate/models"
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

// CreateAgentInstance validates and creates a new agent instance via the filesystem.
func (s *Service) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentCreate(ctx, agent); err != nil {
		return err
	}
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
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
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now
	if err := s.cfgWriter.WriteAgent(defaultWorkspaceName, agent); err != nil {
		return fmt.Errorf("write agent: %w", err)
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

// AgentListFilter specifies optional filters for listing agents.
type AgentListFilter struct {
	Role      string
	Status    string
	ReportsTo string
}

// ListAgentInstancesFiltered returns agents matching the given filters from ConfigLoader.
func (s *Service) ListAgentInstancesFiltered(
	_ context.Context, _ string, filter AgentListFilter,
) ([]*models.AgentInstance, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	all := s.cfgLoader.GetAgents(defaultWorkspaceName)
	return filterAgents(all, filter), nil
}

func filterAgents(agents []*models.AgentInstance, f AgentListFilter) []*models.AgentInstance {
	var result []*models.AgentInstance
	for _, a := range agents {
		if f.Role != "" && string(a.Role) != f.Role {
			continue
		}
		if f.Status != "" && string(a.Status) != f.Status {
			continue
		}
		if f.ReportsTo != "" && a.ReportsTo != f.ReportsTo {
			continue
		}
		result = append(result, a)
	}
	if result == nil {
		result = []*models.AgentInstance{}
	}
	return result
}

// UpdateAgentInstance validates and updates an existing agent instance via the filesystem.
func (s *Service) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentUpdate(ctx, agent); err != nil {
		return err
	}
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	agent.UpdatedAt = time.Now().UTC()
	if err := s.cfgWriter.WriteAgent(defaultWorkspaceName, agent); err != nil {
		return fmt.Errorf("write agent: %w", err)
	}
	return nil
}

// UpdateAgentStatus validates a status transition and updates the agent in-memory.
// Status is runtime state -- not persisted to YAML, reconstructable on restart.
func (s *Service) UpdateAgentStatus(
	_ context.Context, id string, newStatus models.AgentStatus, pauseReason string,
) (*models.AgentInstance, error) {
	agent, err := s.getAgentFromCacheMutable(id)
	if err != nil {
		return nil, err
	}
	if err := validateStatusTransition(agent.Status, newStatus); err != nil {
		return nil, err
	}
	agent.Status = newStatus
	agent.PauseReason = pauseReason
	return agent, nil
}

// getAgentFromCacheMutable returns a pointer to the agent in the ConfigLoader cache.
// The caller can mutate runtime fields (status, pause reason) directly.
func (s *Service) getAgentFromCacheMutable(id string) (*models.AgentInstance, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		if a.ID == id || a.Name == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", id)
}

// DeleteAgentInstance deletes an agent instance from the filesystem.
func (s *Service) DeleteAgentInstance(_ context.Context, id string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	agent, err := s.GetAgentFromConfig(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.cfgWriter.DeleteAgent(defaultWorkspaceName, agent.Name); err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

// validateAgentCreate checks all business rules for creating an agent.
func (s *Service) validateAgentCreate(_ context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		if s.countAgentsByRole(models.AgentRoleCEO, "") > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.Name != "" {
		if err := s.validateAgentNameUnique(agent.Name, ""); err != nil {
			return err
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(agent.ReportsTo, "")
	}
	return nil
}

// validateAgentUpdate checks business rules for updating an agent.
func (s *Service) validateAgentUpdate(_ context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		if s.countAgentsByRole(models.AgentRoleCEO, agent.ID) > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(agent.ReportsTo, agent.ID)
	}
	return nil
}

// countAgentsByRole counts agents with a role, optionally excluding one ID.
func (s *Service) countAgentsByRole(role models.AgentRole, excludeID string) int {
	if s.cfgLoader == nil {
		return 0
	}
	count := 0
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		if a.Role == role && a.ID != excludeID {
			count++
		}
	}
	return count
}

// validateAgentNameUnique ensures no other agent has the same name.
func (s *Service) validateAgentNameUnique(name, excludeID string) error {
	if s.cfgLoader == nil {
		return nil
	}
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		if a.Name == name && a.ID != excludeID {
			return fmt.Errorf("agent name %q already exists", name)
		}
	}
	return nil
}

// validateReportsTo ensures the target agent exists.
func (s *Service) validateReportsTo(reportsTo, selfID string) error {
	if selfID != "" && reportsTo == selfID {
		return ErrAgentReportsToSelf
	}
	_, err := s.GetAgentFromConfig(context.Background(), reportsTo)
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
