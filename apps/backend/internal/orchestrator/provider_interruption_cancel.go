package orchestrator

import "context"

func (s *Service) retireContinuationForHumanDispatch(sessionID string) {
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	defer release()
	if value, ok := s.transientRetries.Load(sessionID); ok {
		if entry, ok := value.(*transientRetryEntry); ok {
			mode, _ := entry.recovery()
			if mode == recoveryModeContinue {
				s.resetTransientRetryWithContextLocked(state, context.Background(), sessionID, true)
			}
		}
	}
}

func (s *Service) recordContinuationAcceptance(sessionID string, entry *transientRetryEntry) {
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	executionID, generation := evidence.executionID, evidence.promptGeneration
	evidence.mu.Unlock()
	entry.mu.Lock()
	entry.acceptedExecution = executionID
	entry.acceptedGeneration = generation
	entry.mu.Unlock()
}

func (s *Service) continuationCancellationOwnsTurn(ctx context.Context, sessionID string, entry *transientRetryEntry) bool {
	current, ok := s.transientRetries.Load(sessionID)
	if !ok || current != entry {
		return false
	}
	entry.mu.Lock()
	executionID, generation := entry.acceptedExecution, entry.acceptedGeneration
	restoredID := entry.restoredExecution
	entry.mu.Unlock()
	if executionID == "" {
		if restoredID == "" {
			return true
		}
		liveID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
		return err == nil && liveID == restoredID
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.executionID == executionID && evidence.promptGeneration == generation
}

func (s *Service) cancelContinuationRetry(ctx context.Context, taskID, sessionID string, entry *transientRetryEntry) bool {
	ctx = context.WithValue(ctx, continuationCancelContextKey{}, entry)
	if err := s.CancelAgent(ctx, sessionID); err != nil {
		return false
	}
	s.finishContinuationManual(context.WithoutCancel(ctx), taskID, sessionID, "", entry, "cancelled")
	return true
}

func (s *Service) cancelRestoredContinuation(ctx context.Context, sessionID string, entry *transientRetryEntry) bool {
	entry.mu.Lock()
	executionID := entry.restoredExecution
	entry.mu.Unlock()
	if executionID == "" {
		return true
	}
	current, ok := s.transientRetries.Load(sessionID)
	if !ok || current != entry {
		return false
	}
	liveID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || liveID != executionID {
		return false
	}
	guard, release := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	liveID, err = s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	current, ok = s.transientRetries.Load(sessionID)
	owned := ok && current == entry && err == nil && liveID == executionID && !s.isCancelInFlight(sessionID)
	if owned {
		err = s.closeInterruptedTurn(ctx, sessionID)
	}
	guard.Unlock()
	release()
	if !owned || err != nil {
		return false
	}
	err = s.CancelAgent(context.WithValue(ctx, continuationCancelContextKey{}, entry), sessionID)
	return err == nil
}
