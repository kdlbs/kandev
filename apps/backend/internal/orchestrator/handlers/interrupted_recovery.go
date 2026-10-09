package handlers

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) wsResumeInterrupted(ctx context.Context, msg *ws.Message, request wsRecoverSessionRequest) (*ws.Message, error) {
	if request.InterruptedResume == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "interrupted_resume is required", nil)
	}
	if request.Action != "resume_interrupted" && request.Action != "resume" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "interrupted recovery requires the resume action", nil)
	}
	if request.SettingsPolicy != executor.ResumeSettingsPolicyStrict {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "interrupted recovery requires strict settings policy", nil)
	}
	recovery := request.InterruptedResume
	if !validInterruptedResumeRequest(recovery) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "acknowledgment, instruction, identity, revision, and idempotency key are required", nil)
	}

	response, err := h.service.ResumeInterruptedSession(ctx, request.TaskID, request.SessionID, *recovery)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Session continuation unavailable", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func (h *Handlers) wsResumeInterruptedBatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request struct {
		Items []orchestrator.InterruptedSessionBatchItem `json:"items"`
	}
	if err := msg.ParsePayload(&request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid recovery batch payload", nil)
	}
	if len(request.Items) < 1 || len(request.Items) > orchestrator.MaxInterruptedRecoveryBatch {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid recovery batch size", nil)
	}
	response, err := h.service.ResumeInterruptedSessions(ctx, request.Items)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid recovery batch selection", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func validInterruptedResumeRequest(recovery *orchestrator.InterruptedSessionResumeRequest) bool {
	identity := recovery.RecoveryIdentity
	return recovery.Acknowledge && strings.TrimSpace(recovery.Instruction) != "" && len(recovery.Instruction) <= 32<<10 && strings.TrimSpace(recovery.IdempotencyKey) != "" && len(recovery.IdempotencyKey) <= 128 && recovery.RecoveryRevision >= 1 && identity.SubmissionID != "" && identity.StreamID != "" && identity.IncarnationID != "" && identity.HarnessGeneration >= 1 && identity.PromptGeneration > 0
}
