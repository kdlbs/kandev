package service

import (
	"context"
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

// Service provides task business logic
type Service struct {
	repo            repository.Repository
	eventBus        bus.EventBus
	logger          *logger.Logger
	discoveryConfig RepositoryDiscoveryConfig
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

func (s *Service) publishWorkspaceEvent(ctx context.Context, eventType string, workspace *models.Workspace) {
	if s.eventBus == nil || workspace == nil {
		return
	}

	data := map[string]interface{}{
		"id":          workspace.ID,
		"name":        workspace.Name,
		"description": workspace.Description,
		"owner_id":    workspace.OwnerID,
		"created_at":  workspace.CreatedAt.Format(time.RFC3339),
		"updated_at":  workspace.UpdatedAt.Format(time.RFC3339),
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

// Comment operations

// CreateCommentRequest contains the data for creating a new comment
type CreateCommentRequest struct {
	TaskID        string `json:"task_id"`
	Content       string `json:"content"`
	AuthorType    string `json:"author_type,omitempty"` // "user" or "agent", defaults to "user"
	AuthorID      string `json:"author_id,omitempty"`
	RequestsInput bool   `json:"requests_input,omitempty"`
	ACPSessionID  string `json:"acp_session_id,omitempty"`
}

// CreateComment creates a new comment on a task
func (s *Service) CreateComment(ctx context.Context, req *CreateCommentRequest) (*models.Comment, error) {
	// Verify task exists
	_, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}

	authorType := models.CommentAuthorUser
	if req.AuthorType == "agent" {
		authorType = models.CommentAuthorAgent
	}

	comment := &models.Comment{
		ID:            uuid.New().String(),
		TaskID:        req.TaskID,
		AuthorType:    authorType,
		AuthorID:      req.AuthorID,
		Content:       req.Content,
		RequestsInput: req.RequestsInput,
		ACPSessionID:  req.ACPSessionID,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.repo.CreateComment(ctx, comment); err != nil {
		s.logger.Error("failed to create comment", zap.Error(err))
		return nil, err
	}

	// Publish comment.added event
	s.publishCommentEvent(ctx, events.CommentAdded, comment)

	s.logger.Info("comment created",
		zap.String("comment_id", comment.ID),
		zap.String("task_id", comment.TaskID),
		zap.String("author_type", string(comment.AuthorType)))

	return comment, nil
}

// GetComment retrieves a comment by ID
func (s *Service) GetComment(ctx context.Context, id string) (*models.Comment, error) {
	return s.repo.GetComment(ctx, id)
}

// ListComments returns all comments for a task
func (s *Service) ListComments(ctx context.Context, taskID string) ([]*models.Comment, error) {
	return s.repo.ListComments(ctx, taskID)
}

// DeleteComment deletes a comment
func (s *Service) DeleteComment(ctx context.Context, id string) error {
	if err := s.repo.DeleteComment(ctx, id); err != nil {
		s.logger.Error("failed to delete comment", zap.String("comment_id", id), zap.Error(err))
		return err
	}

	s.logger.Info("comment deleted", zap.String("comment_id", id))
	return nil
}

// publishCommentEvent publishes comment events to the event bus
func (s *Service) publishCommentEvent(ctx context.Context, eventType string, comment *models.Comment) {
	if s.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"comment_id":     comment.ID,
		"task_id":        comment.TaskID,
		"author_type":    string(comment.AuthorType),
		"author_id":      comment.AuthorID,
		"content":        comment.Content,
		"requests_input": comment.RequestsInput,
		"created_at":     comment.CreatedAt.Format(time.RFC3339),
	}

	if comment.ACPSessionID != "" {
		data["acp_session_id"] = comment.ACPSessionID
	}

	event := bus.NewEvent(eventType, "task-service", data)

	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish comment event",
			zap.String("event_type", eventType),
			zap.String("comment_id", comment.ID),
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
