package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const coordinatorToolActivityDefaultLimit = 20

// handleListCoordinatorActivity backs coordinator.list_activity
// (docs/specs/coordinator/system-design/activity-log.md#read-tool). The
// coordinator is resolved from the principal alone, so no payload field can
// name another coordinator's rows. Read-only.
func (h *Handlers) handleListCoordinatorActivity(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, err := automationPayloadFields(msg.Payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.CoordinatorID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "coordinator not found", nil)
	}
	if h.coordinatorSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "coordinator service is not available", nil)
	}
	limit, err := coordinatorToolLimit(fields)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	before, err := coordinatorToolBefore(fields)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), map[string]interface{}{fieldErrorDetailsKey: "before"})
	}
	page, err := h.coordinatorSvc.ListActivityForCoordinator(ctx, principal.CoordinatorID, coordinator.ListActivityParams{
		Limit:  limit,
		Before: before,
	})
	var fieldErr *coordinator.FieldError
	switch {
	case errors.As(err, &fieldErr):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, fieldErr.Message, map[string]interface{}{fieldErrorDetailsKey: fieldErr.Field})
	case errors.Is(err, coordinator.ErrNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "coordinator not found", nil)
	case err != nil:
		h.logger.Warn("list_activity failed", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list activity", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, page.ForTool())
}

// coordinatorToolLimit reads the optional limit: absent or null is the tool
// default, a non-integer is refused naming the field, and the range is the
// service's.
func coordinatorToolLimit(fields map[string]json.RawMessage) (int, error) {
	raw, present := fields["limit"]
	if !present || string(raw) == jsonNull {
		return coordinatorToolActivityDefaultLimit, nil
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil || n != math.Trunc(n) {
		return 0, &coordinator.FieldError{Field: "limit", Message: "limit must be an integer"}
	}
	if n == 0 {
		return 0, &coordinator.FieldError{Field: "limit", Message: "limit must be between 1 and 50"}
	}
	return int(n), nil
}

// coordinatorToolBefore reads the optional cursor, refusing a non-string value
// instead of treating it as the first page.
func coordinatorToolBefore(fields map[string]json.RawMessage) (string, error) {
	raw, present := fields["before"]
	if !present || string(raw) == jsonNull {
		return "", nil
	}
	var before string
	if err := json.Unmarshal(raw, &before); err != nil {
		return "", &coordinator.FieldError{Field: "before", Message: "before must be a string"}
	}
	return before, nil
}
