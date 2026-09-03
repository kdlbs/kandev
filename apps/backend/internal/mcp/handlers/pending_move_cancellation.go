package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	PendingMoveInvalidArgumentCode    = "pending_move_invalid_argument"
	PendingMoveNotFoundOrChangedCode  = "pending_move_not_found_or_changed"
	PendingMoveCancelFailedCode       = "pending_move_cancel_failed"
	pendingMoveInvalidArgumentMessage = "Pending move cancellation requires all canonical identifiers."
	pendingMoveNotFoundMessage        = "Pending move was not found or no longer matches the requested state."
	pendingMoveCancelFailedMessage    = "Pending move cancellation failed."
)

type cancelPendingMoveRequest struct {
	PendingMoveID                 string `json:"pending_move_id"`
	SessionID                     string `json:"session_id"`
	TaskID                        string `json:"task_id"`
	MoveID                        string `json:"move_id"`
	WorkflowID                    string `json:"workflow_id"`
	ExpectedCurrentWorkflowStepID string `json:"expected_current_workflow_step_id"`
	ExpectedTargetWorkflowStepID  string `json:"expected_target_workflow_step_id"`
}

func (h *Handlers) handleCancelPendingMove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	actor := pendingMoveCancellationActor(ctx)
	if h.pendingMoveCanceller == nil {
		return ws.NewError(msg.ID, msg.Action, PendingMoveCancelFailedCode, pendingMoveCancelFailedMessage, nil)
	}
	var req cancelPendingMoveRequest
	if err := decodePendingMoveRequest(msg.Payload, &req); err != nil {
		present, canonical := pendingMoveRequestShape(msg.Payload)
		auditErr := h.pendingMoveCanceller.AuditInvalidPendingMoveCancellation(ctx, actor, msg.ID, present, canonical)
		if auditErr != nil {
			return pendingMoveCancellationError(msg, auditErr)
		}
		return ws.NewError(msg.ID, msg.Action, PendingMoveInvalidArgumentCode, pendingMoveInvalidArgumentMessage, nil)
	}
	result, err := h.pendingMoveCanceller.ExactCancelPendingMove(ctx, actor, messagequeue.ExactPendingMoveMatch{
		PendingMoveID:                 req.PendingMoveID,
		SessionID:                     req.SessionID,
		TaskID:                        req.TaskID,
		MoveID:                        req.MoveID,
		WorkflowID:                    req.WorkflowID,
		ExpectedCurrentWorkflowStepID: req.ExpectedCurrentWorkflowStepID,
		ExpectedTargetWorkflowStepID:  req.ExpectedTargetWorkflowStepID,
	}, msg.ID)
	if err != nil {
		return pendingMoveCancellationError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func pendingMoveRequestShape(payload []byte) (bool, bool) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&fields); err != nil {
		return false, false
	}
	keys := []string{"pending_move_id", "session_id", "task_id", "move_id", "workflow_id", "expected_current_workflow_step_id", "expected_target_workflow_step_id"}
	for _, key := range keys {
		value, ok := fields[key]
		if !ok {
			return false, false
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil || text == "" {
			return true, false
		}
		parsed, err := uuid.Parse(text)
		if err != nil || parsed.String() != text {
			return true, false
		}
	}
	return true, true
}

func decodePendingMoveRequest(payload json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("pending move payload must contain exactly one JSON document")
	}
	return nil
}

func pendingMoveCancellationActor(ctx context.Context) messagequeue.PendingMoveCancellationActor {
	actor := messagequeue.PendingMoveCancellationActor{Kind: "unknown"}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok {
		actor.Kind = agentAuthorType
		if principal.IsAutomation() {
			actor.Kind = "coordinator"
		}
		actor.ID = principal.CallerTaskID
		actor.WorkspaceID = principal.WorkspaceID
		actor.CallerTaskID = principal.CallerTaskID
		actor.CallerSessionID = principal.CallerSessionID
		actor.CallerExecutionID = principal.CallerExecutionID
	}
	if identity, ok := authn.IdentityFromContext(ctx); ok && !identity.Synthetic {
		actor.UserID = identity.UserID
	}
	return actor
}

func pendingMoveCancellationError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, messagequeue.ErrPendingMoveInvalidArgument):
		return ws.NewError(msg.ID, msg.Action, PendingMoveInvalidArgumentCode, pendingMoveInvalidArgumentMessage, nil)
	case errors.Is(err, messagequeue.ErrPendingMoveNotFoundOrChanged):
		return ws.NewError(msg.ID, msg.Action, PendingMoveNotFoundOrChangedCode, pendingMoveNotFoundMessage, nil)
	default:
		return ws.NewError(msg.ID, msg.Action, PendingMoveCancelFailedCode, pendingMoveCancelFailedMessage, nil)
	}
}
