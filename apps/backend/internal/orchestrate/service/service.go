// Package service provides business logic for the orchestrate domain.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"

	"go.uber.org/zap"
)

// Service provides orchestrate business logic.
type Service struct {
	repo   *sqlite.Repository
	logger *logger.Logger
}

// NewService creates a new orchestrate service.
func NewService(repo *sqlite.Repository, log *logger.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "orchestrate-service")),
	}
}

// Agent instance methods (CRUD + validation + status transitions) are in agents.go.

// -- Skills --

// CreateSkill creates a new skill.
func (s *Service) CreateSkill(ctx context.Context, skill *models.Skill) error {
	return s.repo.CreateSkill(ctx, skill)
}

// GetSkill returns a skill by ID.
func (s *Service) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	return s.repo.GetSkill(ctx, id)
}

// ListSkills returns all skills for a workspace.
func (s *Service) ListSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error) {
	return s.repo.ListSkills(ctx, workspaceID)
}

// UpdateSkill updates a skill.
func (s *Service) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	return s.repo.UpdateSkill(ctx, skill)
}

// DeleteSkill deletes a skill.
func (s *Service) DeleteSkill(ctx context.Context, id string) error {
	return s.repo.DeleteSkill(ctx, id)
}

// -- Projects --

// CreateProject validates and creates a new project.
func (s *Service) CreateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if project.Status == "" {
		project.Status = models.ProjectStatusActive
	}
	if err := s.repo.CreateProject(ctx, project); err != nil {
		return err
	}
	s.logger.Info("project created",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// GetProject returns a project by ID.
func (s *Service) GetProject(ctx context.Context, id string) (*models.Project, error) {
	return s.repo.GetProject(ctx, id)
}

// ListProjects returns all projects for a workspace.
func (s *Service) ListProjects(ctx context.Context, workspaceID string) ([]*models.Project, error) {
	return s.repo.ListProjects(ctx, workspaceID)
}

// ListProjectsWithCounts returns all projects with aggregated task counts.
func (s *Service) ListProjectsWithCounts(ctx context.Context, workspaceID string) ([]*models.ProjectWithCounts, error) {
	return s.repo.ListProjectsWithCounts(ctx, workspaceID)
}

// GetTaskCounts returns task status counts for a project.
func (s *Service) GetTaskCounts(ctx context.Context, projectID string) (*models.TaskCounts, error) {
	return s.repo.GetTaskCounts(ctx, projectID)
}

// UpdateProject validates and updates a project.
func (s *Service) UpdateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if err := s.repo.UpdateProject(ctx, project); err != nil {
		return err
	}
	s.logger.Info("project updated",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// DeleteProject deletes a project.
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	return s.repo.DeleteProject(ctx, id)
}

func (s *Service) validateProject(project *models.Project) error {
	if project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if project.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}
	if project.Status != "" && !models.ValidProjectStatuses[project.Status] {
		return fmt.Errorf("invalid project status: %s", project.Status)
	}
	return validateRepositories(project.Repositories)
}

func validateRepositories(reposJSON string) error {
	if reposJSON == "" || reposJSON == "[]" {
		return nil
	}
	var repos []string
	if err := json.Unmarshal([]byte(reposJSON), &repos); err != nil {
		return fmt.Errorf("repositories must be a JSON array of strings: %w", err)
	}
	for _, repo := range repos {
		if strings.TrimSpace(repo) == "" {
			return fmt.Errorf("repository entry must not be empty")
		}
	}
	return nil
}

// -- Costs --

// ListCostEvents returns cost events for a workspace.
func (s *Service) ListCostEvents(ctx context.Context, wsID string) ([]*models.CostEvent, error) {
	return s.repo.ListCostEvents(ctx, wsID)
}

// GetCostsByAgent returns costs grouped by agent.
func (s *Service) GetCostsByAgent(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByAgent(ctx, wsID)
}

// GetCostsByProject returns costs grouped by project.
func (s *Service) GetCostsByProject(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByProject(ctx, wsID)
}

// GetCostsByModel returns costs grouped by model.
func (s *Service) GetCostsByModel(ctx context.Context, wsID string) ([]*models.CostBreakdown, error) {
	return s.repo.GetCostsByModel(ctx, wsID)
}

// -- Budgets --

// CreateBudgetPolicy creates a new budget policy.
func (s *Service) CreateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	return s.repo.CreateBudgetPolicy(ctx, policy)
}

// ListBudgetPolicies returns all budget policies for a workspace.
func (s *Service) ListBudgetPolicies(ctx context.Context, wsID string) ([]*models.BudgetPolicy, error) {
	return s.repo.ListBudgetPolicies(ctx, wsID)
}

// GetBudgetPolicy returns a budget policy by ID.
func (s *Service) GetBudgetPolicy(ctx context.Context, id string) (*models.BudgetPolicy, error) {
	return s.repo.GetBudgetPolicy(ctx, id)
}

// UpdateBudgetPolicy updates a budget policy.
func (s *Service) UpdateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	return s.repo.UpdateBudgetPolicy(ctx, policy)
}

// DeleteBudgetPolicy deletes a budget policy.
func (s *Service) DeleteBudgetPolicy(ctx context.Context, id string) error {
	return s.repo.DeleteBudgetPolicy(ctx, id)
}

// -- Routines --

// CreateRoutine creates a new routine.
func (s *Service) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	return s.repo.CreateRoutine(ctx, routine)
}

// GetRoutine returns a routine by ID.
func (s *Service) GetRoutine(ctx context.Context, id string) (*models.Routine, error) {
	return s.repo.GetRoutine(ctx, id)
}

// ListRoutines returns all routines for a workspace.
func (s *Service) ListRoutines(ctx context.Context, wsID string) ([]*models.Routine, error) {
	return s.repo.ListRoutines(ctx, wsID)
}

// UpdateRoutine updates a routine.
func (s *Service) UpdateRoutine(ctx context.Context, routine *models.Routine) error {
	return s.repo.UpdateRoutine(ctx, routine)
}

// DeleteRoutine deletes a routine.
func (s *Service) DeleteRoutine(ctx context.Context, id string) error {
	return s.repo.DeleteRoutine(ctx, id)
}

// -- Approvals --

// CreateApproval creates a new approval.
func (s *Service) CreateApproval(ctx context.Context, approval *models.Approval) error {
	return s.repo.CreateApproval(ctx, approval)
}

// ListApprovals returns all approvals for a workspace.
func (s *Service) ListApprovals(ctx context.Context, wsID string) ([]*models.Approval, error) {
	return s.repo.ListApprovals(ctx, wsID)
}

// UpdateApproval updates an approval (for deciding).
func (s *Service) UpdateApproval(ctx context.Context, approval *models.Approval) error {
	return s.repo.UpdateApproval(ctx, approval)
}

// GetApproval returns an approval by ID.
func (s *Service) GetApproval(ctx context.Context, id string) (*models.Approval, error) {
	return s.repo.GetApproval(ctx, id)
}

// -- Activity --

// ListActivity returns recent activity entries for a workspace.
func (s *Service) ListActivity(ctx context.Context, wsID string, limit int) ([]*models.ActivityEntry, error) {
	return s.repo.ListActivityEntries(ctx, wsID, limit)
}

// -- Memory --

// ListAgentMemory returns all memory entries for an agent.
func (s *Service) ListAgentMemory(ctx context.Context, agentID string) ([]*models.AgentMemory, error) {
	return s.repo.ListAgentMemory(ctx, agentID)
}

// UpsertAgentMemory creates or updates an agent memory entry.
func (s *Service) UpsertAgentMemory(ctx context.Context, mem *models.AgentMemory) error {
	return s.repo.UpsertAgentMemory(ctx, mem)
}

// DeleteAgentMemory deletes a memory entry.
func (s *Service) DeleteAgentMemory(ctx context.Context, id string) error {
	return s.repo.DeleteAgentMemory(ctx, id)
}

// -- Wakeups --

// ListWakeupRequests returns wakeup requests for a workspace.
func (s *Service) ListWakeupRequests(ctx context.Context, wsID string) ([]*models.WakeupRequest, error) {
	return s.repo.ListWakeupRequests(ctx, wsID)
}

// -- Dashboard --

// GetDashboard returns dashboard summary data for a workspace.
func (s *Service) GetDashboard(ctx context.Context, wsID string) (int, int, int, []*models.ActivityEntry, error) {
	agents, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	active := 0
	for _, a := range agents {
		if a.Status == models.AgentStatusWorking {
			active++
		}
	}
	approvals, err := s.repo.ListApprovals(ctx, wsID)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	pending := 0
	for _, a := range approvals {
		if a.Status == "pending" {
			pending++
		}
	}
	activity, err := s.repo.ListActivityEntries(ctx, wsID, 10)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	return len(agents), active, pending, activity, nil
}
