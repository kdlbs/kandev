package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/clarification/protocol"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (req askUserQuestionRequest) nativeScope() protocol.NativeQuestionScope {
	return protocol.NativeQuestionScope{SessionID: req.SessionID, NativeRequestID: req.NativeRequestID}
}

func ownsNativeQuestion(ctx context.Context, scope protocol.NativeQuestionScope) bool {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	return ok && execution.SessionID == scope.SessionID && scope.PendingID() != "" && mcporigin.IsTrustedInternalCall(ctx)
}

func (h *Handlers) handleCancelNativeClarification(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var scope protocol.NativeQuestionScope
	if json.Unmarshal(msg.Payload, &scope) != nil || !ownsNativeQuestion(ctx, scope) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "native clarification scope is invalid", nil)
	}
	if err := h.expireNativeQuestion(ctx, scope.SessionID, scope.PendingID()); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to cancel native clarification", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"ok": true})
}

func (h *Handlers) expireNativeQuestion(ctx context.Context, sessionID, pendingID string) error {
	canceller, ok := h.sessionCanceller.(interface {
		ExpireRequestAndNotify(context.Context, string, string) error
	})
	var err error
	if ok {
		err = canceller.ExpireRequestAndNotify(ctx, sessionID, pendingID)
	} else {
		err = errors.New("native clarification expiry is unavailable")
	}
	if err != nil {
		h.logger.Warn("failed to expire native clarification", zap.String("session_id", sessionID), zap.Error(err))
	}
	return err
}
