package wshandlers

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// CreateTaskRequest is the payload for task.create
type CreateTaskRequest struct {
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Priority      int                    `json:"priority,omitempty"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// TaskResponse is the response for task operations
type TaskResponse struct {
	ID            string                 `json:"id"`
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	State         string                 `json:"state"`
	Priority      int                    `json:"priority"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	Position      int                    `json:"position"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ListTasksRequest is the payload for task.list
type ListTasksRequest struct {
	BoardID string `json:"board_id"`
}

// ListTasksResponse is the response for task.list
type ListTasksResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

// CreateTask handles task.create action
func (h *Handlers) CreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CreateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}
	if req.ColumnID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "column_id is required", nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}

	svcReq := &service.CreateTaskRequest{
		BoardID:       req.BoardID,
		ColumnID:      req.ColumnID,
		Title:         req.Title,
		Description:   req.Description,
		Priority:      req.Priority,
		AgentType:     req.AgentType,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
		Metadata:      req.Metadata,
	}

	task, err := h.service.CreateTask(ctx, svcReq)
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, taskToResponse(task))
}

// ListTasks handles task.list action
func (h *Handlers) ListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ListTasksRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	tasks, err := h.service.ListTasks(ctx, req.BoardID)
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list tasks", nil)
	}

	resp := ListTasksResponse{
		Tasks: make([]TaskResponse, len(tasks)),
		Total: len(tasks),
	}
	for i, t := range tasks {
		resp.Tasks[i] = taskToResponse(t)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// GetTaskRequest is the payload for task.get
type GetTaskRequest struct {
	ID string `json:"id"`
}

// GetTask handles task.get action
func (h *Handlers) GetTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GetTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	task, err := h.service.GetTask(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task not found", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, taskToResponse(task))
}

// UpdateTaskRequest is the payload for task.update
type UpdateTaskRequest struct {
	ID          string                 `json:"id"`
	Title       *string                `json:"title,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTask handles task.update action
func (h *Handlers) UpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UpdateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	svcReq := &service.UpdateTaskRequest{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Metadata:    req.Metadata,
	}

	task, err := h.service.UpdateTask(ctx, req.ID, svcReq)
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, taskToResponse(task))
}

// DeleteTaskRequest is the payload for task.delete
type DeleteTaskRequest struct {
	ID string `json:"id"`
}

// DeleteTask handles task.delete action
func (h *Handlers) DeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req DeleteTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	if err := h.service.DeleteTask(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// MoveTaskRequest is the payload for task.move
type MoveTaskRequest struct {
	ID       string `json:"id"`
	ColumnID string `json:"column_id"`
	Position int    `json:"position"`
}

// MoveTask handles task.move action
func (h *Handlers) MoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req MoveTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.ColumnID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "column_id is required", nil)
	}

	task, err := h.service.MoveTask(ctx, req.ID, req.ColumnID, req.Position)
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to move task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, taskToResponse(task))
}

// UpdateTaskStateRequest is the payload for task.state
type UpdateTaskStateRequest struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// UpdateTaskState handles task.state action
func (h *Handlers) UpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UpdateTaskStateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}

	task, err := h.service.UpdateTaskState(ctx, req.ID, v1.TaskState(req.State))
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, taskToResponse(task))
}

// taskToResponse converts a task model to response
func taskToResponse(t *models.Task) TaskResponse {
	return TaskResponse{
		ID:            t.ID,
		BoardID:       t.BoardID,
		ColumnID:      t.ColumnID,
		Title:         t.Title,
		Description:   t.Description,
		State:         string(t.State),
		Priority:      t.Priority,
		AgentType:     t.AgentType,
		RepositoryURL: t.RepositoryURL,
		Branch:        t.Branch,
		Position:      t.Position,
		CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Metadata:      t.Metadata,
	}
}

