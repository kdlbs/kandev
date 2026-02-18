package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/task/dto"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

func (h *TaskHandlers) httpListTasks(c *gin.Context) {
	resp, err := h.controller.ListTasks(c.Request.Context(), dto.ListTasksRequest{WorkflowID: c.Param("id")})
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
	includeArchived := c.Query("include_archived") == queryValueTrue

	resp, err := h.controller.ListTasksByWorkspace(c.Request.Context(), dto.ListTasksByWorkspaceRequest{
		WorkspaceID:     c.Param("id"),
		Query:           query,
		Page:            page,
		PageSize:        pageSize,
		IncludeArchived: includeArchived,
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

func (h *TaskHandlers) httpGetWorkflowTaskCount(c *gin.Context) {
	resp, err := h.controller.CountTasksByWorkflow(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to count tasks by workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetStepTaskCount(c *gin.Context) {
	resp, err := h.controller.CountTasksByWorkflowStep(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to count tasks by step", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpBulkMoveTasksRequest struct {
	SourceWorkflowID string `json:"source_workflow_id"`
	SourceStepID     string `json:"source_step_id,omitempty"`
	TargetWorkflowID string `json:"target_workflow_id"`
	TargetStepID     string `json:"target_step_id"`
}

func (h *TaskHandlers) httpBulkMoveTasks(c *gin.Context) {
	var body httpBulkMoveTasksRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.SourceWorkflowID == "" || body.TargetWorkflowID == "" || body.TargetStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_workflow_id, target_workflow_id, and target_step_id are required"})
		return
	}
	resp, err := h.controller.BulkMoveTasks(c.Request.Context(), dto.BulkMoveTasksRequest{
		SourceWorkflowID: body.SourceWorkflowID,
		SourceStepID:     body.SourceStepID,
		TargetWorkflowID: body.TargetWorkflowID,
		TargetStepID:     body.TargetStepID,
	})
	if err != nil {
		h.logger.Error("failed to bulk move tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bulk move tasks"})
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
	WorkflowID     string                    `json:"workflow_id"`
	WorkflowStepID string                    `json:"workflow_step_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Priority       int                       `json:"priority,omitempty"`
	State          *v1.TaskState             `json:"state,omitempty"`
	Repositories   []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position       int                       `json:"position,omitempty"`
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent     bool                      `json:"start_agent,omitempty"`
	PrepareSession bool                      `json:"prepare_session,omitempty"`
	AgentProfileID string                    `json:"agent_profile_id,omitempty"`
	ExecutorID     string                    `json:"executor_id,omitempty"`
	PlanMode       bool                      `json:"plan_mode,omitempty"`
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
	if (body.StartAgent || body.PrepareSession) && body.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required to start agent"})
		return
	}

	repos, ok := convertCreateTaskRepositories(c, body.Repositories)
	if !ok {
		return
	}

	resp, err := h.controller.CreateTask(c.Request.Context(), dto.CreateTaskRequest{
		WorkspaceID:    body.WorkspaceID,
		WorkflowID:     body.WorkflowID,
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
	// Use the backend-resolved workflow step ID (from the created task) instead of the request's
	resolvedStepID := resp.WorkflowStepID
	h.handlePostCreateTaskSession(c, &response, resp.ID, resp.Description, body, resolvedStepID)

	c.JSON(http.StatusOK, response)
}

// convertCreateTaskRepositories converts httpTaskRepositoryInput slice to dto.TaskRepositoryInput slice.
// Returns (nil, false) and writes a 400 response if any entry is missing both repository_id and local_path.
func convertCreateTaskRepositories(c *gin.Context, inputs []httpTaskRepositoryInput) ([]dto.TaskRepositoryInput, bool) {
	var repos []dto.TaskRepositoryInput
	for _, r := range inputs {
		if r.RepositoryID == "" && r.LocalPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repository_id or local_path is required"})
			return nil, false
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}
	return repos, true
}

// handlePostCreateTaskSession prepares or starts an agent session after a task is created,
// depending on the PrepareSession and StartAgent flags in the request body.
func (h *TaskHandlers) handlePostCreateTaskSession(
	c *gin.Context,
	response *createTaskResponse,
	taskID, description string,
	body httpCreateTaskRequest,
	resolvedStepID string,
) {
	if h.orchestrator == nil || body.AgentProfileID == "" {
		return
	}
	if body.PrepareSession && !body.StartAgent {
		// Create session entry without launching the agent.
		// The session stays in CREATED state until the user triggers it.
		sessionID, err := h.orchestrator.PrepareTaskSession(c.Request.Context(), taskID, body.AgentProfileID, body.ExecutorID, resolvedStepID)
		if err != nil {
			h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", taskID))
		} else {
			response.TaskSessionID = sessionID
		}
	} else if body.StartAgent {
		h.startAgentForNewTask(c.Request.Context(), response, taskID, description, body, resolvedStepID)
	}
}

// startAgentForNewTask prepares a session and launches the agent asynchronously for a
// newly created task when start_agent is requested. It populates response.TaskSessionID
// on success.
func (h *TaskHandlers) startAgentForNewTask(
	ctx context.Context,
	response *createTaskResponse,
	taskID, description string,
	body httpCreateTaskRequest,
	resolvedStepID string,
) {
	// Create session entry synchronously so we can return the session ID immediately
	sessionID, err := h.orchestrator.PrepareTaskSession(ctx, taskID, body.AgentProfileID, body.ExecutorID, resolvedStepID)
	if err != nil {
		h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", taskID))
		// Continue without session - task was created successfully
		return
	}
	response.TaskSessionID = sessionID

	// Launch agent asynchronously so the HTTP request can return immediately.
	// The frontend will receive WebSocket updates when the agent actually starts.
	executorID := body.ExecutorID // Capture for goroutine
	stepID := resolvedStepID     // Capture for goroutine
	go func() {
		// Use a longer timeout to accommodate setup scripts (which can take minutes for npm install, etc.)
		startCtx, cancel := context.WithTimeout(context.Background(), constants.AgentLaunchTimeout)
		defer cancel()
		// Use task description as the initial prompt with workflow step config (prompt prefix/suffix, plan mode)
		execution, err := h.orchestrator.StartTaskWithSession(startCtx, taskID, sessionID, body.AgentProfileID, executorID, body.Priority, description, stepID, body.PlanMode)
		if err != nil {
			h.logger.Error("failed to start agent for task (async)", zap.Error(err), zap.String("task_id", taskID), zap.String("session_id", sessionID))
			return
		}
		h.logger.Info("agent started for task (async)",
			zap.String("task_id", taskID),
			zap.String("session_id", execution.SessionID),
			zap.String("executor_id", executorID),
			zap.String("workflow_step_id", stepID),
			zap.String("execution_id", execution.AgentExecutionID))
	}()
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
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Position       int    `json:"position"`
}

func (h *TaskHandlers) httpMoveTask(c *gin.Context) {
	var body httpMoveTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.WorkflowID == "" || body.WorkflowStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id and workflow_step_id are required"})
		return
	}
	resp, err := h.controller.MoveTask(c.Request.Context(), dto.MoveTaskRequest{
		ID:             c.Param("id"),
		WorkflowID:     body.WorkflowID,
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

func (h *TaskHandlers) httpArchiveTask(c *gin.Context) {
	resp, err := h.controller.ArchiveTask(c.Request.Context(), dto.ArchiveTaskRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not archived")
		return
	}
	c.JSON(http.StatusOK, resp)
}
