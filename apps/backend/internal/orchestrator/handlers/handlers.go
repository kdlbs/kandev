// Package handlers provides WebSocket message handlers for the orchestrator.
package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/controller"
	"github.com/kandev/kandev/internal/orchestrator/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Handlers contains WebSocket handlers for the orchestrator API
type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

// NewHandlers creates a new WebSocket handlers instance
func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "orchestrator-handlers")),
	}
}

// RegisterHandlers registers all orchestrator handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionOrchestratorStatus, h.wsGetStatus)
	d.RegisterFunc(ws.ActionOrchestratorQueue, h.wsGetQueue)
	d.RegisterFunc(ws.ActionOrchestratorTrigger, h.wsTriggerTask)
	d.RegisterFunc(ws.ActionOrchestratorStart, h.wsStartTask)
	d.RegisterFunc(ws.ActionOrchestratorStop, h.wsStopTask)
	d.RegisterFunc(ws.ActionOrchestratorPrompt, h.wsPromptTask)
	d.RegisterFunc(ws.ActionOrchestratorComplete, h.wsCompleteTask)
	d.RegisterFunc(ws.ActionTaskLogs, h.wsGetTaskLogs)
	d.RegisterFunc(ws.ActionPermissionRespond, h.wsRespondToPermission)
}

// WS handlers

func (h *Handlers) wsGetStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.GetStatus(ctx, dto.GetStatusRequest{})
	if err != nil {
		h.logger.Error("failed to get status", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get status", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsGetQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.GetQueue(ctx, dto.GetQueueRequest{})
	if err != nil {
		h.logger.Error("failed to get queue", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get queue", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsTriggerTaskRequest struct {
	TaskID string `json:"task_id"`
}

func (h *Handlers) wsTriggerTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsTriggerTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.TriggerTask(ctx, dto.TriggerTaskRequest{TaskID: req.TaskID})
	if err != nil {
		h.logger.Error("failed to trigger task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to trigger task: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsStartTaskRequest struct {
	TaskID    string `json:"task_id"`
	AgentType string `json:"agent_type,omitempty"`
	Priority  int    `json:"priority,omitempty"`
}

func (h *Handlers) wsStartTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsStartTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.StartTask(ctx, dto.StartTaskRequest{
		TaskID:    req.TaskID,
		AgentType: req.AgentType,
		Priority:  req.Priority,
	})
	if err != nil {
		h.logger.Error("failed to start task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start task: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsStopTaskRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

func (h *Handlers) wsStopTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsStopTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.StopTask(ctx, dto.StopTaskRequest{
		TaskID: req.TaskID,
		Reason: req.Reason,
		Force:  req.Force,
	})
	if err != nil {
		h.logger.Error("failed to stop task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop task: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsPromptTaskRequest struct {
	TaskID string `json:"task_id"`
	Prompt string `json:"prompt"`
}

func (h *Handlers) wsPromptTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsPromptTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required", nil)
	}

	resp, err := h.controller.PromptTask(ctx, dto.PromptTaskRequest{
		TaskID: req.TaskID,
		Prompt: req.Prompt,
	})
	if err != nil {
		h.logger.Error("failed to send prompt", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send prompt: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCompleteTaskRequest struct {
	TaskID string `json:"task_id"`
}

func (h *Handlers) wsCompleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCompleteTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.CompleteTask(ctx, dto.CompleteTaskRequest{TaskID: req.TaskID})
	if err != nil {
		h.logger.Error("failed to complete task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to complete task: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetTaskLogsRequest struct {
	TaskID string `json:"task_id"`
	Limit  int    `json:"limit,omitempty"`
}

func (h *Handlers) wsGetTaskLogs(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetTaskLogsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.controller.GetTaskLogs(ctx, dto.GetTaskLogsRequest{
		TaskID: req.TaskID,
		Limit:  req.Limit,
	})
	if err != nil {
		h.logger.Error("failed to get task logs", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to retrieve logs: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsPermissionRespondRequest struct {
	TaskID    string `json:"task_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

func (h *Handlers) wsRespondToPermission(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsPermissionRespondRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.PendingID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "pending_id is required", nil)
	}
	if !req.Cancelled && req.OptionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "option_id is required when not cancelled", nil)
	}

	h.logger.Info("responding to permission request",
		zap.String("task_id", req.TaskID),
		zap.String("pending_id", req.PendingID),
		zap.String("option_id", req.OptionID),
		zap.Bool("cancelled", req.Cancelled))

	resp, err := h.controller.RespondToPermission(ctx, dto.PermissionRespondRequest{
		TaskID:    req.TaskID,
		PendingID: req.PendingID,
		OptionID:  req.OptionID,
		Cancelled: req.Cancelled,
	})
	if err != nil {
		h.logger.Error("failed to respond to permission", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to respond to permission: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

