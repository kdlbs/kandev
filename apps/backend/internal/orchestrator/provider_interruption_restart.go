package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"math"

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
	if s.hasPendingRemoteRecovery(session.ID, session.AgentExecutionID) {
		return
	}
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
	if s.retireLiveCapacityNotice(ctx, session, notices) {
		return
	}
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
	if len(notices) > 0 && session.State == models.TaskSessionStateWaitingForInput {
		data := watcher.AgentEventData{TaskID: session.TaskID, SessionID: session.ID,
			AttemptID: "replay-restart:" + notices[0].ID, RecoveryDisposition: "restart_interrupted",
			ErrorMessage: "Automatic provider retries were interrupted by a backend restart. Resume or start fresh to continue."}
		_ = s.persistLastAgentError(ctx, data)
		_ = s.createRecoveryStatusMessage(ctx, data, "")
	}
}

func (s *Service) settleRestartedContinuation(ctx context.Context, session *models.TaskSession, notice *models.Message) {
	started := 0
	maximum := transientMaxAttempts
	if notice.Metadata["failure_code"] == string(routingerr.CodeModelCapacity) {
		maximum = capacityMaxAttempts
	}
	if value, ok := notice.Metadata["attempts_started"].(float64); ok && value >= 0 && value <= float64(maximum) && value == math.Trunc(value) {
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

func (s *Service) retireLiveCapacityNotice(ctx context.Context, session *models.TaskSession, notices []*models.Message) bool {
	if len(notices) == 0 || session.State != models.TaskSessionStateWaitingForInput ||
		notices[0].Metadata["retry_window_ends_at"] == nil ||
		(!s.wasSessionRetracked(session.ID) && !s.agentManager.IsAgentRunningForSession(ctx, session.ID)) {
		return false
	}
	notice := notices[0]
	notice.Content = transientFailureLabel(&routingerr.Error{Code: routingerr.CodeModelCapacity})
	notice.Metadata["retrying"] = false
	notice.Metadata[metaKeyVariant] = agentEventError
	notice.Metadata["recovery_disposition"] = "restart_interrupted"
	notice.Metadata["runtime_retained"] = true
	notice.Metadata["failure_scope"] = retainedFailureScope
	for _, key := range []string{"actions", "retry_window_ends_at", "retry_at", "retry_in_seconds", "recovery_phase"} {
		delete(notice.Metadata, key)
	}
	if err := s.transientRetryMessages.UpdateMessage(ctx, notice); err != nil {
		s.logger.Warn("failed to retire live capacity retry notice", zap.String("session_id", session.ID), zap.Error(err))
	}
	s.resolveTransientRetryMessagesLocked(ctx, session.ID)
	return true
}
