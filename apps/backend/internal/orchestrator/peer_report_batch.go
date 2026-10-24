package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) reserveAndDispatchQueueHead(
	ctx context.Context, identity messagequeue.QueueSessionIdentity,
) (bool, bool, error) {
	return s.reserveAndDispatchQueueHeadWithCompletion(ctx, identity, nil)
}

func (s *Service) reserveAndDispatchQueueHeadWithCompletion(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, completionEffect *models.AgentDeliveryEffect,
) (bool, bool, error) {
	ctx = s.queuedRecoveryContext(ctx, nil)
	batch, autoRun, err := s.messageQueue.ReservePeerReportBatchWithAutoRunForSession(ctx, identity)
	if err != nil || !autoRun {
		s.requeuePeerReportBatch(ctx, identity, batch)
		return false, autoRun, err
	}
	if len(batch) == 0 {
		return false, autoRun, nil
	}
	if err := s.settlePeerReportPredecessor(ctx, identity, batch[0], completionEffect); err != nil {
		s.logger.Warn("failed to settle delivery before queued successor",
			zap.String("session_id", identity.SessionID), zap.Error(err))
		s.requeuePeerReportBatch(ctx, identity, batch)
		return false, true, err
	}
	if len(batch) == 1 {
		return s.dispatchTakenQueuedMessageForSession(ctx, identity, batch[0], true), true, nil
	}
	composed := messagequeue.ComposePeerReportBatch(batch)
	if !s.peerReportBatchStillDispatchable(ctx, identity, composed) {
		s.requeuePeerReportBatch(ctx, identity, batch)
		return false, true, nil
	}
	err = s.messageQueue.FoldPeerReportBatch(context.WithoutCancel(ctx), identity, composed, batch)
	if err != nil {
		// A changed claim may already belong to an executor or a new incarnation.
		// Keep its existing recovery authority; never turn it into a fresh retry.
		if !errors.Is(err, messagequeue.ErrQueueDispatchClaimChanged) {
			s.requeuePeerReportBatch(ctx, identity, batch)
		}
		s.logger.Warn("failed to fold peer report dispatch claims",
			zap.String("session_id", identity.SessionID), zap.Error(err))
		return false, true, err
	}
	// From this commit onward, only the combined leader may be dispatched or
	// returned. The ordinary worker retains current native acceptance semantics.
	return s.dispatchTakenQueuedMessageForSession(ctx, identity, composed, true), true, nil
}

func (s *Service) settlePeerReportPredecessor(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, message *messagequeue.QueuedMessage,
	effect *models.AgentDeliveryEffect,
) error {
	if effect == nil {
		return nil
	}
	hasInput, err := s.queuedMessageHasDispatchInput(ctx, message)
	if err != nil || !hasInput {
		return nil
	}
	return s.settleQueuedDeliveryPredecessor(ctx, identity.TaskID, identity.SessionID, effect)
}

func (s *Service) peerReportBatchStillDispatchable(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, composed *messagequeue.QueuedMessage,
) bool {
	if identity.SessionIncarnationID != "" {
		current, err := s.messageQueue.ResolveSessionIdentity(ctx, identity.TaskID, identity.SessionID)
		if err != nil || current != identity {
			return false
		}
	}
	hasInput, err := s.queuedMessageHasDispatchInput(ctx, composed)
	return err == nil && hasInput
}

// Requeue-at-head is reversed so all reserved originals keep their FIFO order.
func (s *Service) requeuePeerReportBatch(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, batch []*messagequeue.QueuedMessage,
) {
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendNowClaimRecoveryTimeout)
	defer cancel()
	for index := len(batch) - 1; index >= 0; index-- {
		entry := batch[index]
		s.requeueMessageForReservation(ctx, &queuedDispatchReservation{identity: identity}, entry, entry.QueuedBy)
	}
	if len(batch) > 0 {
		s.publishQueueStatusEventForIdentity(ctx, identity)
	}
}
