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
	d.RegisterFunc(ws.ActionOrchestratorTrigger, h.wsTriggerTask)
	d.RegisterFunc(ws.ActionOrchestratorStart, h.wsStartTask)
	d.RegisterFunc(ws.ActionOrchestratorStop, h.wsStopTask)
	d.RegisterFunc(ws.ActionOrchestratorPrompt, h.wsPromptTask)
	d.RegisterFunc(ws.ActionOrchestratorComplete, h.wsCompleteTask)
	d.RegisterFunc(ws.ActionPermissionRespond, h.wsRespondToPermission)
	d.RegisterFunc(ws.ActionTaskSessionResume, h.wsResumeTaskSession)
	d.RegisterFunc(ws.ActionTaskSessionStatus, h.wsGetTaskSessionStatus)
	d.RegisterFunc(ws.ActionAgentCancel, h.wsCancelAgent)
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

	// TriggerTask is a placeholder that just returns success
	resp := dto.TriggerTaskResponse{
		Success: true,
		Message: "Task triggered",
		TaskID:  req.TaskID,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsStartTaskRequest struct {
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id,omitempty"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
	ExecutorID     string `json:"executor_id,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	WorkflowStepID string `json:"workflow_step_id,omitempty"`
}

func (h *Handlers) wsStartTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsStartTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	// agent_profile_id is only required when creating a new session (no session_id provided)
	if req.SessionID == "" && req.AgentProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_profile_id is required when creating a new session", nil)
	}

	// If session_id is provided, resume existing session with workflow step config
	if req.SessionID != "" {
		err := h.service.StartSessionForWorkflowStep(ctx, req.TaskID, req.SessionID, req.WorkflowStepID)
		if err != nil {
			h.logger.Error("failed to start task", zap.String("task_id", req.TaskID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start task: "+err.Error(), nil)
		}
		resp := dto.StartTaskResponse{
			Success:       true,
			TaskID:        req.TaskID,
			TaskSessionID: req.SessionID,
			State:         "RUNNING",
		}
		return ws.NewResponse(msg.ID, msg.Action, resp)
	}

	// Otherwise, create a new session
	execution, err := h.service.StartTask(ctx, req.TaskID, req.AgentProfileID, req.ExecutorID, req.Priority, req.Prompt, req.WorkflowStepID, false)
	if err != nil {
		h.logger.Error("failed to start task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start task: "+err.Error(), nil)
	}

	resp := dto.StartTaskResponse{
		Success:          true,
		TaskID:           execution.TaskID,
		AgentExecutionID: execution.AgentExecutionID,
		TaskSessionID:    execution.SessionID,
		State:            string(execution.SessionState),
	}
	if execution.WorktreePath != "" {
		resp.WorktreePath = &execution.WorktreePath
	}
	if execution.WorktreeBranch != "" {
		resp.WorktreeBranch = &execution.WorktreeBranch
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsResumeTaskSessionRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
}

func (h *Handlers) wsResumeTaskSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsResumeTaskSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	execution, err := h.service.ResumeTaskSession(ctx, req.TaskID, req.TaskSessionID)
	if err != nil {
		h.logger.Error("failed to resume task session",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.TaskSessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to resume task session: "+err.Error(), nil)
	}

	resp := dto.ResumeTaskSessionResponse{
		Success:          true,
		TaskID:           execution.TaskID,
		AgentExecutionID: execution.AgentExecutionID,
		TaskSessionID:    execution.SessionID,
		State:            string(execution.SessionState),
	}
	if execution.WorktreePath != "" {
		resp.WorktreePath = &execution.WorktreePath
	}
	if execution.WorktreeBranch != "" {
		resp.WorktreeBranch = &execution.WorktreeBranch
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

type wsPromptTaskRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model,omitempty"`
}

func (h *Handlers) wsPromptTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsPromptTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required", nil)
	}

	result, err := h.service.PromptTask(ctx, req.TaskID, req.TaskSessionID, req.Prompt, req.Model, false, nil)
	if err != nil {
		h.logger.Error("failed to send prompt", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send prompt: "+err.Error(), nil)
	}
	resp := dto.PromptTaskResponse{
		Success:    true,
		StopReason: result.StopReason,
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

	if err := h.service.CompleteTask(ctx, req.TaskID); err != nil {
		h.logger.Error("failed to complete task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to complete task: "+err.Error(), nil)
	}
	resp := dto.CompleteTaskResponse{
		Success: true,
		Message: "task completed",
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
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
