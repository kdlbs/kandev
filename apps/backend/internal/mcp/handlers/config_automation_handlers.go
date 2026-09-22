package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/automation"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// AutomationCreator preserves the automation service's validation and access checks.
type AutomationCreator interface {
	CreateAutomation(context.Context, *automation.CreateAutomationRequest) (*automation.Automation, error)
}

// SetAutomationCreator wires creation for configuration-chat MCP calls.
func (h *Handlers) SetAutomationCreator(creator AutomationCreator) {
	h.automationCreator = creator
}

func (h *Handlers) handleCreateAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.automationCreator == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Automation service is unavailable", nil)
	}
	var req automation.CreateAutomationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid automation payload", nil)
	}
	saved, err := h.automationCreator.CreateAutomation(ctx, &req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	if saved == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to load created automation", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, &automation.CreateAutomationResponse{
		Automation: saved, WebhookSecret: saved.WebhookSecret,
	})
}
