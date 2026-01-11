package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskHandlers struct {
	controller *controller.TaskController
	logger     *logger.Logger
}

func NewTaskHandlers(ctrl *controller.TaskController, log *logger.Logger) *TaskHandlers {
	return &TaskHandlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "task-task-handlers")),
	}
}

func RegisterTaskRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.TaskController, log *logger.Logger) {
	handlers := NewTaskHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *TaskHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/boards/:id/tasks", h.httpListTasks)
	api.GET("/tasks/:id", h.httpGetTask)
	api.POST("/tasks", h.httpCreateTask)
	api.PATCH("/tasks/:id", h.httpUpdateTask)
	api.DELETE("/tasks/:id", h.httpDeleteTask)
}

func (h *TaskHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionTaskList, h.wsListTasks)
	dispatcher.RegisterFunc(ws.ActionTaskCreate, h.wsCreateTask)
	dispatcher.RegisterFunc(ws.ActionTaskGet, h.wsGetTask)
	dispatcher.RegisterFunc(ws.ActionTaskUpdate, h.wsUpdateTask)
	dispatcher.RegisterFunc(ws.ActionTaskDelete, h.wsDeleteTask)
	dispatcher.RegisterFunc(ws.ActionTaskMove, h.wsMoveTask)
	dispatcher.RegisterFunc(ws.ActionTaskState, h.wsUpdateTaskState)
}

// HTTP handlers

func (h *TaskHandlers) httpListTasks(c *gin.Context) {
	resp, err := h.controller.ListTasks(c.Request.Context(), dto.ListTasksRequest{BoardID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetTask(c *gin.Context) {
	resp, err := h.controller.GetTask(c.Request.Context(), dto.GetTaskRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateTaskRequest struct {
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Priority      int                    `json:"priority,omitempty"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Position      int                    `json:"position,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

func (h *TaskHandlers) httpCreateTask(c *gin.Context) {
	var body httpCreateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreateTask(c.Request.Context(), dto.CreateTaskRequest{
		BoardID:       body.BoardID,
		ColumnID:      body.ColumnID,
		Title:         body.Title,
		Description:   body.Description,
		Priority:      body.Priority,
		AgentType:     body.AgentType,
		RepositoryURL: body.RepositoryURL,
		Branch:        body.Branch,
		AssignedTo:    body.AssignedTo,
		Position:      body.Position,
		Metadata:      body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not created")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateTaskRequest struct {
	Title       *string                `json:"title,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	AgentType   *string                `json:"agent_type,omitempty"`
	AssignedTo  *string                `json:"assigned_to,omitempty"`
	Position    *int                   `json:"position,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (h *TaskHandlers) httpUpdateTask(c *gin.Context) {
	var body httpUpdateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateTask(c.Request.Context(), dto.UpdateTaskRequest{
		ID:          c.Param("id"),
		Title:       body.Title,
		Description: body.Description,
		Priority:    body.Priority,
		AgentType:   body.AgentType,
		AssignedTo:  body.AssignedTo,
		Position:    body.Position,
		Metadata:    body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not updated")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpDeleteTask(c *gin.Context) {
	resp, err := h.controller.DeleteTask(c.Request.Context(), dto.DeleteTaskRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not deleted")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

type wsListTasksRequest struct {
	BoardID string `json:"board_id"`
}

func (h *TaskHandlers) wsListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListTasksRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	resp, err := h.controller.ListTasks(ctx, dto.ListTasksRequest{BoardID: req.BoardID})
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list tasks", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateTaskRequest struct {
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Priority      int                    `json:"priority,omitempty"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Position      int                    `json:"position,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

func (h *TaskHandlers) wsCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateTaskRequest
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

	resp, err := h.controller.CreateTask(ctx, dto.CreateTaskRequest{
		BoardID:       req.BoardID,
		ColumnID:      req.ColumnID,
		Title:         req.Title,
		Description:   req.Description,
		Priority:      req.Priority,
		AgentType:     req.AgentType,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
		AssignedTo:    req.AssignedTo,
		Position:      req.Position,
		Metadata:      req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetTaskRequest struct {
	ID string `json:"id"`
}

func (h *TaskHandlers) wsGetTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.GetTask(ctx, dto.GetTaskRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateTaskRequest struct {
	ID          string                 `json:"id"`
	Title       *string                `json:"title,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	AgentType   *string                `json:"agent_type,omitempty"`
	AssignedTo  *string                `json:"assigned_to,omitempty"`
	Position    *int                   `json:"position,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (h *TaskHandlers) wsUpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.UpdateTask(ctx, dto.UpdateTaskRequest{
		ID:          req.ID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AgentType:   req.AgentType,
		AssignedTo:  req.AssignedTo,
		Position:    req.Position,
		Metadata:    req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteTaskRequest struct {
	ID string `json:"id"`
}

func (h *TaskHandlers) wsDeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.DeleteTask(ctx, dto.DeleteTaskRequest{ID: req.ID})
	if err != nil {
		h.logger.Error("failed to delete task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsMoveTaskRequest struct {
	ID       string `json:"id"`
	ColumnID string `json:"column_id"`
	Position int    `json:"position"`
}

func (h *TaskHandlers) wsMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsMoveTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.ColumnID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "column_id is required", nil)
	}

	resp, err := h.controller.MoveTask(ctx, dto.MoveTaskRequest{
		ID:       req.ID,
		ColumnID: req.ColumnID,
		Position: req.Position,
	})
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to move task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateTaskStateRequest struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (h *TaskHandlers) wsUpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateTaskStateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}

	resp, err := h.controller.UpdateTaskState(ctx, dto.UpdateTaskStateRequest{
		ID:    req.ID,
		State: v1.TaskState(req.State),
	})
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
