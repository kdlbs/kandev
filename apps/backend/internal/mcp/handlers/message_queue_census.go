package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
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

type messageQueueTaskAuthorizer interface {
	AuthorizeTaskAccess(context.Context, string) error
	AuthorizeSessionAccess(context.Context, string) error
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

func (h *Handlers) handleGetMessageQueueCensus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req messageQueueScopeRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	var authorizer messageQueueTaskAuthorizer
	if h.taskSvc != nil {
		authorizer = h.taskSvc
	}
	identity, response, err := authorizeOwnMessageQueue(ctx, msg, req.TaskID, req.SessionID, authorizer)
	if response != nil {
		return response, err
	}
	if h.queueManager == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "message queue management is not available", nil)
	}
	census, err := h.queueManager.CensusForSession(ctx, identity)
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
	var authorizer messageQueueTaskAuthorizer
	if h.taskSvc != nil {
		authorizer = h.taskSvc
	}
	identity, response, err := authorizeOwnMessageQueue(ctx, msg, req.TaskID, req.SessionID, authorizer)
	if response != nil {
		return response, err
	}
	if len(req.Entries) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entries must contain at least one census claim", nil)
	}
	if h.queueManager == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "message queue management is not available", nil)
	}
	result, err := h.queueManager.DisposeExactForSession(ctx, identity, req.Entries)
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
	authorizer messageQueueTaskAuthorizer,
) (messagequeue.QueueSessionIdentity, *ws.Message, error) {
	taskID = strings.TrimSpace(taskID)
	sessionID = strings.TrimSpace(sessionID)
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.WorkspaceID == "" || taskID == "" || sessionID == "" ||
		principal.CallerTaskID != taskID || principal.CallerSessionID != sessionID {
		return queueAccessForbidden(msg)
	}
	if authorizer == nil {
		// Lightweight in-memory handler fixtures have no task-session store.
		// Production wiring always supplies taskSvc, which supplies the durable
		// incarnation used by the repository transaction.
		return messagequeue.QueueSessionIdentity{TaskID: taskID, SessionID: sessionID, SessionIncarnationID: "in-memory"}, nil, nil
	}
	if err := authorizer.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return queueAccessForbidden(msg)
	}
	if err := authorizer.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return queueAccessForbidden(msg)
	}
	session, err := authorizer.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID || session.AgentExecutionID == "" {
		return queueAccessForbidden(msg)
	}
	return messagequeue.QueueSessionIdentity{TaskID: taskID, SessionID: sessionID, SessionIncarnationID: session.AgentExecutionID}, nil, nil
}

func queueAccessForbidden(msg *ws.Message) (messagequeue.QueueSessionIdentity, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
		"message queue access is limited to the calling task's current session", nil)
	return messagequeue.QueueSessionIdentity{}, response, err
}
