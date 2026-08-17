package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) handleListSessionWakes(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	wakes, err := h.taskSvc.ListSessionWakes(ctx, req.TaskID, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"wakes": wakes})
}

func (h *Handlers) handleUpsertSessionWake(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		SessionID      string `json:"session_id"`
		Marker         string `json:"marker"`
		Prompt         string `json:"prompt"`
		CronExpression string `json:"cron_expression"`
		Timezone       string `json:"timezone"`
		ExpiresAt      string `json:"expires_at"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "expires_at must be RFC3339", nil)
	}
	wake, action, err := h.taskSvc.UpsertSessionWake(ctx, req.TaskID, req.SessionID, service.UpsertSessionWakeRequest{Marker: req.Marker, Prompt: req.Prompt, CronExpression: req.CronExpression, Timezone: req.Timezone, ExpiresAt: expiresAt})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"action": action, "wake": wake})
}

func (h *Handlers) handleDeleteSessionWake(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Marker    string `json:"marker"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	deleted, err := h.taskSvc.DeleteSessionWake(ctx, req.TaskID, req.SessionID, req.Marker)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"deleted": deleted})
}
