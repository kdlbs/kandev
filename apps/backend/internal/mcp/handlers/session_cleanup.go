package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type closeTaskSessionRequest = scopedTerminalSessionRequest

type taskSessionCleanupReceiptReader interface {
	GetTaskSessionCleanupReceipt(context.Context, string, string) (*models.TaskSessionCleanupReceipt, error)
}

type taskSessionCloser interface {
	DeleteSession(context.Context, string) error
}

func (h *Handlers) handleCloseTaskSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, workspaceID, parseErr := parseScopedTerminalSessionRequest(ctx, msg.Payload)
	if response, err := terminalSessionScopeError(msg, parseErr, workspaceID, "session cleanup"); response != nil || err != nil {
		return response, err
	}
	return h.closeAuthorizedTaskSession(ctx, msg, req, workspaceID)
}

func (h *Handlers) closeAuthorizedTaskSession(
	ctx context.Context, msg *ws.Message, req closeTaskSessionRequest, workspaceID string,
) (*ws.Message, error) {
	if response, err := h.validateCleanupCaller(ctx, msg, req); response != nil || err != nil {
		return response, err
	}
	reader, ok := h.sessionRepo.(taskSessionCleanupReceiptReader)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup receipt is unavailable", nil)
	}
	target, response, err := h.resolveCleanupTarget(ctx, msg, req, workspaceID, reader)
	if response != nil || err != nil {
		return response, err
	}
	closer, ok := h.sessionLauncher.(taskSessionCloser)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup is unavailable", nil)
	}
	if err := closer.DeleteSession(ctx, target.ID); err != nil {
		return h.cleanupExecutionError(msg, err)
	}
	receipt, err := reader.GetTaskSessionCleanupReceipt(ctx, req.TaskID, req.TargetSessionID)
	if err != nil || receipt == nil || receipt.WorkspaceID != workspaceID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup committed without a readable receipt", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"receipt": receipt})
}

func (h *Handlers) validateCleanupCaller(ctx context.Context, msg *ws.Message, req closeTaskSessionRequest) (*ws.Message, error) {
	caller, err := h.sessionRepo.GetTaskSession(ctx, req.CallerSessionID)
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		if h.logger != nil {
			h.logger.Error("session cleanup caller lookup failed", zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup is unavailable", nil)
	}
	if err != nil || caller == nil || caller.TaskID != req.TaskID || !caller.IsPrimary || caller.ArchivedAt != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "session cleanup requires the current primary session", nil)
	}
	return nil, nil
}

func (h *Handlers) cleanupExecutionError(msg *ws.Message, err error) (*ws.Message, error) {
	if errors.Is(err, messagequeue.ErrSessionQueueNotEmpty) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "target session has pending queued work", nil)
	}
	if errors.Is(err, orchestrator.ErrSessionResetInProgress) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "target session is busy", nil)
	}
	if h.logger != nil {
		h.logger.Error("session cleanup failed", zap.Error(err))
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup failed", nil)
}

func (h *Handlers) resolveCleanupTarget(
	ctx context.Context, msg *ws.Message, req closeTaskSessionRequest, workspaceID string,
	reader taskSessionCleanupReceiptReader,
) (*models.TaskSession, *ws.Message, error) {
	target, err := h.sessionRepo.GetTaskSession(ctx, req.TargetSessionID)
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		if h.logger != nil {
			h.logger.Error("session cleanup target lookup failed", zap.Error(err))
		}
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session cleanup is unavailable", nil)
		return nil, response, responseErr
	}
	if err == nil && (target.TaskID != req.TaskID || target.IsPrimary) {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "target session is outside the calling task", nil)
		return nil, response, responseErr
	}
	if err == nil && target.ArchivedAt == nil {
		if !isTerminalCleanupState(target.State) {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "target session is not terminal", nil)
			return nil, response, responseErr
		}
		return target, nil, nil
	}
	receipt, receiptErr := reader.GetTaskSessionCleanupReceipt(ctx, req.TaskID, req.TargetSessionID)
	if receiptErr == nil && receipt != nil && receipt.WorkspaceID == workspaceID {
		response, responseErr := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"receipt": receipt})
		return nil, response, responseErr
	}
	code, message := ws.ErrorCodeForbidden, "target session is outside the calling task"
	if err == nil {
		code, message = ws.ErrorCodeInternalError, "archived session has no cleanup receipt"
	}
	response, responseErr := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return nil, response, responseErr
}

func isTerminalCleanupState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCompleted || state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}
