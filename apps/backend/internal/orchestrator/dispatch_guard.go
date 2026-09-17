package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"go.uber.org/zap"
)

// SetDispatchGuard wires optional application policy without coupling the core
// execution pipeline to orchestration or Office persistence.
func (s *Service) SetDispatchGuard(guard executor.DispatchGuard, resolve func(context.Context, string) (string, error)) {
	s.executor.SetDispatchGuard(guard)
	s.messageQueue.SetDispatchContextResolver(resolve)
}

func (s *Service) checkQueuedContext(ctx context.Context, msg *messagequeue.QueuedMessage) bool {
	if s.executor == nil {
		return true
	}
	if err := s.executor.CheckDispatch(ctx, msg.TaskID, msg.SessionID, ""); err != nil {
		s.logger.Warn("queued context requires refresh; retaining message", zap.String("queue_id", msg.ID), zap.Error(err))
		s.restoreQueuedMessage(ctx, msg)
		return false
	}
	return true
}
