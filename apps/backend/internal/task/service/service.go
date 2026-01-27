package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// WorktreeCleanup provides worktree cleanup on task deletion.
type WorktreeCleanup interface {
	// OnTaskDeleted is called when a task is deleted to clean up its worktree.
	OnTaskDeleted(ctx context.Context, taskID string) error
}

// WorktreeProvider extends WorktreeCleanup with query capabilities.
// Implementations that support this can be type-asserted from WorktreeCleanup.
type WorktreeProvider interface {
	WorktreeCleanup
	// GetAllByTaskID returns all worktrees associated with a task.
	GetAllByTaskID(ctx context.Context, taskID string) ([]*worktree.Worktree, error)
}

// WorktreeBatchCleaner extends WorktreeProvider with batch cleanup.
type WorktreeBatchCleaner interface {
	WorktreeProvider
	// CleanupWorktrees removes multiple worktrees in a single operation.
	CleanupWorktrees(ctx context.Context, worktrees []*worktree.Worktree) error
}

// TaskExecutionStopper stops active task execution (agent session + instance).
type TaskExecutionStopper interface {
	StopTask(ctx context.Context, taskID, reason string, force bool) error
}

// WorkflowStepCreator creates workflow steps from a template for a board.
type WorkflowStepCreator interface {
	CreateStepsFromTemplate(ctx context.Context, boardID, templateID string) error
}

// WorkflowStepGetter retrieves workflow step information.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*WorkflowStep, error)
}

// WorkflowStep represents a workflow step (mirrors workflow/models.WorkflowStep).
// Defined here to avoid circular dependency with workflow package.
type WorkflowStep struct {
	ID               string
	BoardID          string
	Name             string
	StepType         string
	Position         int
	Color            string
	AutoStartAgent   bool
	PlanMode         bool
	RequireApproval  bool
	PromptPrefix     string
	PromptSuffix     string
	AllowManualMove  bool
	OnCompleteStepID *string
	OnApprovalStepID *string
}

var (
	ErrActiveTaskSessions        = errors.New("active agent sessions exist")
	ErrInvalidRepositorySettings = errors.New("invalid repository settings")
	ErrInvalidExecutorConfig     = errors.New("invalid executor config")
)

func validateExecutorConfig(config map[string]string) error {
	if config == nil {
		return nil
	}
	policy := strings.TrimSpace(config["mcp_policy"])
	if policy == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(policy), &decoded); err != nil {
		return fmt.Errorf("%w: mcp_policy must be valid JSON", ErrInvalidExecutorConfig)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%w: mcp_policy must be a JSON object", ErrInvalidExecutorConfig)
	}
	return nil
}

// Service provides task business logic
type Service struct {
	repo                repository.Repository
	eventBus            bus.EventBus
	logger              *logger.Logger
	discoveryConfig     RepositoryDiscoveryConfig
	worktreeCleanup     WorktreeCleanup
	executionStopper    TaskExecutionStopper
	workflowStepCreator WorkflowStepCreator
	workflowStepGetter  WorkflowStepGetter
}

// NewService creates a new task service
func NewService(repo repository.Repository, eventBus bus.EventBus, log *logger.Logger, discoveryConfig RepositoryDiscoveryConfig) *Service {
	return &Service{
		repo:            repo,
		eventBus:        eventBus,
		logger:          log,
		discoveryConfig: discoveryConfig,
	}
}

// SetWorktreeCleanup sets the worktree cleanup handler for task deletion.
func (s *Service) SetWorktreeCleanup(cleanup WorktreeCleanup) {
	s.worktreeCleanup = cleanup
}

// SetExecutionStopper wires the task execution stopper (orchestrator).
func (s *Service) SetExecutionStopper(stopper TaskExecutionStopper) {
	s.executionStopper = stopper
}

// SetWorkflowStepCreator wires the workflow step creator for board creation.
func (s *Service) SetWorkflowStepCreator(creator WorkflowStepCreator) {
	s.workflowStepCreator = creator
}

// SetWorkflowStepGetter wires the workflow step getter for MoveTask.
func (s *Service) SetWorkflowStepGetter(getter WorkflowStepGetter) {
	s.workflowStepGetter = getter
}

// Request types

// TaskRepositoryInput for creating/updating task repositories
type TaskRepositoryInput struct {
	RepositoryID  string `json:"repository_id"`
	BaseBranch    string `json:"base_branch"`
	LocalPath     string `json:"local_path,omitempty"`
	Name          string `json:"name,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// CreateTaskRequest contains the data for creating a new task
type CreateTaskRequest struct {
	WorkspaceID    string                 `json:"workspace_id"`
	BoardID        string                 `json:"board_id"`
	WorkflowStepID string                 `json:"workflow_step_id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Priority       int                    `json:"priority"`
	State          *v1.TaskState          `json:"state,omitempty"`
	Repositories   []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position       int                    `json:"position"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest contains the data for updating a task
type UpdateTaskRequest struct {
	Title        *string                `json:"title,omitempty"`
	Description  *string                `json:"description,omitempty"`
	Priority     *int                   `json:"priority,omitempty"`
	State        *v1.TaskState          `json:"state,omitempty"`
	Repositories []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position     *int                   `json:"position,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CreateBoardRequest contains the data for creating a new board
type CreateBoardRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

// UpdateBoardRequest contains the data for updating a board
type UpdateBoardRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateWorkspaceRequest contains the data for creating a new workspace
type CreateWorkspaceRequest struct {
	Name                  string  `json:"name"`
	Description           string  `json:"description"`
	OwnerID               string  `json:"owner_id"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}

// UpdateWorkspaceRequest contains the data for updating a workspace
type UpdateWorkspaceRequest struct {
	Name                  *string `json:"name,omitempty"`
	Description           *string `json:"description,omitempty"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}

// CreateRepositoryRequest contains the data for creating a new repository
type CreateRepositoryRequest struct {
	WorkspaceID          string `json:"workspace_id"`
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	LocalPath            string `json:"local_path"`
	Provider             string `json:"provider"`
	ProviderRepoID       string `json:"provider_repo_id"`
	ProviderOwner        string `json:"provider_owner"`
	ProviderName         string `json:"provider_name"`
	DefaultBranch        string `json:"default_branch"`
	WorktreeBranchPrefix string `json:"worktree_branch_prefix"`
	PullBeforeWorktree   *bool  `json:"pull_before_worktree"`
	SetupScript          string `json:"setup_script"`
	CleanupScript        string `json:"cleanup_script"`
	DevScript            string `json:"dev_script"`
}

// UpdateRepositoryRequest contains the data for updating a repository
type UpdateRepositoryRequest struct {
	Name                 *string `json:"name,omitempty"`
	SourceType           *string `json:"source_type,omitempty"`
	LocalPath            *string `json:"local_path,omitempty"`
	Provider             *string `json:"provider,omitempty"`
	ProviderRepoID       *string `json:"provider_repo_id,omitempty"`
	ProviderOwner        *string `json:"provider_owner,omitempty"`
	ProviderName         *string `json:"provider_name,omitempty"`
	DefaultBranch        *string `json:"default_branch,omitempty"`
	WorktreeBranchPrefix *string `json:"worktree_branch_prefix,omitempty"`
	PullBeforeWorktree   *bool   `json:"pull_before_worktree,omitempty"`
	SetupScript          *string `json:"setup_script,omitempty"`
	CleanupScript        *string `json:"cleanup_script,omitempty"`
	DevScript            *string `json:"dev_script,omitempty"`
}

// CreateExecutorRequest contains the data for creating an executor
type CreateExecutorRequest struct {
	Name      string                `json:"name"`
	Type      models.ExecutorType   `json:"type"`
	Status    models.ExecutorStatus `json:"status"`
	IsSystem  bool                  `json:"is_system"`
	Resumable bool                  `json:"resumable"`
	Config    map[string]string     `json:"config,omitempty"`
}

// UpdateExecutorRequest contains the data for updating an executor
type UpdateExecutorRequest struct {
	Name      *string                `json:"name,omitempty"`
	Type      *models.ExecutorType   `json:"type,omitempty"`
	Status    *models.ExecutorStatus `json:"status,omitempty"`
	Resumable *bool                  `json:"resumable,omitempty"`
	Config    map[string]string      `json:"config,omitempty"`
}

// CreateEnvironmentRequest contains the data for creating an environment
type CreateEnvironmentRequest struct {
	Name         string                 `json:"name"`
	Kind         models.EnvironmentKind `json:"kind"`
	WorktreeRoot string                 `json:"worktree_root,omitempty"`
	ImageTag     string                 `json:"image_tag,omitempty"`
	Dockerfile   string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string      `json:"build_config,omitempty"`
}

// UpdateEnvironmentRequest contains the data for updating an environment
type UpdateEnvironmentRequest struct {
	Name         *string                 `json:"name,omitempty"`
	Kind         *models.EnvironmentKind `json:"kind,omitempty"`
	WorktreeRoot *string                 `json:"worktree_root,omitempty"`
	ImageTag     *string                 `json:"image_tag,omitempty"`
	Dockerfile   *string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string       `json:"build_config,omitempty"`
}

type ListMessagesRequest struct {
	TaskSessionID string
	Limit         int
	Before        string
	After         string
	Sort          string
}

// CreateRepositoryScriptRequest contains the data for creating a repository script
type CreateRepositoryScriptRequest struct {
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	Command      string `json:"command"`
	Position     int    `json:"position"`
}

// UpdateRepositoryScriptRequest contains the data for updating a repository script
type UpdateRepositoryScriptRequest struct {
	Name     *string `json:"name,omitempty"`
	Command  *string `json:"command,omitempty"`
	Position *int    `json:"position,omitempty"`
}

// Task operations

// CreateTask creates a new task and publishes a task.created event
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (*models.Task, error) {
	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	task := &models.Task{
		ID:             uuid.New().String(),
		WorkspaceID:    req.WorkspaceID,
		BoardID:        req.BoardID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		State:          state,
		Priority:       req.Priority,
		Position:       req.Position,
		Metadata:       req.Metadata,
	}

	if err := s.repo.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err))
		return nil, err
	}

	// Create task-repository associations
	var repoByPath map[string]*models.Repository
	for _, repoInput := range req.Repositories {
		if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
			repos, err := s.repo.ListRepositories(ctx, req.WorkspaceID)
			if err != nil {
				s.logger.Error("failed to list repositories", zap.Error(err))
				return nil, err
			}
			repoByPath = make(map[string]*models.Repository, len(repos))
			for _, repo := range repos {
				if repo.LocalPath == "" {
					continue
				}
				repoByPath[repo.LocalPath] = repo
			}
			break
		}
	}

	for i, repoInput := range req.Repositories {
		repositoryID := repoInput.RepositoryID
		baseBranch := repoInput.BaseBranch
		if repositoryID == "" && repoInput.LocalPath != "" {
			repo := repoByPath[repoInput.LocalPath]
			if repo == nil {
				name := strings.TrimSpace(repoInput.Name)
				if name == "" {
					name = filepath.Base(repoInput.LocalPath)
				}
				defaultBranch := repoInput.DefaultBranch
				if defaultBranch == "" {
					defaultBranch = repoInput.BaseBranch
				}
				created, err := s.CreateRepository(ctx, &CreateRepositoryRequest{
					WorkspaceID:   req.WorkspaceID,
					Name:          name,
					SourceType:    "local",
					LocalPath:     repoInput.LocalPath,
					DefaultBranch: defaultBranch,
				})
				if err != nil {
					return nil, err
				}
				repo = created
				repoByPath[repoInput.LocalPath] = repo
			}
			repositoryID = repo.ID
			if baseBranch == "" {
				baseBranch = repo.DefaultBranch
			}
		}
		if repositoryID == "" {
			return nil, fmt.Errorf("repository_id is required")
		}
		taskRepo := &models.TaskRepository{
			TaskID:       task.ID,
			RepositoryID: repositoryID,
			BaseBranch:   baseBranch,
			Position:     i,
			Metadata:     make(map[string]interface{}),
		}
		if err := s.repo.CreateTaskRepository(ctx, taskRepo); err != nil {
			s.logger.Error("failed to create task repository", zap.Error(err))
			return nil, err
		}
	}

	// Load repositories into task for response
	repos, err := s.repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	s.publishTaskEvent(ctx, events.TaskCreated, task, nil)
	s.logger.Info("task created", zap.String("task_id", task.ID), zap.String("title", task.Title))

	return task, nil
}

// GetTask retrieves a task by ID and populates repositories
func (s *Service) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load task repositories
	repos, err := s.repo.ListTaskRepositories(ctx, id)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	return task, nil
}

// UpdateTask updates an existing task and publishes a task.updated event
func (s *Service) UpdateTask(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	var oldState *v1.TaskState
	stateChanged := false

	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.State != nil && task.State != *req.State {
		current := task.State
		oldState = &current
		task.State = *req.State
		stateChanged = true
	}
	if req.Position != nil {
		task.Position = *req.Position
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Update task repositories if provided
	if req.Repositories != nil {
		// Delete existing repositories
		if err := s.repo.DeleteTaskRepositoriesByTask(ctx, id); err != nil {
			s.logger.Error("failed to delete task repositories", zap.Error(err))
			return nil, err
		}

		// Create new repositories
		for i, repoInput := range req.Repositories {
			taskRepo := &models.TaskRepository{
				TaskID:       task.ID,
				RepositoryID: repoInput.RepositoryID,
				BaseBranch:   repoInput.BaseBranch,
				Position:     i,
				Metadata:     make(map[string]interface{}),
			}
			if err := s.repo.CreateTaskRepository(ctx, taskRepo); err != nil {
				s.logger.Error("failed to create task repository", zap.Error(err))
				return nil, err
			}
		}
	}

	// Load repositories into task for response
	repos, err := s.repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

// DeleteTask deletes a task and publishes a task.deleted event
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	start := time.Now()
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}

	sessions, err := s.repo.ListTaskSessions(ctx, id)
	if err != nil {
		s.logger.Warn("failed to list task sessions for delete",
			zap.String("task_id", id),
			zap.Error(err))
	}

	var worktrees []*worktree.Worktree
	if s.worktreeCleanup != nil {
		if provider, ok := s.worktreeCleanup.(WorktreeProvider); ok {
			worktrees, err = provider.GetAllByTaskID(ctx, id)
			if err != nil {
				s.logger.Warn("failed to list worktrees for delete",
					zap.String("task_id", id),
					zap.Error(err))
			}
		} else {
			// Fallback for legacy implementations: cleanup before delete.
			if err := s.worktreeCleanup.OnTaskDeleted(ctx, id); err != nil {
				s.logger.Warn("failed to cleanup worktree on task deletion",
					zap.String("task_id", id),
					zap.Error(err))
			}
		}
	}

	// Stop any running execution BEFORE deleting from database
	// This must happen first because StopTask queries the database for active sessions
	if s.executionStopper != nil {
		if err := s.executionStopper.StopTask(ctx, id, "task deleted", true); err != nil {
			s.logger.Warn("failed to stop task execution on delete",
				zap.String("task_id", id),
				zap.Error(err))
			// Continue with deletion even if stop fails
		}
	}

	if err := s.repo.DeleteTask(ctx, id); err != nil {
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return err
	}

	s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// Perform remaining cleanup synchronously with dedicated timeout
	// Use background context since the original request may complete
	if s.worktreeCleanup != nil || len(sessions) > 0 {
		cleanupStart := time.Now()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees)

		if len(cleanupErrors) > 0 {
			s.logger.Warn("task cleanup completed with errors",
				zap.String("task_id", id),
				zap.Int("error_count", len(cleanupErrors)),
				zap.Duration("duration", time.Since(cleanupStart)))
		} else {
			s.logger.Info("task cleanup completed",
				zap.String("task_id", id),
				zap.Duration("duration", time.Since(cleanupStart)))
		}
	}

	return nil
}

// performTaskCleanup handles post-deletion cleanup operations.
// Note: Execution stopping is done BEFORE database deletion in DeleteTask,
// so this function only handles worktree cleanup and executor_running records.
// Returns a slice of errors encountered (empty if all succeeded).
func (s *Service) performTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
) []error {
	var errs []error

	// Cleanup worktrees
	if len(worktrees) > 0 {
		if cleaner, ok := s.worktreeCleanup.(WorktreeBatchCleaner); ok {
			if err := cleaner.CleanupWorktrees(ctx, worktrees); err != nil {
				s.logger.Warn("failed to cleanup worktrees after delete",
					zap.String("task_id", taskID),
					zap.Error(err))
				errs = append(errs, fmt.Errorf("cleanup worktrees: %w", err))
			}
		}
	}

	// Delete executor running records for sessions
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		if err := s.repo.DeleteExecutorRunningBySessionID(ctx, session.ID); err != nil {
			s.logger.Debug("failed to delete executor runtime for session",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			// Don't add to errs - this is a debug-level issue
		}
	}

	return errs
}

// ListTasks returns all tasks for a board
func (s *Service) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	tasks, err := s.repo.ListTasks(ctx, boardID)
	if err != nil {
		return nil, err
	}

	// Load repositories for each task
	for _, task := range tasks {
		repos, err := s.repo.ListTaskRepositories(ctx, task.ID)
		if err != nil {
			s.logger.Error("failed to list task repositories", zap.String("task_id", task.ID), zap.Error(err))
		} else {
			task.Repositories = repos
		}
	}

	return tasks, nil
}

// ListTaskSessions returns all sessions for a task.
func (s *Service) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	return s.repo.ListTaskSessions(ctx, taskID)
}

// GetTaskSession returns a single session by ID.
func (s *Service) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	return s.repo.GetTaskSession(ctx, sessionID)
}

// GetPrimarySession returns the primary session for a task.
func (s *Service) GetPrimarySession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return s.repo.GetPrimarySessionByTaskID(ctx, taskID)
}

// GetPrimarySessionIDsForTasks returns a map of task ID to primary session ID for the given task IDs.
// Tasks without a primary session are not included in the result.
func (s *Service) GetPrimarySessionIDsForTasks(ctx context.Context, taskIDs []string) (map[string]string, error) {
	return s.repo.GetPrimarySessionIDsByTaskIDs(ctx, taskIDs)
}

// SetPrimarySession sets a session as the primary session for its task.
// This will unset any existing primary session for the same task.
func (s *Service) SetPrimarySession(ctx context.Context, sessionID string) error {
	if err := s.repo.SetSessionPrimary(ctx, sessionID); err != nil {
		s.logger.Error("failed to set primary session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	return nil
}

// MoveSessionToStep moves a session to a different workflow step.
func (s *Service) MoveSessionToStep(ctx context.Context, sessionID string, stepID string) error {
	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, stepID); err != nil {
		s.logger.Error("failed to move session to step",
			zap.String("session_id", sessionID),
			zap.String("step_id", stepID),
			zap.Error(err))
		return err
	}
	return nil
}

// UpdateSessionReviewStatus updates the review status of a session.
func (s *Service) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, status); err != nil {
		s.logger.Error("failed to update session review status",
			zap.String("session_id", sessionID),
			zap.String("status", status),
			zap.Error(err))
		return err
	}
	return nil
}

// ApproveSessionResult contains the result of approving a session
type ApproveSessionResult struct {
	Session      *models.TaskSession
	Task         *models.Task
	WorkflowStep *WorkflowStep
}

// ApproveSession approves a session's current step and moves it to the on_approval_step_id
func (s *Service) ApproveSession(ctx context.Context, sessionID string) (*ApproveSessionResult, error) {
	// Update review status to approved
	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, "approved"); err != nil {
		return nil, fmt.Errorf("failed to update review status: %w", err)
	}

	result := &ApproveSessionResult{}

	// Reload session to get updated review status
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session: %w", err)
	}
	result.Session = session

	// Get the current workflow step to check for on_approval_step_id
	if session.WorkflowStepID != nil && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, *session.WorkflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for approval transition",
				zap.String("workflow_step_id", *session.WorkflowStepID),
				zap.Error(err))
		} else if step.OnApprovalStepID != nil && *step.OnApprovalStepID != "" {
			// Move session and task to the on_approval_step_id
			newStepID := *step.OnApprovalStepID
			if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, newStepID); err != nil {
				s.logger.Error("failed to move session to approval step",
					zap.String("session_id", sessionID),
					zap.String("step_id", newStepID),
					zap.Error(err))
			} else {
				// Also move the task to the new step
				task, err := s.repo.GetTask(ctx, session.TaskID)
				if err != nil {
					s.logger.Error("failed to get task for approval transition",
						zap.String("task_id", session.TaskID),
						zap.Error(err))
				} else {
					task.WorkflowStepID = newStepID
					task.UpdatedAt = time.Now().UTC()
					if err := s.repo.UpdateTask(ctx, task); err != nil {
						s.logger.Error("failed to move task to approval step",
							zap.String("task_id", session.TaskID),
							zap.String("step_id", newStepID),
							zap.Error(err))
					} else {
						s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
						result.Task = task
					}
				}

				// Reload session with new step
				session, _ = s.repo.GetTaskSession(ctx, sessionID)
				result.Session = session

				// Get the new workflow step for the response
				newStep, err := s.workflowStepGetter.GetStep(ctx, newStepID)
				if err == nil {
					result.WorkflowStep = newStep
				}

				s.logger.Info("session approved and moved to next step",
					zap.String("session_id", sessionID),
					zap.String("from_step", *session.WorkflowStepID),
					zap.String("to_step", newStepID))
			}
		}
	}

	return result, nil
}

// UpdateTaskState updates the state of a task, moves it to the matching column,
// and publishes a task.state_changed event
func (s *Service) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	oldState := task.State

	if err := s.repo.UpdateTaskState(ctx, id, state); err != nil {
		s.logger.Error("failed to update task state", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Reload task to get updated state
	task, err = s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("task state updated",
		zap.String("task_id", id),
		zap.String("workflow_step_id", task.WorkflowStepID),
		zap.String("state", string(task.State)))

	s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	s.logger.Info("task state changed",
		zap.String("task_id", id),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(state)))

	return task, nil
}

// UpdateTaskMetadata updates only the metadata of a task (merges with existing)
func (s *Service) UpdateTaskMetadata(ctx context.Context, id string, metadata map[string]interface{}) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Merge metadata (existing keys are preserved, new keys are added/updated)
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		task.Metadata[k] = v
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task metadata", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.logger.Debug("task metadata updated", zap.String("task_id", id), zap.Any("metadata", metadata))
	return task, nil
}

// MoveTaskResult contains the result of a MoveTask operation.
type MoveTaskResult struct {
	Task         *models.Task
	WorkflowStep *WorkflowStep
}

// MoveTask moves a task to a different workflow step and position
func (s *Service) MoveTask(ctx context.Context, id string, boardID string, workflowStepID string, position int) (*MoveTaskResult, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if task's primary session is in a review step with pending approval
	// If so, prevent moving forward - user must use Approve button or send a message
	if task.WorkflowStepID != workflowStepID {
		primarySession, err := s.repo.GetPrimarySessionByTaskID(ctx, id)
		if err == nil && primarySession != nil {
			if primarySession.ReviewStatus != nil && *primarySession.ReviewStatus == "pending" {
				// Check if current step requires approval
				if s.workflowStepGetter != nil && primarySession.WorkflowStepID != nil {
					currentStep, err := s.workflowStepGetter.GetStep(ctx, *primarySession.WorkflowStepID)
					if err == nil && currentStep.RequireApproval {
						return nil, fmt.Errorf("task is pending approval - use Approve button to proceed or send a message to request changes")
					}
				}
			}
		}
	}

	oldState := task.State
	task.BoardID = boardID
	task.WorkflowStepID = workflowStepID
	task.Position = position
	task.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Publish state_changed event if state changed, otherwise just updated
	if oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	} else {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	s.logger.Info("task moved",
		zap.String("task_id", id),
		zap.String("board_id", boardID),
		zap.String("workflow_step_id", workflowStepID),
		zap.Int("position", position))

	result := &MoveTaskResult{Task: task}

	// Fetch the workflow step info if getter is available
	if s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for MoveTask response",
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
			// Don't fail the operation, just log and continue
		} else {
			result.WorkflowStep = step
		}
	}

	return result, nil
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (s *Service) CreateWorkspace(ctx context.Context, req *CreateWorkspaceRequest) (*models.Workspace, error) {
	workspace := &models.Workspace{
		ID:                    uuid.New().String(),
		Name:                  req.Name,
		Description:           req.Description,
		OwnerID:               req.OwnerID,
		DefaultExecutorID:     normalizeOptionalID(req.DefaultExecutorID),
		DefaultEnvironmentID:  normalizeOptionalID(req.DefaultEnvironmentID),
		DefaultAgentProfileID: normalizeOptionalID(req.DefaultAgentProfileID),
	}

	if err := s.repo.CreateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to create workspace", zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceCreated, workspace)
	s.logger.Info("workspace created", zap.String("workspace_id", workspace.ID), zap.String("name", workspace.Name))
	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID
func (s *Service) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	return s.repo.GetWorkspace(ctx, id)
}

// UpdateWorkspace updates an existing workspace
func (s *Service) UpdateWorkspace(ctx context.Context, id string, req *UpdateWorkspaceRequest) (*models.Workspace, error) {
	workspace, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		workspace.Name = *req.Name
	}
	if req.Description != nil {
		workspace.Description = *req.Description
	}
	if req.DefaultExecutorID != nil {
		workspace.DefaultExecutorID = normalizeOptionalID(req.DefaultExecutorID)
	}
	if req.DefaultEnvironmentID != nil {
		workspace.DefaultEnvironmentID = normalizeOptionalID(req.DefaultEnvironmentID)
	}
	if req.DefaultAgentProfileID != nil {
		workspace.DefaultAgentProfileID = normalizeOptionalID(req.DefaultAgentProfileID)
	}
	workspace.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to update workspace", zap.String("workspace_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkspaceEvent(ctx, events.WorkspaceUpdated, workspace)
	s.logger.Info("workspace updated", zap.String("workspace_id", workspace.ID))
	return workspace, nil
}

// DeleteWorkspace deletes a workspace
func (s *Service) DeleteWorkspace(ctx context.Context, id string) error {
	workspace, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteWorkspace(ctx, id); err != nil {
		s.logger.Error("failed to delete workspace", zap.String("workspace_id", id), zap.Error(err))
		return err
	}
	s.publishWorkspaceEvent(ctx, events.WorkspaceDeleted, workspace)
	s.logger.Info("workspace deleted", zap.String("workspace_id", id))
	return nil
}

func normalizeOptionalID(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ListWorkspaces returns all workspaces
func (s *Service) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	return s.repo.ListWorkspaces(ctx)
}

// Board operations

// CreateBoard creates a new board
func (s *Service) CreateBoard(ctx context.Context, req *CreateBoardRequest) (*models.Board, error) {
	board := &models.Board{
		ID:                 uuid.New().String(),
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	}

	if err := s.repo.CreateBoard(ctx, board); err != nil {
		s.logger.Error("failed to create board", zap.Error(err))
		return nil, err
	}

	// Create workflow steps from template if specified
	if req.WorkflowTemplateID != nil && *req.WorkflowTemplateID != "" && s.workflowStepCreator != nil {
		if err := s.workflowStepCreator.CreateStepsFromTemplate(ctx, board.ID, *req.WorkflowTemplateID); err != nil {
			s.logger.Error("failed to create workflow steps from template",
				zap.String("board_id", board.ID),
				zap.String("template_id", *req.WorkflowTemplateID),
				zap.Error(err))
			// Don't fail board creation, just log the error
		}
	}

	s.publishBoardEvent(ctx, events.BoardCreated, board)
	s.logger.Info("board created", zap.String("board_id", board.ID), zap.String("name", board.Name))
	return board, nil
}

// GetBoard retrieves a board by ID
func (s *Service) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	return s.repo.GetBoard(ctx, id)
}

// UpdateBoard updates an existing board
func (s *Service) UpdateBoard(ctx context.Context, id string, req *UpdateBoardRequest) (*models.Board, error) {
	board, err := s.repo.GetBoard(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		board.Name = *req.Name
	}
	if req.Description != nil {
		board.Description = *req.Description
	}
	board.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateBoard(ctx, board); err != nil {
		s.logger.Error("failed to update board", zap.String("board_id", id), zap.Error(err))
		return nil, err
	}

	s.publishBoardEvent(ctx, events.BoardUpdated, board)
	s.logger.Info("board updated", zap.String("board_id", board.ID))
	return board, nil
}

// DeleteBoard deletes a board
func (s *Service) DeleteBoard(ctx context.Context, id string) error {
	board, err := s.repo.GetBoard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteBoard(ctx, id); err != nil {
		s.logger.Error("failed to delete board", zap.String("board_id", id), zap.Error(err))
		return err
	}

	s.publishBoardEvent(ctx, events.BoardDeleted, board)
	s.logger.Info("board deleted", zap.String("board_id", id))
	return nil
}

// ListBoards returns all boards for a workspace (or all if empty)
func (s *Service) ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error) {
	return s.repo.ListBoards(ctx, workspaceID)
}

// Repository operations

func (s *Service) CreateRepository(ctx context.Context, req *CreateRepositoryRequest) (*models.Repository, error) {
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "local"
	}
	prefix := strings.TrimSpace(req.WorktreeBranchPrefix)
	if err := worktree.ValidateBranchPrefix(prefix); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
	}
	if prefix == "" {
		prefix = worktree.DefaultBranchPrefix
	}
	pullBeforeWorktree := true
	if req.PullBeforeWorktree != nil {
		pullBeforeWorktree = *req.PullBeforeWorktree
	}
	repository := &models.Repository{
		ID:                   uuid.New().String(),
		WorkspaceID:          req.WorkspaceID,
		Name:                 req.Name,
		SourceType:           sourceType,
		LocalPath:            req.LocalPath,
		Provider:             req.Provider,
		ProviderRepoID:       req.ProviderRepoID,
		ProviderOwner:        req.ProviderOwner,
		ProviderName:         req.ProviderName,
		DefaultBranch:        req.DefaultBranch,
		WorktreeBranchPrefix: prefix,
		PullBeforeWorktree:   pullBeforeWorktree,
		SetupScript:          req.SetupScript,
		CleanupScript:        req.CleanupScript,
		DevScript:            req.DevScript,
	}

	if err := s.repo.CreateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to create repository", zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryCreated, repository)
	s.logger.Info("repository created", zap.String("repository_id", repository.ID))
	return repository, nil
}

func (s *Service) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return s.repo.GetRepository(ctx, id)
}

func (s *Service) UpdateRepository(ctx context.Context, id string, req *UpdateRepositoryRequest) (*models.Repository, error) {
	repository, err := s.repo.GetRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		repository.Name = *req.Name
	}
	if req.SourceType != nil {
		repository.SourceType = *req.SourceType
	}
	if req.LocalPath != nil {
		repository.LocalPath = *req.LocalPath
	}
	if req.Provider != nil {
		repository.Provider = *req.Provider
	}
	if req.ProviderRepoID != nil {
		repository.ProviderRepoID = *req.ProviderRepoID
	}
	if req.ProviderOwner != nil {
		repository.ProviderOwner = *req.ProviderOwner
	}
	if req.ProviderName != nil {
		repository.ProviderName = *req.ProviderName
	}
	if req.DefaultBranch != nil {
		repository.DefaultBranch = *req.DefaultBranch
	}
	if req.WorktreeBranchPrefix != nil {
		prefix := strings.TrimSpace(*req.WorktreeBranchPrefix)
		if err := worktree.ValidateBranchPrefix(prefix); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidRepositorySettings, err)
		}
		repository.WorktreeBranchPrefix = prefix
	}
	if req.PullBeforeWorktree != nil {
		repository.PullBeforeWorktree = *req.PullBeforeWorktree
	}
	if req.SetupScript != nil {
		repository.SetupScript = *req.SetupScript
	}
	if req.CleanupScript != nil {
		repository.CleanupScript = *req.CleanupScript
	}
	if req.DevScript != nil {
		repository.DevScript = *req.DevScript
	}
	repository.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateRepository(ctx, repository); err != nil {
		s.logger.Error("failed to update repository", zap.String("repository_id", id), zap.Error(err))
		return nil, err
	}

	s.publishRepositoryEvent(ctx, events.RepositoryUpdated, repository)
	s.logger.Info("repository updated", zap.String("repository_id", repository.ID))
	return repository, nil
}

func (s *Service) DeleteRepository(ctx context.Context, id string) error {
	repository, err := s.repo.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	active, err := s.repo.HasActiveTaskSessionsByRepository(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteRepository(ctx, id); err != nil {
		s.logger.Error("failed to delete repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
	s.logger.Info("repository deleted", zap.String("repository_id", id))
	return nil
}

func (s *Service) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	return s.repo.ListRepositories(ctx, workspaceID)
}

// Repository script operations

func (s *Service) CreateRepositoryScript(ctx context.Context, req *CreateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script := &models.RepositoryScript{
		ID:           uuid.New().String(),
		RepositoryID: req.RepositoryID,
		Name:         req.Name,
		Command:      req.Command,
		Position:     req.Position,
	}
	if err := s.repo.CreateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to create repository script", zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptCreated, script)
	s.logger.Info("repository script created", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	return s.repo.GetRepositoryScript(ctx, id)
}

func (s *Service) UpdateRepositoryScript(ctx context.Context, id string, req *UpdateRepositoryScriptRequest) (*models.RepositoryScript, error) {
	script, err := s.repo.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		script.Name = *req.Name
	}
	if req.Command != nil {
		script.Command = *req.Command
	}
	if req.Position != nil {
		script.Position = *req.Position
	}
	script.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateRepositoryScript(ctx, script); err != nil {
		s.logger.Error("failed to update repository script", zap.String("script_id", id), zap.Error(err))
		return nil, err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptUpdated, script)
	s.logger.Info("repository script updated", zap.String("script_id", script.ID))
	return script, nil
}

func (s *Service) DeleteRepositoryScript(ctx context.Context, id string) error {
	script, err := s.repo.GetRepositoryScript(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRepositoryScript(ctx, id); err != nil {
		s.logger.Error("failed to delete repository script", zap.String("script_id", id), zap.Error(err))
		return err
	}
	s.publishRepositoryScriptEvent(ctx, events.RepositoryScriptDeleted, script)
	s.logger.Info("repository script deleted", zap.String("script_id", id))
	return nil
}

func (s *Service) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	return s.repo.ListRepositoryScripts(ctx, repositoryID)
}

// Executor operations

func (s *Service) CreateExecutor(ctx context.Context, req *CreateExecutorRequest) (*models.Executor, error) {
	if err := validateExecutorConfig(req.Config); err != nil {
		return nil, err
	}
	executor := &models.Executor{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Type:      req.Type,
		Status:    req.Status,
		IsSystem:  req.IsSystem,
		Resumable: req.Resumable,
		Config:    req.Config,
	}

	if err := s.repo.CreateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorCreated, executor)
	return executor, nil
}

func (s *Service) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	return s.repo.GetExecutor(ctx, id)
}

func (s *Service) UpdateExecutor(ctx context.Context, id string, req *UpdateExecutorRequest) (*models.Executor, error) {
	executor, err := s.repo.GetExecutor(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Config != nil {
		if err := validateExecutorConfig(req.Config); err != nil {
			return nil, err
		}
	}
	if executor.IsSystem {
		if req.Name != nil && *req.Name != executor.Name {
			return nil, fmt.Errorf("system executors cannot be modified")
		}
		if req.Type != nil && *req.Type != executor.Type {
			return nil, fmt.Errorf("system executors cannot be modified")
		}
		if req.Status != nil && *req.Status != executor.Status {
			return nil, fmt.Errorf("system executors cannot be modified")
		}
		if req.Resumable != nil && *req.Resumable != executor.Resumable {
			return nil, fmt.Errorf("system executors cannot be modified")
		}
	}
	if req.Name != nil {
		executor.Name = *req.Name
	}
	if req.Type != nil {
		executor.Type = *req.Type
	}
	if req.Status != nil {
		executor.Status = *req.Status
	}
	if req.Resumable != nil {
		executor.Resumable = *req.Resumable
	}
	if req.Config != nil {
		executor.Config = req.Config
	}
	executor.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateExecutor(ctx, executor); err != nil {
		return nil, err
	}
	s.publishExecutorEvent(ctx, events.ExecutorUpdated, executor)
	return executor, nil
}

func (s *Service) DeleteExecutor(ctx context.Context, id string) error {
	executor, err := s.repo.GetExecutor(ctx, id)
	if err != nil {
		return err
	}
	if executor.IsSystem {
		return fmt.Errorf("system executors cannot be deleted")
	}
	active, err := s.repo.HasActiveTaskSessionsByExecutor(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for executor", zap.String("executor_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteExecutor(ctx, id); err != nil {
		return err
	}
	s.publishExecutorEvent(ctx, events.ExecutorDeleted, executor)
	return nil
}

func (s *Service) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	return s.repo.ListExecutors(ctx)
}

// Environment operations

func (s *Service) CreateEnvironment(ctx context.Context, req *CreateEnvironmentRequest) (*models.Environment, error) {
	environment := &models.Environment{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Kind:         req.Kind,
		IsSystem:     false,
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	}
	if err := s.repo.CreateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentCreated, environment)
	return environment, nil
}

func (s *Service) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	return s.repo.GetEnvironment(ctx, id)
}

func (s *Service) UpdateEnvironment(ctx context.Context, id string, req *UpdateEnvironmentRequest) (*models.Environment, error) {
	environment, err := s.repo.GetEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	if environment.IsSystem {
		if req.Name != nil || req.Kind != nil || req.ImageTag != nil || req.Dockerfile != nil || req.BuildConfig != nil {
			return nil, fmt.Errorf("system environments can only update the worktree root")
		}
	}
	if req.Name != nil {
		environment.Name = *req.Name
	}
	if req.Kind != nil {
		environment.Kind = *req.Kind
	}
	if req.WorktreeRoot != nil {
		environment.WorktreeRoot = *req.WorktreeRoot
	}
	if req.ImageTag != nil {
		environment.ImageTag = *req.ImageTag
	}
	if req.Dockerfile != nil {
		environment.Dockerfile = *req.Dockerfile
	}
	if req.BuildConfig != nil {
		environment.BuildConfig = req.BuildConfig
	}
	environment.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateEnvironment(ctx, environment); err != nil {
		return nil, err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentUpdated, environment)
	return environment, nil
}

func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	environment, err := s.repo.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	if environment.IsSystem {
		return fmt.Errorf("system environments cannot be deleted")
	}
	active, err := s.repo.HasActiveTaskSessionsByEnvironment(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for environment", zap.String("environment_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveTaskSessions
	}
	if err := s.repo.DeleteEnvironment(ctx, id); err != nil {
		return err
	}
	s.publishEnvironmentEvent(ctx, events.EnvironmentDeleted, environment)
	return nil
}

func (s *Service) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	return s.repo.ListEnvironments(ctx)
}

// publishTaskEvent publishes task events to the event bus
func (s *Service) publishTaskEvent(ctx context.Context, eventType string, task *models.Task, oldState *v1.TaskState) {
	if s.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"task_id":          task.ID,
		"board_id":         task.BoardID,
		"workflow_step_id": task.WorkflowStepID,
		"title":            task.Title,
		"description":      task.Description,
		"state":            string(task.State),
		"priority":         task.Priority,
		"position":         task.Position,
		"created_at":       task.CreatedAt.Format(time.RFC3339),
		"updated_at":       task.UpdatedAt.Format(time.RFC3339),
	}

	if task.Metadata != nil {
		data["metadata"] = task.Metadata
	}

	if oldState != nil {
		data["old_state"] = string(*oldState)
		data["new_state"] = string(task.State)
	}

	event := bus.NewEvent(eventType, "task-service", data)

	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish task event",
			zap.String("event_type", eventType),
			zap.String("task_id", task.ID),
			zap.Error(err))
	}
}

func (s *Service) publishWorkspaceEvent(ctx context.Context, eventType string, workspace *models.Workspace) {
	if s.eventBus == nil || workspace == nil {
		return
	}

	data := map[string]interface{}{
		"id":                       workspace.ID,
		"name":                     workspace.Name,
		"description":              workspace.Description,
		"owner_id":                 workspace.OwnerID,
		"default_executor_id":      workspace.DefaultExecutorID,
		"default_environment_id":   workspace.DefaultEnvironmentID,
		"default_agent_profile_id": workspace.DefaultAgentProfileID,
		"created_at":               workspace.CreatedAt.Format(time.RFC3339),
		"updated_at":               workspace.UpdatedAt.Format(time.RFC3339),
	}

	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish workspace event",
			zap.String("event_type", eventType),
			zap.String("workspace_id", workspace.ID),
			zap.Error(err))
	}
}

func (s *Service) publishBoardEvent(ctx context.Context, eventType string, board *models.Board) {
	if s.eventBus == nil || board == nil {
		return
	}

	data := map[string]interface{}{
		"id":           board.ID,
		"workspace_id": board.WorkspaceID,
		"name":         board.Name,
		"description":  board.Description,
		"created_at":   board.CreatedAt.Format(time.RFC3339),
		"updated_at":   board.UpdatedAt.Format(time.RFC3339),
	}

	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish board event",
			zap.String("event_type", eventType),
			zap.String("board_id", board.ID),
			zap.Error(err))
	}
}

func (s *Service) publishExecutorEvent(ctx context.Context, eventType string, executor *models.Executor) {
	if s.eventBus == nil || executor == nil {
		return
	}

	data := map[string]interface{}{
		"id":         executor.ID,
		"name":       executor.Name,
		"type":       executor.Type,
		"status":     executor.Status,
		"is_system":  executor.IsSystem,
		"resumable":  executor.Resumable,
		"config":     executor.Config,
		"created_at": executor.CreatedAt.Format(time.RFC3339),
		"updated_at": executor.UpdatedAt.Format(time.RFC3339),
	}

	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish executor event",
			zap.String("event_type", eventType),
			zap.String("executor_id", executor.ID),
			zap.Error(err))
	}
}

func (s *Service) publishEnvironmentEvent(ctx context.Context, eventType string, environment *models.Environment) {
	if s.eventBus == nil || environment == nil {
		return
	}

	data := map[string]interface{}{
		"id":            environment.ID,
		"name":          environment.Name,
		"kind":          environment.Kind,
		"is_system":     environment.IsSystem,
		"worktree_root": environment.WorktreeRoot,
		"image_tag":     environment.ImageTag,
		"dockerfile":    environment.Dockerfile,
		"build_config":  environment.BuildConfig,
		"created_at":    environment.CreatedAt.Format(time.RFC3339),
		"updated_at":    environment.UpdatedAt.Format(time.RFC3339),
	}

	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish environment event",
			zap.String("event_type", eventType),
			zap.String("environment_id", environment.ID),
			zap.Error(err))
	}
}

// Message operations

// CreateMessageRequest contains the data for creating a new message
type CreateMessageRequest struct {
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id,omitempty"`
	TurnID        string                 `json:"turn_id"`
	Content       string                 `json:"content"`
	AuthorType    string                 `json:"author_type,omitempty"` // "user" or "agent", defaults to "user"
	AuthorID      string                 `json:"author_id,omitempty"`
	RequestsInput bool                   `json:"requests_input,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CreateMessage creates a new message on an agent session
func (s *Service) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*models.Message, error) {
	session, err := s.repo.GetTaskSession(ctx, req.TaskSessionID)
	if err != nil {
		return nil, err
	}

	authorType := models.MessageAuthorUser
	if req.AuthorType == "agent" {
		authorType = models.MessageAuthorAgent
	}

	messageType := models.MessageType(req.Type)
	if messageType == "" {
		messageType = models.MessageTypeMessage
	}

	taskID := req.TaskID
	if taskID == "" && session != nil {
		taskID = session.TaskID
	}

	// Ensure we have a turn ID - get active turn or start a new one
	turnID := req.TurnID
	if turnID == "" {
		turn, err := s.getOrStartTurn(ctx, req.TaskSessionID)
		if err != nil {
			s.logger.Warn("failed to get or start turn for message",
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
			// Continue with empty turn ID - will fail on foreign key if turn is required
		} else if turn != nil {
			turnID = turn.ID
		}
	}

	message := &models.Message{
		ID:            uuid.New().String(),
		TaskSessionID: req.TaskSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    authorType,
		AuthorID:      req.AuthorID,
		Content:       req.Content,
		Type:          messageType,
		Metadata:      req.Metadata,
		RequestsInput: req.RequestsInput,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.repo.CreateMessage(ctx, message); err != nil {
		s.logger.Error("failed to create message", zap.Error(err))
		return nil, err
	}

	// Publish message.added event
	s.publishMessageEvent(ctx, events.MessageAdded, message)

	s.logger.Info("message created",
		zap.String("message_id", message.ID),
		zap.String("session_id", message.TaskSessionID),
		zap.String("author_type", string(message.AuthorType)))

	return message, nil
}

// CreateMessageWithID creates a new message with a pre-generated ID.
// This is used for streaming messages where the ID is generated by the caller.
// It includes retry logic to handle transient database errors and ensure
// message chunks are not lost during streaming.
func (s *Service) CreateMessageWithID(ctx context.Context, id string, req *CreateMessageRequest) (*models.Message, error) {
	const maxRetries = 5
	const retryDelay = 50 * time.Millisecond

	var session *models.TaskSession
	var err error

	// Retry loop for getting the session (may not exist yet if events arrive out of order)
	for attempt := 0; attempt < maxRetries; attempt++ {
		session, err = s.repo.GetTaskSession(ctx, req.TaskSessionID)
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt < maxRetries-1 {
			s.logger.Debug("session not found for message create, retrying",
				zap.String("session_id", req.TaskSessionID),
				zap.String("message_id", id),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("session not found for message create after retries",
			zap.String("session_id", req.TaskSessionID),
			zap.String("message_id", id),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return nil, err
	}

	authorType := models.MessageAuthorUser
	if req.AuthorType == "agent" {
		authorType = models.MessageAuthorAgent
	}

	messageType := models.MessageType(req.Type)
	if messageType == "" {
		messageType = models.MessageTypeMessage
	}

	taskID := req.TaskID
	if taskID == "" && session != nil {
		taskID = session.TaskID
	}

	// Ensure we have a turn ID - get active turn or start a new one
	turnID := req.TurnID
	if turnID == "" {
		turn, err := s.getOrStartTurn(ctx, req.TaskSessionID)
		if err != nil {
			s.logger.Warn("failed to get or start turn for streaming message",
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
			// Continue with empty turn ID - will fail on foreign key if turn is required
		} else if turn != nil {
			turnID = turn.ID
		}
	}

	message := &models.Message{
		ID:            id,
		TaskSessionID: req.TaskSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    authorType,
		AuthorID:      req.AuthorID,
		Content:       req.Content,
		Type:          messageType,
		Metadata:      req.Metadata,
		RequestsInput: req.RequestsInput,
		CreatedAt:     time.Now().UTC(),
	}

	// Retry loop for creating the message (handles transient DB errors)
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = s.repo.CreateMessage(ctx, message)
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt < maxRetries-1 {
			s.logger.Debug("failed to create message, retrying",
				zap.String("message_id", id),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Error("failed to create message with ID after retries",
			zap.String("id", id),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return nil, err
	}

	// Publish message.added event
	s.publishMessageEvent(ctx, events.MessageAdded, message)

	s.logger.Info("message created with ID",
		zap.String("message_id", message.ID),
		zap.String("session_id", message.TaskSessionID),
		zap.String("author_type", string(message.AuthorType)))

	return message, nil
}

// GetMessage retrieves a message by ID
func (s *Service) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	return s.repo.GetMessage(ctx, id)
}

// ListMessages returns all messages for a session.
func (s *Service) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	return s.repo.ListMessages(ctx, sessionID)
}

// ListMessagesPaginated returns messages for a session with pagination options.
func (s *Service) ListMessagesPaginated(ctx context.Context, req ListMessagesRequest) ([]*models.Message, bool, error) {
	limit := req.Limit
	if limit <= 0 && (req.Before != "" || req.After != "") {
		limit = DefaultMessagesPageSize
	}
	if limit > MaxMessagesPageSize {
		limit = MaxMessagesPageSize
	}
	return s.repo.ListMessagesPaginated(ctx, req.TaskSessionID, models.ListMessagesOptions{
		Limit:  limit,
		Before: req.Before,
		After:  req.After,
		Sort:   req.Sort,
	})
}

// DeleteMessage deletes a message
func (s *Service) DeleteMessage(ctx context.Context, id string) error {
	if err := s.repo.DeleteMessage(ctx, id); err != nil {
		s.logger.Error("failed to delete message", zap.String("message_id", id), zap.Error(err))
		return err
	}

	s.logger.Info("message deleted", zap.String("message_id", id))
	return nil
}

// UpdateMessage updates an existing message and publishes an event.
func (s *Service) UpdateMessage(ctx context.Context, message *models.Message) error {
	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update message",
			zap.String("message_id", message.ID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event for real-time streaming
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	return nil
}

// AppendMessageContent appends additional content to an existing message.
// This is used for streaming agent responses where content arrives incrementally.
func (s *Service) AppendMessageContent(ctx context.Context, messageID, additionalContent string) error {
	message, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		s.logger.Warn("message not found for append",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Append the new content
	message.Content = message.Content + additionalContent

	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to append message content",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event for real-time streaming
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Debug("message content appended",
		zap.String("message_id", messageID),
		zap.Int("appended_length", len(additionalContent)),
		zap.Int("total_length", len(message.Content)))

	return nil
}

// UpdateToolCallMessage updates a tool call message's status, and optionally title/args.
// It includes retry logic to handle race conditions where the complete event
// may arrive before the message has been created by the start event.
func (s *Service) UpdateToolCallMessage(ctx context.Context, sessionID, toolCallID, status, result, title string, args map[string]interface{}) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	var message *models.Message
	var err error

	// Retry loop to handle race condition where complete event arrives before start event
	// has finished creating the message in the database
	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.repo.GetMessageByToolCallID(ctx, sessionID, toolCallID)
		if err == nil {
			break // Found the message, proceed with update
		}

		// If context is cancelled, don't retry
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Log retry attempt (only warn on final failure)
		if attempt < maxRetries-1 {
			s.logger.Debug("tool call message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("tool_call_id", toolCallID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("tool call message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = status
	if result != "" {
		message.Metadata["result"] = result
	}

	// Update title if provided and current title is just the tool name (not yet filled)
	// This handles the case where the first event only had the tool name
	if title != "" {
		currentTitle, _ := message.Metadata["title"].(string)
		if currentTitle == "" || currentTitle == message.Metadata["tool_name"] {
			message.Content = title
			message.Metadata["title"] = title
		}
	}

	// Update args if provided and current args are empty
	if len(args) > 0 {
		currentArgs, _ := message.Metadata["args"].(map[string]interface{})
		if len(currentArgs) == 0 {
			message.Metadata["args"] = args
		}
	}

	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update tool call message",
			zap.String("message_id", message.ID),
			zap.String("tool_call_id", toolCallID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Info("tool call message updated",
		zap.String("message_id", message.ID),
		zap.String("tool_call_id", toolCallID),
		zap.String("status", status))

	return nil
}

// UpdatePermissionMessage updates a permission request message's status.
// It includes retry logic to handle race conditions.
func (s *Service) UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	var message *models.Message
	var err error

	// Retry loop to handle race condition
	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.repo.GetMessageByPendingID(ctx, sessionID, pendingID)
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < maxRetries-1 {
			s.logger.Debug("permission message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("permission message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("pending_id", pendingID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = status

	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update permission message",
			zap.String("message_id", message.ID),
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Info("permission message updated",
		zap.String("message_id", message.ID),
		zap.String("pending_id", pendingID),
		zap.String("status", status))

	return nil
}

// Turn operations

// StartTurn creates a new turn for a session and publishes the turn.started event.
// Returns the created turn.
func (s *Service) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	turn := &models.Turn{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        session.TaskID,
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.repo.CreateTurn(ctx, turn); err != nil {
		s.logger.Error("failed to create turn", zap.Error(err))
		return nil, err
	}

	s.publishTurnEvent(events.TurnStarted, turn)

	s.logger.Debug("started turn",
		zap.String("turn_id", turn.ID),
		zap.String("session_id", sessionID),
		zap.String("task_id", turn.TaskID))

	return turn, nil
}

// CompleteTurn marks a turn as completed and publishes the turn.completed event.
func (s *Service) CompleteTurn(ctx context.Context, turnID string) error {
	if turnID == "" {
		return nil // No active turn to complete
	}

	if err := s.repo.CompleteTurn(ctx, turnID); err != nil {
		s.logger.Error("failed to complete turn", zap.String("turn_id", turnID), zap.Error(err))
		return err
	}

	// Fetch the completed turn to get the completed_at timestamp
	turn, err := s.repo.GetTurn(ctx, turnID)
	if err != nil {
		s.logger.Warn("failed to refetch completed turn", zap.String("turn_id", turnID), zap.Error(err))
		// Still publish with the old turn data
	}

	s.publishTurnEvent(events.TurnCompleted, turn)

	s.logger.Debug("completed turn",
		zap.String("turn_id", turnID),
		zap.String("session_id", turn.TaskSessionID),
		zap.String("task_id", turn.TaskID))

	return nil
}

// GetActiveTurn returns the currently active (non-completed) turn for a session.
func (s *Service) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return s.repo.GetActiveTurnBySessionID(ctx, sessionID)
}

// getOrStartTurn returns the active turn for a session, or starts a new one if none exists.
// This is used to ensure messages always have a valid turn ID.
func (s *Service) getOrStartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	// First try to get an active turn
	turn, err := s.repo.GetActiveTurnBySessionID(ctx, sessionID)
	if err == nil && turn != nil {
		return turn, nil
	}

	// No active turn, start a new one
	return s.StartTurn(ctx, sessionID)
}

// publishTurnEvent publishes a turn event to the event bus.
func (s *Service) publishTurnEvent(eventType string, turn *models.Turn) {
	if s.eventBus == nil {
		return
	}
	_ = s.eventBus.Publish(context.Background(), eventType, bus.NewEvent(eventType, "task-service", map[string]interface{}{
		"id":           turn.ID,
		"session_id":   turn.TaskSessionID,
		"task_id":      turn.TaskID,
		"started_at":   turn.StartedAt,
		"completed_at": turn.CompletedAt,
		"metadata":     turn.Metadata,
		"created_at":   turn.CreatedAt,
		"updated_at":   turn.UpdatedAt,
	}))
}

// publishMessageEvent publishes message events to the event bus.
// System-injected content (wrapped in <kandev-system> tags) is stripped from the content
// so users don't see workflow step prompt modifications in the UI.
func (s *Service) publishMessageEvent(ctx context.Context, eventType string, message *models.Message) {
	if s.eventBus == nil {
		s.logger.Warn("publishMessageEvent: eventBus is nil, skipping")
		return
	}

	messageType := string(message.Type)
	if messageType == "" {
		messageType = "message"
	}

	data := map[string]interface{}{
		"message_id":     message.ID,
		"session_id":     message.TaskSessionID,
		"task_id":        message.TaskID,
		"author_type":    string(message.AuthorType),
		"author_id":      message.AuthorID,
		"content":        models.StripSystemContent(message.Content),
		"type":           messageType,
		"requests_input": message.RequestsInput,
		"created_at":     message.CreatedAt.Format(time.RFC3339),
	}

	if message.Metadata != nil {
		data["metadata"] = message.Metadata
	}

	event := bus.NewEvent(eventType, "task-service", data)

	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish message event",
			zap.String("event_type", eventType),
			zap.String("message_id", message.ID),
			zap.Error(err))
	}
}

func (s *Service) publishRepositoryEvent(ctx context.Context, eventType string, repository *models.Repository) {
	if s.eventBus == nil || repository == nil {
		return
	}
	data := map[string]interface{}{
		"id":                     repository.ID,
		"workspace_id":           repository.WorkspaceID,
		"name":                   repository.Name,
		"source_type":            repository.SourceType,
		"local_path":             repository.LocalPath,
		"provider":               repository.Provider,
		"provider_repo_id":       repository.ProviderRepoID,
		"provider_owner":         repository.ProviderOwner,
		"provider_name":          repository.ProviderName,
		"default_branch":         repository.DefaultBranch,
		"worktree_branch_prefix": repository.WorktreeBranchPrefix,
		"pull_before_worktree":   repository.PullBeforeWorktree,
		"setup_script":           repository.SetupScript,
		"cleanup_script":         repository.CleanupScript,
		"created_at":             repository.CreatedAt.Format(time.RFC3339),
		"updated_at":             repository.UpdatedAt.Format(time.RFC3339),
	}
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish repository event",
			zap.String("event_type", eventType),
			zap.String("repository_id", repository.ID),
			zap.Error(err))
	}
}

func (s *Service) publishRepositoryScriptEvent(ctx context.Context, eventType string, script *models.RepositoryScript) {
	if s.eventBus == nil || script == nil {
		return
	}
	data := map[string]interface{}{
		"id":            script.ID,
		"repository_id": script.RepositoryID,
		"name":          script.Name,
		"command":       script.Command,
		"position":      script.Position,
		"created_at":    script.CreatedAt.Format(time.RFC3339),
		"updated_at":    script.UpdatedAt.Format(time.RFC3339),
	}
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish repository script event",
			zap.String("event_type", eventType),
			zap.String("script_id", script.ID),
			zap.Error(err))
	}
}

// GetGitSnapshots retrieves git snapshots for a session
func (s *Service) GetGitSnapshots(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	return s.repo.GetGitSnapshotsBySession(ctx, sessionID, limit)
}

// GetLatestGitSnapshot retrieves the latest git snapshot for a session
func (s *Service) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return s.repo.GetLatestGitSnapshot(ctx, sessionID)
}

// GetFirstGitSnapshot retrieves the first git snapshot for a session (oldest)
func (s *Service) GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return s.repo.GetFirstGitSnapshot(ctx, sessionID)
}

// GetSessionCommits retrieves commits for a session
func (s *Service) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	return s.repo.GetSessionCommits(ctx, sessionID)
}

// GetLatestSessionCommit retrieves the latest commit for a session
func (s *Service) GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error) {
	return s.repo.GetLatestSessionCommit(ctx, sessionID)
}

// GetCumulativeDiff computes the cumulative diff from base commit to current HEAD
// by using the first snapshot's base_commit and the latest snapshot's files
func (s *Service) GetCumulativeDiff(ctx context.Context, sessionID string) (*models.CumulativeDiff, error) {
	// Get the first snapshot to find the base commit
	firstSnapshot, err := s.repo.GetFirstGitSnapshot(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get first git snapshot: %w", err)
	}

	// Get the latest snapshot for current state
	latestSnapshot, err := s.repo.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest git snapshot: %w", err)
	}

	// Count total commits for this session
	commits, err := s.repo.GetSessionCommits(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session commits: %w", err)
	}

	return &models.CumulativeDiff{
		SessionID:    sessionID,
		BaseCommit:   firstSnapshot.BaseCommit,
		HeadCommit:   latestSnapshot.HeadCommit,
		TotalCommits: len(commits),
		Files:        latestSnapshot.Files,
	}, nil
}

// GetWorkspaceInfoForSession returns workspace information for a task session.
// This implements the lifecycle.WorkspaceInfoProvider interface.
// The taskID parameter is optional - if empty, it will be looked up from the session.
func (s *Service) GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*lifecycle.WorkspaceInfo, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session %s: %w", sessionID, err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Use session's TaskID if not provided
	if taskID == "" {
		taskID = session.TaskID
	}

	// Get workspace path from the session's worktree
	var workspacePath string
	if len(session.Worktrees) > 0 {
		workspacePath = session.Worktrees[0].WorktreePath
	}

	// If no worktree, try to get from repository snapshot
	if workspacePath == "" && session.RepositorySnapshot != nil {
		if path, ok := session.RepositorySnapshot["path"].(string); ok {
			workspacePath = path
		}
	}

	// Get agent ID from profile snapshot
	var agentID string
	if session.AgentProfileSnapshot != nil {
		if id, ok := session.AgentProfileSnapshot["agent_id"].(string); ok {
			agentID = id
		}
	}

	// Get ACP session ID from metadata
	var acpSessionID string
	if session.Metadata != nil {
		if id, ok := session.Metadata["acp_session_id"].(string); ok {
			acpSessionID = id
		}
	}

	return &lifecycle.WorkspaceInfo{
		TaskID:         taskID,
		SessionID:      sessionID,
		WorkspacePath:  workspacePath,
		AgentProfileID: session.AgentProfileID,
		AgentID:        agentID,
		ACPSessionID:   acpSessionID,
	}, nil
}
