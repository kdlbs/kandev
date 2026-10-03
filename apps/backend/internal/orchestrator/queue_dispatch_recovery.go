package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const unknownQueueDispatchRecoveryReason = "unknown_queue_dispatch_outcome"

func (s *Service) reconcilePendingQueueDispatchesOnStartup(ctx context.Context) error {
	if s.messageQueue == nil || !s.messageQueue.PendingQueueDispatchPersistenceAvailable() {
		return nil
	}
	pending, err := s.messageQueue.ListPendingQueueDispatches(ctx)
	if err != nil {
		return fmt.Errorf("list pending queue dispatches: %w", err)
	}
	for i := range pending {
		dispatch := &pending[i]
		msg := &dispatch.Message
		if dispatch.Accepted {
			if err := s.messageQueue.DeletePendingQueueDispatch(ctx, msg); err != nil {
				return fmt.Errorf("acknowledge accepted queue dispatch %s: %w", msg.ID, err)
			}
			continue
		}
		_, findErr := s.messageQueue.FindEntryByID(ctx, msg.ID)
		switch {
		case findErr == nil:
			if err := s.messageQueue.DeletePendingQueueDispatch(ctx, msg); err != nil {
				return fmt.Errorf("acknowledge restored queue dispatch %s: %w", msg.ID, err)
			}
		case errors.Is(findErr, messagequeue.ErrEntryNotFound):
			if _, supported := s.repo.(sessionRecoveryBlockStore); !supported {
				// Focused legacy compositions do not own the canonical task
				// repository. Preserve their historical recovery behavior, while
				// production wiring remains fail-closed through the block below.
				if _, restoreErr := s.messageQueue.RestoreMessage(context.WithoutCancel(ctx), msg); restoreErr != nil {
					return fmt.Errorf("restore pending queue dispatch %s: %w", msg.ID, restoreErr)
				}
				continue
			}
			recoveryErr := &sessionRecoveryRequiredError{
				Block: &models.SessionRecoveryBlock{Reason: unknownQueueDispatchRecoveryReason},
			}
			if err := s.recordSessionRecoveryBlock(ctx, msg.SessionID, "queue_dispatch", recoveryErr); err != nil {
				return fmt.Errorf("park pending queue dispatch %s for recovery: %w", msg.ID, err)
			}
			s.logger.Warn("parked queued dispatch with unknown pre-crash delivery outcome",
				zap.String("entry_id", msg.ID),
				zap.String("session_id", msg.SessionID),
				zap.String("task_id", msg.TaskID))
		default:
			return fmt.Errorf("locate pending queue dispatch %s: %w", msg.ID, findErr)
		}
	}
	return nil
}

// restorePendingQueueDispatchesForRecovery releases queue claims only after an
// operator has completed an explicit session recovery action. Startup never
// calls this helper: an unknown pre-crash outcome remains parked until that
// action provides the authorization to make the original queue entry visible.
func (s *Service) restorePendingQueueDispatchesForRecovery(ctx context.Context, sessionID string) error {
	if s.messageQueue == nil || sessionID == "" {
		return nil
	}
	if s.messageQueue.PendingQueueDispatchPersistenceAvailable() {
		pending, err := s.messageQueue.ListPendingQueueDispatches(ctx)
		if err != nil {
			return fmt.Errorf("list pending queue dispatches for recovery: %w", err)
		}
		for i := range pending {
			if err := s.restoreOneQueueDispatchForRecovery(ctx, sessionID, &pending[i]); err != nil {
				return err
			}
		}
	}
	if err := s.restorePendingSendNowClaimsForRecovery(ctx, sessionID); err != nil {
		return fmt.Errorf("release parked Send Now work after session recovery: %w", err)
	}
	s.PublishQueueStatusEvent(ctx, sessionID)
	return nil
}

func (s *Service) restoreOneQueueDispatchForRecovery(
	ctx context.Context,
	sessionID string,
	claim *messagequeue.PendingQueueDispatch,
) error {
	if claim == nil || claim.Message.SessionID != sessionID {
		return nil
	}
	if claim.Accepted {
		if err := s.messageQueue.DeletePendingQueueDispatch(ctx, &claim.Message); err != nil {
			return fmt.Errorf("acknowledge recovered queue dispatch %s: %w", claim.Message.ID, err)
		}
		return nil
	}
	_, findErr := s.messageQueue.FindEntryByID(ctx, claim.Message.ID)
	if findErr == nil {
		if err := s.messageQueue.DeletePendingQueueDispatch(ctx, &claim.Message); err != nil {
			return fmt.Errorf("clear duplicate recovered queue dispatch %s: %w", claim.Message.ID, err)
		}
		return nil
	}
	if !errors.Is(findErr, messagequeue.ErrEntryNotFound) {
		return fmt.Errorf("locate recovered queue dispatch %s: %w", claim.Message.ID, findErr)
	}
	if _, err := s.messageQueue.RestoreMessage(context.WithoutCancel(ctx), &claim.Message); err != nil {
		return fmt.Errorf("restore recovered queue dispatch %s: %w", claim.Message.ID, err)
	}
	return nil
}
