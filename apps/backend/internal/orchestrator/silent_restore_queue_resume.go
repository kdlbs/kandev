package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"go.uber.org/zap"
)

func (s *Service) resumeSilentRestoreQueue(ctx context.Context, identity messagequeue.QueueSessionIdentity, ownerID string) {
	if s.messageQueue == nil || !s.config.ResumeQueueAfterSilentRestore {
		return
	}
	resumed, err := s.messageQueue.ResumeAutoRunAfterRecovery(ctx, identity, ownerID)
	if err != nil {
		s.logger.Warn("failed to resume queue after silent restore",
			zap.String("session_id", identity.SessionID), zap.Error(err))
		return
	}
	if !resumed {
		return
	}
	s.publishTaskQueueStatusEvent(ctx, identity.TaskID, identity.SessionID)
	// The outer restore entrypoint drains through the existing owned worker
	// after route and lifecycle locks have been released.
	s.markQueuedDispatchDrainPending(identity.SessionID)
}
