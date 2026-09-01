// Package handlers provides WebSocket message handlers for the orchestrator.
package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/dto"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
	d.RegisterFunc(ws.ActionTaskLaunchRecover, h.wsRecoverTaskLaunch)
	d.RegisterFunc(ws.ActionSessionResetContext, h.wsResetContext)
	d.RegisterFunc(ws.ActionSessionStop, h.wsStopSession)
	d.RegisterFunc(ws.ActionSessionDelete, h.wsDeleteSession)
	d.RegisterFunc(ws.ActionSessionSetPrimary, h.wsSetPrimarySession)
	d.RegisterFunc(ws.ActionSessionSetPlanMode, h.wsSetPlanMode)
	d.RegisterFunc(ws.ActionSessionRename, h.wsRenameSession)
	d.RegisterFunc(ws.ActionSessionRouteAction, h.wsRouteAction)
	d.RegisterFunc(ws.ActionGitHubCheckSessionPR, h.wsCheckSessionPR)
	d.RegisterFunc(ws.ActionGitLabCheckSessionMR, h.wsCheckSessionMR)
}

type wsRouteActionRequest struct {
	SessionID          string `json:"session_id"`
	Action             string `json:"action"`
	ExpectedGeneration int64  `json:"expected_generation"`
}

func (h *Handlers) wsRouteAction(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRouteActionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Action != string(orchestrator.RouteActionRetry) &&
		req.Action != string(orchestrator.RouteActionTryNext) &&
		req.Action != string(orchestrator.RouteActionSkip) &&
		req.Action != string(orchestrator.RouteActionCancelWait) &&
		req.Action != string(orchestrator.RouteActionStop) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "unsupported route action", nil)
	}
	result, err := h.service.ApplyRouteAction(ctx, orchestrator.RouteActionRequest{
		SessionID: req.SessionID, Action: orchestrator.RouteAction(req.Action),
		ExpectedGeneration: req.ExpectedGeneration,
	})
	if err != nil {
		var conflict *orchestrator.RouteActionConflictError
		if errors.As(err, &conflict) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), map[string]interface{}{
				"route": conflict.Result,
			})
		}
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
		}
		if errors.Is(err, orchestrator.ErrRouteActionActiveTurn) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), nil)
		}
		h.logger.Warn("route action failed", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "route action failed", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
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

// wsGetQueue returns the queue of tasks waiting for an agent.
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

// wsLaunchSession starts or resumes a task's agent session.
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
		// A launch failing because the root context was cancelled or the
		// session is already terminal is an expected shutdown teardown race,
		// not a fault: log WARN (no stack trace) so it does not masquerade as a
		// crash. Genuine failures (unknown task, validation) stay ERROR.
		if orchestrator.IsBenignLaunchTeardownErr(err) {
			h.logger.Warn("session launch aborted during shutdown",
				zap.String("task_id", req.TaskID),
				zap.String("intent", string(orchestrator.ResolveIntent(&req))),
				zap.String("error", err.Error()))
		} else {
			h.logger.Error("failed to launch session",
				zap.String("task_id", req.TaskID),
				zap.String("intent", string(orchestrator.ResolveIntent(&req))),
				zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to launch session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsEnsureSessionRequest struct {
	TaskID    string `json:"task_id"`
	AutoStart *bool  `json:"auto_start,omitempty"`
}

// wsEnsureSession ensures a task has a session, honoring the auto_start override.
func (h *Handlers) wsEnsureSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsEnsureSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	resp, err := h.service.EnsureSession(ctx, req.TaskID, orchestrator.EnsureSessionOptions{AutoStart: req.AutoStart})
	if err != nil {
		h.logger.Error("failed to ensure session",
			zap.String("task_id", req.TaskID),
			zap.Error(err))
		// Mirror httpEnsureTaskSession's NotFound mapping so the frontend can
		// distinguish unknown task ids from real server errors. EnsureSession
		// wraps the repo's ErrTaskNotFound.
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to ensure session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsResetContextRequest struct {
	SessionID string `json:"session_id"`
}

// wsResetContext clears the session's agent context window.
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

// wsSetPlanMode toggles plan mode for a session.
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
	Action    string `json:"action"` // "resume", "resume_new_branch", "fresh_start", "runtime_retry", or "cancel_retry"
}

func branchRecoveryConflictResponse(msg *ws.Message, err error) (*ws.Message, error) {
	var branchRecoveryErr *orchestrator.BranchRecoveryError
	if !errors.As(err, &branchRecoveryErr) {
		return nil, nil
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), branchRecoveryErr.Details())
}

// wsRecoverSession recovers a session by id (resume or fresh start).
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
	// Cancel an in-progress transient provider retry loop and surface
	// the manual recovery banner. Distinct from resume/fresh_start: it does not
	// relaunch the agent, it stops the backoff timer.
	if req.Action == "cancel_retry" {
		cancelled := h.service.CancelTransientRetry(ctx, req.TaskID, req.SessionID)
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"cancelled": cancelled})
	}

	if req.Action != "resume" && req.Action != "resume_new_branch" && req.Action != "fresh_start" && req.Action != "runtime_retry" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "action must be 'resume', 'resume_new_branch', 'fresh_start', 'runtime_retry', or 'cancel_retry'", nil)
	}

	resp, err := h.service.RecoverSession(ctx, req.TaskID, req.SessionID, req.Action)
	if err != nil {
		if recoveryResponse, responseErr := branchRecoveryConflictResponse(msg, err); recoveryResponse != nil || responseErr != nil {
			return recoveryResponse, responseErr
		}
		h.logger.Error("failed to recover session",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.String("action", req.Action),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to recover session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// wsRecoverTaskLaunch applies one task-scoped launch recovery action. The
// service authorizes task_id before it reads the task, session, repository, or
// workflow rows named by the request.
func (h *Handlers) wsRecoverTaskLaunch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req orchestrator.TaskLaunchRecoveryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Action == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "action is required", nil)
	}
	if req.ErrorStamp == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "error_stamp is required", nil)
	}
	if req.Action != "mark_review_done" && req.TaskRepositoryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_repository_id is required for branch recovery", nil)
	}
	if req.Action == "pick_base_branch" && req.BaseBranch == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "base_branch is required for branch selection", nil)
	}

	response, err := h.service.RecoverTaskLaunch(ctx, &req)
	if err != nil {
		h.logger.Error("failed to recover task launch",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.String("action", req.Action),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

type wsStopTaskRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// wsStopTask stops the agent running for a task.
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
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
	// Rejected is true when the user explicitly clicked Deny. Distinct from
	// Cancelled (user dismissed the dialog) so the backend can persist
	// "rejected" status without triggering the cancellation event path.
	Rejected bool `json:"rejected,omitempty"`
}

// wsRespondToPermission forwards the user's permission decision.
func (h *Handlers) wsRespondToPermission(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsPermissionRespondRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.RequestID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "request_id is required", nil)
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
		zap.Bool("cancelled", req.Cancelled),
		zap.Bool("rejected", req.Rejected))

	if err := h.service.RespondToPermission(ctx, req.TaskID, req.SessionID, req.RequestID, req.PendingID, req.OptionID, req.Cancelled, req.Rejected); err != nil {
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

type wsCheckSessionAssociationRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

// wsCheckSessionAssociation is the shared body for the "check session has a
// linked PR/MR" WS handlers (github.check_session_pr, gitlab.check_session_mr).
// The two providers differ only in which Service method answers the check and
// the label used in logs/errors.
func (h *Handlers) wsCheckSessionAssociation(
	ctx context.Context, msg *ws.Message, req wsCheckSessionAssociationRequest,
	check func(context.Context, string, string) (bool, error), label string,
) (*ws.Message, error) {
	if req.TaskID == "" || req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and session_id are required", nil)
	}

	found, err := check(ctx, req.TaskID, req.SessionID)
	if err != nil {
		h.logger.Error("failed to check "+label,
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to check "+label, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"found": found})
}

// wsCheckSessionPR refreshes the PR association for a session.
func (h *Handlers) wsCheckSessionPR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCheckSessionAssociationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	return h.wsCheckSessionAssociation(ctx, msg, req, h.service.CheckSessionPR, "session PR")
}

// wsCheckSessionMR refreshes the MR association for a session.
func (h *Handlers) wsCheckSessionMR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCheckSessionAssociationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	return h.wsCheckSessionAssociation(ctx, msg, req, h.service.CheckSessionMR, "session MR")
}

type wsGetTaskSessionStatusRequest struct {
	TaskID        string `json:"task_id"`
	TaskSessionID string `json:"session_id"`
}

// wsGetTaskSessionStatus returns a task session's detailed status.
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

// parseSessionAction parses a session-scoped action request, returning an error message on failure.
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

// wsStopSession stops a session's agent.
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

// wsDeleteSession deletes a session.
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

// wsSetPrimarySession marks a session as the task's primary session.
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

type wsRenameSessionRequest struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
}

// wsRenameSession renames a session.
func (h *Handlers) wsRenameSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRenameSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if err := h.service.RenameSession(ctx, req.SessionID, req.Name); err != nil {
		h.logger.Error("failed to rename session", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to rename session: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SuccessResponse{Success: true})
}

type wsCancelAgentRequest struct {
	SessionID string `json:"session_id"`
}

// wsCancelAgent cancels the in-flight agent turn.
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
