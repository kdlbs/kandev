package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *QueueHandlers) wsSetAutoSortMerge(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSetAutoMergeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" || req.SessionIncarnationID == "" || req.Enabled == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, session_incarnation_id, and enabled are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	controller, ok := h.queueService.(QueueAutoSortMergeController)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue Auto-sort-merge is unavailable", nil)
	}
	identity := messagequeue.QueueSessionIdentity{TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID}
	result, err := controller.SetSessionAutoSortMerge(ctx, identity, *req.Enabled)
	if err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) || errors.Is(err, messagequeue.ErrTaskInactive) {
			return queueAccessDeniedResponse(msg), nil
		}
		if errors.Is(err, messagequeue.ErrSessionAdmissionTimeout) {
			return queueAdmissionErrorResponse(msg)
		}
		h.logger.Error("failed to set queue Auto-sort-merge", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set queue Auto-sort-merge", nil)
	}
	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		fieldTaskID: identity.TaskID, fieldSessionID: identity.SessionID, fieldSessionIncarnation: identity.SessionIncarnationID,
		"auto_sort_merge_available": true, "auto_sort_merge_enabled": result.AutoSortMerge.Enabled,
		"auto_sort_merge_source": result.AutoSortMerge.Source, "auto_sort_merge_revision": result.AutoSortMerge.Revision,
		"compacted_count": result.CompactedCount,
	})
}
