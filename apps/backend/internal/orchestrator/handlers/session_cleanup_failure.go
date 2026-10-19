package handlers

import (
	"errors"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func runtimeCleanupFailureResponse(msg *ws.Message, err error, sessionID string) (*ws.Message, error) {
	var failure *executor.ResumeCleanupFailure
	if !errors.As(err, &failure) {
		return nil, nil
	}
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationResume)
	if !ok {
		return nil, nil
	}
	details := map[string]interface{}{
		"kind":       models.AgentErrorCauseCodeRuntimeCleanupFailed,
		"session_id": sessionID,
		"causes":     []models.AgentErrorCause{cause},
	}
	if identity, ok := orchestrator.RecoveryFailureIdentity(err); ok {
		details["attempt_id"] = identity.AttemptID
		details["error_stamp"] = identity.ErrorStamp
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, failure.SafeDetail(), details)
}
