package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) createTransientRetryStatusMessage(
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) {
	state, release := s.acquireTransientRetryNoticeState(data.SessionID)
	if state == nil {
		return
	}
	state.mu.Lock()
	s.createTransientRetryStatusMessageLocked(state, ctx, data, classified, attempt, delay, retryAt)
	state.mu.Unlock()
	release()
}

func (s *Service) createTransientRetryStatusMessageLocked(
	state *transientRetryNoticeState,
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) {
	if s.messageCreator == nil || state.retired.Load() {
		return
	}
	if value, ok := s.transientRetries.Load(data.SessionID); ok {
		if entry, ok := value.(*transientRetryEntry); ok {
			entry.mu.Lock()
			data.RecoveryAttemptsStarted = entry.started
			entry.mu.Unlock()
		}
	}
	content, meta := transientRetryStatusMessage(data, classified, attempt, delay, retryAt)
	if s.transientRetryMessages != nil {
		s.updateTransientRetryStatusMessageLocked(ctx, data, content, meta)
		return
	}
	s.createTransientRetryStatusMessageRecord(ctx, data, content, meta)
}

func transientRetryStatusMessage(
	data watcher.AgentEventData,
	classified *routingerr.Error,
	attempt int,
	delay time.Duration,
	retryAt time.Time,
) (string, map[string]interface{}) {
	secs := int(delay.Seconds())
	label := transientFailureLabel(classified)
	content := fmt.Sprintf("%s. Retrying in %ds (attempt %d/%d)", label, secs, attempt, transientMaxAttempts)
	phase := data.RecoveryPhase
	if phase == "" {
		phase = "waiting"
	}
	cancelAction := wsRecoveryAction(data.TaskID, data.SessionID, recoverActionCancelRetry,
		"Cancel", "x", "Stop retrying and choose how to recover", recoveryCancelRetryButtonTestID)
	meta := map[string]interface{}{
		metaKeyVariant:     metaVariantWarning,
		"retrying":         true,
		"recovery_mode":    data.RecoveryMode,
		"recovery_phase":   phase,
		"attempts_started": data.RecoveryAttemptsStarted,
		"attempt":          attempt,
		"max_attempts":     transientMaxAttempts,
		"retry_in_seconds": secs,
		"retry_at":         retryAt.UTC().Format(time.RFC3339Nano),
		metaKeySessionID:   data.SessionID,
		metaKeyTaskID:      data.TaskID,
		"actions":          []map[string]interface{}{cancelAction},
	}
	if classified != nil {
		meta["failure_code"] = string(classified.Code)
	}
	providerID := data.AgentID
	if providerError := data.ProviderError; providerError != nil {
		if providerError.ProviderID != "" {
			providerID = providerError.ProviderID
		}
		if modelID := routingerr.Sanitize(providerError.ModelID); modelID != "" {
			meta["model_id"] = modelID
		}
	}
	if providerID = routingerr.Sanitize(providerID); providerID != "" {
		meta["provider_name"] = providerID
	}
	return content, meta
}

func (s *Service) createTransientRetryStatusMessageRecord(
	ctx context.Context,
	data watcher.AgentEventData,
	content string,
	meta map[string]interface{},
) {
	turnID := ""
	if data.RecoveryMode != recoveryModeContinue {
		turnID = s.getActiveTurnID(data.SessionID)
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		data.TaskID,
		content,
		data.SessionID,
		string(v1.MessageTypeStatus),
		turnID,
		meta,
		false,
	); err != nil {
		s.logger.Warn("failed to create transient retry status message",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}
}

func (s *Service) updateTransientRetryStatusMessageLocked(
	ctx context.Context,
	data watcher.AgentEventData,
	content string,
	metadata map[string]interface{},
) {
	// state.mu intentionally covers this DB I/O. Retirement and notice writes
	// must serialize so cancellation cannot delete a row that this update then
	// resurrects.
	messages, err := s.transientRetryMessages.ListMessages(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to list transient retry status messages before write",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	notices := transientRetryNotices(messages, data.TaskID, data.SessionID)
	if len(notices) == 0 {
		s.createTransientRetryStatusMessageRecord(ctx, data, content, metadata)
		return
	}

	current := notices[0]
	current.Content = content
	current.Type = models.MessageTypeStatus
	current.Metadata = metadata
	current.RequestsInput = false
	if err := s.transientRetryMessages.UpdateMessage(ctx, current); err != nil {
		s.logger.Warn("failed to update transient retry status message",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("message_id", current.ID),
			zap.Error(err))
		return
	}

	for _, duplicate := range notices[1:] {
		if err := s.transientRetryMessages.DeleteMessage(ctx, duplicate.ID); err != nil {
			s.logger.Warn("failed to delete duplicate transient retry status message",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("message_id", duplicate.ID),
				zap.Error(err))
		}
	}
}

func transientRetryNotices(messages []*models.Message, taskID, sessionID string) []*models.Message {
	notices := make([]*models.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil || message.TaskID != taskID || message.TaskSessionID != sessionID ||
			message.Type != models.MessageTypeStatus || message.Metadata == nil {
			continue
		}
		if retrying, ok := message.Metadata["retrying"].(bool); !ok || !retrying {
			continue
		}
		notices = append(notices, message)
	}
	sort.SliceStable(notices, func(i, j int) bool {
		if notices[i].CreatedAt.Equal(notices[j].CreatedAt) {
			return notices[i].ID > notices[j].ID
		}
		return notices[i].CreatedAt.After(notices[j].CreatedAt)
	})
	return notices
}

func classifyKanbanFailure(data watcher.AgentEventData) *routingerr.Error {
	providerID, message, resetHint, occurredAt := kanbanFailureDetails(data)
	classified := routingerr.Classify(routingerr.Input{
		Phase:      kanbanFailurePhase(data),
		ProviderID: providerID,
		ResetHint:  resetHint,
		OccurredAt: occurredAt,
		Stderr:     message,
	})
	if isExactUnknownDynamicProviderFailure(data, classified) {
		// An exact terminal provider diagnostic can identify the unknown result
		// shape without changing the global post-start classifier contract.
		classified = cloneRoutingErrorWithCode(classified, routingerr.CodeUnknownProvider)
	}
	return classified
}

func kanbanFailureDetails(data watcher.AgentEventData) (string, string, *time.Time, time.Time) {
	providerID, message := data.AgentID, data.ErrorMessage
	var resetHint *time.Time
	var occurredAt time.Time
	if providerError := data.ProviderError; providerError != nil {
		providerID = kanbanFailureProviderID(providerID, providerError.ProviderID)
		if providerError.Message != "" {
			message = providerError.Message
		}
		resetHint = providerError.ResetAt
		occurredAt = providerError.OccurredAt
	}
	return providerID, message, resetHint, occurredAt
}

func kanbanFailureProviderID(agentID, providerID string) string {
	// Provider rules are keyed by agent ID. OpenCode diagnostics carry the
	// model-provider ID instead ("opencode-go"), which has no rules; keeping
	// the agent ID there lets its usage-limit rule advance dynamic routing.
	if providerID != "" && !routingerr.HasProviderRules(agentID) && routingerr.HasProviderRules(providerID) {
		return providerID
	}
	return agentID
}

func kanbanFailurePhase(data watcher.AgentEventData) routingerr.Phase {
	if !data.DynamicRouteAttempt {
		return routingerr.PhasePromptSend
	}
	if data.EffectObserved {
		return routingerr.PhaseToolExecution
	}
	if data.OutputObserved || !data.EvidenceKnown {
		// Unknown attempt state remains outside the pre-result phases because
		// dynamic routing requires explicit evidence before it can advance.
		return routingerr.PhaseStreaming
	}
	return routingerr.PhasePromptSend
}

func isExactUnknownDynamicProviderFailure(data watcher.AgentEventData, classified *routingerr.Error) bool {
	return hasNoDynamicFailureActivity(data) &&
		hasCompleteProviderDiagnostic(data.ProviderError) &&
		isUnknownPostStartClassification(classified)
}

func hasNoDynamicFailureActivity(data watcher.AgentEventData) bool {
	return data.DynamicRouteAttempt && data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved
}

func hasCompleteProviderDiagnostic(providerError *streams.ProviderError) bool {
	return providerError != nil && providerError.Valid() && providerError.DiagnosticIdentityComplete &&
		completeProviderDiagnosticSource(providerError.Source)
}

func isUnknownPostStartClassification(classified *routingerr.Error) bool {
	return classified != nil && classified.Code == routingerr.CodeAgentRuntime &&
		classified.Class == routingerr.ClassUnclassified && classified.ClassifierRule == "phase.poststart.unknown"
}

func cloneRoutingErrorWithCode(classified *routingerr.Error, code routingerr.Code) *routingerr.Error {
	if classified == nil {
		return nil
	}
	clone := *classified
	clone.Code = code
	clone.Class = routingerr.ClassForCode(code)
	return &clone
}

func transientFailureLabel(classified *routingerr.Error) string {
	if classified == nil {
		return "Provider temporarily unavailable"
	}
	switch classified.Code {
	case routingerr.CodeModelCapacity:
		return "Model at capacity"
	case routingerr.CodeNetworkUnavailable:
		return "Network unavailable"
	case routingerr.CodeProviderOverloaded:
		return "Provider overloaded"
	case routingerr.CodeRateLimited:
		return "Rate limited"
	case routingerr.CodeAgentTransportLost:
		return "Agent connection lost"
	case routingerr.CodeProviderResourceExhausted:
		return "Resource exhausted"
	default:
		return "Provider temporarily unavailable"
	}
}

func transientFailureManualMessage(classified *routingerr.Error) string {
	return transientFailureLabel(classified) + ". Resume to try again, or start a fresh session."
}
