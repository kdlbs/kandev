package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) handleAgentTurnFailed(ctx context.Context, data watcher.AgentEventData) {
	if !validRetainedTurnFailureEvent(data) {
		return
	}
	data = s.withPromptAttemptEvidence(data)
	if data.DynamicRouteAttempt {
		s.handleAgentFailed(ctx, data)
		return
	}
	if !s.currentRetainedTurnFailureAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration) {
		return
	}

	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	lock.Lock()
	guardHeld := true
	releaseGuard := func() {
		if guardHeld {
			lock.Unlock()
			release()
			guardHeld = false
		}
	}
	defer func() {
		releaseGuard()
	}()
	if !s.retainedTurnFailureGuardAllows(data) {
		return
	}
	session, task, ok := s.loadRetainedTurnFailureOwner(ctx, data)
	if !ok {
		return
	}
	if s.handleAutomationOwnedTurnFailure(ctx, task, data, releaseGuard) {
		return
	}
	if task.IsFromOffice || !s.currentRetainedTurnFailureAttempt(
		data.SessionID, data.AgentExecutionID, data.PromptGeneration,
	) {
		return
	}
	s.settleRetainedTurnFailure(ctx, data, session)
}

func validRetainedTurnFailureEvent(data watcher.AgentEventData) bool {
	return data.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime &&
		data.PromptFailureDisposition.Valid() && data.TaskID != "" && data.SessionID != "" &&
		data.AgentExecutionID != "" && data.PromptGeneration != 0 &&
		data.OwnerKind == string(agentruntime.ExecutionOwnerTask)
}

func (s *Service) retainedTurnFailureGuardAllows(data watcher.AgentEventData) bool {
	return !s.isCancelInFlight(data.SessionID) &&
		s.resumeAttemptAllowsExecution(data.SessionID, data.AgentExecutionID, data.AttemptID)
}

func (s *Service) loadRetainedTurnFailureOwner(
	ctx context.Context,
	data watcher.AgentEventData,
) (*models.TaskSession, *models.Task, bool) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.TaskID != data.TaskID ||
		session.State != models.TaskSessionStateRunning ||
		(session.AgentExecutionID != "" && session.AgentExecutionID != data.AgentExecutionID) {
		return nil, nil, false
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || taskArchived(task) {
		return nil, nil, false
	}
	return session, task, true
}

func (s *Service) handleAutomationOwnedTurnFailure(
	ctx context.Context,
	task *models.Task,
	data watcher.AgentEventData,
	releaseGuard func(),
) bool {
	if task == nil || !models.IsAutomationTaskOrigin(task.Origin) {
		return false
	}
	// Automation owns terminal turn handling and concurrency-slot release.
	// Reuse that owner under the identity guard instead of parking the
	// failure in interactive Chat state.
	dispatch := s.handleAgentFailedLocked(ctx, data)
	releaseGuard()
	if dispatch != nil {
		s.startAgentFailureRecovery(dispatch)
	}
	return true
}

func (s *Service) settleRetainedTurnFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) {
	priorRetry, priorAttempt, priorStarted := s.retainedRetryProgress(data.SessionID)
	if s.handleTransientFailure(ctx, data) {
		return
	}
	if priorRetry {
		data.RecoveryAttemptsStarted = priorStarted
		if priorStarted >= transientMaxAttempts {
			data.RecoveryDisposition = recoveryDispositionExhausted
		} else {
			data.RecoveryDisposition = "refused"
		}
		if priorAttempt > 0 {
			s.resetTransientRetry(data.SessionID)
		}
	} else if routingerr.Decide(
		routingerr.ContextKanban,
		classifyKanbanFailure(data),
		time.Now().UTC(),
	) == routingerr.DecisionShortRetry {
		data.RecoveryDisposition = "refused"
	}

	failedTurnID, err := s.markRetainedFailureTurn(ctx, data)
	if err != nil {
		s.logger.Warn("failed to mark retained provider turn as failed",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID),
			zap.String("execution_id", data.AgentExecutionID), zap.Uint64("prompt_generation", data.PromptGeneration),
			zap.Error(err))
		return
	}
	inputState, inputOutcome := retainedFailureManagedInputDisposition(data)
	if err := s.settleManagedInputTurn(
		ctx, data.TaskID, data.SessionID, failedTurnID, data.AgentExecutionID, inputState, inputOutcome,
	); err != nil {
		s.logger.Warn("failed to settle managed input after provider turn failure",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID),
			zap.String("turn_id", failedTurnID), zap.Error(err))
		return
	}
	if err := s.persistRetainedTurnFailureMessage(ctx, data, failedTurnID); err != nil {
		s.logger.Warn("failed to persist retained provider turn failure",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID),
			zap.String("turn_id", failedTurnID), zap.Error(err))
		return
	}
	if err := s.completeTurnForTaskSessionCheckedOwned(ctx, data.TaskID, data.SessionID, failedTurnID); err != nil {
		s.logger.Warn("failed to complete retained provider turn",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID),
			zap.String("turn_id", failedTurnID), zap.Error(err))
		return
	}
	if _, changed := s.updateTaskSessionStateWithHook(
		ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false, nil, session,
	); !changed {
		return
	}
	s.writeTaskReviewState(ctx, data.TaskID, data.SessionID)
	s.clearPromptAttemptEvidence(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
}

func retainedFailureManagedInputDisposition(
	data watcher.AgentEventData,
) (messagequeue.ManagedInputState, string) {
	if data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved {
		return messagequeue.ManagedInputStateFailed, "execution_failed_without_observed_effects"
	}
	return messagequeue.ManagedInputStateUncertain, "execution_failed_effects_unknown"
}

func (s *Service) retainedRetryProgress(sessionID string) (bool, int, int) {
	value, ok := s.transientRetries.Load(sessionID)
	if !ok {
		return false, 0, 0
	}
	entry, ok := value.(*transientRetryEntry)
	if !ok || entry.retainedRuntime == nil {
		return false, 0, 0
	}
	entry.mu.Lock()
	attempt, started := entry.attempt, entry.started
	entry.mu.Unlock()
	return true, attempt, started
}

func (s *Service) currentRetainedTurnFailureAttempt(sessionID, executionID string, generation uint64) bool {
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.evidenceKnown && evidence.executionID == executionID &&
		evidence.promptGeneration == generation
}

func (s *Service) markRetainedFailureTurn(ctx context.Context, data watcher.AgentEventData) (string, error) {
	turnID := data.TurnID
	if turnID == "" {
		var err error
		turnID, err = s.peekActiveTurnID(ctx, data.SessionID)
		if err != nil {
			return "", err
		}
	}
	if err := s.verifyExpectedTurnOwnership(ctx, data.SessionID, turnID); err != nil {
		return "", err
	}
	if turnID == "" || s.turnService == nil {
		return turnID, nil
	}
	if err := s.turnService.PatchTurnMetadata(ctx, data.SessionID, turnID, map[string]interface{}{
		models.TurnMetaKeyErrorTerminated: true,
	}); err != nil {
		return "", err
	}
	return turnID, nil
}

func (s *Service) persistRetainedTurnFailureMessage(
	ctx context.Context,
	data watcher.AgentEventData,
	turnID string,
) error {
	if s.messageCreator == nil {
		return fmt.Errorf("turn failure message creator is unavailable")
	}
	message := retainedTurnFailureMessage(data)
	classified := routingerr.Classify(routingerr.Input{
		Phase: routingerr.PhasePromptSend, ProviderID: data.AgentID, Stderr: message,
	})
	metadata := map[string]interface{}{
		"variant":           "error",
		"scope":             "turn",
		"failure_scope":     "turn",
		"failure_code":      string(classified.Code),
		"recovery_actions":  false,
		"runtime_retained":  true,
		"session_id":        data.SessionID,
		"task_id":           data.TaskID,
		"execution_id":      data.AgentExecutionID,
		"prompt_generation": data.PromptGeneration,
		"attempts_started":  data.RecoveryAttemptsStarted,
	}
	if data.RecoveryDisposition != "" {
		metadata["recovery_disposition"] = data.RecoveryDisposition
	}
	if turnID != "" {
		metadata["turn_id"] = turnID
	}
	if data.AgentID != "" {
		metadata["agent_id"] = data.AgentID
	}
	if data.ProviderError != nil && data.ProviderError.Valid() {
		metadata["provider_error"] = data.ProviderError
	}
	messageID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf(
		"session-turn-failure:%s:%s:%d", data.SessionID, data.AgentExecutionID, data.PromptGeneration,
	))).String()
	return s.messageCreator.CreateSessionMessageIdempotent(
		ctx, messageID, data.TaskID, message, data.SessionID,
		string(v1.MessageTypeStatus), turnID, metadata, false,
	)
}

func retainedTurnFailureMessage(data watcher.AgentEventData) string {
	message := data.ErrorMessage
	if data.ProviderError != nil && data.ProviderError.Valid() {
		message = data.ProviderError.Message
	}
	message = streams.SanitizeProviderMessage(message)
	if strings.TrimSpace(message) == "" {
		return defaultAgentFailedMessage
	}
	return message
}
