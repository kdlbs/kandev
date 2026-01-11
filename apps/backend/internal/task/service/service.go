package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Service provides task business logic
type Service struct {
	repo     repository.Repository
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewService creates a new task service
func NewService(repo repository.Repository, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		repo:     repo,
		eventBus: eventBus,
		logger:   log,
	}
}

// Request types

// CreateTaskRequest contains the data for creating a new task
type CreateTaskRequest struct {
	WorkspaceID   string                 `json:"workspace_id"`
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	Priority      int                    `json:"priority"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Position      int                    `json:"position"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest contains the data for updating a task
type UpdateTaskRequest struct {
	Title       *string                `json:"title,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	AgentType   *string                `json:"agent_type,omitempty"`
	AssignedTo  *string                `json:"assigned_to,omitempty"`
	Position    *int                   `json:"position,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
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
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
}

// UpdateWorkspaceRequest contains the data for updating a workspace
type UpdateWorkspaceRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateColumnRequest contains the data for creating a new column
type CreateColumnRequest struct {
	BoardID  string       `json:"board_id"`
	Name     string       `json:"name"`
	Position int          `json:"position"`
	State    v1.TaskState `json:"state"`
}

// Task operations

// CreateTask creates a new task and publishes a task.created event
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (*models.Task, error) {
	task := &models.Task{
		ID:            uuid.New().String(),
		WorkspaceID:   req.WorkspaceID,
		BoardID:       req.BoardID,
		ColumnID:      req.ColumnID,
		Title:         req.Title,
		Description:   req.Description,
		State:         v1.TaskStateTODO,
		Priority:      req.Priority,
		AgentType:     req.AgentType,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
		AssignedTo:    req.AssignedTo,
		Position:      req.Position,
		Metadata:      req.Metadata,
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

	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.AgentType != nil {
		task.AgentType = *req.AgentType
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

// UpdateTaskState updates the state of a task and publishes a task.state_changed event
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
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     req.OwnerID,
	}

	if err := s.repo.CreateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to create workspace", zap.Error(err))
		return nil, err
	}

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
	workspace.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateWorkspace(ctx, workspace); err != nil {
		s.logger.Error("failed to update workspace", zap.String("workspace_id", id), zap.Error(err))
		return nil, err
	}

	s.logger.Info("workspace updated", zap.String("workspace_id", workspace.ID))
	return workspace, nil
}

// DeleteWorkspace deletes a workspace
func (s *Service) DeleteWorkspace(ctx context.Context, id string) error {
	if err := s.repo.DeleteWorkspace(ctx, id); err != nil {
		s.logger.Error("failed to delete workspace", zap.String("workspace_id", id), zap.Error(err))
		return err
	}
	s.logger.Info("workspace deleted", zap.String("workspace_id", id))
	return nil
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

	s.logger.Info("board updated", zap.String("board_id", board.ID))
	return board, nil
}

// DeleteBoard deletes a board
func (s *Service) DeleteBoard(ctx context.Context, id string) error {
	if err := s.repo.DeleteBoard(ctx, id); err != nil {
		s.logger.Error("failed to delete board", zap.String("board_id", id), zap.Error(err))
		return err
	}

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
	column := &models.Column{
		ID:       uuid.New().String(),
		BoardID:  req.BoardID,
		Name:     req.Name,
		Position: req.Position,
		State:    req.State,
	}

	if err := s.repo.CreateColumn(ctx, column); err != nil {
		s.logger.Error("failed to create column", zap.Error(err))
		return nil, err
	}

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

	if task.AgentType != "" {
		data["agent_type"] = task.AgentType
	}
	if task.AssignedTo != "" {
		data["assigned_to"] = task.AssignedTo
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
