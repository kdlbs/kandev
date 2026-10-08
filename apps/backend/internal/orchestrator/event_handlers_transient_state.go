package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// clearTransientRetryState clears a session's retry entry, cancels its timer,
// and drops the cached prompt (which may hold large/sensitive attachment data).
func (s *Service) clearTransientRetryState(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	active := s.clearTransientRetryStateLocked(sessionID, state)
	state.mu.Unlock()
	release()
	return active
}

func (s *Service) clearTransientRetryStateLocked(sessionID string, state *transientRetryNoticeState) bool {
	s.lastTurnPrompt.Delete(sessionID)
	v, ok := s.transientRetries.LoadAndDelete(sessionID)
	if ok {
		if entry, ok := v.(*transientRetryEntry); ok {
			entry.mu.Lock()
			failureData := entry.failureData
			if failureData != nil && failureData.RecoveryMode != recoveryModeReplay {
				failureData = nil
			}
			entry.failureData = nil
			cancel := entry.cancel
			entry.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			if failureData != nil {
				s.clearPromptAttemptEvidence(
					sessionID, failureData.AgentExecutionID, failureData.PromptGeneration,
				)
			}
		}
	}
	state.owned.Store(false)
	return ok
}

func (s *Service) clearTransientRetryEntryLocked(
	sessionID string,
	state *transientRetryNoticeState,
	entry *transientRetryEntry,
) bool {
	if sessionID == "" || entry == nil || !s.transientRetries.CompareAndDelete(sessionID, entry) {
		return false
	}
	s.lastTurnPrompt.Delete(sessionID)
	if entry.cancel != nil {
		entry.cancel()
	}
	state.owned.Store(false)
	return true
}

// resetTransientRetry clears in-memory retry state and retires the persisted
// retry notice(s). The detached context keeps durable cleanup best effort even
// when the event that ended the retry was cancelled by its caller.
func (s *Service) resetTransientRetry(sessionID string) {
	s.resetTransientRetryWithContext(context.Background(), sessionID, false)
}

// forceResolve is used by explicit stop/cancel and terminal paths where a
// persisted notice can outlive the in-memory retry entry. Normal successful
// turns skip the transcript scan when no retry loop was owned.
func (s *Service) resetTransientRetryWithContext(ctx context.Context, sessionID string, forceResolve bool) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.resetTransientRetryWithContextLocked(state, ctx, sessionID, forceResolve)
	state.mu.Unlock()
	release()
}

func (s *Service) resetTransientRetryWithContextLocked(
	state *transientRetryNoticeState,
	ctx context.Context,
	sessionID string,
	forceResolve bool,
) {
	if !s.clearTransientRetryStateLocked(sessionID, state) && !forceResolve {
		return
	}
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.resolveTransientRetryMessagesLocked(context.WithoutCancel(ctx), sessionID)
}

// resolveTransientRetryMessages removes every persisted retry status message
// for a session. The task service owns the durable write and MessageDeleted
// publication. Cleanup is intentionally non-fatal to the transition that
// ended the retry loop.
func (s *Service) resolveTransientRetryMessages(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.resolveTransientRetryMessagesLocked(ctx, sessionID)
	state.mu.Unlock()
	release()
}

func (s *Service) resolveTransientRetryMessagesLocked(ctx context.Context, sessionID string) {
	if s.transientRetryMessages == nil || sessionID == "" {
		return
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to list transient retry status messages",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	for _, message := range messages {
		if message == nil || message.Metadata == nil {
			continue
		}
		retrying, ok := message.Metadata["retrying"].(bool)
		if !ok || !retrying {
			continue
		}
		if err := s.transientRetryMessages.DeleteMessage(ctx, message.ID); err != nil {
			s.logger.Warn("failed to delete transient retry status message",
				zap.String("session_id", sessionID),
				zap.String("message_id", message.ID),
				zap.Error(err))
		}
	}
}

// retireAndClearTransientRetryState closes the in-memory lifecycle under the
// notice mutex. Callers use it when they already hold the task runtime mutex;
// durable cleanup remains outside that broader runtime critical section.
func (s *Service) retireAndClearTransientRetryState(sessionID string) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.clearTransientRetryStateLocked(sessionID, state)
	state.mu.Unlock()
	release()
}

// cancelAllTransientRetries drains local retry ownership at shutdown.
// Continuation notices survive for startup reconciliation; accepted work
// belongs to the runtime's shutdown and live-adoption policy.
func (s *Service) cancelAllTransientRetries() {
	s.transientRetries.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if entry, ok := value.(*transientRetryEntry); ok {
				mode, _ := entry.recovery()
				if mode == recoveryModeContinue {
					s.retireContinuationOnShutdown(keyStr, entry)
					return true
				}
			}
			s.resetTransientRetry(keyStr)
		}
		return true
	})
}

// CancelTransientRetry stops an in-progress retry loop (user clicked Cancel)
// and surfaces the manual recovery banner so they can Resume or Start fresh.
// Returns true if a retry loop was active.
func (s *Service) CancelTransientRetry(ctx context.Context, taskID, sessionID string) bool {
	// Reports "nothing to cancel" on denial: the bool return carries no error
	// channel, and a foreign session must not be distinguishable from an idle
	// one. Guard first — resetTransientRetry below mutates retry state.
	//
	// Both IDs: taskID is handed to handleRecoverableFailure, which writes
	// against that task, so the session check alone would leave it free to
	// point at someone else's.
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return false
	}
	noticeState, releaseNoticeState := s.acquireTransientRetryNoticeState(sessionID)
	if noticeState == nil {
		return false
	}
	noticeState.mu.Lock()
	value, active := s.transientRetries.Load(sessionID)
	entry, hasEntry := value.(*transientRetryEntry)
	if hasEntry && entry.retainedRuntime != nil {
		noticeState.mu.Unlock()
		releaseNoticeState()
		return s.cancelRetainedRuntimeRetry(ctx, taskID, sessionID, entry)
	}
	if hasEntry {
		mode, _ := entry.recovery()
		if mode == recoveryModeContinue {
			noticeState.mu.Unlock()
			releaseNoticeState()
			return s.cancelContinuationRetry(ctx, taskID, sessionID, entry)
		}
	}
	s.resetTransientRetryWithContextLocked(noticeState, ctx, sessionID, true)
	noticeState.mu.Unlock()
	releaseNoticeState()
	if !active {
		return false
	}
	s.logger.Info("user cancelled transient retry loop",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	execID, _ := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	s.handleRecoverableFailure(ctx, watcher.AgentEventData{
		TaskID:              taskID,
		SessionID:           sessionID,
		AgentExecutionID:    execID,
		ErrorMessage:        "Automatic provider retries cancelled. Resume or start fresh to continue.",
		RecoveryDisposition: "cancelled",
		UserInitiated:       true,
	})
	return true
}

func (s *Service) cancelRetainedRuntimeRetry(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
) bool {
	entry.mu.Lock()
	started, acceptedExecution := entry.started, entry.acceptedExecution
	entry.mu.Unlock()
	if started == 0 && acceptedExecution == "" {
		if entry.cancel != nil {
			entry.cancel()
		}
		s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "cancelled")
		return true
	}
	if entry.mode == recoveryModeContinue {
		return s.cancelContinuationRetry(ctx, taskID, sessionID, entry)
	}
	ctx = context.WithValue(ctx, continuationCancelContextKey{}, entry)
	if err := s.CancelAgent(ctx, sessionID); err != nil {
		return false
	}
	s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "cancelled")
	return true
}
