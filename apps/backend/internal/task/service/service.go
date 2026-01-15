package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// WorktreeCleanup provides worktree cleanup on task deletion.
type WorktreeCleanup interface {
	// OnTaskDeleted is called when a task is deleted to clean up its worktree.
	OnTaskDeleted(ctx context.Context, taskID string) error
}

// TaskExecutionStopper stops active task execution (agent session + instance).
type TaskExecutionStopper interface {
	StopTask(ctx context.Context, taskID, reason string, force bool) error
}

var ErrActiveAgentSessions = errors.New("active agent sessions exist")

// Service provides task business logic
type Service struct {
	repo             repository.Repository
	eventBus         bus.EventBus
	logger           *logger.Logger
	discoveryConfig  RepositoryDiscoveryConfig
	worktreeCleanup  WorktreeCleanup
	executionStopper TaskExecutionStopper
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

// Request types

// CreateTaskRequest contains the data for creating a new task
type CreateTaskRequest struct {
	WorkspaceID  string                 `json:"workspace_id"`
	BoardID      string                 `json:"board_id"`
	ColumnID     string                 `json:"column_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Priority     int                    `json:"priority"`
	State        *v1.TaskState          `json:"state,omitempty"`
	RepositoryID string                 `json:"repository_id,omitempty"`
	BaseBranch   string                 `json:"base_branch,omitempty"`
	AssignedTo   string                 `json:"assigned_to,omitempty"`
	Position     int                    `json:"position"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest contains the data for updating a task
type UpdateTaskRequest struct {
	Title        *string                `json:"title,omitempty"`
	Description  *string                `json:"description,omitempty"`
	Priority     *int                   `json:"priority,omitempty"`
	State        *v1.TaskState          `json:"state,omitempty"`
	RepositoryID *string                `json:"repository_id,omitempty"`
	BaseBranch   *string                `json:"base_branch,omitempty"`
	AssignedTo   *string                `json:"assigned_to,omitempty"`
	Position     *int                   `json:"position,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CreateBoardRequest contains the data for creating a new board
type CreateBoardRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
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

// CreateColumnRequest contains the data for creating a new column
type CreateColumnRequest struct {
	BoardID  string       `json:"board_id"`
	Name     string       `json:"name"`
	Position int          `json:"position"`
	State    v1.TaskState `json:"state"`
	Color    string       `json:"color"`
}

// UpdateColumnRequest contains the data for updating a column
type UpdateColumnRequest struct {
	Name     *string       `json:"name,omitempty"`
	Position *int          `json:"position,omitempty"`
	State    *v1.TaskState `json:"state,omitempty"`
	Color    *string       `json:"color,omitempty"`
}

// CreateRepositoryRequest contains the data for creating a new repository
type CreateRepositoryRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	LocalPath      string `json:"local_path"`
	Provider       string `json:"provider"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	DefaultBranch  string `json:"default_branch"`
	SetupScript    string `json:"setup_script"`
	CleanupScript  string `json:"cleanup_script"`
}

// UpdateRepositoryRequest contains the data for updating a repository
type UpdateRepositoryRequest struct {
	Name           *string `json:"name,omitempty"`
	SourceType     *string `json:"source_type,omitempty"`
	LocalPath      *string `json:"local_path,omitempty"`
	Provider       *string `json:"provider,omitempty"`
	ProviderRepoID *string `json:"provider_repo_id,omitempty"`
	ProviderOwner  *string `json:"provider_owner,omitempty"`
	ProviderName   *string `json:"provider_name,omitempty"`
	DefaultBranch  *string `json:"default_branch,omitempty"`
	SetupScript    *string `json:"setup_script,omitempty"`
	CleanupScript  *string `json:"cleanup_script,omitempty"`
}

// CreateExecutorRequest contains the data for creating an executor
type CreateExecutorRequest struct {
	Name     string                `json:"name"`
	Type     models.ExecutorType   `json:"type"`
	Status   models.ExecutorStatus `json:"status"`
	IsSystem bool                  `json:"is_system"`
	Config   map[string]string     `json:"config,omitempty"`
}

// UpdateExecutorRequest contains the data for updating an executor
type UpdateExecutorRequest struct {
	Name   *string                `json:"name,omitempty"`
	Type   *models.ExecutorType   `json:"type,omitempty"`
	Status *models.ExecutorStatus `json:"status,omitempty"`
	Config map[string]string      `json:"config,omitempty"`
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
	AgentSessionID string
	Limit          int
	Before         string
	After          string
	Sort           string
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
	if strings.TrimSpace(req.RepositoryID) == "" {
		return nil, fmt.Errorf("repository_id is required")
	}
	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	task := &models.Task{
		ID:           uuid.New().String(),
		WorkspaceID:  req.WorkspaceID,
		BoardID:      req.BoardID,
		ColumnID:     req.ColumnID,
		Title:        req.Title,
		Description:  req.Description,
		State:        state,
		Priority:     req.Priority,
		RepositoryID: req.RepositoryID,
		BaseBranch:   req.BaseBranch,
		AssignedTo:   req.AssignedTo,
		Position:     req.Position,
		Metadata:     req.Metadata,
	}

	if err := s.repo.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err))
		return nil, err
	}

	s.publishTaskEvent(ctx, events.TaskCreated, task, nil)
	s.logger.Info("task created", zap.String("task_id", task.ID), zap.String("title", task.Title))

	return task, nil
}

// GetTask retrieves a task by ID
func (s *Service) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return s.repo.GetTask(ctx, id)
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
	if req.RepositoryID != nil {
		task.RepositoryID = *req.RepositoryID
	}
	if req.BaseBranch != nil {
		task.BaseBranch = *req.BaseBranch
	}
	if req.AssignedTo != nil {
		task.AssignedTo = *req.AssignedTo
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

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

// DeleteTask deletes a task and publishes a task.deleted event
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if s.executionStopper != nil {
		if err := s.executionStopper.StopTask(ctx, id, "task deleted", true); err != nil {
			s.logger.Warn("failed to stop task execution on delete",
				zap.String("task_id", id),
				zap.Error(err))
		}
	}

	// Clean up worktree BEFORE deleting task (CASCADE would delete worktree DB records)
	// This must happen first so we can find the worktree paths to remove from disk
	if s.worktreeCleanup != nil {
		if err := s.worktreeCleanup.OnTaskDeleted(ctx, id); err != nil {
			s.logger.Warn("failed to cleanup worktree on task deletion",
				zap.String("task_id", id),
				zap.Error(err))
		}
	}

	if err := s.repo.DeleteTask(ctx, id); err != nil {
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return err
	}

	s.publishTaskEvent(ctx, events.TaskDeleted, task, nil)
	s.logger.Info("task deleted", zap.String("task_id", id))

	return nil
}

// ListTasks returns all tasks for a board
func (s *Service) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	return s.repo.ListTasks(ctx, boardID)
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

	// Find the column that maps to the new state and move the task there
	// This ensures the Kanban board stays in sync with task state
	if task.BoardID != "" {
		column, err := s.repo.GetColumnByState(ctx, task.BoardID, state)
		if err != nil {
			s.logger.Warn("no column found for state, task will stay in current column",
				zap.String("task_id", id),
				zap.String("state", string(state)),
				zap.Error(err))
		} else if column.ID != task.ColumnID {
			// Move task to the new column (keep it at position 0 - top of the column)
			if err := s.repo.AddTaskToBoard(ctx, id, task.BoardID, column.ID, 0); err != nil {
				s.logger.Error("failed to move task to new column",
					zap.String("task_id", id),
					zap.String("column_id", column.ID),
					zap.Error(err))
			} else {
				s.logger.Info("task moved to column matching new state",
					zap.String("task_id", id),
					zap.String("column_id", column.ID),
					zap.String("column_name", column.Name),
					zap.String("state", string(state)))
			}
		}
	}

	// Reload task to get updated state and column
	task, err = s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("task reloaded after column move",
		zap.String("task_id", id),
		zap.String("column_id", task.ColumnID),
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

// MoveTask moves a task to a different column and position
func (s *Service) MoveTask(ctx context.Context, id string, boardID string, columnID string, position int) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get the column to determine the new state
	column, err := s.repo.GetColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}

	oldState := task.State
	task.BoardID = boardID
	task.ColumnID = columnID
	task.Position = position
	task.State = column.State
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
		zap.String("column_id", columnID),
		zap.Int("position", position))

	return task, nil
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
		ID:          uuid.New().String(),
		WorkspaceID: req.WorkspaceID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := s.repo.CreateBoard(ctx, board); err != nil {
		s.logger.Error("failed to create board", zap.Error(err))
		return nil, err
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

// Column operations

// CreateColumn creates a new column
func (s *Service) CreateColumn(ctx context.Context, req *CreateColumnRequest) (*models.Column, error) {
	color := req.Color
	if color == "" {
		color = "bg-neutral-400"
	}
	column := &models.Column{
		ID:       uuid.New().String(),
		BoardID:  req.BoardID,
		Name:     req.Name,
		Position: req.Position,
		State:    req.State,
		Color:    color,
	}

	if err := s.repo.CreateColumn(ctx, column); err != nil {
		s.logger.Error("failed to create column", zap.Error(err))
		return nil, err
	}

	s.publishColumnEvent(ctx, events.ColumnCreated, column)
	s.logger.Info("column created",
		zap.String("column_id", column.ID),
		zap.String("board_id", column.BoardID),
		zap.String("name", column.Name))
	return column, nil
}

// GetColumn retrieves a column by ID
func (s *Service) GetColumn(ctx context.Context, id string) (*models.Column, error) {
	return s.repo.GetColumn(ctx, id)
}

// ListColumns returns all columns for a board
func (s *Service) ListColumns(ctx context.Context, boardID string) ([]*models.Column, error) {
	return s.repo.ListColumns(ctx, boardID)
}

// UpdateColumn updates an existing column
func (s *Service) UpdateColumn(ctx context.Context, id string, req *UpdateColumnRequest) (*models.Column, error) {
	column, err := s.repo.GetColumn(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		column.Name = *req.Name
	}
	if req.Position != nil {
		column.Position = *req.Position
	}
	if req.State != nil {
		column.State = *req.State
	}
	if req.Color != nil {
		column.Color = *req.Color
	}
	column.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateColumn(ctx, column); err != nil {
		s.logger.Error("failed to update column", zap.Error(err))
		return nil, err
	}

	s.publishColumnEvent(ctx, events.ColumnUpdated, column)
	s.logger.Info("column updated",
		zap.String("column_id", column.ID),
		zap.String("board_id", column.BoardID),
		zap.String("name", column.Name))
	return column, nil
}

// DeleteColumn deletes an existing column
func (s *Service) DeleteColumn(ctx context.Context, id string) error {
	column, err := s.repo.GetColumn(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteColumn(ctx, id); err != nil {
		s.logger.Error("failed to delete column", zap.Error(err))
		return err
	}
	s.publishColumnEvent(ctx, events.ColumnDeleted, column)
	s.logger.Info("column deleted", zap.String("column_id", id))
	return nil
}

// Repository operations

func (s *Service) CreateRepository(ctx context.Context, req *CreateRepositoryRequest) (*models.Repository, error) {
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "local"
	}
	repository := &models.Repository{
		ID:             uuid.New().String(),
		WorkspaceID:    req.WorkspaceID,
		Name:           req.Name,
		SourceType:     sourceType,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		DefaultBranch:  req.DefaultBranch,
		SetupScript:    req.SetupScript,
		CleanupScript:  req.CleanupScript,
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
	if req.SetupScript != nil {
		repository.SetupScript = *req.SetupScript
	}
	if req.CleanupScript != nil {
		repository.CleanupScript = *req.CleanupScript
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
	active, err := s.repo.HasActiveAgentSessionsByRepository(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for repository", zap.String("repository_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveAgentSessions
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
	executor := &models.Executor{
		ID:       uuid.New().String(),
		Name:     req.Name,
		Type:     req.Type,
		Status:   req.Status,
		IsSystem: req.IsSystem,
		Config:   req.Config,
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
	if executor.IsSystem {
		return nil, fmt.Errorf("system executors cannot be modified")
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
	active, err := s.repo.HasActiveAgentSessionsByExecutor(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for executor", zap.String("executor_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveAgentSessions
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
	active, err := s.repo.HasActiveAgentSessionsByEnvironment(ctx, id)
	if err != nil {
		s.logger.Error("failed to check active agent sessions for environment", zap.String("environment_id", id), zap.Error(err))
		return err
	}
	if active {
		return ErrActiveAgentSessions
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
		"task_id":     task.ID,
		"board_id":    task.BoardID,
		"column_id":   task.ColumnID,
		"title":       task.Title,
		"description": task.Description,
		"state":       string(task.State),
		"priority":    task.Priority,
		"position":    task.Position,
		"created_at":  task.CreatedAt.Format(time.RFC3339),
		"updated_at":  task.UpdatedAt.Format(time.RFC3339),
	}

	if task.AssignedTo != "" {
		data["assigned_to"] = task.AssignedTo
	}
	if task.RepositoryID != "" {
		data["repository_id"] = task.RepositoryID
	}
	if task.BaseBranch != "" {
		data["base_branch"] = task.BaseBranch
	}
	if task.Metadata != nil {
		data["metadata"] = task.Metadata
	}

	if oldState != nil {
		data["old_state"] = string(*oldState)
		data["new_state"] = string(task.State)
	}

	event := bus.NewEvent(eventType, "task-service", data)

	// Debug logging for state changes
	s.logger.Info("publishing task event",
		zap.String("event_type", eventType),
		zap.String("task_id", task.ID),
		zap.String("board_id", task.BoardID),
		zap.String("column_id", task.ColumnID),
		zap.String("state", string(task.State)))

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

func (s *Service) publishColumnEvent(ctx context.Context, eventType string, column *models.Column) {
	if s.eventBus == nil || column == nil {
		return
	}

	data := map[string]interface{}{
		"id":         column.ID,
		"board_id":   column.BoardID,
		"name":       column.Name,
		"position":   column.Position,
		"state":      string(column.State),
		"color":      column.Color,
		"created_at": column.CreatedAt.Format(time.RFC3339),
		"updated_at": column.UpdatedAt.Format(time.RFC3339),
	}

	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish column event",
			zap.String("event_type", eventType),
			zap.String("column_id", column.ID),
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
	AgentSessionID string                 `json:"agent_session_id"`
	TaskID         string                 `json:"task_id,omitempty"`
	Content        string                 `json:"content"`
	AuthorType     string                 `json:"author_type,omitempty"` // "user" or "agent", defaults to "user"
	AuthorID       string                 `json:"author_id,omitempty"`
	RequestsInput  bool                   `json:"requests_input,omitempty"`
	Type           string                 `json:"type,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// CreateMessage creates a new message on an agent session
func (s *Service) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*models.Message, error) {
	session, err := s.repo.GetAgentSession(ctx, req.AgentSessionID)
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

	message := &models.Message{
		ID:             uuid.New().String(),
		AgentSessionID: req.AgentSessionID,
		TaskID:         taskID,
		AuthorType:     authorType,
		AuthorID:       req.AuthorID,
		Content:        req.Content,
		Type:           messageType,
		Metadata:       req.Metadata,
		RequestsInput:  req.RequestsInput,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.repo.CreateMessage(ctx, message); err != nil {
		s.logger.Error("failed to create message", zap.Error(err))
		return nil, err
	}

	// Publish message.added event
	s.publishMessageEvent(ctx, events.MessageAdded, message)

	s.logger.Info("message created",
		zap.String("message_id", message.ID),
		zap.String("agent_session_id", message.AgentSessionID),
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
	return s.repo.ListMessagesPaginated(ctx, req.AgentSessionID, repository.ListMessagesOptions{
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

// UpdateToolCallMessage updates a tool call message's status
func (s *Service) UpdateToolCallMessage(ctx context.Context, sessionID, toolCallID, status, result string) error {
	message, err := s.repo.GetMessageByToolCallID(ctx, sessionID, toolCallID)
	if err != nil {
		s.logger.Warn("tool call message not found for update",
			zap.String("agent_session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
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

// publishMessageEvent publishes message events to the event bus
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
		"message_id":       message.ID,
		"agent_session_id": message.AgentSessionID,
		"task_id":          message.TaskID,
		"author_type":      string(message.AuthorType),
		"author_id":        message.AuthorID,
		"content":          message.Content,
		"type":             messageType,
		"requests_input":   message.RequestsInput,
		"created_at":       message.CreatedAt.Format(time.RFC3339),
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
		"id":               repository.ID,
		"workspace_id":     repository.WorkspaceID,
		"name":             repository.Name,
		"source_type":      repository.SourceType,
		"local_path":       repository.LocalPath,
		"provider":         repository.Provider,
		"provider_repo_id": repository.ProviderRepoID,
		"provider_owner":   repository.ProviderOwner,
		"provider_name":    repository.ProviderName,
		"default_branch":   repository.DefaultBranch,
		"setup_script":     repository.SetupScript,
		"cleanup_script":   repository.CleanupScript,
		"created_at":       repository.CreatedAt.Format(time.RFC3339),
		"updated_at":       repository.UpdatedAt.Format(time.RFC3339),
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
