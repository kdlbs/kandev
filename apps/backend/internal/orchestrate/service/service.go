// Package service provides business logic for the orchestrate domain.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// WorkspaceCreator creates a DB workspace row for kanban compatibility.
// Implemented by the task service or its repository.
type WorkspaceCreator interface {
	CreateWorkspace(ctx context.Context, name, description string) error
	// FindWorkspaceIDByName returns the kanban workspace UUID for a given name.
	// Returns empty string if not found.
	FindWorkspaceIDByName(ctx context.Context, name string) (string, error)
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
	workspaceCreator  WorkspaceCreator
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

// SetWorkspaceCreator sets the DB workspace creator for dual workspace creation.
func (s *Service) SetWorkspaceCreator(creator WorkspaceCreator) {
	s.workspaceCreator = creator
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

// CreateSkill creates a new skill in the DB.
func (s *Service) CreateSkill(ctx context.Context, skill *models.Skill) error {
	if skill.SourceType == "" {
		skill.SourceType = SkillSourceTypeInline
	}
	if skill.FileInventory == "" {
		skill.FileInventory = "[]"
	}
	if err := s.repo.CreateSkill(ctx, skill); err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

// GetSkill returns a skill by ID.
func (s *Service) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	return s.GetSkillFromConfig(ctx, id)
}

// ListSkills returns all skills for a workspace.
func (s *Service) ListSkills(ctx context.Context, wsID string) ([]*models.Skill, error) {
	return s.ListSkillsFromConfig(ctx, wsID)
}

// UpdateSkill updates a skill in the DB.
func (s *Service) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	if err := s.repo.UpdateSkill(ctx, skill); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return nil
}

// DeleteSkill deletes a skill from the DB.
func (s *Service) DeleteSkill(ctx context.Context, id string) error {
	skill, err := s.GetSkillFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteSkill(ctx, skill.ID); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// -- Projects --

// CreateProject validates and creates a new project in the DB.
func (s *Service) CreateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if project.Status == "" {
		project.Status = models.ProjectStatusActive
	}
	if project.Repositories == "" {
		project.Repositories = "[]"
	}
	if project.ExecutorConfig == "" {
		project.ExecutorConfig = "{}"
	}
	if err := s.repo.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	s.logger.Info("project created",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// GetProject returns a project by ID.
func (s *Service) GetProject(ctx context.Context, id string) (*models.Project, error) {
	return s.GetProjectFromConfig(ctx, id)
}

// ListProjects returns all projects for a workspace.
func (s *Service) ListProjects(ctx context.Context, wsID string) ([]*models.Project, error) {
	return s.ListProjectsFromConfig(ctx, wsID)
}

// ListProjectsWithCounts returns all projects with aggregated task counts.
func (s *Service) ListProjectsWithCounts(ctx context.Context, wsID string) ([]*models.ProjectWithCounts, error) {
	return s.ListProjectsWithCountsFromConfig(ctx, wsID)
}

// GetTaskCounts returns task status counts for a project.
func (s *Service) GetTaskCounts(ctx context.Context, projectID string) (*models.TaskCounts, error) {
	return s.repo.GetTaskCounts(ctx, projectID)
}

// UpdateProject validates and updates a project in the DB.
func (s *Service) UpdateProject(ctx context.Context, project *models.Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if err := s.repo.UpdateProject(ctx, project); err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	s.logger.Info("project updated",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// DeleteProject deletes a project from the DB.
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	project, err := s.GetProjectFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteProject(ctx, project.ID); err != nil {
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

// CreateRoutine creates a new routine in the DB.
func (s *Service) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	if err := s.repo.CreateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("create routine: %w", err)
	}
	return nil
}

// GetRoutine returns a routine by ID.
func (s *Service) GetRoutine(ctx context.Context, id string) (*models.Routine, error) {
	return s.GetRoutineFromConfig(ctx, id)
}

// ListRoutines returns all routines for a workspace.
func (s *Service) ListRoutines(ctx context.Context, wsID string) ([]*models.Routine, error) {
	return s.ListRoutinesFromConfig(ctx, wsID)
}

// UpdateRoutine updates a routine in the DB.
func (s *Service) UpdateRoutine(ctx context.Context, routine *models.Routine) error {
	if err := s.repo.UpdateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("update routine: %w", err)
	}
	return nil
}

// DeleteRoutine deletes a routine from the DB.
func (s *Service) DeleteRoutine(ctx context.Context, id string) error {
	routine, err := s.GetRoutineFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRoutine(ctx, routine.ID); err != nil {
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

// CreateOrchestrateWorkspace writes workspace config to the filesystem and,
// if a WorkspaceCreator is configured, also creates a DB workspace row for
// kanban board compatibility.
func (s *Service) CreateOrchestrateWorkspace(ctx context.Context, name, description string) error {
	// 1. Write kandev.yml to filesystem.
	if s.cfgWriter != nil {
		settings := &configloader.WorkspaceSettings{
			Name:        name,
			Description: description,
			TaskPrefix:  "KAN",
		}
		data, err := configloader.MarshalSettings(*settings)
		if err != nil {
			return fmt.Errorf("marshal workspace settings: %w", err)
		}
		wsDir := filepath.Join(s.cfgLoader.BasePath(), "workspaces", name)
		if mkErr := os.MkdirAll(wsDir, 0o755); mkErr != nil {
			return fmt.Errorf("create workspace dir: %w", mkErr)
		}
		settingsPath := filepath.Join(wsDir, "kandev.yml")
		if writeErr := os.WriteFile(settingsPath, data, 0o644); writeErr != nil {
			return fmt.Errorf("write workspace settings: %w", writeErr)
		}
		if reloadErr := s.cfgLoader.Reload(name); reloadErr != nil {
			return fmt.Errorf("reload workspace config: %w", reloadErr)
		}
	}

	// 2. Create DB row for kanban compatibility.
	if s.workspaceCreator != nil {
		if err := s.workspaceCreator.CreateWorkspace(ctx, name, description); err != nil {
			s.logger.Warn("dual workspace DB creation failed",
				zap.String("name", name), zap.Error(err))
		}
	}
	return nil
}
