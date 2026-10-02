package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) retireContinuationOnShutdown(sessionID string, entry *transientRetryEntry) {
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	defer release()
	if !s.transientRetries.CompareAndDelete(sessionID, entry) {
		return
	}
	entry.mu.Lock()
	accepted := entry.acceptedExecution != ""
	entry.mu.Unlock()
	if !accepted && entry.cancel != nil {
		entry.cancel()
	}
	state.owned.Store(false)
}

func (s *Service) retireInterruptedNoticesOnStartup(ctx context.Context) {
	if s.transientRetryMessages == nil {
		return
	}
	sessions, err := s.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		s.logger.Warn("failed to enumerate stale retry notices on startup", zap.Error(err))
		return
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		s.retireInterruptedNoticeOnStartup(ctx, session)
	}
}

func (s *Service) retireInterruptedNoticeOnStartup(ctx context.Context, session *models.TaskSession) {
	guard, releaseGuard := s.acquireCancelInFlightGuard(session.ID)
	guard.Lock()
	defer guard.Unlock()
	defer releaseGuard()
	state, release := s.acquireTransientRetryNoticeState(session.ID)
	state.mu.Lock()
	defer state.mu.Unlock()
	defer release()
	if _, owned := s.transientRetries.Load(session.ID); owned {
		return
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, session.ID)
	if err != nil {
		return
	}
	notices := transientRetryNotices(messages, session.TaskID, session.ID)
	s.resolveTransientRetryMessagesLocked(ctx, session.ID)
	if s.wasSessionRetracked(session.ID) || s.agentManager.IsAgentRunningForSession(ctx, session.ID) {
		return
	}
	for _, notice := range notices {
		if notice.Metadata["recovery_mode"] == recoveryModeContinue {
			s.settleRestartedContinuation(ctx, session, notice)
			return
		}
	}
}

func (s *Service) settleRestartedContinuation(ctx context.Context, session *models.TaskSession, notice *models.Message) {
	started := 0
	if value, ok := notice.Metadata["attempts_started"].(float64); ok && value >= 0 && value <= transientMaxAttempts {
		started = int(value)
	}
	data := watcher.AgentEventData{TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: session.AgentExecutionID,
		AttemptID: "continuation-restart:" + notice.ID, RecoveryMode: recoveryModeContinue,
		RecoveryDisposition: "restart_interrupted", RecoveryAttemptsStarted: started,
		ErrorMessage: "Automatic continuation was interrupted by a backend restart. Resume or start fresh to continue."}
	_ = s.settleContinuationInterruption(ctx, data)
	_ = s.persistLastAgentError(ctx, data)
	_ = s.createRecoveryStatusMessage(ctx, data, "")
	s.updateTaskSessionState(ctx, session.TaskID, session.ID, models.TaskSessionStateWaitingForInput, data.ErrorMessage, false)
}
