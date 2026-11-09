package handlers

import (
	"context"
	"github.com/kandev/kandev/internal/authz"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type queueActionAuthorizer interface {
	AuthorizeQueueSessionIdentity(context.Context, string, string, string) error
	AuthorizeQueueSessionScope(context.Context, string, authz.Scope) error
}

func (h *QueueHandlers) authorizeQueueScope(ctx context.Context, msg *ws.Message, sessionID string, scope authz.Scope) *ws.Message {
	authorizer, ok := h.accessAuthorizer.(queueActionAuthorizer)
	if !ok {
		return nil
	}
	if err := authorizer.AuthorizeQueueSessionScope(ctx, sessionID, scope); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

func (h *QueueHandlers) authorizeQueueAction(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	scope := authz.ScopeSessionControl
	switch msg.Action {
	case ws.ActionMessageQueueGet:
		scope = authz.ScopeWorkspaceRead
	case ws.ActionMessageQueueAdd, ws.ActionMessageQueueAppend, ws.ActionMessageQueueUpdate,
		ws.ActionMessageQueueEditBegin, ws.ActionMessageQueueEditRenew, ws.ActionMessageQueueEditEnd,
		ws.ActionMessageQueueDrain:
		scope = authz.ScopeSessionPrompt
	}
	if denied := h.authorizeQueueScope(ctx, msg, sessionID, scope); denied != nil {
		return denied
	}
	if msg.Action == ws.ActionMessageQueueSendNow {
		return h.authorizeQueueScope(ctx, msg, sessionID, authz.ScopeSessionPrompt)
	}
	return nil
}
