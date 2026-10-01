package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	coordinatorItemFieldKind    = "kind"
	coordinatorItemKindProposal = "proposal"
	coordinatorItemKindStall    = "stall"
)

// coordinatorStallItem is get_coordinator_item_kandev's stall-kind result
// shape (docs/specs/coordinator/system-design/copilot-tools.md#item-read).
// Unlike StallDTO (the needs-you stalls list route body, scoped to one
// workspace by its URL), this tool answers a coordinator's own-workspace
// reference and so includes workspace_id explicitly.
type coordinatorStallItem struct {
	TaskID       string    `json:"task_id"`
	WorkspaceID  string    `json:"workspace_id"`
	StalledForMs int64     `json:"stalled_for_ms"`
	LastEventAt  time.Time `json:"last_event_at"`
	DetectedAt   time.Time `json:"detected_at"`
}

// handleGetCoordinatorItem backs coordinator.get_item
// (docs/specs/coordinator/system-design/copilot-tools.md#item-read): the
// read behind an Ask about this bracketed reference. Read-only: it never
// writes, never runs proposal recovery and never starts or messages a
// session. The guard (authorizeCoordinatorRequest) already refuses a
// non-coordinator principal before this handler runs; the check here is
// defense in depth against a guard bypass, matching handleProposeTask.
func (h *Handlers) handleGetCoordinatorItem(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, err := automationPayloadFields(msg.Payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	kind := jsonStringField(fields, coordinatorItemFieldKind)
	if kind != coordinatorItemKindProposal && kind != coordinatorItemKindStall {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, `kind must be "proposal" or "stall"`, nil)
	}
	id := strings.TrimSpace(jsonStringField(fields, "id"))
	if id == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "id is required", nil)
	}

	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator principal is required", nil)
	}
	if h.coordinatorSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "coordinator service is not available", nil)
	}

	if kind == coordinatorItemKindProposal {
		return h.getCoordinatorProposalItem(ctx, msg, principal, id)
	}
	return h.getCoordinatorStallItem(ctx, msg, principal, id)
}

// getCoordinatorProposalItem answers {kind: "proposal", id}: a proposal of
// another coordinator, another workspace, or none at all answers the same
// NOT_FOUND "target not found" as a foreign id, so a caller cannot tell one
// from the other.
func (h *Handlers) getCoordinatorProposalItem(
	ctx context.Context, msg *ws.Message, principal mcpscope.Principal, id string,
) (*ws.Message, error) {
	proposal, err := h.coordinatorSvc.GetProposal(ctx, principal.WorkspaceID, principal.CoordinatorID, id)
	if errors.Is(err, coordinator.ErrNotFound) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	}
	if err != nil {
		h.logger.Warn("get_item failed to read proposal",
			zap.String(coordinatorItemFieldKind, coordinatorItemKindProposal), zap.String("id", id),
			zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to read item", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		coordinatorItemFieldKind: coordinatorItemKindProposal,
		"proposal":               coordinator.NewProposalDTO(proposal),
	})
}

// getCoordinatorStallItem answers {kind: "stall", id}: id is a task id, read
// through the task service first (a missing task or one outside the
// principal's workspace answers the same NOT_FOUND "target not found" as a
// foreign proposal id). Once the task is confirmed in-workspace, a task with
// no stall row answers a distinct NOT_FOUND, since the task itself is
// already known to be visible to the caller.
func (h *Handlers) getCoordinatorStallItem(
	ctx context.Context, msg *ws.Message, principal mcpscope.Principal, id string,
) (*ws.Message, error) {
	task, err := h.taskSvc.GetTask(ctx, id)
	switch {
	case errors.Is(err, repoerrors.ErrTaskNotFound), service.IsForbidden(err):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	case err != nil:
		h.logger.Warn("get_item failed to read task",
			zap.String(coordinatorItemFieldKind, coordinatorItemKindStall), zap.String("id", id),
			zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to read item", nil)
	case task == nil || task.WorkspaceID != principal.WorkspaceID:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	case h.coordinatorSvc.Phase2() && !h.coordinatorWatchesTask(ctx, principal, task.ID, task.WorkflowID):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	}
	stall, err := h.coordinatorSvc.GetStall(ctx, principal.WorkspaceID, id)
	if errors.Is(err, coordinator.ErrNotFound) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "no stall record for this task", nil)
	}
	if err != nil {
		h.logger.Warn("get_item failed to read stall",
			zap.String(coordinatorItemFieldKind, coordinatorItemKindStall), zap.String("id", id),
			zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to read item", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		coordinatorItemFieldKind: coordinatorItemKindStall,
		"stall": coordinatorStallItem{
			TaskID:       stall.TaskID,
			WorkspaceID:  stall.WorkspaceID,
			StalledForMs: stall.StalledForMs,
			LastEventAt:  stall.LastEventAt,
			DetectedAt:   stall.DetectedAt,
		},
	})
}
