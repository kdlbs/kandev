package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleLaunchSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID            string `json:"task_id"`
		AgentProfileID    string `json:"agent_profile_id"`
		ExecutorID        string `json:"executor_id"`
		ExecutorProfileID string `json:"executor_profile_id"`
		Prompt            string `json:"prompt"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if h.sessionLauncher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "orchestrator not available", nil)
	}

	resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:            req.TaskID,
		AgentProfileID:    req.AgentProfileID,
		ExecutorID:        req.ExecutorID,
		ExecutorProfileID: req.ExecutorProfileID,
		Prompt:            req.Prompt,
	})
	if err != nil {
		h.logger.Error("failed to launch session", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to launch session", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleStopSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
		Reason    string `json:"reason"`
		Force     bool   `json:"force"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if h.sessionLauncher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "orchestrator not available", nil)
	}

	reason := req.Reason
	if reason == "" {
		reason = "stopped via MCP"
	}
	if err := h.sessionLauncher.StopSession(ctx, req.SessionID, reason, req.Force); err != nil {
		h.logger.Error("failed to stop session", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to stop session", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleGetTaskSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "task_id", "failed to list task sessions", "Failed to list task sessions",
		func(ctx context.Context, taskID string) (any, error) {
			sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.TaskSessionDTO, 0, len(sessions))
			for _, s := range sessions {
				dtos = append(dtos, dto.FromTaskSession(s))
			}
			return map[string]interface{}{
				"sessions": dtos,
				"total":    len(dtos),
			}, nil
		})
}
