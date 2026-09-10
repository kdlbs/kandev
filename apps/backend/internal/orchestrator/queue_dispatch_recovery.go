package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"go.uber.org/zap"
)

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
			s.logger.Warn("restoring queued dispatch with unknown pre-crash delivery outcome",
				zap.String("entry_id", msg.ID),
				zap.String("session_id", msg.SessionID),
				zap.String("task_id", msg.TaskID))
			if _, err := s.messageQueue.RestoreMessage(context.WithoutCancel(ctx), msg); err != nil {
				return fmt.Errorf("restore pending queue dispatch %s: %w", msg.ID, err)
			}
		default:
			return fmt.Errorf("locate pending queue dispatch %s: %w", msg.ID, findErr)
		}
	}
	return nil
}
