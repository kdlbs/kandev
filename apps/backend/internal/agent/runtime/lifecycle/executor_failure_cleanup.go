package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"strings"
	"time"
)

const executorCleanupFailedReason = "cleanup_failed"

func (m *Manager) cleanupStaleExecutionWithCause(ctx context.Context, execution *AgentExecution) error {
	prompt, startup := execution.promptGenerationSnapshot(), execution.startupAttemptSnapshot()
	cleanupErr := m.cleanupStaleExecution(ctx, execution)
	if cleanupErr == nil || errors.Is(cleanupErr, context.Canceled) || !m.supportsExecutorInspection(execution.RuntimeName) {
		return cleanupErr
	}
	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	target, err := m.executorObservationTarget(inspectCtx, execution)
	if err != nil || !m.disconnectStillCurrent(execution, prompt, startup) {
		return cleanupErr
	}
	observation, err := m.InspectExecutor(inspectCtx, target)
	if err != nil || (!observation.ConfirmedLoss() && !observation.ReportedUnavailable()) || !m.disconnectStillCurrent(execution, prompt, startup) {
		return cleanupErr
	}
	observation.Secondary = append(observation.Secondary, models.ExecutorOperationEvidence{Operation: "cleanup", Reason: executorCleanupFailureReason(cleanupErr), OccurredAt: time.Now().UTC()})
	if m.executorObservationHandler != nil && m.streamManager != nil {
		m.streamManager.start(func() {
			recordCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = m.executorObservationHandler(recordCtx, target, observation.Clone())
		})
	}
	return &ExecutorUnavailableError{Observation: observation, Cause: cleanupErr}
}

func executorCleanupFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "cleanup_timeout"
	}
	message := err.Error()
	if strings.Contains(message, "status 401") || strings.Contains(message, "HTTP 401") {
		return "cleanup_unauthorized"
	}
	return executorCleanupFailedReason
}
