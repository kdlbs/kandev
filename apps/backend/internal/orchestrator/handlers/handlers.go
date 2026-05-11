// Package handlers provides WebSocket message handlers for the orchestrator.
package handlers

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/dto"
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
		logger:  log.WithFields(zap.String("component", "orchestrator-handlers")),
	}
}

// RegisterHandlers registers all orchestrator handlers with the dispatcher
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionOrchestratorStatus, h.wsGetStatus)
	d.RegisterFunc(ws.ActionOrchestratorQueue, h.wsGetQueue)
	d.RegisterFunc(ws.ActionOrchestratorStop, h.wsStopTask)
	d.RegisterFunc(ws.ActionPermissionRespond, h.wsRespondToPermission)
	d.RegisterFunc(ws.ActionTaskSessionStatus, h.wsGetTaskSessionStatus)
	d.RegisterFunc(ws.ActionAgentCancel, h.wsCancelAgent)
	d.RegisterFunc(ws.ActionSessionLaunch, h.wsLaunchSession)
	d.RegisterFunc(ws.ActionSessionEnsure, h.wsEnsureSession)
	d.RegisterFunc(ws.ActionSessionRecover, h.wsRecoverSession)
	d.RegisterFunc(ws.ActionSessionResetContext, h.wsResetContext)
	d.RegisterFunc(ws.ActionSessionStop, h.wsStopSession)
	d.RegisterFunc(ws.ActionSessionDelete, h.wsDeleteSession)
	d.RegisterFunc(ws.ActionSessionSetPrimary, h.wsSetPrimarySession)
	d.RegisterFunc(ws.ActionSessionSetPlanMode, h.wsSetPlanMode)
	d.RegisterFunc(ws.ActionGitHubCheckSessionPR, h.wsCheckSessionPR)
}

// WS handlers

func (h *Handlers) wsGetStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	status := h.service.GetStatus()
	resp := dto.StatusResponse{
		Running:      status.Running,
		ActiveAgents: status.ActiveAgents,
		QueuedTasks:  status.QueuedTasks,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsGetQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	queuedTasks := h.service.GetQueuedTasks()
	tasks := make([]dto.QueuedTaskDTO, 0, len(queuedTasks))
	for _, qt := range queuedTasks {
		tasks = append(tasks, dto.QueuedTaskDTO{
			TaskID:   qt.TaskID,
			Priority: qt.Priority,
			QueuedAt: qt.QueuedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	resp := dto.QueueResponse{
		Tasks: tasks,
		Total: len(tasks),
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) wsLaunchSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req orchestrator.LaunchSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.service.LaunchSession(ctx, &req)
	if err != nil {
		h.logger.Error("failed to launch session",
			zap.String("task_id", req.TaskID),
			zap.String("intent", string(orchestrator.ResolveIntent(&req))),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to launch session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsEnsureSessionRequest struct {
	TaskID string `json:"task_id"`
}

func (h *Handlers) wsEnsureSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsEnsureSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.service.EnsureSession(ctx, req.TaskID)
	if err != nil {
		h.logger.Error("failed to ensure session",
			zap.String("task_id", req.TaskID),
			zap.Error(err))
		// Mirror httpEnsureTaskSession's NotFound mapping so the frontend can
		// distinguish unknown task ids from real server errors. EnsureSession
		// wraps the repo error as "task not found: %w".
		if strings.Contains(err.Error(), "task not found") {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to ensure session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsResetContextRequest struct {
	SessionID string `json:"session_id"`
}

func (h *Handlers) wsResetContext(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsResetContextRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	if err := h.service.ResetAgentContext(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to reset agent context",
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reset agent context: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"success":    true,
		"session_id": req.SessionID,
	})
}

type wsSetPlanModeRequest struct {
	SessionID string `json:"session_id"`
	Enabled   bool   `json:"enabled"`
}

func (h *Handlers) wsSetPlanMode(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSetPlanModeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	if err := h.service.SetSessionPlanModeByID(ctx, req.SessionID, req.Enabled); err != nil {
		h.logger.Error("failed to set session plan mode",
			zap.String("session_id", req.SessionID),
			zap.Bool("enabled", req.Enabled),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set plan mode: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"success":    true,
		"session_id": req.SessionID,
		"enabled":    req.Enabled,
	})
}

type wsRecoverSessionRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Action    string `json:"action"` // "resume" or "fresh_start"
}

func (h *Handlers) wsRecoverSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRecoverSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Action != "resume" && req.Action != "fresh_start" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "action must be 'resume' or 'fresh_start'", nil)
	}

	resp, err := h.service.RecoverSession(ctx, req.TaskID, req.SessionID, req.Action)
	if err != nil {
		h.logger.Error("failed to recover session",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.String("action", req.Action),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to recover session: "+err.Error(), nil)
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

	reason := req.Reason
	if reason == "" {
		reason = "stopped via API"
	}
	if err := h.service.StopTask(ctx, req.TaskID, reason, req.Force); err != nil {
		h.logger.Error("failed to stop task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop task: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}

type wsPermissionRespondRequest struct {
	SessionID string `json:"session_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

func (h *Handlers) wsRespondToPermission(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsPermissionRespondRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.PendingID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "pending_id is required", nil)
	}
	if !req.Cancelled && req.OptionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "option_id is required when not cancelled", nil)
	}

	h.logger.Info("responding to permission request",
		zap.String("session_id", req.SessionID),
		zap.String("pending_id", req.PendingID),
		zap.String("option_id", req.OptionID),
		zap.Bool("cancelled", req.Cancelled))

	if err := h.service.RespondToPermission(ctx, req.SessionID, req.PendingID, req.OptionID, req.Cancelled); err != nil {
		h.logger.Error("failed to respond to permission", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to respond to permission: "+err.Error(), nil)
	}
	resp := dto.PermissionRespondResponse{
		Success:   true,
		SessionID: req.SessionID,
		PendingID: req.PendingID,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCheckSessionPRRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

func (h *Handlers) wsCheckSessionPR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCheckSessionPRRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and session_id are required", nil)
	}

	found, err := h.service.CheckSessionPR(ctx, req.TaskID, req.SessionID)
	if err != nil {
		h.logger.Error("failed to check session PR",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to check session PR", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"found": found})
}

type wsGetTaskSessionStatusRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
}

func (h *Handlers) wsGetTaskSessionStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetTaskSessionStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid request payload", nil)
	}

	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "task_id is required", nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id is required", nil)
	}

	// Service returns dto.TaskSessionStatusResponse directly
	resp, err := h.service.GetTaskSessionStatus(ctx, req.TaskID, req.TaskSessionID)
	if err != nil {
		h.logger.Error("failed to get task session status",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.TaskSessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task session status: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsSessionActionRequest struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

func (h *Handlers) parseSessionAction(msg *ws.Message) (*wsSessionActionRequest, *ws.Message) {
	var req wsSessionActionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return nil, resp
	}
	if req.SessionID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return nil, resp
	}
	return &req, nil
}

func (h *Handlers) wsStopSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errResp := h.parseSessionAction(msg)
	if errResp != nil {
		return errResp, nil
	}
	reason := req.Reason
	if reason == "" {
		reason = "stopped via API"
	}
	if err := h.service.StopSession(ctx, req.SessionID, reason, req.Force); err != nil {
		h.logger.Error("failed to stop session", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}

func (h *Handlers) wsDeleteSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errResp := h.parseSessionAction(msg)
	if errResp != nil {
		return errResp, nil
	}
	if err := h.service.DeleteSession(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to delete session", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}

func (h *Handlers) wsSetPrimarySession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errResp := h.parseSessionAction(msg)
	if errResp != nil {
		return errResp, nil
	}
	if err := h.service.SetPrimarySession(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to set primary session", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set primary session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}

type wsCancelAgentRequest struct {
	SessionID string `json:"session_id"`
}

func (h *Handlers) wsCancelAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCancelAgentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	h.logger.Info("cancelling agent turn",
		zap.String("session_id", req.SessionID))

	if err := h.service.CancelAgent(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to cancel agent", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to cancel agent: "+err.Error(), nil)
	}
	resp := dto.CancelAgentResponse{
		Success:   true,
		SessionID: req.SessionID,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
