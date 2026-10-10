package orchestrator

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

func interruptedInstructionMessageID(snapshotID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("kandev:interrupted-instruction:"+snapshotID)).String()
}

func (s *Service) reconcileInterruptedAcceptance(ctx context.Context, taskID, sessionID string, request InterruptedSessionResumeRequest, snapshot *models.ContinuationSnapshot, store interruptedContinuationStore, response *SessionDeliveryRecoveryResponse) *SessionDeliveryRecoveryResponse {
	// The canonical instruction is written only at agentctl acceptance. Backend
	// submission admission alone cannot prove that the receiver accepted it.
	reader, ok := s.repo.(interface {
		GetMessage(context.Context, string) (*models.Message, error)
	})
	if !ok {
		return response
	}
	message, err := reader.GetMessage(ctx, interruptedInstructionMessageID(snapshot.ID))
	if err != nil || message == nil || message.TaskID != taskID || message.TaskSessionID != sessionID || message.AuthorType != models.MessageAuthorUser || message.Content != request.Instruction || message.TurnID == "" {
		return response
	}
	now := time.Now().UTC()
	if err = store.CompleteRestoreAttempt(ctx, snapshot.ID, "interrupted_continued", now); err != nil {
		return response
	}
	if err = store.CompleteContinuationSnapshot(ctx, snapshot.ID, models.ContinuitySnapshotConsumed, now); err != nil {
		return response
	}
	response.Outcome = SessionDeliveryRecoveryContinued
	response.Reason = ""
	return response
}
