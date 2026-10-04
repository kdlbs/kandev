package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) retainedRuntimeRetryForFailure(ctx context.Context, data watcher.AgentEventData) *retainedRuntimeRetry {
	if data.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime ||
		!data.PromptFailureDisposition.Valid() || data.OwnerKind != queueStatusScopeTask ||
		data.TaskID == "" || data.SessionID == "" || data.AgentExecutionID == "" || data.PromptGeneration == 0 {
		return nil
	}

	failure := data
	if data.ProviderError != nil {
		providerError := *data.ProviderError
		failure.ProviderError = &providerError
	}
	retained := &retainedRuntimeRetry{
		executionID: data.AgentExecutionID,
		generation:  data.PromptGeneration,
		profileID:   data.AgentProfileID,
		failure:     failure,
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.TaskID != data.TaskID ||
		(data.AgentProfileID != "" && session.AgentProfileID != data.AgentProfileID) {
		return retained
	}
	retained.nativeID = continuationNativeID(session)
	retained.identity = continuationSessionIdentity(session)
	return retained
}

func (s *Service) retainedRuntimeRetryDisposition(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
) retainedRuntimeRetryDisposition {
	if ctx.Err() != nil || entry == nil || entry.retainedRuntime == nil || s.agentManager == nil {
		return retainedRuntimeRetryBlocked
	}
	retained := entry.retainedRuntime
	if disposition := s.retainedRuntimeExecutionDisposition(ctx, sessionID, retained.executionID); disposition != retainedRuntimeRetryUsable {
		return disposition
	}
	if !s.retainedRuntimeRetrySessionMatches(ctx, taskID, sessionID, retained) ||
		!s.retainedRuntimeRetryGenerationMatches(ctx, sessionID, retained) ||
		!s.retainedRuntimeRetryQueueIsClear(ctx, sessionID) {
		return retainedRuntimeRetryBlocked
	}
	return retainedRuntimeRetryUsable
}

func (s *Service) retainedRuntimeExecutionDisposition(
	ctx context.Context,
	sessionID, expectedExecutionID string,
) retainedRuntimeRetryDisposition {
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, agentruntime.ErrNoExecutionForSession) || agentruntime.IsNotFound(err) {
			return retainedRuntimeRetryLost
		}
		return retainedRuntimeRetryBlocked
	}
	if executionID == "" {
		return retainedRuntimeRetryLost
	}
	if executionID != expectedExecutionID {
		return retainedRuntimeRetryBlocked
	}
	running, probeErr := s.probeAgentRunning(ctx, sessionID)
	if probeErr != nil {
		return retainedRuntimeRetryBlocked
	}
	if !running {
		return retainedRuntimeRetryLost
	}
	if !s.agentManager.IsAgentReadyForPrompt(ctx, sessionID) || s.executor == nil {
		return retainedRuntimeRetryBlocked
	}
	execution, ok := s.executor.GetExecutionBySession(sessionID)
	if !ok || execution == nil || execution.AgentExecutionID != expectedExecutionID {
		return retainedRuntimeRetryBlocked
	}
	return retainedRuntimeRetryUsable
}

func (s *Service) retainedRuntimeRetrySessionMatches(
	ctx context.Context,
	taskID, sessionID string,
	retained *retainedRuntimeRetry,
) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || !retainedRuntimeRetrySessionIdentityMatches(session, taskID, retained) {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.IsFromOffice || task.ArchivedAt != nil {
		return false
	}
	return true
}

func retainedRuntimeRetrySessionIdentityMatches(
	session *models.TaskSession,
	taskID string,
	retained *retainedRuntimeRetry,
) bool {
	if session == nil || retained == nil {
		return false
	}
	validState := session.State == models.TaskSessionStateWaitingForInput ||
		session.State == models.TaskSessionStateRunning
	return session.TaskID == taskID && validState && retained.profileID != "" &&
		session.AgentProfileID == retained.profileID && retained.nativeID != "" &&
		continuationNativeID(session) == retained.nativeID && retained.identity != ([32]byte{}) &&
		continuationSessionIdentity(session) == retained.identity
}

func (s *Service) retainedRuntimeRetryGenerationMatches(
	ctx context.Context,
	sessionID string,
	retained *retainedRuntimeRetry,
) bool {
	generationReader, ok := s.agentManager.(interface {
		GetPromptGenerationForSession(context.Context, string) (uint64, error)
	})
	if !ok {
		return false
	}
	generation, err := generationReader.GetPromptGenerationForSession(ctx, sessionID)
	if err != nil || generation != retained.generation ||
		!s.currentRetainedTurnFailureAttempt(sessionID, retained.executionID, retained.generation) {
		return false
	}
	return true
}

func (s *Service) retainedRuntimeRetryQueueIsClear(ctx context.Context, sessionID string) bool {
	if s.messageQueue != nil {
		pending, err := s.messageQueue.HasPendingForSession(ctx, sessionID)
		if err != nil || pending {
			return false
		}
	}
	return true
}

func (s *Service) retryRetainedRuntimePrompt(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
	prompt capturedPrompt,
) {
	started := false
	beforeDispatch := func() error {
		if s.retainedRuntimeRetryDisposition(ctx, taskID, sessionID, entry) != retainedRuntimeRetryUsable {
			return ErrResumeAttemptCancelled
		}
		if !started {
			entry.mu.Lock()
			entry.started++
			entry.mu.Unlock()
			started = true
		}
		return nil
	}
	onAccepted := func(turnID string) {
		if prompt.onAccepted != nil {
			prompt.onAccepted(turnID)
		}
		generation := uint64(0)
		if reader, ok := s.agentManager.(interface {
			GetPromptGenerationForSession(context.Context, string) (uint64, error)
		}); ok {
			generation, _ = reader.GetPromptGenerationForSession(context.WithoutCancel(ctx), sessionID)
		}
		entry.mu.Lock()
		entry.acceptedExecution = entry.retainedRuntime.executionID
		entry.acceptedGeneration = generation
		entry.mu.Unlock()
	}
	_, err := s.promptTask(ctx, taskID, sessionID, prompt.text, prompt.model, prompt.planMode, prompt.attachments,
		false, launchOriginAutomatic, promptTaskOptions{
			onAccepted:     onAccepted,
			beforeDispatch: beforeDispatch,
		})
	if err == nil || ctx.Err() != nil {
		return
	}
	var retainedFailure *agentruntime.RetainedPromptFailureError
	if errors.As(err, &retainedFailure) {
		return
	}
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry {
		return
	}
	s.logger.Warn("retained-runtime transient replay could not dispatch",
		zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	s.finishRetainedRetryWithoutDispatch(ctx, taskID, sessionID, entry, "refused")
}

func (s *Service) finishRetainedRetryWithoutDispatch(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
	disposition string,
) {
	if entry == nil || entry.retainedRuntime == nil || sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	if state == nil {
		return
	}
	state.mu.Lock()
	defer func() {
		state.mu.Unlock()
		release()
	}()
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry {
		return
	}
	data := entry.retainedRuntime.failure
	data.RecoveryDisposition = disposition
	entry.mu.Lock()
	data.RecoveryAttemptsStarted = entry.started
	entry.mu.Unlock()
	settlementCtx := context.WithoutCancel(ctx)
	if err := s.persistRetainedTurnFailureMessage(settlementCtx, data, data.TurnID); err != nil {
		s.logger.Warn("failed to persist retained provider failure after retry refusal",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return
	}

	if !s.clearTransientRetryEntryLocked(sessionID, state, entry) {
		return
	}
	s.retireTransientRetryNoticeLocked(sessionID, state)
	s.resolveTransientRetryMessagesLocked(settlementCtx, sessionID)
}
