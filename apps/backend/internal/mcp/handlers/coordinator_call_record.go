package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const maxRecordedActionLen = 128

// withCoordinatorCaller carries a coordinator session through the guarded call
// so the writes it makes can name the ledger turn.
func (h *Handlers) withCoordinatorCaller(ctx context.Context) context.Context {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ctx
	}
	return coordinator.WithCallerSession(ctx, principal.CallerSessionID)
}

// recordCoordinatorCall hands one allow or refuse decision to the turn ledger.
// It never blocks, fails or alters the call.
func (h *Handlers) recordCoordinatorCall(ctx context.Context, msg *ws.Message, allowed bool) {
	if h.coordinatorSvc == nil || msg == nil {
		return
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() || principal.CallerSessionID == "" {
		return
	}
	action := msg.Action
	if len(action) > maxRecordedActionLen {
		action = action[:maxRecordedActionLen]
	}
	target := ""
	if fields, err := automationPayloadFields(msg.Payload); err == nil {
		target = jsonStringField(fields, "task_id")
	}
	h.coordinatorSvc.RecordCall(principal.CallerSessionID, action, target, allowed)
}
