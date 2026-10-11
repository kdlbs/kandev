package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type nativeResumeDeliveryAcknowledger interface {
	AcknowledgeNativeResumeDelivery(context.Context, string) error
}

type nativeResumeDeliverySubmissionReader interface {
	ListAgentDeliverySubmissions(context.Context, string) ([]*models.AgentDeliverySubmission, error)
}

func (s *Service) checkNativeResumeDeliveryBlock(ctx context.Context, taskID, sessionID string) error {
	block, err := s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	if err != nil || block == nil || block.Reason != durableDeliveryUnresolvedReason {
		return err
	}
	// Office recovery releases scheduler admission rather than resuming a
	// native conversation here, so its existing durable-work gate remains.
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return err
	}
	if isOfficeTask {
		return &sessionRecoveryRequiredError{Block: block}
	}
	_, canAcknowledge := s.agentManager.(nativeResumeDeliveryAcknowledger)
	reader, canRead := s.repo.(nativeResumeDeliverySubmissionReader)
	if !canAcknowledge || !canRead {
		return &sessionRecoveryRequiredError{Block: block}
	}
	submissions, err := reader.ListAgentDeliverySubmissions(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("read native resume delivery evidence: %w", err)
	}
	unknown := false
	for _, submission := range submissions {
		if !nativeResumeSubmissionMatchesBlock(submission, block) {
			continue
		}
		switch submission.State {
		case models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching:
			return &sessionRecoveryRequiredError{Block: block}
		case models.DeliverySubmissionInterruptedUnknown:
			unknown = true
		}
	}
	if !unknown {
		return &sessionRecoveryRequiredError{Block: block}
	}
	return nil
}

func (s *Service) acknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	manager, supported := s.agentManager.(nativeResumeDeliveryAcknowledger)
	if !supported {
		return nil
	}
	if err := manager.AcknowledgeNativeResumeDelivery(ctx, sessionID); err != nil {
		return fmt.Errorf("acknowledge resumed conversation delivery: %w", err)
	}
	return nil
}

func nativeResumeSubmissionMatchesBlock(submission *models.AgentDeliverySubmission, block *models.SessionRecoveryBlock) bool {
	return submission != nil && submission.IncarnationID == block.IncarnationID && submission.HarnessGeneration == block.ExpectedGeneration
}
