package orchestrator

import (
	"context"
	"fmt"
	"time"

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
		case models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted:
			return &sessionRecoveryRequiredError{Block: block}
		case models.DeliverySubmissionDispatching:
			if block.ConsumerReference == agentDeliveryConsumer && block.DeliverySubmissionID == "" && block.DeliveryStreamID == "" {
				unknown = true
				continue
			}
			return &sessionRecoveryRequiredError{Block: block}
		case models.DeliverySubmissionInterruptedUnknown:
			unknown = true
		}
	}
	if !unknown && !s.canProveSettledNativeDeliveryBlock(block) {
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

type nativeResumeDeliveryEvidenceReader interface {
	GetNativeResumeDeliveryEvidence(context.Context, string) (*models.NativeResumeDeliveryEvidence, error)
}

type nativeResumeEvidenceBlockResolver interface {
	ResolveSessionRecoveryBlockWithNativeEvidence(context.Context, string, *models.NativeResumeDeliveryEvidence, time.Time) (bool, error)
}

func (s *Service) resolveNativeDeliveryBlock(ctx context.Context, block *models.SessionRecoveryBlock, completedAt time.Time) (bool, bool, error) {
	reader, canRead := s.agentManager.(nativeResumeDeliveryEvidenceReader)
	resolver, canResolve := s.repo.(nativeResumeEvidenceBlockResolver)
	if !canRead || !canResolve || block.ConsumerReference != agentDeliveryConsumer {
		return false, false, nil
	}
	proof, err := reader.GetNativeResumeDeliveryEvidence(ctx, block.SessionID)
	if err != nil {
		return false, true, err
	}
	if proof == nil {
		return false, false, nil
	}
	changed, err := resolver.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, block.ID, proof, completedAt)
	return changed, true, err
}

type loadedNativeDeliveryReconciler interface {
	ReconcileLoadedNativeDelivery(context.Context, string) (*models.NativeResumeDeliveryEvidence, error)
}

func (s *Service) finishLoadedNativeDeliveryBlock(ctx context.Context, block *models.SessionRecoveryBlock) (bool, error) {
	manager, supported := s.agentManager.(loadedNativeDeliveryReconciler)
	resolver, canResolve := s.repo.(nativeResumeEvidenceBlockResolver)
	if !supported || !canResolve || block == nil || block.ConsumerReference != agentDeliveryConsumer {
		return false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, block.SessionID)
	if err != nil || session == nil {
		return false, err
	}
	if s.automationOwnsRetrySession(ctx, session.TaskID, session.ID) {
		return false, nil
	}
	office, err := s.lookupOfficeTask(ctx, session.TaskID)
	if err != nil || office {
		return false, err
	}
	proof, err := manager.ReconcileLoadedNativeDelivery(ctx, block.SessionID)
	if err != nil || proof == nil {
		return false, err
	}
	changed, err := resolver.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, block.ID, proof, time.Now().UTC())
	if err == nil && changed {
		s.publishAgentDeliveryRecoveryState(ctx, block.SessionID)
	}
	return changed, err
}

// Empty delivery fences can outlive fully settled SQL history. Admission still
// waits for authenticated native evidence and the transactional inventory check.
func (s *Service) canProveSettledNativeDeliveryBlock(block *models.SessionRecoveryBlock) bool {
	_, canProve := s.agentManager.(nativeResumeDeliveryEvidenceReader)
	_, canResolve := s.repo.(nativeResumeEvidenceBlockResolver)
	return canProve && canResolve && block.ConsumerReference == agentDeliveryConsumer &&
		block.DeliverySubmissionID == "" && block.DeliveryStreamID == ""
}
