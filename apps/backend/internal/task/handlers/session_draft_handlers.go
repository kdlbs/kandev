package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// wsSaveSessionDraft saves the chat input draft for a session.
func (h *TaskHandlers) wsSaveSessionDraft(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string          `json:"session_id"`
		Text      string          `json:"text"`
		Content   json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	// Build JSON blob from text + content fields
	var draftContent string
	if req.Text != "" || len(req.Content) > 0 {
		blob, err := json.Marshal(map[string]interface{}{
			"text":    req.Text,
			"content": req.Content,
		})
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to serialize draft", nil)
		}
		draftContent = string(blob)
	}

	if err := h.service.SaveSessionDraft(ctx, req.SessionID, draftContent); err != nil {
		if err == service.ErrSessionIDRequired {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to save draft", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

// wsGetSessionDraft retrieves the chat input draft for a session.
func (h *TaskHandlers) wsGetSessionDraft(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	content, err := h.service.GetSessionDraft(ctx, req.SessionID)
	if err != nil {
		if err == service.ErrSessionIDRequired {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get draft", nil)
	}

	if content == "" {
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}

	// Parse stored JSON blob and return fields separately
	var draft map[string]interface{}
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		// Corrupt data — return empty
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, draft)
}
