// Package service provides business logic for the orchestrate domain.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"

	"go.uber.org/zap"
)

// TaskStarter launches agent sessions on behalf of the orchestrate scheduler.
// Implemented by the orchestrator service; the orchestrate package depends only
// on this interface to avoid a direct import of the orchestrator package.
type TaskStarter interface {
	// StartTask starts agent execution for a task, creating a new session.
	StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string,
		executorProfileID string, priority int, prompt string, workflowStepID string,
		planMode bool, attachments []v1.MessageAttachment) error
}

// TaskStarterFunc adapts a function to the TaskStarter interface.
// Useful for wrapping callers whose StartTask returns additional values.
type TaskStarterFunc func(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority int, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment) error

// StartTask implements TaskStarter.
func (f TaskStarterFunc) StartTask(ctx context.Context, taskID, agentProfileID, executorID,
	executorProfileID string, priority int, prompt, workflowStepID string,
	planMode bool, attachments []v1.MessageAttachment) error {
	return f(ctx, taskID, agentProfileID, executorID, executorProfileID,
		priority, prompt, workflowStepID, planMode, attachments)
}

// Service provides orchestrate business logic.
type Service struct {
	repo              *sqlite.Repository
	cfgLoader         *configloader.ConfigLoader
	cfgWriter         *configloader.FileWriter
	gitManager        *configloader.GitManager
	logger            *logger.Logger
	eb                bus.EventBus
	relay             *ChannelRelay
	agentTypeResolver AgentTypeResolver
	taskStarter       TaskStarter
}

// NewService creates a new orchestrate service.
func NewService(repo *sqlite.Repository, log *logger.Logger) *Service {
	svc := &Service{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "orchestrate-service")),
	}
	svc.relay = NewChannelRelay(svc)
	return svc
}

// SetConfigLoader sets the filesystem-based config loader and writer.
// Called after construction because the config loader is created later in startup.
func (s *Service) SetConfigLoader(loader *configloader.ConfigLoader, writer *configloader.FileWriter) {
	s.cfgLoader = loader
	s.cfgWriter = writer
}

// SetTaskStarter sets the interface used to launch agent sessions.
// Called after construction because the orchestrator service is created later in startup.
func (s *Service) SetTaskStarter(starter TaskStarter) {
	s.taskStarter = starter
}

// SetGitManager sets the git manager for workspace git operations.
func (s *Service) SetGitManager(gm *configloader.GitManager) {
	s.gitManager = gm
}

// GitManager returns the git manager (may be nil).
func (s *Service) GitManager() *configloader.GitManager {
	return s.gitManager
}

// ConfigLoader returns the filesystem config loader (may be nil).
func (s *Service) ConfigLoader() *configloader.ConfigLoader {
	return s.cfgLoader
}

// ConfigWriter returns the filesystem config writer (may be nil).
func (s *Service) ConfigWriter() *configloader.FileWriter {
	return s.cfgWriter
}

// defaultWorkspaceName is used for ConfigLoader lookups when we only have a
// DB workspace ID. Most single-user installs have one workspace named "default".
const defaultWorkspaceName = "default"

// Agent instance methods (CRUD + validation + status transitions) are in agents.go.

// -- Skills --

// CreateSkill creates a new skill via the filesystem.
func (s *Service) CreateSkill(_ context.Context, skill *models.Skill) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	// Skill ID matches slug for filesystem-based identity.
	if skill.ID == "" {
		skill.ID = skill.Slug
	}
	now := time.Now().UTC()
	skill.CreatedAt = now
	skill.UpdatedAt = now
	// Always write SKILL.md so the skill is registered on disk.
	content := skill.Content
	if content == "" {
		content = "# " + skill.Name + "\n"
	}
	if err := s.cfgWriter.WriteSkill(defaultWorkspaceName, skill.Slug, content); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	return nil
}

// GetSkill returns a skill by ID from the ConfigLoader.
func (s *Service) GetSkill(_ context.Context, id string) (*models.Skill, error) {
	return s.GetSkillFromConfig(context.Background(), id)
}

// ListSkills returns all skills for a workspace from the ConfigLoader.
func (s *Service) ListSkills(_ context.Context, _ string) ([]*models.Skill, error) {
	return s.ListSkillsFromConfig(context.Background(), "")
}

// UpdateSkill updates a skill via the filesystem.
func (s *Service) UpdateSkill(_ context.Context, skill *models.Skill) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	skill.UpdatedAt = time.Now().UTC()
	content := skill.Content
	if content == "" {
		content = "# " + skill.Name + "\n"
	}
	if err := s.cfgWriter.WriteSkill(defaultWorkspaceName, skill.Slug, content); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	return nil
}

// DeleteSkill deletes a skill from the filesystem.
func (s *Service) DeleteSkill(_ context.Context, id string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	skill, err := s.GetSkillFromConfig(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.cfgWriter.DeleteSkill(defaultWorkspaceName, skill.Slug); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// -- Projects --

// CreateProject validates and creates a new project via the filesystem.
func (s *Service) CreateProject(_ context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	if project.ID == "" {
		project.ID = uuid.New().String()
	}
	if project.Status == "" {
		project.Status = models.ProjectStatusActive
	}
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	if err := s.cfgWriter.WriteProject(defaultWorkspaceName, project); err != nil {
		return fmt.Errorf("write project: %w", err)
	}
	s.logger.Info("project created",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// GetProject returns a project by ID from the ConfigLoader.
func (s *Service) GetProject(_ context.Context, id string) (*models.Project, error) {
	return s.GetProjectFromConfig(context.Background(), id)
}

// ListProjects returns all projects for a workspace from the ConfigLoader.
func (s *Service) ListProjects(_ context.Context, _ string) ([]*models.Project, error) {
	return s.ListProjectsFromConfig(context.Background(), "")
}

// ListProjectsWithCounts returns all projects with aggregated task counts.
func (s *Service) ListProjectsWithCounts(ctx context.Context, _ string) ([]*models.ProjectWithCounts, error) {
	return s.ListProjectsWithCountsFromConfig(ctx, "")
}

// GetTaskCounts returns task status counts for a project.
func (s *Service) GetTaskCounts(ctx context.Context, projectID string) (*models.TaskCounts, error) {
	return s.repo.GetTaskCounts(ctx, projectID)
}

// UpdateProject validates and updates a project via the filesystem.
func (s *Service) UpdateProject(_ context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	project.UpdatedAt = time.Now().UTC()
	if err := s.cfgWriter.WriteProject(defaultWorkspaceName, project); err != nil {
		return fmt.Errorf("write project: %w", err)
	}
	s.logger.Info("project updated",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// DeleteProject deletes a project from the filesystem.
func (s *Service) DeleteProject(_ context.Context, id string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	project, err := s.GetProjectFromConfig(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.cfgWriter.DeleteProject(defaultWorkspaceName, project.Name); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
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

// CreateRoutine creates a new routine via the filesystem.
func (s *Service) CreateRoutine(_ context.Context, routine *models.Routine) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	if routine.ID == "" {
		routine.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	routine.CreatedAt = now
	routine.UpdatedAt = now
	if err := s.cfgWriter.WriteRoutine(defaultWorkspaceName, routine); err != nil {
		return fmt.Errorf("write routine: %w", err)
	}
	return nil
}

// GetRoutine returns a routine by ID from the ConfigLoader.
func (s *Service) GetRoutine(_ context.Context, id string) (*models.Routine, error) {
	return s.GetRoutineFromConfig(context.Background(), id)
}

// ListRoutines returns all routines for a workspace from the ConfigLoader.
func (s *Service) ListRoutines(_ context.Context, _ string) ([]*models.Routine, error) {
	return s.ListRoutinesFromConfig(context.Background(), "")
}

// UpdateRoutine updates a routine via the filesystem.
func (s *Service) UpdateRoutine(_ context.Context, routine *models.Routine) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	routine.UpdatedAt = time.Now().UTC()
	if err := s.cfgWriter.WriteRoutine(defaultWorkspaceName, routine); err != nil {
		return fmt.Errorf("write routine: %w", err)
	}
	return nil
}

// DeleteRoutine deletes a routine from the filesystem.
func (s *Service) DeleteRoutine(_ context.Context, id string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	routine, err := s.GetRoutineFromConfig(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.cfgWriter.DeleteRoutine(defaultWorkspaceName, routine.Name); err != nil {
		return fmt.Errorf("delete routine: %w", err)
	}
	return nil
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

// -- Task Checkout --

// CheckoutTask atomically acquires an exclusive lock on a task for an agent.
func (s *Service) CheckoutTask(ctx context.Context, taskID, agentID string) (bool, error) {
	return s.repo.CheckoutTask(ctx, taskID, agentID)
}

// ReleaseTaskCheckout releases the exclusive lock on a task.
func (s *Service) ReleaseTaskCheckout(ctx context.Context, taskID string) error {
	return s.repo.ReleaseTaskCheckout(ctx, taskID)
}

// -- Wakeups --

// ListWakeupRequests returns wakeup requests for a workspace.
func (s *Service) ListWakeupRequests(ctx context.Context, wsID string) ([]*models.WakeupRequest, error) {
	return s.repo.ListWakeupRequests(ctx, wsID)
}

// -- Task Search --

// SearchTasks searches for tasks matching the query string in a workspace.
func (s *Service) SearchTasks(ctx context.Context, wsID, query string, limit int) ([]*sqlite.TaskSearchResult, error) {
	return s.repo.SearchTasks(ctx, wsID, query, limit)
}

// -- Dashboard --

// GetDashboard returns dashboard summary data for a workspace.
func (s *Service) GetDashboard(ctx context.Context, wsID string) (int, int, int, []*models.ActivityEntry, error) {
	agents, err := s.ListAgentsFromConfig(ctx, wsID)
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
