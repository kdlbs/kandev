package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type FreshCIRunService interface {
	RequestFreshCIRun(context.Context, github.RequestFreshCIRunInput) (*github.CIRunReceipt, error)
}

func (h *Handlers) SetFreshCIRunService(service FreshCIRunService) {
	h.freshCIRuns = service
}

func (h *Handlers) handleRequestFreshCIRun(
	ctx context.Context,
	msg *ws.Message,
) (*ws.Message, error) {
	var request github.RequestFreshCIRunInput
	decoder := json.NewDecoder(bytes.NewReader(msg.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	if request.ActorTaskID == "" || request.ActorSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"trusted caller task and session identity are required", nil)
	}
	if h.freshCIRuns == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"fresh CI run service is not available", nil)
	}
	receipt, err := h.freshCIRuns.RequestFreshCIRun(ctx, request)
	if err != nil {
		var requestErr *github.CIRunRequestError
		if errors.As(err, &requestErr) {
			return ws.NewError(msg.ID, msg.Action, ciRunErrorCode(requestErr.Class), err.Error(), map[string]any{
				"failure_class": requestErr.Class,
			})
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"fresh CI run request failed", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, receipt)
}

func ciRunErrorCode(class github.CIRunFailureClass) string {
	switch class {
	case github.CIRunFailureNotAuthorized, github.CIRunFailureCrossWorkspace,
		github.CIRunFailureInstallationPermission:
		return ws.ErrorCodeForbidden
	case github.CIRunFailureHeadDrift, github.CIRunFailureSourceRunMismatch,
		github.CIRunFailureWorkflowStepMismatch, github.CIRunFailureIdempotencyConflict,
		github.CIRunFailureDispatchDenied, github.CIRunFailureMergeEvidenceUnavailable:
		return ws.ErrorCodeConflict
	default:
		return ws.ErrorCodeInternalError
	}
}
