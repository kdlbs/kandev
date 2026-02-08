package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type TaskHandlers struct {
	controller   *controller.TaskController
	orchestrator OrchestratorStarter
	repo         repository.Repository
	planService  *service.PlanService
	logger       *logger.Logger
}

type OrchestratorStarter interface {
	// StartTask starts agent execution for a task.
	// If workflowStepID is provided, prompt prefix/suffix and plan mode from the step are applied.
	StartTask(ctx context.Context, taskID string, agentProfileID string, executorID string, priority int, prompt string, workflowStepID string) (*executor.TaskExecution, error)
	// PrepareTaskSession creates a session entry without launching the agent.
	// Returns the session ID immediately so it can be returned in the HTTP response.
	PrepareTaskSession(ctx context.Context, taskID string, agentProfileID string, executorID string, workflowStepID string) (string, error)
	// StartTaskWithSession starts agent execution for a task using a pre-created session.
	StartTaskWithSession(ctx context.Context, taskID string, sessionID string, agentProfileID string, executorID string, priority int, prompt string, workflowStepID string) (*executor.TaskExecution, error)
}

func NewTaskHandlers(ctrl *controller.TaskController, orchestrator OrchestratorStarter, repo repository.Repository, planService *service.PlanService, log *logger.Logger) *TaskHandlers {
	return &TaskHandlers{
		controller:   ctrl,
		orchestrator: orchestrator,
		repo:         repo,
		planService:  planService,
		logger:       log.WithFields(zap.String("component", "task-task-handlers")),
	}
}

func RegisterTaskRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.TaskController, orchestrator OrchestratorStarter, repo repository.Repository, planService *service.PlanService, log *logger.Logger) {
	handlers := NewTaskHandlers(ctrl, orchestrator, repo, planService, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *TaskHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/boards/:id/tasks", h.httpListTasks)
	api.GET("/workspaces/:id/tasks", h.httpListTasksByWorkspace)
	api.GET("/tasks/:id", h.httpGetTask)
	api.GET("/task-sessions/:id", h.httpGetTaskSession)
	api.GET("/tasks/:id/sessions", h.httpListTaskSessions)
	api.GET("/task-sessions/:id/turns", h.httpListSessionTurns)
	api.POST("/tasks", h.httpCreateTask)
	api.PATCH("/tasks/:id", h.httpUpdateTask)
	api.POST("/tasks/:id/move", h.httpMoveTask)
	api.DELETE("/tasks/:id", h.httpDeleteTask)

	// Session workflow review endpoints
	api.POST("/sessions/:id/approve", h.httpApproveSession)
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
	// Session file review handlers
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewGet, h.wsGetSessionFileReviews)
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewUpdate, h.wsUpdateSessionFileReview)
	dispatcher.RegisterFunc(ws.ActionSessionFileReviewReset, h.wsResetSessionFileReviews)
	// Task plan handlers
	dispatcher.RegisterFunc(ws.ActionTaskPlanCreate, h.wsCreateTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanGet, h.wsGetTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanUpdate, h.wsUpdateTaskPlan)
	dispatcher.RegisterFunc(ws.ActionTaskPlanDelete, h.wsDeleteTaskPlan)
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

func (h *TaskHandlers) httpListTasksByWorkspace(c *gin.Context) {
	page := 1
	pageSize := 50

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	query := c.Query("query")

	resp, err := h.controller.ListTasksByWorkspace(c.Request.Context(), dto.ListTasksByWorkspaceRequest{
		WorkspaceID: c.Param("id"),
		Query:       query,
		Page:        page,
		PageSize:    pageSize,
	})
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

func (h *TaskHandlers) httpListSessionTurns(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	turns, err := h.repo.ListTurnsBySession(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to list turns", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list turns"})
		return
	}

	// Convert to DTO
	turnDTOs := make([]dto.TurnDTO, 0, len(turns))
	for _, turn := range turns {
		turnDTOs = append(turnDTOs, dto.FromTurn(turn))
	}

	c.JSON(http.StatusOK, dto.ListTurnsResponse{Turns: turnDTOs, Total: len(turnDTOs)})
}

func (h *TaskHandlers) httpApproveSession(c *gin.Context) {
	resp, err := h.controller.ApproveSession(
		c.Request.Context(),
		dto.ApproveSessionRequest{SessionID: c.Param("id")},
	)
	if err != nil {
		h.logger.Error("failed to approve session", zap.String("session_id", c.Param("id")), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	WorkflowStepID string                    `json:"workflow_step_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	State          *v1.TaskState             `json:"state,omitempty"`
	Repositories   []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position       int                       `json:"position,omitempty"`
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent     bool                      `json:"start_agent,omitempty"`
	AgentProfileID string                    `json:"agent_profile_id,omitempty"`
	ExecutorID     string                    `json:"executor_id,omitempty"`
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
		WorkspaceID:    body.WorkspaceID,
		BoardID:        body.BoardID,
		WorkflowStepID: body.WorkflowStepID,
		Title:          body.Title,
		Description:    body.Description,
		Priority:       body.Priority,
		State:          body.State,
		Repositories:   repos,
		Position:       body.Position,
		Metadata:       body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not created")
		return
	}

	response := createTaskResponse{TaskDTO: resp}
	if body.StartAgent && body.AgentProfileID != "" && h.orchestrator != nil {
		// Create session entry synchronously so we can return the session ID immediately
		sessionID, err := h.orchestrator.PrepareTaskSession(reqCtx, resp.ID, body.AgentProfileID, body.ExecutorID, body.WorkflowStepID)
		if err != nil {
			h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", resp.ID))
			// Continue without session - task was created successfully
		} else {
			response.TaskSessionID = sessionID

			// Launch agent asynchronously so the HTTP request can return immediately.
			// The frontend will receive WebSocket updates when the agent actually starts.
			executorID := body.ExecutorID         // Capture for goroutine
			workflowStepID := body.WorkflowStepID // Capture for goroutine
			go func() {
				// Use a longer timeout to accommodate setup scripts (which can take minutes for npm install, etc.)
				startCtx, cancel := context.WithTimeout(context.Background(), constants.AgentLaunchTimeout)
				defer cancel()
				// Use task description as the initial prompt with workflow step config (prompt prefix/suffix, plan mode)
				execution, err := h.orchestrator.StartTaskWithSession(startCtx, resp.ID, sessionID, body.AgentProfileID, executorID, body.Priority, resp.Description, workflowStepID)
				if err != nil {
					h.logger.Error("failed to start agent for task (async)", zap.Error(err), zap.String("task_id", resp.ID), zap.String("session_id", sessionID))
					return
				}
				h.logger.Info("agent started for task (async)",
					zap.String("task_id", resp.ID),
					zap.String("session_id", execution.SessionID),
					zap.String("executor_id", executorID),
					zap.String("workflow_step_id", workflowStepID),
					zap.String("execution_id", execution.AgentExecutionID))
			}()
		}
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
	BoardID        string `json:"board_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Position       int    `json:"position"`
}

func (h *TaskHandlers) httpMoveTask(c *gin.Context) {
	var body httpMoveTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.BoardID == "" || body.WorkflowStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board_id and workflow_step_id are required"})
		return
	}
	resp, err := h.controller.MoveTask(c.Request.Context(), dto.MoveTaskRequest{
		ID:             c.Param("id"),
		BoardID:        body.BoardID,
		WorkflowStepID: body.WorkflowStepID,
		Position:       body.Position,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not moved")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpDeleteTask(c *gin.Context) {
	deleteCtx, cancel := context.WithTimeout(context.Background(), constants.TaskDeleteTimeout)
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
	WorkflowStepID string                    `json:"workflow_step_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	State          *v1.TaskState             `json:"state,omitempty"`
	Repositories   []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position       int                       `json:"position,omitempty"`
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent     bool                      `json:"start_agent,omitempty"`
	AgentProfileID string                    `json:"agent_profile_id,omitempty"`
	ExecutorID     string                    `json:"executor_id,omitempty"`
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
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
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
		WorkspaceID:    req.WorkspaceID,
		BoardID:        req.BoardID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Priority:       req.Priority,
		State:          req.State,
		Repositories:   repos,
		Position:       req.Position,
		Metadata:       req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	response := createTaskResponse{TaskDTO: resp}
	if req.StartAgent && req.AgentProfileID != "" && h.orchestrator != nil {
		// Use task description as the initial prompt with workflow step config (prompt prefix/suffix, plan mode)
		execution, err := h.orchestrator.StartTask(ctx, resp.ID, req.AgentProfileID, req.ExecutorID, req.Priority, resp.Description, req.WorkflowStepID)
		if err != nil {
			h.logger.Error("failed to start agent for task", zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start agent for task", nil)
		}
		h.logger.Info("wsCreateTask started agent",
			zap.String("task_id", resp.ID),
			zap.String("executor_id", req.ExecutorID),
			zap.String("workflow_step_id", req.WorkflowStepID),
			zap.String("session_id", execution.SessionID))
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
	ID             string `json:"id"`
	BoardID        string `json:"board_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Position       int    `json:"position"`
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
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}

	resp, err := h.controller.MoveTask(ctx, dto.MoveTaskRequest{
		ID:             req.ID,
		BoardID:        req.BoardID,
		WorkflowStepID: req.WorkflowStepID,
		Position:       req.Position,
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


// Session File Review Handlers

type wsGetSessionFileReviewsRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsGetSessionFileReviews(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetSessionFileReviewsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	reviews, err := h.repo.GetSessionFileReviews(ctx, req.SessionID)
	if err != nil {
		h.logger.Error("failed to get session file reviews", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get session file reviews", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": req.SessionID,
		"reviews":    reviews,
	})
}

type wsUpdateSessionFileReviewRequest struct {
	SessionID string `json:"session_id"`
	FilePath  string `json:"file_path"`
	Reviewed  bool   `json:"reviewed"`
	DiffHash  string `json:"diff_hash"`
}

func (h *TaskHandlers) wsUpdateSessionFileReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateSessionFileReviewRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.FilePath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "file_path is required", nil)
	}

	review := &models.SessionFileReview{
		SessionID: req.SessionID,
		FilePath:  req.FilePath,
		Reviewed:  req.Reviewed,
		DiffHash:  req.DiffHash,
	}
	if req.Reviewed {
		now := time.Now().UTC()
		review.ReviewedAt = &now
	}

	if err := h.repo.UpsertSessionFileReview(ctx, review); err != nil {
		h.logger.Error("failed to update session file review", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update session file review", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"review":  review,
	})
}

type wsResetSessionFileReviewsRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsResetSessionFileReviews(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsResetSessionFileReviewsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	if err := h.repo.DeleteSessionFileReviews(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to reset session file reviews", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reset session file reviews", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// Task Plan Handlers

// wsCreateTaskPlan creates a new task plan
func (h *TaskHandlers) wsCreateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	// Default to "user" for frontend requests
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}

	plan, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsGetTaskPlan retrieves a task plan
func (h *TaskHandlers) wsGetTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	plan, err := h.planService.GetPlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task plan", nil)
	}
	if plan == nil {
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsUpdateTaskPlan updates an existing task plan
func (h *TaskHandlers) wsUpdateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	// Default to "user" for frontend requests
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}

	plan, err := h.planService.UpdatePlan(ctx, service.UpdatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsDeleteTaskPlan deletes a task plan
func (h *TaskHandlers) wsDeleteTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	err := h.planService.DeletePlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}