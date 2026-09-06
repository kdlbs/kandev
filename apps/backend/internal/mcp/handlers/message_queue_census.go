package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type messageQueueScopeRequest struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

type disposeMessageQueueRequest struct {
	TaskID    string                         `json:"task_id"`
	SessionID string                         `json:"session_id"`
	Entries   []messagequeue.QueueEntryClaim `json:"entries"`
}

func (h *Handlers) handleGetMessageQueueCensus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req messageQueueScopeRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if response, err := authorizeOwnMessageQueue(ctx, msg, req.TaskID, req.SessionID); response != nil {
		return response, err
	}
	if h.queueManager == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "message queue management is not available", nil)
	}
	census, err := h.queueManager.Census(ctx, req.SessionID)
	if err != nil {
		h.logger.Error("message queue census failed", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to read message queue census", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":      req.TaskID,
		"session_id":   req.SessionID,
		"entries":      census.Entries,
		"before_count": census.BeforeCount,
		"max":          census.Max,
		"auto_run":     census.AutoRun,
	})
}

func (h *Handlers) handleDisposeMessageQueueEntries(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req disposeMessageQueueRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if response, err := authorizeOwnMessageQueue(ctx, msg, req.TaskID, req.SessionID); response != nil {
		return response, err
	}
	if len(req.Entries) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entries must contain at least one census claim", nil)
	}
	if h.queueManager == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "message queue management is not available", nil)
	}
	result, err := h.queueManager.DisposeExact(ctx, req.SessionID, req.Entries)
	if err != nil {
		if errors.Is(err, messagequeue.ErrInvalidQueueDisposition) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		h.logger.Error("exact message queue disposition failed", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to dispose message queue entries", nil)
	}
	if queue, ok := h.queueManager.(*messagequeue.Service); ok {
		h.publishQueueStatusEvent(ctx, req.SessionID, queue)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":      req.TaskID,
		"session_id":   req.SessionID,
		"before_count": result.BeforeCount,
		"after_count":  result.AfterCount,
		"outcomes":     result.Outcomes,
	})
}

func authorizeOwnMessageQueue(
	ctx context.Context,
	msg *ws.Message,
	taskID string,
	sessionID string,
) (*ws.Message, error) {
	taskID = strings.TrimSpace(taskID)
	sessionID = strings.TrimSpace(sessionID)
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.WorkspaceID == "" || taskID == "" || sessionID == "" ||
		principal.CallerTaskID != taskID || principal.CallerSessionID != sessionID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"message queue access is limited to the calling task's current session", nil)
	}
	return nil, nil
}
