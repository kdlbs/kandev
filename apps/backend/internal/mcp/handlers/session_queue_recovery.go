package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type recoverSessionQueueRequest = scopedTerminalSessionRequest

type sessionQueueRecoveryCoordinator interface {
	RecoverTaskSessionQueue(context.Context, messagequeue.QueueRecoveryScope) (*messagequeue.QueueRecoveryResult, error)
}

func (h *Handlers) handleRecoverSessionQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, workspaceID, parseErr := parseScopedTerminalSessionRequest(ctx, msg.Payload)
	if response, err := terminalSessionScopeError(msg, parseErr, workspaceID, "queue recovery"); response != nil || err != nil {
		return response, err
	}
	return h.recoverAuthorizedSessionQueue(ctx, msg, req, workspaceID)
}

func (h *Handlers) recoverAuthorizedSessionQueue(
	ctx context.Context, msg *ws.Message, req recoverSessionQueueRequest, workspaceID string,
) (*ws.Message, error) {
	caller, err := h.sessionRepo.GetTaskSession(ctx, req.CallerSessionID)
	if err != nil || caller.TaskID != req.TaskID || !caller.IsPrimary {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "queue recovery requires the calling task's current primary session", nil)
	}
	target, err := h.sessionRepo.GetTaskSession(ctx, req.TargetSessionID)
	if err != nil || target.TaskID != req.TaskID || target.IsPrimary {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "queue recovery target is outside the calling task", nil)
	}
	if target.State != models.TaskSessionStateCompleted && target.State != models.TaskSessionStateFailed &&
		target.State != models.TaskSessionStateCancelled {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "queue recovery target must be terminal", nil)
	}
	coordinator, ok := h.sessionRepo.(sessionQueueRecoveryCoordinator)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "message queue recovery is unavailable", nil)
	}
	result, err := coordinator.RecoverTaskSessionQueue(ctx, messagequeue.QueueRecoveryScope{
		TaskID: req.TaskID, WorkspaceID: workspaceID,
		SourceSessionID: req.TargetSessionID, DestinationSessionID: req.CallerSessionID,
	})
	if err != nil {
		return h.queueRecoveryError(msg, req.TargetSessionID, err)
	}
	if queue, ok := h.queueManager.(*messagequeue.Service); ok {
		for _, sessionID := range []string{req.TargetSessionID, req.CallerSessionID} {
			identity, identityErr := queue.ResolveSessionIdentity(ctx, req.TaskID, sessionID)
			if identityErr == nil {
				h.publishQueueStatusEvent(ctx, identity, queue)
			}
		}
	}
	entries := messagequeue.RecoveryEntries(result.Entries)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id": req.TaskID, "source_session_id": req.TargetSessionID,
		"destination_session_id": req.CallerSessionID, "entries": entries,
		"readback_count": len(entries), "receipt": result.Receipt,
	})
}

func (h *Handlers) queueRecoveryError(msg *ws.Message, targetSessionID string, err error) (*ws.Message, error) {
	if errors.Is(err, messagequeue.ErrQueueRecoveryUnauthorized) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "queue recovery authorization changed", nil)
	}
	if errors.Is(err, messagequeue.ErrQueueRecoveryConflict) ||
		errors.Is(err, messagequeue.ErrQueueRecoveryTargetNotTerminal) ||
		errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	if h.logger != nil {
		h.logger.Error("session queue recovery failed", zap.String("target_session_id", targetSessionID), zap.Error(err))
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to recover session queue", nil)
}
