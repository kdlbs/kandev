package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// SetManagedFailureRecovery delegates conversation retry ownership to its
// durable run queue while keeping session transitions under the execution guard.
func (s *Service) SetManagedFailureRecovery(
	failure func(context.Context, watcher.AgentEventData) (int, time.Time, error),
	cancel func(context.Context, string, string) bool,
) {
	s.managedFailure = failure
	s.managedRetryCancel = cancel
}

func (s *Service) handleManagedFailure(ctx context.Context, data watcher.AgentEventData) bool {
	if s.managedFailure == nil {
		return false
	}
	attempt, retryAt, err := s.managedFailure(ctx, data)
	if err != nil {
		s.logger.Error("managed conversation recovery failed", zap.String("session_id", data.SessionID), zap.Error(err))
		return false
	}
	if attempt == 0 {
		return false
	}
	classified := classifyKanbanFailure(data)
	s.createTransientRetryStatusMessage(ctx, data, classified, attempt, time.Until(retryAt), retryAt)
	s.completeTurnForSession(ctx, data.SessionID)
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)
	s.retireExecutionActivityAndPublish(context.WithoutCancel(ctx), data.TaskID, data.SessionID, data.AgentExecutionID)
	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
	return true
}

// ResolveManagedRecovery retires the waiting notice before the next execution starts.
func (s *Service) ResolveManagedRecovery(ctx context.Context, sessionID string) {
	s.resolveTransientRetryMessages(context.WithoutCancel(ctx), sessionID)
}
