package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) reconcileAgentDeliverySettlements(ctx context.Context, sessionID string) error {
	settlements, ok := s.repo.(repository.AgentDeliverySettlementRepository)
	if !ok {
		return nil
	}
	effects, err := settlements.ListPendingAgentDeliverySettlements(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list pending delivery terminal settlements: %w", err)
	}
	submissions, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok && len(effects) > 0 {
		return errors.New("agent delivery submission repository is unavailable")
	}
	for _, effect := range effects {
		if effect != nil {
			if err := s.settleAgentDeliveryEffect(ctx, settlements, submissions, effect); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) settleAgentDeliveryEffect(
	ctx context.Context,
	settlements repository.AgentDeliverySettlementRepository,
	submissions repository.AgentDeliveryRepository,
	effect *models.AgentDeliveryEffect,
) error {
	outcome := models.DeliverySubmissionState(effect.Outcome)
	if !validTerminalDeliveryOutcome(outcome) {
		return fmt.Errorf("terminal settlement %q has invalid outcome %q", effect.EffectKey, effect.Outcome)
	}
	settled, err := settlements.SettleAgentDeliveryTerminal(
		ctx, effect.StreamID, effect.Sequence, outcome, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("settle delivery terminal %q: %w", effect.EffectKey, err)
	}
	if !settled {
		return nil
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, effect.SubmissionID)
	if err != nil {
		return fmt.Errorf("load terminal submission %q: %w", effect.SubmissionID, err)
	}
	if submission.SessionID != effect.SessionID || submission.IncarnationID != effect.IncarnationID ||
		submission.HarnessGeneration != effect.HarnessGeneration {
		return fmt.Errorf("terminal submission %q owner changed before queue settlement", effect.SubmissionID)
	}
	if err := s.acknowledgeTerminalDeliveryClaims(ctx, effect, submission.DispatchAttemptID); err != nil {
		return fmt.Errorf("settle queue claim for terminal submission %q: %w", effect.SubmissionID, err)
	}
	if err := s.clearAgentDeliveryRecoveryNotice(
		ctx, effect.SessionID, effect.IncarnationID, effect.SubmissionID, effect.HarnessGeneration,
	); err != nil {
		return fmt.Errorf("clear settled recovery notice for submission %q: %w", effect.SubmissionID, err)
	}
	if _, err := settlements.CompleteAgentDeliveryTerminalSettlement(ctx, effect.EffectKey, time.Now().UTC()); err != nil {
		return fmt.Errorf("complete delivery terminal outbox %q: %w", effect.EffectKey, err)
	}
	return nil
}

func validTerminalDeliveryOutcome(outcome models.DeliverySubmissionState) bool {
	return outcome == models.DeliverySubmissionCompleted || outcome == models.DeliverySubmissionFailed ||
		outcome == models.DeliverySubmissionCancelled
}

func (s *Service) acknowledgeTerminalDeliveryClaims(
	ctx context.Context,
	effect *models.AgentDeliveryEffect,
	dispatchAttemptID string,
) error {
	if s.messageQueue == nil || dispatchAttemptID == "" {
		return nil
	}
	queueChanged, err := s.acknowledgeTerminalQueueDispatches(ctx, effect.SessionID, dispatchAttemptID)
	if err != nil {
		return err
	}
	sendNowChanged, err := s.acknowledgeTerminalSendNowClaims(ctx, effect.SessionID, dispatchAttemptID)
	if err != nil {
		return err
	}
	if queueChanged || sendNowChanged {
		s.PublishQueueStatusEvent(ctx, effect.SessionID)
	}
	return nil
}

func (s *Service) acknowledgeTerminalQueueDispatches(
	ctx context.Context,
	sessionID, dispatchAttemptID string,
) (bool, error) {
	if !s.messageQueue.PendingQueueDispatchPersistenceAvailable() {
		return false, nil
	}
	pending, err := s.messageQueue.ListPendingQueueDispatches(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for i := range pending {
		claim := &pending[i]
		if !matchesTerminalQueueDispatch(claim.Message, sessionID, dispatchAttemptID) {
			continue
		}
		if err := s.messageQueue.AcknowledgeDurablePendingQueueDispatch(ctx, &claim.Message); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func matchesTerminalQueueDispatch(message messagequeue.QueuedMessage, sessionID, dispatchAttemptID string) bool {
	_, submissionID, _ := message.DeliverySubmission()
	return message.SessionID == sessionID && submissionID == dispatchAttemptID
}

func (s *Service) acknowledgeTerminalSendNowClaims(
	ctx context.Context,
	sessionID, dispatchAttemptID string,
) (bool, error) {
	if !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return false, nil
	}
	pending, err := s.messageQueue.ListPendingSendNowClaims(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for i := range pending {
		claim := &pending[i].Claim
		if claim.Identity.SessionID != sessionID || claim.DeliverySubmissionID != dispatchAttemptID {
			continue
		}
		if err := s.messageQueue.AcknowledgeSendNowClaim(ctx, claim); err != nil {
			return false, err
		}
		changed = true
	}
	if changed {
		return true, nil
	}
	return false, nil
}
