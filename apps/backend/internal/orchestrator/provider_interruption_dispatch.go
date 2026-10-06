package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const continuationPrepareTimeout = time.Minute
const continuationInstruction = "continue"
const capacityContinuationInstruction = "Your previous turn stopped because the selected model was at capacity. Continue the unfinished request using this conversation's existing history. Preserve all completed actions and do not repeat completed work. If any earlier outcome is uncertain, stop and ask the user before acting."

func (s *Service) retryInterruptedContinuation(ctx context.Context, taskID, sessionID, execID string, entry *transientRetryEntry) {
	if err := s.validateContinuationOwner(ctx, taskID, sessionID, entry); err != nil {
		if entry.retainedRuntime != nil {
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
			return
		}
		s.finishContinuationManual(ctx, taskID, sessionID, execID, entry)
		return
	}
	if entry.continuationPolicy == continuationPolicyCapacityLive {
		if entry.retainedRuntime == nil {
			s.finishContinuationManual(ctx, taskID, sessionID, execID, entry)
			return
		}
		if s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) != retainedRuntimeRetryUsable {
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
			return
		}
		if !s.retryRetainedRuntimeContinuation(ctx, taskID, sessionID, entry, capacityContinuationInstruction) {
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
		}
		return
	}
	if entry.retainedRuntime != nil {
		switch s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) {
		case retainedRuntimeRetryUsable:
			if s.retryRetainedRuntimeContinuation(ctx, taskID, sessionID, entry, continuationInstruction) {
				return
			}
		case retainedRuntimeRetryBlocked:
			s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
			return
		}
	}
	entry.mu.Lock()
	entry.started++
	stopped := entry.predecessorStopped
	entry.mu.Unlock()
	s.updateContinuationPhase(ctx, taskID, sessionID, entry, "reconnecting")
	if !stopped {
		if err := s.stopContinuationPredecessor(ctx, taskID, sessionID, execID); err != nil {
			s.finishContinuationManual(ctx, taskID, sessionID, execID, entry)
			return
		}
		entry.mu.Lock()
		entry.predecessorStopped = true
		entry.mu.Unlock()
	}
	prepareCtx, cancel := context.WithCancel(ctx)
	dispatchAccepted := false
	defer func() {
		if !dispatchAccepted {
			cancel()
		}
	}()
	timer := time.AfterFunc(continuationPrepareTimeout, cancel)
	defer timer.Stop()
	prepareCtx = context.WithValue(prepareCtx, continuationOwnedContextKey{}, true)
	dispatchStarted := false
	execution, err := s.resumeTaskSessionWithContinuation(prepareCtx, taskID, sessionID,
		executor.ResumeOptions{RequiredNativeConversationID: entry.continuation.nativeID, Origin: string(launchOriginAutomatic)},
		func(promptCtx context.Context, attempt *resumeAttempt, _ *executor.TaskExecution) error {
			entry.mu.Lock()
			entry.restoredExecution = attempt.execution()
			entry.mu.Unlock()
			_, err := s.promptTask(promptCtx, taskID, sessionID, continuationInstruction, "", false, nil, true, launchOriginAutomatic, promptTaskOptions{
				resumeAttempt: attempt, internalContinuation: true, preservePromptContext: true, disableDispatchRetry: true, requireNonterminalSession: true, reserveTurnUntilDispatch: true,
				beforeDispatch: func() error {
					if err := s.validateContinuationOwner(promptCtx, taskID, sessionID, entry); err != nil {
						return err
					}
					dispatchStarted = true
					return nil
				},
				onAccepted: func(string) {
					dispatchAccepted = true
					timer.Stop()
					s.recordContinuationAcceptance(sessionID, entry)
					s.updateContinuationPhase(promptCtx, taskID, sessionID, entry, "continuing")
				},
			})
			return err
		})
	if execution != nil {
		entry.mu.Lock()
		entry.restoredExecution = execution.AgentExecutionID
		entry.mu.Unlock()
	}
	if err != nil {
		s.logger.Warn("automatic continuation preparation or dispatch failed", zap.String("session_id", sessionID), zap.Bool("dispatch_started", dispatchStarted), zap.Error(routingerr.SanitizeError(err)))
		if !dispatchStarted && s.stopFailedContinuationRestore(ctx, taskID, sessionID, entry) &&
			s.retryContinuationPreparation(ctx, taskID, sessionID, entry, err) {
			return
		}
		s.finishContinuationManual(ctx, taskID, sessionID, execID, entry)
	}
}

func (s *Service) retryRetainedRuntimeContinuation(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
	instruction string,
) bool {
	if s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) != retainedRuntimeRetryUsable {
		return false
	}
	var liveExecutionFence *promptTaskLiveExecutionFence
	if entry.continuationPolicy == continuationPolicyCapacityLive {
		if entry.retainedRuntime == nil || entry.continuation == nil {
			return false
		}
		liveExecutionFence = &promptTaskLiveExecutionFence{
			executionID:    entry.retainedRuntime.executionID,
			nativeID:       entry.continuation.nativeID,
			identity:       entry.continuation.identity,
			workflowStepID: entry.continuation.workflowStepID,
		}
	}
	dispatchStarted := false
	dispatchAccepted := false
	markDispatchStarted := func() {
		if dispatchStarted {
			return
		}
		entry.mu.Lock()
		entry.started++
		entry.mu.Unlock()
		dispatchStarted = true
	}
	beforeDispatch := func() error {
		if err := s.validateContinuationOwner(ctx, taskID, sessionID, entry); err != nil {
			return err
		}
		if s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) != retainedRuntimeRetryUsable {
			return ErrResumeAttemptCancelled
		}
		if liveExecutionFence != nil {
			if err := s.validatePromptLiveExecutionFence(ctx, taskID, sessionID, liveExecutionFence); err != nil {
				return err
			}
		}
		if liveExecutionFence == nil {
			markDispatchStarted()
		}
		return nil
	}
	onAccepted := func(string) {
		if liveExecutionFence != nil {
			markDispatchStarted()
		}
		dispatchAccepted = true
		s.recordContinuationAcceptance(sessionID, entry)
		s.updateContinuationPhase(context.WithoutCancel(ctx), taskID, sessionID, entry, "continuing")
	}
	beforeProviderAdmission := func() error {
		if liveExecutionFence == nil {
			return nil
		}
		if err := s.validateContinuationOwner(ctx, taskID, sessionID, entry); err != nil {
			return err
		}
		admissionCtx := ctx
		if entry.continuation != nil && entry.continuation.initiatorKnown {
			admissionCtx = authn.WithIdentity(ctx, entry.continuation.initiator)
		}
		if err := s.authorizeSessionPrompt(admissionCtx, sessionID); err != nil {
			return err
		}
		return s.validateContinuationOwner(ctx, taskID, sessionID, entry)
	}
	_, err := s.promptTask(ctx, taskID, sessionID, instruction, "", false, nil, true,
		launchOriginAutomatic, promptTaskOptions{
			internalContinuation:      true,
			preservePromptContext:     true,
			disableDispatchRetry:      true,
			requireNonterminalSession: true,
			reserveTurnUntilDispatch:  true,
			liveExecutionFence:        liveExecutionFence,
			beforeDispatch:            beforeDispatch,
			beforeProviderAdmission:   beforeProviderAdmission,
			onAccepted:                onAccepted,
		})
	if err == nil || dispatchAccepted {
		return true
	}
	var retainedFailure *agentruntime.RetainedPromptFailureError
	if errors.As(err, &retainedFailure) {
		return true
	}
	if ctx.Err() != nil {
		return true
	}
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry {
		return true
	}
	if !dispatchStarted && s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) == retainedRuntimeRetryLost {
		return false
	}
	s.logger.Warn("retained-runtime continuation could not dispatch",
		zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(routingerr.SanitizeError(err)))
	s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
	return true
}

func (s *Service) validatePromptLiveExecutionFence(
	ctx context.Context,
	taskID, sessionID string,
	fence *promptTaskLiveExecutionFence,
) error {
	if !validPromptLiveExecutionFence(s, fence) {
		return ErrResumeAttemptCancelled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validatePromptLiveExecutionOwnership(ctx, taskID, sessionID, fence); err != nil {
		return err
	}
	if err := s.validatePromptLiveRuntime(ctx, sessionID, fence.nativeID); err != nil {
		return err
	}
	if err := s.validatePromptExecutionMap(sessionID, fence.executionID); err != nil {
		return err
	}
	// Revalidate ownership after runtime probes before admitting the provider call.
	return s.validatePromptLiveExecutionOwnership(ctx, taskID, sessionID, fence)
}

func validPromptLiveExecutionFence(s *Service, fence *promptTaskLiveExecutionFence) bool {
	return s != nil && fence != nil && fence.executionID != "" && fence.nativeID != "" &&
		fence.identity != ([32]byte{}) && s.agentManager != nil && s.executor != nil
}

func (s *Service) validatePromptLiveExecutionOwnership(
	ctx context.Context,
	taskID, sessionID string,
	fence *promptTaskLiveExecutionFence,
) error {
	if err := s.validatePromptLiveSession(ctx, taskID, sessionID, fence); err != nil {
		return err
	}
	if err := s.validatePromptLiveTask(ctx, taskID, fence); err != nil {
		return err
	}
	if err := s.validatePromptStoredExecution(ctx, sessionID, fence.executionID); err != nil {
		return err
	}
	return s.validatePromptManagerExecution(ctx, sessionID, fence.executionID)
}

func (s *Service) validatePromptLiveSession(
	ctx context.Context,
	taskID, sessionID string,
	fence *promptTaskLiveExecutionFence,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || !continuationSessionMatches(taskID, session, &continuationBinding{
		nativeID: fence.nativeID, identity: fence.identity,
	}) {
		return executor.ErrExecutionNotFound
	}
	return nil
}

func (s *Service) validatePromptLiveTask(ctx context.Context, taskID string, fence *promptTaskLiveExecutionFence) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt != nil || task.IsFromOffice ||
		task.WorkflowStepID != fence.workflowStepID || models.IsAutomationTaskOrigin(task.Origin) {
		return ErrResumeAttemptCancelled
	}
	return nil
}

func (s *Service) validatePromptStoredExecution(ctx context.Context, sessionID, executionID string) error {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.AgentExecutionID != executionID {
		return executor.ErrExecutionNotFound
	}
	return nil
}

func (s *Service) validatePromptManagerExecution(ctx context.Context, sessionID, executionID string) error {
	currentExecutionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || currentExecutionID != executionID {
		return executor.ErrExecutionNotFound
	}
	return nil
}

func (s *Service) validatePromptLiveRuntime(ctx context.Context, sessionID, nativeID string) error {
	runningProcess, probeErr := s.probeAgentRunning(ctx, sessionID)
	if probeErr != nil || !runningProcess {
		return ErrResumeAttemptCancelled
	}
	if !s.agentManager.IsAgentReadyForPrompt(ctx, sessionID) {
		return ErrResumeAttemptCancelled
	}
	if currentNativeID := s.currentACPSessionID(sessionID); currentNativeID == "" || currentNativeID != nativeID {
		return ErrResumeAttemptCancelled
	}
	return nil
}

func (s *Service) validatePromptExecutionMap(sessionID, executionID string) error {
	execution, ok := s.executor.GetExecutionBySession(sessionID)
	if !ok || execution == nil || execution.AgentExecutionID != executionID {
		return executor.ErrExecutionNotFound
	}
	return nil
}

func (s *Service) validateContinuationOwner(ctx context.Context, taskID, sessionID string, entry *transientRetryEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	current, ok := s.transientRetries.Load(sessionID)
	if !ok || current != entry || entry.continuation == nil || entry.continuationPolicy != entry.continuation.policy {
		return ErrResumeAttemptCancelled
	}
	switch entry.continuationPolicy {
	case continuationPolicySavedHistoryRestore:
		if !s.config.ProviderInterruptionContinuation {
			return ErrResumeAttemptCancelled
		}
	case continuationPolicyCapacityLive:
	default:
		return ErrResumeAttemptCancelled
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !continuationSessionMatches(taskID, session, entry.continuation) {
		return errors.New("interrupted conversation identity changed")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if !continuationTaskMatches(task, entry.continuation) {
		return errors.New("interrupted task is no longer eligible")
	}
	if s.messageQueue != nil {
		pending, err := s.messageQueue.HasPendingForSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if pending {
			return errors.New("queued work takes priority over automatic continuation")
		}
	}
	return nil
}

func continuationSessionMatches(taskID string, session *models.TaskSession, binding *continuationBinding) bool {
	return session != nil && session.TaskID == taskID && !session.IsPassthrough && !isTerminalSessionState(session.State) &&
		continuationNativeID(session) == binding.nativeID && continuationSessionIdentity(session) == binding.identity
}

func continuationTaskMatches(task *models.Task, binding *continuationBinding) bool {
	return task != nil && task.ArchivedAt == nil && !task.IsFromOffice && task.WorkflowStepID == binding.workflowStepID &&
		(binding.policy != continuationPolicyCapacityLive || !models.IsAutomationTaskOrigin(task.Origin))
}

func (s *Service) stopContinuationPredecessor(ctx context.Context, taskID, sessionID, executionID string) error {
	if executionID == "" || !s.claimForcedExecutionCleanup(sessionID, executionID) {
		return errors.New("predecessor teardown is already owned")
	}
	claim, claimed := s.executionTeardownClaimFor(sessionID, executionID)
	err := s.stopTransientRetryExecution(ctx, executionID)
	if err != nil && !agentruntime.IsNotFound(err) {
		if claimed {
			s.executionTeardownClaims.CompareAndDelete(terminalExecutionKey(sessionID, executionID), claim)
		}
		return err
	}
	if claimed {
		s.completeExecutionTeardownClaim(sessionID, executionID, claim)
	}
	s.retireExecutionActivityAndPublish(context.WithoutCancel(ctx), taskID, sessionID, executionID)
	return ctx.Err()
}

func (s *Service) finishContinuationManual(ctx context.Context, taskID, sessionID, executionID string, entry *transientRetryEntry, disposition ...string) {
	if ctx.Err() != nil {
		return
	}
	cancelConfirmed := true
	if len(disposition) == 0 || disposition[0] != stopReasonCancelled {
		cancelConfirmed = s.cancelRestoredContinuation(ctx, sessionID, entry)
	}
	guard, release := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	defer guard.Unlock()
	defer release()
	current, ok := s.transientRetries.Load(sessionID)
	if !ok || current != entry {
		return
	}
	settlementCtx := context.WithoutCancel(ctx)
	entry.mu.Lock()
	if entry.restoredExecution != "" {
		executionID = entry.restoredExecution
	}
	entry.mu.Unlock()
	data := watcher.AgentEventData{TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		RecoveryMode: recoveryModeContinue, ErrorMessage: "Automatic continuation could not safely proceed. Resume or start fresh to continue."}
	if len(disposition) > 0 {
		data.RecoveryDisposition = disposition[0]
	}
	nextState := models.TaskSessionStateWaitingForInput
	if err := s.settleContinuationInterruption(settlementCtx, data); err != nil {
		nextState = models.TaskSessionStateFailed
		s.logger.Warn("failed to persist interrupted continuation", zap.String("session_id", sessionID), zap.Error(err))
	}
	s.resetTransientRetry(sessionID)
	_ = s.persistLastAgentError(settlementCtx, data)
	_ = s.createContinuationRecoveryMessage(settlementCtx, data, entry)
	if !cancelConfirmed {
		nextState = models.TaskSessionStateFailed
	}
	s.updateTaskSessionState(settlementCtx, taskID, sessionID, nextState, data.ErrorMessage, false)
}

func (s *Service) settleContinuationInterruption(ctx context.Context, data watcher.AgentEventData) error {
	if s.turnService != nil {
		turn, err := s.turnService.GetActiveTurn(ctx, data.SessionID)
		if err != nil && !isNoActiveTurnError(err) {
			return err
		}
		if turn != nil {
			if err := s.settleManagedInputTurn(ctx, data.TaskID, data.SessionID, turn.ID, data.AgentExecutionID,
				messagequeue.ManagedInputStateUncertain, "provider_interrupted"); err != nil {
				return err
			}
		}
	}
	if err := s.closeInterruptedTurn(ctx, data.SessionID); err != nil {
		return err
	}
	return s.repo.SetSessionMetadataKey(ctx, data.SessionID, models.SessionMetaKeyInterruptedRecoveryPending,
		fmt.Sprintf("continuation:%s:%d", data.AgentExecutionID, data.PromptGeneration))
}

func (s *Service) closeInterruptedTurn(ctx context.Context, sessionID string) error {
	if s.turnService == nil {
		return nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil && !isNoActiveTurnError(err) {
		return err
	}
	if turn != nil {
		turn.CompletedAt = &turn.StartedAt
		if turn.Metadata == nil {
			turn.Metadata = map[string]any{}
		}
		turn.Metadata["interrupted"] = true
		if err := s.turnService.UpdateTurn(ctx, turn); err != nil {
			return err
		}
		s.activeTurns.CompareAndDelete(sessionID, turn.ID)
	}
	return nil
}
