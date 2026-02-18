package service

import (
	"context"
	"database/sql"
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
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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
	StopSession(ctx context.Context, sessionID, reason string, force bool) error
}

// WorkflowStepCreator creates workflow steps from a template for a workflow.
type WorkflowStepCreator interface {
	CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error
}

// WorkflowStepGetter retrieves workflow step information.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	// GetNextStepByPosition returns the next step after the given position for a workflow.
	// Returns nil if there is no next step (i.e., current step is the last one).
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
}

// StartStepResolver resolves the starting step for a workflow.
type StartStepResolver interface {
	ResolveStartStep(ctx context.Context, workflowID string) (string, error)
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
	startStepResolver   StartStepResolver
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

// SetWorkflowStepCreator wires the workflow step creator for workflow creation.
func (s *Service) SetWorkflowStepCreator(creator WorkflowStepCreator) {
	s.workflowStepCreator = creator
}

// SetWorkflowStepGetter wires the workflow step getter for MoveTask.
func (s *Service) SetWorkflowStepGetter(getter WorkflowStepGetter) {
	s.workflowStepGetter = getter
}

// SetStartStepResolver wires the start step resolver for CreateTask.
func (s *Service) SetStartStepResolver(resolver StartStepResolver) {
	s.startStepResolver = resolver
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
	WorkflowID        string                 `json:"workflow_id"`
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

// CreateWorkflowRequest contains the data for creating a new workflow
type CreateWorkflowRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

// UpdateWorkflowRequest contains the data for updating a workflow
type UpdateWorkflowRequest struct {
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
	// Auto-resolve start step if not provided
	workflowStepID := req.WorkflowStepID
	if workflowStepID == "" && req.WorkflowID != "" && s.startStepResolver != nil {
		resolvedID, err := s.startStepResolver.ResolveStartStep(ctx, req.WorkflowID)
		if err != nil {
			s.logger.Warn("failed to resolve start step, using empty",
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
		} else {
			workflowStepID = resolvedID
		}
	}

	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	task := &models.Task{
		ID:             uuid.New().String(),
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:        req.WorkflowID,
		WorkflowStepID: workflowStepID,
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

	if err := s.createTaskRepositories(ctx, task.ID, req.WorkspaceID, req.Repositories); err != nil {
		return nil, err
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

// createTaskRepositories creates task-repository associations, resolving local paths to repository IDs.
func (s *Service) createTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	var repoByPath map[string]*models.Repository
	for _, repoInput := range repositories {
		if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
			repos, err := s.repo.ListRepositories(ctx, workspaceID)
			if err != nil {
				s.logger.Error("failed to list repositories", zap.Error(err))
				return err
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

	for i, repoInput := range repositories {
		repositoryID, baseBranch, err := s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
		if err != nil {
			return err
		}
		if repositoryID == "" {
			return fmt.Errorf("repository_id is required")
		}
		taskRepo := &models.TaskRepository{
			TaskID:       taskID,
			RepositoryID: repositoryID,
			BaseBranch:   baseBranch,
			Position:     i,
			Metadata:     make(map[string]interface{}),
		}
		if err := s.repo.CreateTaskRepository(ctx, taskRepo); err != nil {
			s.logger.Error("failed to create task repository", zap.Error(err))
			return err
		}
	}
	return nil
}

// resolveRepoInput resolves a RepositoryInput to a repositoryID and baseBranch,
// creating the repository if it doesn't exist yet.
func (s *Service) resolveRepoInput(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, repoByPath map[string]*models.Repository) (repositoryID, baseBranch string, err error) {
	repositoryID = repoInput.RepositoryID
	baseBranch = repoInput.BaseBranch
	if repositoryID != "" || repoInput.LocalPath == "" {
		return repositoryID, baseBranch, nil
	}
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
		created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
			WorkspaceID:   workspaceID,
			Name:          name,
			SourceType:    "local",
			LocalPath:     repoInput.LocalPath,
			DefaultBranch: defaultBranch,
		})
		if createErr != nil {
			return "", "", createErr
		}
		repo = created
		if repoByPath != nil {
			repoByPath[repoInput.LocalPath] = repo
		}
	}
	repositoryID = repo.ID
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repositoryID, baseBranch, nil
}

// replaceTaskRepositories deletes all existing task-repository associations and recreates them.
func (s *Service) replaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	if err := s.repo.DeleteTaskRepositoriesByTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete task repositories", zap.Error(err))
		return err
	}
	return s.createTaskRepositories(ctx, taskID, workspaceID, repositories)
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
		if err := s.replaceTaskRepositories(ctx, task.ID, task.WorkspaceID, req.Repositories); err != nil {
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

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

// ArchiveTask archives a task by setting its archived_at timestamp.
// The task remains in the DB but is excluded from active board views.
// Active agent sessions are stopped and worktrees cleaned up in background.
func (s *Service) ArchiveTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task and verify it exists
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if task.ArchivedAt != nil {
		return fmt.Errorf("task is already archived: %s", id)
	}

	// 2. Gather data needed for cleanup BEFORE archive
	var activeSessionIDs []string
	if s.executionStopper != nil {
		activeSessions, err := s.repo.ListActiveTaskSessionsByTaskID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list active sessions for archive",
				zap.String("task_id", id),
				zap.Error(err))
		}
		for _, sess := range activeSessions {
			activeSessionIDs = append(activeSessionIDs, sess.ID)
		}
	}

	sessions, err := s.repo.ListTaskSessions(ctx, id)
	if err != nil {
		s.logger.Warn("failed to list task sessions for archive",
			zap.String("task_id", id),
			zap.Error(err))
	}

	var worktrees []*worktree.Worktree
	if s.worktreeCleanup != nil {
		if provider, ok := s.worktreeCleanup.(WorktreeProvider); ok {
			worktrees, err = provider.GetAllByTaskID(ctx, id)
			if err != nil {
				s.logger.Warn("failed to list worktrees for archive",
					zap.String("task_id", id),
					zap.Error(err))
			}
		}
	}

	// 3. Set archived_at in DB
	if err := s.repo.ArchiveTask(ctx, id); err != nil {
		return err
	}

	// 4. Re-read task for updated archived_at field
	task, err = s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 5. Publish task.updated event so frontend removes from board
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task archived",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Background: Stop agents and cleanup worktrees
	if len(activeSessionIDs) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 {
		s.runAsyncTaskCleanup(id, sessions, worktrees, activeSessionIDs,
			"task archived", "failed to stop session on task archive", "task archive cleanup completed")
	}

	return nil
}

// DeleteTask deletes a task and publishes a task.deleted event.
// For fast UI response, the DB delete and event publish happen synchronously,
// while agent stopping and worktree cleanup happen asynchronously.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	start := time.Now()

	// 1. Get task (sync, fast)
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}

	// 2. Gather data needed for cleanup BEFORE delete (sync, fast)
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

	// 3. Get active session IDs for stopping agents (sync, fast)
	// Must query before delete since DB records will be gone
	var activeSessionIDs []string
	if s.executionStopper != nil {
		activeSessions, err := s.repo.ListActiveTaskSessionsByTaskID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to list active sessions for delete",
				zap.String("task_id", id),
				zap.Error(err))
		}
		for _, sess := range activeSessions {
			activeSessionIDs = append(activeSessionIDs, sess.ID)
		}
	}

	// 4. Delete from DB (sync, fast)
	if err := s.repo.DeleteTask(ctx, id); err != nil {
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return err
	}

	// 5. Publish event (sync, fast) - frontend removes task immediately
	s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Return immediately - all remaining cleanup is async

	// 7. Background: Stop agents and cleanup worktrees
	if len(activeSessionIDs) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 {
		s.runAsyncTaskCleanup(id, sessions, worktrees, activeSessionIDs,
			"task deleted", "failed to stop session on task delete", "task cleanup completed")
	}

	return nil
}

func (s *Service) runAsyncTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	activeSessionIDs []string,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	go func() {
		cleanupStart := time.Now()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if s.executionStopper != nil && len(activeSessionIDs) > 0 {
			for _, sessionID := range activeSessionIDs {
				if err := s.executionStopper.StopSession(cleanupCtx, sessionID, stopReason, true); err != nil {
					s.logger.Warn(stopFailMsg,
						zap.String("task_id", id),
						zap.String("session_id", sessionID),
						zap.Error(err))
				}
			}
		}

		cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees)

		if len(cleanupErrors) > 0 {
			s.logger.Warn(cleanupMsg+" with errors",
				zap.String("task_id", id),
				zap.Int("error_count", len(cleanupErrors)),
				zap.Duration("duration", time.Since(cleanupStart)))
		} else {
			s.logger.Info(cleanupMsg,
				zap.String("task_id", id),
				zap.Duration("duration", time.Since(cleanupStart)))
		}
	}()
}

// performTaskCleanup handles post-deletion cleanup operations.
// Handles worktree cleanup and executor_running records.
// Agent stopping is handled separately in the DeleteTask background goroutine.
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

// ListTasks returns all tasks for a workflow
func (s *Service) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	tasks, err := s.repo.ListTasks(ctx, workflowID)
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

// ListTasksByWorkspace returns paginated tasks for a workspace with task repositories loaded.
// If query is non-empty, filters by task title, description, repository name, or repository path.
func (s *Service) ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int, includeArchived bool) ([]*models.Task, int, error) {
	tasks, total, err := s.repo.ListTasksByWorkspace(ctx, workspaceID, query, page, pageSize, includeArchived)
	if err != nil {
		return nil, 0, err
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

	return tasks, total, nil
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

// GetSessionCountsForTasks returns a map of task ID to session count for the given task IDs.
func (s *Service) GetSessionCountsForTasks(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return s.repo.GetSessionCountsByTaskIDs(ctx, taskIDs)
}

// GetPrimarySessionInfoForTasks returns a map of task ID to primary session info for the given task IDs.
func (s *Service) GetPrimarySessionInfoForTasks(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return s.repo.GetPrimarySessionInfoByTaskIDs(ctx, taskIDs)
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
	WorkflowStep *wfmodels.WorkflowStep
}

// ApproveSession approves a session's current step and moves it to the next step.
// It reads the step's on_turn_complete actions to determine where to transition.
// If no transition actions are configured, it falls back to the next step by position.
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

	// Get the current workflow step to check for transition targets
	if session.WorkflowStepID != nil && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, *session.WorkflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for approval transition",
				zap.String("workflow_step_id", *session.WorkflowStepID),
				zap.Error(err))
		} else {
			s.applyApprovalStepTransition(ctx, sessionID, step, result)
		}
	}

	return result, nil
}

// applyApprovalStepTransition resolves the next workflow step and updates session/task accordingly.
func (s *Service) applyApprovalStepTransition(ctx context.Context, sessionID string, step *wfmodels.WorkflowStep, result *ApproveSessionResult) {
	newStepID := s.resolveApprovalNextStep(ctx, step)

	if newStepID == "" {
		s.logger.Info("session approved but no next step found (may be at final step)",
			zap.String("session_id", sessionID),
			zap.String("current_step", step.ID),
			zap.String("current_step_name", step.Name))
		return
	}

	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, newStepID); err != nil {
		s.logger.Error("failed to move session to next step after approval",
			zap.String("session_id", sessionID),
			zap.String("step_id", newStepID),
			zap.Error(err))
		return
	}

	// Also move the task to the new step
	if task, err := s.repo.GetTask(ctx, result.Session.TaskID); err != nil {
		s.logger.Error("failed to get task for approval transition",
			zap.String("task_id", result.Session.TaskID),
			zap.Error(err))
	} else {
		task.WorkflowStepID = newStepID
		task.UpdatedAt = time.Now().UTC()
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			s.logger.Error("failed to move task to next step after approval",
				zap.String("task_id", result.Session.TaskID),
				zap.String("step_id", newStepID),
				zap.Error(err))
		} else {
			s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
			result.Task = task
		}
	}

	// Reload session with new step
	result.Session, _ = s.repo.GetTaskSession(ctx, sessionID)

	// Get the new workflow step for the response
	if newStep, err := s.workflowStepGetter.GetStep(ctx, newStepID); err == nil {
		result.WorkflowStep = newStep
	}

	s.logger.Info("session approved and moved to next step",
		zap.String("session_id", sessionID),
		zap.String("from_step", step.ID),
		zap.String("to_step", newStepID))
}

// resolveApprovalNextStep determines the target step ID from a step's on_turn_complete actions,
// falling back to the next step by position when no actions are configured.
func (s *Service) resolveApprovalNextStep(ctx context.Context, step *wfmodels.WorkflowStep) string {
	var newStepID string
	for _, action := range step.Events.OnTurnComplete {
		switch action.Type {
		case "move_to_next":
			nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
			if err != nil {
				s.logger.Warn("failed to get next step by position",
					zap.String("workflow_id", step.WorkflowID),
					zap.Int("current_position", step.Position),
					zap.Error(err))
			} else if nextStep != nil {
				newStepID = nextStep.ID
			}
		case "move_to_step":
			if stepID, ok := action.Config["step_id"].(string); ok && stepID != "" {
				newStepID = stepID
			}
		}
		if newStepID != "" {
			return newStepID
		}
	}

	// Fall back to next step by position if no transition actions found
	if len(step.Events.OnTurnComplete) == 0 {
		nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
		if err != nil {
			s.logger.Warn("failed to get next step by position for fallback",
				zap.String("workflow_id", step.WorkflowID),
				zap.Int("current_position", step.Position),
				zap.Error(err))
		} else if nextStep != nil {
			s.logger.Info("using next step by position for approval transition (fallback)",
				zap.String("current_step", step.Name),
				zap.String("next_step", nextStep.Name))
			newStepID = nextStep.ID
		}
	}

	return newStepID
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
	WorkflowStep *wfmodels.WorkflowStep
}

// MoveTask moves a task to a different workflow step and position
func (s *Service) MoveTask(ctx context.Context, id string, workflowID string, workflowStepID string, position int) (*MoveTaskResult, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if task's primary session is in a step with pending approval
	// If so, prevent moving forward - user must use Approve button or send a message
	if task.WorkflowStepID != workflowStepID {
		primarySession, err := s.repo.GetPrimarySessionByTaskID(ctx, id)
		if err == nil && primarySession != nil {
			if primarySession.ReviewStatus != nil && *primarySession.ReviewStatus == "pending" {
				return nil, fmt.Errorf("task is pending approval - use Approve button to proceed or send a message to request changes")
			}
		}
	}

	oldState := task.State
	task.WorkflowID = workflowID
	task.WorkflowStepID = workflowStepID
	task.Position = position
	task.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Update active session's workflow_step_id to match the new task workflow step
	// This ensures workflow transitions work correctly when tasks are manually moved
	if task.WorkflowStepID != "" {
		activeSession, err := s.repo.GetActiveTaskSessionByTaskID(ctx, id)
		if err == nil && activeSession != nil {
			// Only update if the session's workflow_step_id doesn't match
			if activeSession.WorkflowStepID == nil || *activeSession.WorkflowStepID != task.WorkflowStepID {
				if err := s.repo.UpdateSessionWorkflowStep(ctx, activeSession.ID, task.WorkflowStepID); err != nil {
					s.logger.Warn("failed to update session workflow step after task move",
						zap.String("task_id", id),
						zap.String("session_id", activeSession.ID),
						zap.String("workflow_step_id", task.WorkflowStepID),
						zap.Error(err))
					// Don't fail the operation, just log the warning
				} else {
					s.logger.Info("updated session workflow step to match moved task",
						zap.String("task_id", id),
						zap.String("session_id", activeSession.ID),
						zap.String("workflow_step_id", task.WorkflowStepID))
				}
			}
		}
	}

	// Publish state_changed event if state changed, otherwise just updated
	if oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	} else {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	s.logger.Info("task moved",
		zap.String("task_id", id),
		zap.String("workflow_id", workflowID),
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

// CountTasksByWorkflow returns the number of tasks in a workflow
func (s *Service) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	return s.repo.CountTasksByWorkflow(ctx, workflowID)
}

// CountTasksByWorkflowStep returns the number of tasks in a workflow step
func (s *Service) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	return s.repo.CountTasksByWorkflowStep(ctx, stepID)
}

// BulkMoveTasksResult contains the result of a BulkMoveTasks operation.
type BulkMoveTasksResult struct {
	MovedCount int
}

// BulkMoveTasks moves all tasks from a source workflow/step to a target workflow/step.
// If sourceStepID is empty, all tasks in the source workflow are moved.
func (s *Service) BulkMoveTasks(ctx context.Context, sourceWorkflowID, sourceStepID, targetWorkflowID, targetStepID string) (*BulkMoveTasksResult, error) {
	// Get the tasks to move
	var tasks []*models.Task
	var err error
	if sourceStepID != "" {
		tasks, err = s.repo.ListTasksByWorkflowStep(ctx, sourceStepID)
	} else {
		tasks, err = s.repo.ListTasks(ctx, sourceWorkflowID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks for bulk move: %w", err)
	}

	if len(tasks) == 0 {
		return &BulkMoveTasksResult{MovedCount: 0}, nil
	}

	now := time.Now().UTC()
	for i, task := range tasks {
		task.WorkflowID = targetWorkflowID
		task.WorkflowStepID = targetStepID
		task.Position = i
		task.UpdatedAt = now

		if err := s.repo.UpdateTask(ctx, task); err != nil {
			s.logger.Error("failed to move task in bulk move",
				zap.String("task_id", task.ID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to move task %s: %w", task.ID, err)
		}

		// Update active session's workflow_step_id
		activeSession, err := s.repo.GetActiveTaskSessionByTaskID(ctx, task.ID)
		if err == nil && activeSession != nil {
			if activeSession.WorkflowStepID == nil || *activeSession.WorkflowStepID != targetStepID {
				if err := s.repo.UpdateSessionWorkflowStep(ctx, activeSession.ID, targetStepID); err != nil {
					s.logger.Warn("failed to update session workflow step during bulk move",
						zap.String("task_id", task.ID),
						zap.String("session_id", activeSession.ID),
						zap.Error(err))
				}
			}
		}

		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}

	s.logger.Info("bulk moved tasks",
		zap.String("source_workflow_id", sourceWorkflowID),
		zap.String("source_step_id", sourceStepID),
		zap.String("target_workflow_id", targetWorkflowID),
		zap.String("target_step_id", targetStepID),
		zap.Int("moved_count", len(tasks)))

	return &BulkMoveTasksResult{MovedCount: len(tasks)}, nil
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

// Workflow operations

// CreateWorkflow creates a new workflow
func (s *Service) CreateWorkflow(ctx context.Context, req *CreateWorkflowRequest) (*models.Workflow, error) {
	workflow := &models.Workflow{
		ID:                 uuid.New().String(),
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	}

	if err := s.repo.CreateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to create workflow", zap.Error(err))
		return nil, err
	}

	// Create workflow steps from template if specified
	if req.WorkflowTemplateID != nil && *req.WorkflowTemplateID != "" && s.workflowStepCreator != nil {
		if err := s.workflowStepCreator.CreateStepsFromTemplate(ctx, workflow.ID, *req.WorkflowTemplateID); err != nil {
			s.logger.Error("failed to create workflow steps from template",
				zap.String("workflow_id", workflow.ID),
				zap.String("template_id", *req.WorkflowTemplateID),
				zap.Error(err))
			// Don't fail workflow creation, just log the error
		}
	}

	s.publishWorkflowEvent(ctx, events.WorkflowCreated, workflow)
	s.logger.Info("workflow created", zap.String("workflow_id", workflow.ID), zap.String("name", workflow.Name))
	return workflow, nil
}

// GetWorkflow retrieves a workflow by ID
func (s *Service) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	return s.repo.GetWorkflow(ctx, id)
}

// UpdateWorkflow updates an existing workflow
func (s *Service) UpdateWorkflow(ctx context.Context, id string, req *UpdateWorkflowRequest) (*models.Workflow, error) {
	workflow, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		workflow.Name = *req.Name
	}
	if req.Description != nil {
		workflow.Description = *req.Description
	}
	workflow.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateWorkflow(ctx, workflow); err != nil {
		s.logger.Error("failed to update workflow", zap.String("workflow_id", id), zap.Error(err))
		return nil, err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	s.logger.Info("workflow updated", zap.String("workflow_id", workflow.ID))
	return workflow, nil
}

// DeleteWorkflow deletes a workflow
func (s *Service) DeleteWorkflow(ctx context.Context, id string) error {
	workflow, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteWorkflow(ctx, id); err != nil {
		s.logger.Error("failed to delete workflow", zap.String("workflow_id", id), zap.Error(err))
		return err
	}

	s.publishWorkflowEvent(ctx, events.WorkflowDeleted, workflow)
	s.logger.Info("workflow deleted", zap.String("workflow_id", id))
	return nil
}

// ListWorkflows returns all workflows for a workspace (or all if empty)
func (s *Service) ListWorkflows(ctx context.Context, workspaceID string) ([]*models.Workflow, error) {
	return s.repo.ListWorkflows(ctx, workspaceID)
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
		"workflow_id":         task.WorkflowID,
		"workflow_step_id": task.WorkflowStepID,
		"title":            task.Title,
		"description":      task.Description,
		"state":            string(task.State),
		"priority":         task.Priority,
		"position":         task.Position,
		"created_at":       task.CreatedAt.Format(time.RFC3339),
		"updated_at":       task.UpdatedAt.Format(time.RFC3339),
	}

	// Fetch session count and primary session info for the task
	sessionCountMap, err := s.GetSessionCountsForTasks(ctx, []string{task.ID})
	if err == nil {
		if count, ok := sessionCountMap[task.ID]; ok {
			data["session_count"] = count
		}
	}

	primarySessionInfoMap, err := s.GetPrimarySessionInfoForTasks(ctx, []string{task.ID})
	if err == nil {
		if sessionInfo, ok := primarySessionInfoMap[task.ID]; ok && sessionInfo != nil {
			data["primary_session_id"] = sessionInfo.ID
			if sessionInfo.ReviewStatus != nil {
				data["review_status"] = *sessionInfo.ReviewStatus
			}
		}
	}

	if task.ArchivedAt != nil {
		data["archived_at"] = task.ArchivedAt.Format(time.RFC3339)
	}

	if len(task.Repositories) > 0 {
		data["repository_id"] = task.Repositories[0].RepositoryID
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

func (s *Service) publishEventToBus(ctx context.Context, eventType, resourceType, resourceID string, data map[string]interface{}) {
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish "+resourceType+" event",
			zap.String("event_type", eventType),
			zap.String(resourceType+"_id", resourceID),
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

	s.publishEventToBus(ctx, eventType, "workspace", workspace.ID, data)
}

func (s *Service) publishWorkflowEvent(ctx context.Context, eventType string, workflow *models.Workflow) {
	if s.eventBus == nil || workflow == nil {
		return
	}

	data := map[string]interface{}{
		"id":           workflow.ID,
		"workspace_id": workflow.WorkspaceID,
		"name":         workflow.Name,
		"description":  workflow.Description,
		"created_at":   workflow.CreatedAt.Format(time.RFC3339),
		"updated_at":   workflow.UpdatedAt.Format(time.RFC3339),
	}

	s.publishEventToBus(ctx, eventType, "workflow", workflow.ID, data)
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

	s.publishEventToBus(ctx, eventType, "executor", executor.ID, data)
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

	s.publishEventToBus(ctx, eventType, "environment", environment.ID, data)
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

	session, err := s.getSessionWithRetry(ctx, req.TaskSessionID, id, maxRetries, retryDelay)
	if err != nil {
		return nil, err
	}

	message := s.buildMessage(ctx, id, req, session)

	if err := s.createMessageWithRetry(ctx, message, maxRetries, retryDelay); err != nil {
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

// getSessionWithRetry fetches a session, retrying on transient errors caused by out-of-order events.
func (s *Service) getSessionWithRetry(ctx context.Context, sessionID, messageID string, maxRetries int, retryDelay time.Duration) (*models.TaskSession, error) {
	var session *models.TaskSession
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err == nil {
			return session, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("session not found for message create, retrying",
				zap.String("session_id", sessionID),
				zap.String("message_id", messageID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}
	s.logger.Warn("session not found for message create after retries",
		zap.String("session_id", sessionID),
		zap.String("message_id", messageID),
		zap.Int("retries", maxRetries),
		zap.Error(err))
	return nil, err
}

// buildMessage constructs a Message model from a CreateMessageRequest and resolved session.
func (s *Service) buildMessage(ctx context.Context, id string, req *CreateMessageRequest, session *models.TaskSession) *models.Message {
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

	turnID := req.TurnID
	if turnID == "" {
		if turn, err := s.getOrStartTurn(ctx, req.TaskSessionID); err != nil {
			s.logger.Warn("failed to get or start turn for streaming message",
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
		} else if turn != nil {
			turnID = turn.ID
		}
	}

	return &models.Message{
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
}

// createMessageWithRetry persists a message with retry logic for transient DB errors.
func (s *Service) createMessageWithRetry(ctx context.Context, message *models.Message, maxRetries int, retryDelay time.Duration) error {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = s.repo.CreateMessage(ctx, message)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("failed to create message, retrying",
				zap.String("message_id", message.ID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}
	s.logger.Error("failed to create message with ID after retries",
		zap.String("id", message.ID),
		zap.Int("retries", maxRetries),
		zap.Error(err))
	return err
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
	message.Content += additionalContent

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

// AppendThinkingContent appends additional thinking content to an existing thinking message.
// This updates the metadata.thinking field for streaming agent reasoning.
func (s *Service) AppendThinkingContent(ctx context.Context, messageID, additionalContent string) error {
	message, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		s.logger.Warn("thinking message not found for append",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Initialize metadata if nil
	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}

	// Get existing thinking content and append
	existingThinking := ""
	if existing, ok := message.Metadata["thinking"].(string); ok {
		existingThinking = existing
	}
	message.Metadata["thinking"] = existingThinking + additionalContent

	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to append thinking content",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event for real-time streaming
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Debug("thinking content appended",
		zap.String("message_id", messageID),
		zap.Int("appended_length", len(additionalContent)))

	return nil
}

// UpdateToolCallMessage updates a tool call message's status, optionally title and normalized data.
// It includes retry logic to handle race conditions where the complete event
// may arrive before the message has been created by the start event.
// If the message is not found after retries and taskID/turnID/msgType are provided, it creates the message.
// The normalized parameter contains typed tool payload data that gets added to metadata.
func (s *Service) UpdateToolCallMessage(ctx context.Context, sessionID, toolCallID, status, result, title string, normalized *streams.NormalizedPayload) error {
	return s.UpdateToolCallMessageWithCreate(ctx, sessionID, toolCallID, "", status, result, title, normalized, "", "", "")
}

// UpdateToolCallMessageWithCreate is like UpdateToolCallMessage but can create the message if not found.
// If taskID, turnID, and msgType are provided, the message will be created if it doesn't exist.
// parentToolCallID is used for subagent nesting (empty for top-level).
func (s *Service) UpdateToolCallMessageWithCreate(ctx context.Context, sessionID, toolCallID, parentToolCallID, status, result, title string, normalized *streams.NormalizedPayload, taskID, turnID, msgType string) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	message, err := s.getToolCallMessageWithRetry(ctx, sessionID, toolCallID, maxRetries, retryDelay)

	// If message not found and we have enough info to create it, do so
	if err != nil && taskID != "" && msgType != "" {
		return s.createToolCallMessageFallback(ctx, sessionID, toolCallID, parentToolCallID, status, title, turnID, taskID, msgType, normalized)
	}

	if err != nil {
		s.logger.Warn("tool call message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	s.applyToolCallMessageUpdate(message, status, result, title, normalized)

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

// getToolCallMessageWithRetry fetches a tool call message with retry logic for race conditions.
func (s *Service) getToolCallMessageWithRetry(ctx context.Context, sessionID, toolCallID string, maxRetries int, retryDelay time.Duration) (*models.Message, error) {
	var message *models.Message
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.repo.GetMessageByToolCallID(ctx, sessionID, toolCallID)
		if err == nil {
			return message, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("tool call message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("tool_call_id", toolCallID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}
	return nil, err
}

// createToolCallMessageFallback creates a tool call message when it cannot be found via GetMessageByToolCallID.
func (s *Service) createToolCallMessageFallback(ctx context.Context, sessionID, toolCallID, parentToolCallID, status, title, turnID, taskID, msgType string, normalized *streams.NormalizedPayload) error {
	s.logger.Info("tool call message not found, creating it",
		zap.String("session_id", sessionID),
		zap.String("tool_call_id", toolCallID),
		zap.String("task_id", taskID),
		zap.String("msg_type", msgType))

	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	if normalized != nil {
		metadata["normalized"] = normalized
	}

	msg, createErr := s.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          msgType,
		Metadata:      metadata,
	})
	if createErr != nil {
		s.logger.Error("failed to create tool call message as fallback",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
			zap.Error(createErr))
		return createErr
	}

	s.logger.Info("created tool call message as fallback",
		zap.String("message_id", msg.ID),
		zap.String("tool_call_id", toolCallID),
		zap.String("status", status))
	return nil
}

// applyToolCallMessageUpdate applies status, result, normalized data, and title to a tool call message.
func (s *Service) applyToolCallMessageUpdate(message *models.Message, status, result, title string, normalized *streams.NormalizedPayload) {
	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = status
	if result != "" {
		message.Metadata["result"] = result
	}

	if normalized != nil {
		message.Metadata["normalized"] = normalized
		// Update message type if the normalized kind changed
		// This handles cases like Read on a directory converting to code_search
		newMsgType := models.MessageType(normalized.Kind().ToMessageType())
		if newMsgType != message.Type {
			s.logger.Debug("updating message type based on normalized kind",
				zap.String("message_id", message.ID),
				zap.String("old_type", string(message.Type)),
				zap.String("new_type", string(newMsgType)),
				zap.String("normalized_kind", string(normalized.Kind())))
			message.Type = newMsgType
		}
	}

	// Update title/content if provided and different from current
	if title != "" && title != message.Content {
		message.Content = title
		message.Metadata["title"] = title
	}
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

	// When a permission expires, also mark the related tool call as cancelled
	// so the UI no longer shows a loading spinner on the tool call.
	if status == "expired" {
		if toolCallID, ok := message.Metadata["tool_call_id"].(string); ok && toolCallID != "" {
			if err := s.UpdateToolCallMessage(ctx, sessionID, toolCallID, "error", "", "", nil); err != nil {
				s.logger.Warn("failed to cancel related tool call message",
					zap.String("tool_call_id", toolCallID),
					zap.String("pending_id", pendingID),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("permission message updated",
		zap.String("message_id", message.ID),
		zap.String("pending_id", pendingID),
		zap.String("status", status))

	return nil
}

// UpdateClarificationMessage updates a clarification request message's status and response.
// It includes retry logic to handle race conditions.
// The answers parameter should be a slice of answer objects with question_id, selected_options, and custom_text.
func (s *Service) UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, status string, answers interface{}) error {
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
			s.logger.Debug("clarification message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("clarification message not found for update after retries",
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
	if answers != nil {
		message.Metadata["response"] = answers
	}

	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update clarification message",
			zap.String("message_id", message.ID),
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Info("clarification message updated",
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

	// Safety net: mark any tool calls still in "running" state as "complete"
	if affected, err := s.repo.CompleteRunningToolCallsForTurn(ctx, turnID); err != nil {
		s.logger.Warn("failed to complete running tool calls for turn", zap.String("turn_id", turnID), zap.Error(err))
	} else if affected > 0 {
		s.logger.Info("completed stale running tool calls on turn end",
			zap.String("turn_id", turnID),
			zap.Int64("affected", affected))
	}

	// Fetch the completed turn to get the completed_at timestamp
	turn, err := s.repo.GetTurn(ctx, turnID)
	if err != nil {
		s.logger.Warn("failed to refetch completed turn", zap.String("turn_id", turnID), zap.Error(err))
		// Turn was likely deleted (task deletion race), skip publishing
		return nil
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
	if turn == nil {
		s.logger.Warn("publishTurnEvent: turn is nil, skipping", zap.String("event_type", eventType))
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
		"turn_id":        message.TurnID,
		"author_type":    string(message.AuthorType),
		"author_id":      message.AuthorID,
		"content":        sysprompt.StripSystemContent(message.Content),
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No snapshots yet — valid state for fresh tasks
		}
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
