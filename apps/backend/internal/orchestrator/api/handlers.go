package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/errors"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Handler contains HTTP handlers for the orchestrator API
type Handler struct {
	service *orchestrator.Service
	logger  *logger.Logger
}

// NewHandler creates a new API handler
func NewHandler(service *orchestrator.Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log.WithFields(zap.String("component", "orchestrator-api")),
	}
}

// GetStatus returns the overall orchestrator status
// GET /api/v1/orchestrator/status
func (h *Handler) GetStatus(c *gin.Context) {
	status := h.service.GetStatus()
	c.JSON(http.StatusOK, status)
}

// GetQueue returns the current task queue
// GET /api/v1/orchestrator/queue
func (h *Handler) GetQueue(c *gin.Context) {
	queuedTasks := h.service.GetQueuedTasks()

	tasks := make([]QueuedTaskResponse, 0, len(queuedTasks))
	for _, qt := range queuedTasks {
		tasks = append(tasks, QueuedTaskResponse{
			TaskID:   qt.TaskID,
			Priority: qt.Priority,
			QueuedAt: qt.QueuedAt,
		})
	}

	c.JSON(http.StatusOK, QueueResponse{
		Tasks: tasks,
		Total: len(tasks),
	})
}

// TriggerTask manually triggers orchestration for a specific task
// POST /api/v1/orchestrator/trigger
func (h *Handler) TriggerTask(c *gin.Context) {
	var req TriggerTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.ValidationError("request", err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Create a minimal task for enqueueing
	task := &v1.Task{
		ID: req.TaskID,
	}

	if err := h.service.EnqueueTask(c.Request.Context(), task); err != nil {
		h.logger.Error("failed to trigger task", zap.String("task_id", req.TaskID), zap.Error(err))
		appErr := errors.Wrap(err, "failed to trigger task")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "task triggered successfully",
		"task_id": req.TaskID,
	})
}

// StartTask starts agent execution for a task
// POST /api/v1/orchestrator/tasks/:taskId/start
func (h *Handler) StartTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req StartTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body, use defaults
		req = StartTaskRequest{}
	}

	execution, err := h.service.StartTask(c.Request.Context(), taskID, req.AgentType, req.Priority)
	if err != nil {
		h.logger.Error("failed to start task", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.Wrap(err, "failed to start task")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	startedAt := execution.StartedAt
	c.JSON(http.StatusAccepted, TaskStatusResponse{
		TaskID:          execution.TaskID,
		State:           string(execution.Status),
		AgentInstanceID: execution.AgentInstanceID,
		AgentType:       execution.AgentType,
		StartedAt:       &startedAt,
		Progress:        execution.Progress,
	})
}

// StopTask stops agent execution for a task
// POST /api/v1/orchestrator/tasks/:taskId/stop
func (h *Handler) StopTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req StopTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body, use defaults
		req = StopTaskRequest{}
	}

	if err := h.service.StopTask(c.Request.Context(), taskID, req.Reason, req.Force); err != nil {
		h.logger.Error("failed to stop task", zap.String("task_id", taskID), zap.Error(err))
		appErr := errors.Wrap(err, "failed to stop task")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "task stopped successfully",
		"task_id": taskID,
	})
}

// PromptTask sends a follow-up prompt to a running agent
// POST /api/v1/orchestrator/tasks/:taskId/prompt
func (h *Handler) PromptTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	var req PromptTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appErr := errors.ValidationError("request", err.Error())
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	if err := h.service.PromptTask(c.Request.Context(), taskID, req.Prompt); err != nil {
		h.logger.Error("failed to prompt task", zap.String("task_id", taskID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptTaskResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, PromptTaskResponse{Success: true})
}

// CompleteTask explicitly completes a running agent and marks the task as completed
// POST /api/v1/orchestrator/tasks/:taskId/complete
func (h *Handler) CompleteTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	if err := h.service.CompleteTask(c.Request.Context(), taskID); err != nil {
		h.logger.Error("failed to complete task", zap.String("task_id", taskID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "task completed"})
}

// PauseTask pauses agent execution (if supported)
// POST /api/v1/orchestrator/tasks/:taskId/pause
func (h *Handler) PauseTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Pause is not currently implemented
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    "NOT_IMPLEMENTED",
		"message": "pause functionality is not yet implemented",
		"task_id": taskID,
	})
}

// ResumeTask resumes paused agent execution
// POST /api/v1/orchestrator/tasks/:taskId/resume
func (h *Handler) ResumeTask(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Resume is not currently implemented
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    "NOT_IMPLEMENTED",
		"message": "resume functionality is not yet implemented",
		"task_id": taskID,
	})
}

// GetTaskStatus returns detailed execution status for a task
// GET /api/v1/orchestrator/tasks/:taskId/status
func (h *Handler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	execution, found := h.service.GetTaskExecution(taskID)
	if !found {
		appErr := errors.NotFound("task execution", taskID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	startedAt := execution.StartedAt
	c.JSON(http.StatusOK, TaskStatusResponse{
		TaskID:          execution.TaskID,
		State:           string(execution.Status),
		AgentInstanceID: execution.AgentInstanceID,
		AgentType:       execution.AgentType,
		StartedAt:       &startedAt,
		Progress:        execution.Progress,
	})
}

// GetTaskLogs returns historical logs for a task's agent execution
// GET /api/v1/orchestrator/tasks/:taskId/logs
func (h *Handler) GetTaskLogs(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Check if task execution exists
	_, found := h.service.GetTaskExecution(taskID)
	if !found {
		appErr := errors.NotFound("task execution", taskID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Logs retrieval is not fully implemented yet
	// Return empty logs for now
	c.JSON(http.StatusOK, LogsResponse{
		Logs:  []LogEntry{},
		Total: 0,
	})
}

// GetTaskArtifacts lists artifacts generated by agent
// GET /api/v1/orchestrator/tasks/:taskId/artifacts
func (h *Handler) GetTaskArtifacts(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		appErr := errors.BadRequest("taskId is required")
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Check if task execution exists
	_, found := h.service.GetTaskExecution(taskID)
	if !found {
		appErr := errors.NotFound("task execution", taskID)
		c.JSON(appErr.HTTPStatus, appErr)
		return
	}

	// Artifacts retrieval is not fully implemented yet
	// Return empty artifacts for now
	c.JSON(http.StatusOK, ArtifactsResponse{
		Artifacts: []ArtifactResponse{},
	})
}

