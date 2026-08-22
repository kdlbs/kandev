package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/adminmetrics"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type settleStaleSessionRequest struct {
	SessionID       string `json:"session_id"`
	TurnID          string `json:"turn_id"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

// handleSettleStaleSession provides the deliberately narrow alternative to
// stop_task_kandev. The caller identity comes only from the task-mode MCP
// server, never the model-supplied request body.
func (h *Handlers) handleSettleStaleSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, failure := parseSettleStaleSessionRequest(msg)
	if failure != nil {
		return failure, nil
	}
	actor, actorSession, target, targetSession, failure := h.resolveStaleSettlementScope(ctx, msg, req)
	if failure != nil {
		return failure, nil
	}
	basis, allowed := staleSettlementAuthority(actor, actorSession, target, targetSession)
	if !allowed {
		adminmetrics.RecordStaleControlDenial("relation_denied")
		h.logStaleSessionDenied(req, "relation_denied")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "caller is not authorized to settle this session", nil)
	}
	if h.staleSessionSettler == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "stale session settlement is not configured", nil)
	}
	result, err := h.staleSessionSettler.SettleStaleSession(ctx, orchestrator.StaleSessionSettlementRequest{
		ActorTaskID: actor.ID, ActorSessionID: actorSession.ID, TargetTaskID: target.ID,
		TargetSessionID: targetSession.ID, TargetTurnID: req.TurnID, AuthorityBasis: basis,
	})
	if errors.Is(err, orchestrator.ErrStaleSessionNotStale) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "active_turn/not_stale", nil)
	}
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to settle stale session", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": targetSession.ID, "turn_id": req.TurnID, "status": result.Status,
	})
}

func parseSettleStaleSessionRequest(msg *ws.Message) (settleStaleSessionRequest, *ws.Message) {
	var req settleStaleSessionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return req, staleSessionError(msg, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error())
	}
	for _, field := range []*string{&req.SessionID, &req.TurnID, &req.SenderTaskID, &req.SenderSessionID} {
		*field = strings.TrimSpace(*field)
	}
	if req.SessionID == "" || req.TurnID == "" {
		return req, staleSessionError(msg, ws.ErrorCodeValidation, "session_id and turn_id are required")
	}
	if req.SenderTaskID == "" || req.SenderSessionID == "" {
		return req, staleSessionError(msg, ws.ErrorCodeValidation, "trusted sender task and session identity are required")
	}
	return req, nil
}

func (h *Handlers) resolveStaleSettlementScope(ctx context.Context, msg *ws.Message, req settleStaleSessionRequest) (*models.Task, *models.TaskSession, *models.Task, *models.TaskSession, *ws.Message) {
	if h.taskSvc == nil || h.stopTaskGetter == nil {
		return nil, nil, nil, nil, staleSessionError(msg, ws.ErrorCodeInternalError, "task and session lookup are not configured")
	}
	actor, actorErr := h.lookupStopTask(ctx, msg, req.SenderTaskID, "sender")
	if actorErr != nil {
		return nil, nil, nil, nil, actorErr.response
	}
	targetSession, err := h.taskSvc.GetTaskSession(ctx, req.SessionID)
	if err != nil || targetSession == nil {
		return nil, nil, nil, nil, staleSessionError(msg, ws.ErrorCodeNotFound, "target session not found")
	}
	target, targetErr := h.lookupStopTask(ctx, msg, targetSession.TaskID, "target")
	if targetErr != nil {
		return nil, nil, nil, nil, targetErr.response
	}
	actorSession, err := h.taskSvc.GetTaskSession(ctx, req.SenderSessionID)
	if err != nil || actorSession == nil || actorSession.TaskID != actor.ID {
		h.logStaleSessionDenied(req, "sender_session_mismatch")
		return nil, nil, nil, nil, staleSessionError(msg, ws.ErrorCodeForbidden, "caller session does not belong to caller task")
	}
	return actor, actorSession, target, targetSession, nil
}

func staleSessionError(msg *ws.Message, code, message string) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return response
}

func staleSettlementAuthority(actor *models.Task, actorSession *models.TaskSession, target *models.Task, targetSession *models.TaskSession) (string, bool) {
	if actor == nil || actorSession == nil || target == nil || targetSession == nil ||
		actor.WorkspaceID == "" || target.WorkspaceID == "" || actor.WorkspaceID != target.WorkspaceID {
		return "", false
	}
	if actor.ID == targetSession.TaskID && actorSession.ID != targetSession.ID {
		return "same_task_peer", true
	}
	if target.ParentID == actor.ID {
		return "direct_parent", true
	}
	if supervision, ok := models.LoadSessionSpawnSupervision(targetSession.Metadata); ok && supervision.SupervisorTaskID == actor.ID {
		return "persisted_supervisor", true
	}
	return "", false
}

func (h *Handlers) logStaleSessionDenied(req settleStaleSessionRequest, code string) {
	h.logger.Warn("stale session settlement denied", zap.String("code", code), zap.String("sender_task_id", req.SenderTaskID), zap.String("sender_session_id", req.SenderSessionID), zap.String("target_session_id", req.SessionID), zap.String("target_turn_id", req.TurnID))
}
