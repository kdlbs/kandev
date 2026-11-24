package client

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kandev/kandev/internal/clarification/protocol"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type nativeQuestionDispatch struct {
	active sync.Map // pending ID to context.CancelCauseFunc
}

// Register the waiter before dispatching it. A cancellation on the same
// stream can then fence it even if its handler has yet to create the bundle.
func (d *nativeQuestionDispatch) start(ctx context.Context, msg *ws.Message, dispatch func(context.Context)) {
	if msg.Action != ws.ActionMCPAskUserQuestion && msg.Action != ws.ActionMCPCancelNativeClarification {
		go dispatch(ctx)
		return
	}
	var scope protocol.NativeQuestionScope
	if !mcporigin.MessageCarriesTrustedInternalCall(msg) || json.Unmarshal(msg.Payload, &scope) != nil || scope.PendingID() == "" {
		go dispatch(ctx)
		return
	}
	id := scope.PendingID()
	if msg.Action == ws.ActionMCPCancelNativeClarification {
		if cancel, ok := d.active.Load(id); ok {
			cancel.(context.CancelCauseFunc)(protocol.ErrNativeQuestionCancelled)
		}
		go dispatch(ctx)
		return
	}
	ctx, cancel := context.WithCancelCause(ctx)
	_, alreadyWaiting := d.active.LoadOrStore(id, cancel)
	go func() {
		defer cancel(nil)
		if !alreadyWaiting {
			defer d.active.Delete(id)
		}
		dispatch(ctx)
	}()
}
