// Package wshandlers provides WebSocket message handlers for the orchestrator.
package wshandlers

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Handlers contains WebSocket handlers for the orchestrator API
type Handlers struct {
	service *orchestrator.Service
	logger  *logger.Logger
}

// NewHandlers creates a new WebSocket handlers instance
func NewHandlers(svc *orchestrator.Service, log *logger.Logger) *Handlers {
	return &Handlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "orchestrator-ws-handlers")),
	}
}

// RegisterHandlers registers all orchestrator handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionOrchestratorStatus, h.GetStatus)
	d.RegisterFunc(ws.ActionOrchestratorQueue, h.GetQueue)
	d.RegisterFunc(ws.ActionOrchestratorTrigger, h.TriggerTask)
	d.RegisterFunc(ws.ActionOrchestratorStart, h.StartTask)
	d.RegisterFunc(ws.ActionOrchestratorStop, h.StopTask)
	d.RegisterFunc(ws.ActionOrchestratorPrompt, h.PromptTask)
	d.RegisterFunc(ws.ActionOrchestratorComplete, h.CompleteTask)
}

// GetStatus handles orchestrator.status action
func (h *Handlers) GetStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	status := h.service.GetStatus()
	return ws.NewResponse(msg.ID, msg.Action, status)
}

// QueuedTaskResponse represents a task in the queue
type QueuedTaskResponse struct {
	TaskID   string `json:"task_id"`
	Priority int    `json:"priority"`
	QueuedAt string `json:"queued_at"`
}

// QueueResponse is the response for orchestrator.queue
type QueueResponse struct {
	Tasks []QueuedTaskResponse `json:"tasks"`
	Total int                  `json:"total"`
}

// GetQueue handles orchestrator.queue action
func (h *Handlers) GetQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	queuedTasks := h.service.GetQueuedTasks()

	tasks := make([]QueuedTaskResponse, 0, len(queuedTasks))
	for _, qt := range queuedTasks {
		tasks = append(tasks, QueuedTaskResponse{
			TaskID:   qt.TaskID,
			Priority: qt.Priority,
			QueuedAt: qt.QueuedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, QueueResponse{
		Tasks: tasks,
		Total: len(tasks),
	})
}

// TriggerTaskRequest is the payload for orchestrator.trigger
type TriggerTaskRequest struct {
	TaskID string `json:"task_id"`
}

// TriggerTask handles orchestrator.trigger action
func (h *Handlers) TriggerTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req TriggerTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	// For now just log - the service would need task details
	h.logger.Info("Triggering task", zap.String("task_id", req.TaskID))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"message": "Task triggered",
		"task_id": req.TaskID,
	})
}

// StartTaskRequest is the payload for orchestrator.start
type StartTaskRequest struct {
	TaskID    string `json:"task_id"`
	AgentType string `json:"agent_type,omitempty"`
	Priority  int    `json:"priority,omitempty"`
}

// StartTask handles orchestrator.start action
func (h *Handlers) StartTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req StartTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	execution, err := h.service.StartTask(ctx, req.TaskID, req.AgentType, req.Priority)
	if err != nil {
		h.logger.Error("failed to start task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start task: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":           true,
		"task_id":           execution.TaskID,
		"agent_instance_id": execution.AgentInstanceID,
		"status":            execution.Status,
	})
}

// TaskIDRequest is a common request with just task_id
type TaskIDRequest struct {
	TaskID string `json:"task_id"`
}

// StopTaskRequest is the payload for orchestrator.stop
type StopTaskRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// StopTask handles orchestrator.stop action
func (h *Handlers) StopTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req StopTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	reason := req.Reason
	if reason == "" {
		reason = "stopped via WebSocket API"
	}

	if err := h.service.StopTask(ctx, req.TaskID, reason, req.Force); err != nil {
		h.logger.Error("failed to stop task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop task: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// PromptTaskRequest is the payload for orchestrator.prompt
type PromptTaskRequest struct {
	TaskID string `json:"task_id"`
	Prompt string `json:"prompt"`
}

// PromptTask handles orchestrator.prompt action
func (h *Handlers) PromptTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req PromptTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required", nil)
	}

	if err := h.service.PromptTask(ctx, req.TaskID, req.Prompt); err != nil {
		h.logger.Error("failed to send prompt", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send prompt: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// CompleteTask handles orchestrator.complete action
func (h *Handlers) CompleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req TaskIDRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.service.CompleteTask(ctx, req.TaskID); err != nil {
		h.logger.Error("failed to complete task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to complete task: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"message": "task completed",
	})
}
