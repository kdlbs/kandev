package orchestrator

import (
	"context"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) stopFailedContinuationRestore(ctx context.Context, taskID, sessionID string, entry *transientRetryEntry) bool {
	if err := s.validateContinuationOwner(ctx, taskID, sessionID, entry); err != nil {
		return false
	}
	entry.mu.Lock()
	executionID := entry.restoredExecution
	entry.mu.Unlock()
	if executionID == "" {
		return true
	}
	liveID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil && !agentruntime.IsNotFound(err) && !errors.Is(err, agentruntime.ErrNoExecutionForSession) {
		return false
	}
	if liveID != "" && liveID != executionID {
		return false
	}
	if s.stopContinuationPredecessor(ctx, taskID, sessionID, executionID) != nil {
		return false
	}
	entry.mu.Lock()
	entry.restoredExecution = ""
	entry.mu.Unlock()
	return true
}

func (s *Service) retryContinuationPreparation(ctx context.Context, taskID, sessionID string, entry *transientRetryEntry, failure error) bool {
	if ctx.Err() != nil || entry.attempt >= transientMaxAttempts {
		return false
	}
	classified := routingerr.Classify(routingerr.Input{Phase: routingerr.PhaseSessionInit, Stderr: failure.Error()})
	var typed *routingerr.Error
	if errors.As(failure, &typed) {
		classified = typed
	}
	if classified.Confidence != routingerr.ConfHigh || routingerr.Decide(routingerr.ContextKanban, classified, time.Now().UTC()) != routingerr.DecisionShortRetry {
		return false
	}
	if !s.continuationExecutionAbsent(ctx, sessionID) {
		return false
	}
	guard, release := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	defer guard.Unlock()
	defer release()
	if err := s.validateContinuationOwner(ctx, taskID, sessionID, entry); err != nil {
		return false
	}
	parked := s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)
	if parked == nil || parked.State != models.TaskSessionStateWaitingForInput {
		return false
	}
	state, releaseNotice := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	defer releaseNotice()
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry || state.retired.Load() {
		return false
	}
	attempt := s.nextTransientAttemptLocked(sessionID)
	delay := transientRetryDelayFor(classified, attempt, time.Now().UTC())
	retryAt := time.Now().UTC().Add(delay)
	next := s.reserveTransientRetryWithMetadataLocked(state, sessionID, attempt, func(next *transientRetryEntry) {
		next.predecessorStopped = true
	})
	if next == nil {
		return false
	}
	data := watcher.AgentEventData{TaskID: taskID, SessionID: sessionID, RecoveryMode: recoveryModeContinue}
	data.ProviderError = &streams.ProviderError{ProviderID: next.providerID, ModelID: next.modelID}
	s.createTransientRetryStatusMessageLocked(state, context.WithoutCancel(ctx), data, classified, attempt, delay, retryAt)
	s.armTransientRetryEntryLocked(taskID, sessionID, "", next, delay)
	return true
}

func (s *Service) continuationExecutionAbsent(ctx context.Context, sessionID string) bool {
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	return executionID == "" && (err == nil || agentruntime.IsNotFound(err) || errors.Is(err, agentruntime.ErrNoExecutionForSession))
}

func (s *Service) createContinuationRecoveryMessage(ctx context.Context, data watcher.AgentEventData, entry *transientRetryEntry) error {
	entry.mu.Lock()
	data.RecoveryAttemptsStarted = entry.started
	entry.mu.Unlock()
	if data.RecoveryDisposition == "" {
		data.RecoveryDisposition = recoveryDispositionManual
		if data.RecoveryAttemptsStarted >= transientMaxAttempts {
			data.RecoveryDisposition = recoveryDispositionExhausted
		}
	}
	return s.createRecoveryStatusMessage(ctx, data, "")
}
