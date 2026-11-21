package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	maxRecordedActionLen = 128
	maxRecordedTargetLen = 64
)

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
	if tool, ok := coordinator.ToolForAction(action); ok {
		action = tool
	}
	if len(action) > maxRecordedActionLen {
		action = action[:maxRecordedActionLen]
	}
	target := ""
	if fields, err := automationPayloadFields(msg.Payload); err == nil {
		target = jsonStringField(fields, "task_id")
		if msg.Action == coordinator.ActionGetItem && jsonStringField(fields, "kind") == "stall" {
			target = jsonStringField(fields, "id")
		}
	}
	if len(target) > maxRecordedTargetLen || coordinator.ContainsCredential(target) {
		target = ""
	}
	h.coordinatorSvc.RecordCall(principal.CallerSessionID, action, target, allowed)
}

const proposeActionPrefix = "coordinator.propose_"

// withProposalNotes carries a coordinator proposal call's note_ids to the
// proposal insert, so every proposal kind validates and links them through the
// same path. Other calls pass through unchanged.
func (h *Handlers) withProposalNotes(ctx context.Context, msg *ws.Message) context.Context {
	if h.coordinatorSvc == nil || msg == nil || !strings.HasPrefix(msg.Action, proposeActionPrefix) {
		return ctx
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ctx
	}
	var payload struct {
		NoteIDs json.RawMessage `json:"note_ids"`
	}
	if json.Unmarshal(msg.Payload, &payload) != nil {
		return ctx
	}
	return h.coordinatorSvc.WithProposalNotes(ctx, principal.CoordinatorID, principal.CallerSessionID, payload.NoteIDs)
}

// proposalNotesErrorResponse maps a refused note_ids to a validation error
// carrying the reason and the offending entries.
func proposalNotesErrorResponse(msg *ws.Message, err error) (*ws.Message, bool) {
	var notesErr *coordinator.ProposalNotesError
	if !errors.As(err, &notesErr) {
		return nil, false
	}
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, notesErr.Error(), map[string]interface{}{
		refusalCodeKey:       notesErr.Reason,
		fieldErrorDetailsKey: "note_ids",
		"notes":              notesErr.Offences,
	})
	return response, true
}
