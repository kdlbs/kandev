package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskHandlers struct {
	controller   *controller.TaskController
	orchestrator OrchestratorStarter
	logger       *logger.Logger
}

type OrchestratorStarter interface {
	StartTask(ctx context.Context, taskID string, agentProfileID string, priority int, prompt string) (*executor.TaskExecution, error)
}

func NewTaskHandlers(ctrl *controller.TaskController, orchestrator OrchestratorStarter, log *logger.Logger) *TaskHandlers {
	return &TaskHandlers{
		controller:   ctrl,
		orchestrator: orchestrator,
		logger:       log.WithFields(zap.String("component", "task-task-handlers")),
	}
}

func RegisterTaskRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.TaskController, orchestrator OrchestratorStarter, log *logger.Logger) {
	handlers := NewTaskHandlers(ctrl, orchestrator, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *TaskHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/boards/:id/tasks", h.httpListTasks)
	api.GET("/tasks/:id", h.httpGetTask)
	api.GET("/task-sessions/:id", h.httpGetTaskSession)
	api.GET("/tasks/:id/sessions", h.httpListTaskSessions)
	api.POST("/tasks", h.httpCreateTask)
	api.PATCH("/tasks/:id", h.httpUpdateTask)
	api.POST("/tasks/:id/move", h.httpMoveTask)
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
	dispatcher.RegisterFunc(ws.ActionTaskSessionList, h.wsListTaskSessions)
	// Git snapshot and commit handlers
	dispatcher.RegisterFunc(ws.ActionSessionGitSnapshots, h.wsGetGitSnapshots)
	dispatcher.RegisterFunc(ws.ActionSessionGitCommits, h.wsGetSessionCommits)
	dispatcher.RegisterFunc(ws.ActionSessionCumulativeDiff, h.wsGetCumulativeDiff)
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

func (h *TaskHandlers) httpListTaskSessions(c *gin.Context) {
	resp, err := h.controller.ListTaskSessions(c.Request.Context(), dto.ListTaskSessionsRequest{TaskID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "task sessions not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetTaskSession(c *gin.Context) {
	resp, err := h.controller.GetTaskSession(
		c.Request.Context(),
		dto.GetTaskSessionRequest{TaskSessionID: c.Param("id")},
	)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpTaskRepositoryInput struct {
	RepositoryID  string `json:"repository_id"`
	BaseBranch    string `json:"base_branch"`
	LocalPath     string `json:"local_path"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
}

type httpCreateTaskRequest struct {
	WorkspaceID    string                    `json:"workspace_id"`
	BoardID        string                    `json:"board_id"`
	ColumnID       string                    `json:"column_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	State          *v1.TaskState             `json:"state,omitempty"`
	Repositories   []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position       int                       `json:"position,omitempty"`
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent     bool                      `json:"start_agent,omitempty"`
	AgentProfileID string                    `json:"agent_profile_id,omitempty"`
}

type createTaskResponse struct {
	dto.TaskDTO
	TaskSessionID    string `json:"session_id,omitempty"`
	AgentExecutionID string `json:"agent_execution_id,omitempty"`
}

func (h *TaskHandlers) httpCreateTask(c *gin.Context) {
	var body httpCreateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	if body.StartAgent && body.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required to start agent"})
		return
	}

	// Convert repositories
	var repos []dto.TaskRepositoryInput
	for _, r := range body.Repositories {
		if r.RepositoryID == "" && r.LocalPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repository_id or local_path is required"})
			return
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	reqCtx := c.Request.Context()
	resp, err := h.controller.CreateTask(reqCtx, dto.CreateTaskRequest{
		WorkspaceID:  body.WorkspaceID,
		BoardID:      body.BoardID,
		ColumnID:     body.ColumnID,
		Title:        body.Title,
		Description:  body.Description,
		Priority:     body.Priority,
		State:        body.State,
		Repositories: repos,
		Position:     body.Position,
		Metadata:     body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not created")
		return
	}

	response := createTaskResponse{TaskDTO: resp}
	if body.StartAgent && body.AgentProfileID != "" && h.orchestrator != nil {
		startCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Use task description as the initial prompt
		execution, err := h.orchestrator.StartTask(startCtx, resp.ID, body.AgentProfileID, body.Priority, resp.Description)
		if err != nil {
			h.logger.Error("failed to start agent for task", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start agent for task"})
			return
		}
		response.TaskSessionID = execution.SessionID
		response.AgentExecutionID = execution.AgentExecutionID
	}

	c.JSON(http.StatusOK, response)
}

type httpUpdateTaskRequest struct {
	Title        *string                   `json:"title,omitempty"`
	Description  *string                   `json:"description,omitempty"`
	Priority     *int                      `json:"priority,omitempty"`
	State        *v1.TaskState             `json:"state,omitempty"`
	Repositories []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position     *int                      `json:"position,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
}

func (h *TaskHandlers) httpUpdateTask(c *gin.Context) {
	var body httpUpdateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Convert repositories if provided
	var repos []dto.TaskRepositoryInput
	if body.Repositories != nil {
		for _, r := range body.Repositories {
			repos = append(repos, dto.TaskRepositoryInput{
				RepositoryID:  r.RepositoryID,
				BaseBranch:    r.BaseBranch,
				LocalPath:     r.LocalPath,
				Name:          r.Name,
				DefaultBranch: r.DefaultBranch,
			})
		}
	}

	resp, err := h.controller.UpdateTask(c.Request.Context(), dto.UpdateTaskRequest{
		ID:           c.Param("id"),
		Title:        body.Title,
		Description:  body.Description,
		Priority:     body.Priority,
		State:        body.State,
		Repositories: repos,
		Position:     body.Position,
		Metadata:     body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not updated")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpMoveTaskRequest struct {
	BoardID  string `json:"board_id"`
	ColumnID string `json:"column_id"`
	Position int    `json:"position"`
}

func (h *TaskHandlers) httpMoveTask(c *gin.Context) {
	var body httpMoveTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.BoardID == "" || body.ColumnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board_id and column_id are required"})
		return
	}
	resp, err := h.controller.MoveTask(c.Request.Context(), dto.MoveTaskRequest{
		ID:       c.Param("id"),
		BoardID:  body.BoardID,
		ColumnID: body.ColumnID,
		Position: body.Position,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not moved")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpDeleteTask(c *gin.Context) {
	deleteCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := h.controller.DeleteTask(deleteCtx, dto.DeleteTaskRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not deleted")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

type wsListTaskSessionsRequest struct {
	TaskID string `json:"task_id"`
}

func (h *TaskHandlers) wsListTaskSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListTaskSessionsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.ListTaskSessions(ctx, dto.ListTaskSessionsRequest{TaskID: req.TaskID})
	if err != nil {
		h.logger.Error("failed to list task sessions", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task sessions", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

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
	WorkspaceID    string                    `json:"workspace_id"`
	BoardID        string                    `json:"board_id"`
	ColumnID       string                    `json:"column_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	State          *v1.TaskState             `json:"state,omitempty"`
	Repositories   []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position       int                       `json:"position,omitempty"`
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent     bool                      `json:"start_agent,omitempty"`
	AgentProfileID string                    `json:"agent_profile_id,omitempty"`
}

func (h *TaskHandlers) wsCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
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
	if req.StartAgent && req.AgentProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_profile_id is required to start agent", nil)
	}

	// Convert repositories
	var repos []dto.TaskRepositoryInput
	for _, r := range req.Repositories {
		if r.RepositoryID == "" && r.LocalPath == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "repository_id or local_path is required", nil)
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	resp, err := h.controller.CreateTask(ctx, dto.CreateTaskRequest{
		WorkspaceID:  req.WorkspaceID,
		BoardID:      req.BoardID,
		ColumnID:     req.ColumnID,
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		State:        req.State,
		Repositories: repos,
		Position:     req.Position,
		Metadata:     req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	response := createTaskResponse{TaskDTO: resp}
	if req.StartAgent && req.AgentProfileID != "" && h.orchestrator != nil {
		// Use task description as the initial prompt
		execution, err := h.orchestrator.StartTask(ctx, resp.ID, req.AgentProfileID, req.Priority, resp.Description)
		if err != nil {
			h.logger.Error("failed to start agent for task", zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start agent for task", nil)
		}
		response.TaskSessionID = execution.SessionID
		response.AgentExecutionID = execution.AgentExecutionID
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
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
	ID           string                    `json:"id"`
	Title        *string                   `json:"title,omitempty"`
	Description  *string                   `json:"description,omitempty"`
	Priority     *int                      `json:"priority,omitempty"`
	State        *v1.TaskState             `json:"state,omitempty"`
	Repositories []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position     *int                      `json:"position,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
}

func (h *TaskHandlers) wsUpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	// Convert repositories if provided
	var repos []dto.TaskRepositoryInput
	if req.Repositories != nil {
		for _, r := range req.Repositories {
			repos = append(repos, dto.TaskRepositoryInput{
				RepositoryID:  r.RepositoryID,
				BaseBranch:    r.BaseBranch,
				LocalPath:     r.LocalPath,
				Name:          r.Name,
				DefaultBranch: r.DefaultBranch,
			})
		}
	}

	resp, err := h.controller.UpdateTask(ctx, dto.UpdateTaskRequest{
		ID:           req.ID,
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		State:        req.State,
		Repositories: repos,
		Position:     req.Position,
		Metadata:     req.Metadata,
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
	BoardID  string `json:"board_id"`
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
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}
	if req.ColumnID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "column_id is required", nil)
	}

	resp, err := h.controller.MoveTask(ctx, dto.MoveTaskRequest{
		ID:       req.ID,
		BoardID:  req.BoardID,
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

// Git Snapshot and Commit Handlers

type wsGetGitSnapshotsRequest struct {
	SessionID string `json:"session_id"`
	Limit     int    `json:"limit,omitempty"`
}

func (h *TaskHandlers) wsGetGitSnapshots(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetGitSnapshotsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	snapshots, err := h.controller.GetGitSnapshots(ctx, req.SessionID, req.Limit)
	if err != nil {
		h.logger.Error("failed to get git snapshots", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get git snapshots", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": req.SessionID,
		"snapshots":  snapshots,
	})
}

type wsGetSessionCommitsRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsGetSessionCommits(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetSessionCommitsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	commits, err := h.controller.GetSessionCommits(ctx, req.SessionID)
	if err != nil {
		h.logger.Error("failed to get session commits", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get session commits", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": req.SessionID,
		"commits":    commits,
	})
}

type wsGetCumulativeDiffRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsGetCumulativeDiff(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetCumulativeDiffRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	diff, err := h.controller.GetCumulativeDiff(ctx, req.SessionID)
	if err != nil {
		h.logger.Error("failed to get cumulative diff", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get cumulative diff", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"cumulative_diff": diff,
	})
}
