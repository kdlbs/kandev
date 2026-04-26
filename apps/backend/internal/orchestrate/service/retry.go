package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

// MaxRetryCount is the maximum number of retry attempts before marking failed.
const MaxRetryCount = 4

// retryDelays defines the backoff schedule for wakeup retries.
var retryDelays = []time.Duration{
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// HandleWakeupFailure handles a failed wakeup by scheduling a retry or
// marking it as permanently failed and escalating to the CEO agent.
func (s *Service) HandleWakeupFailure(
	ctx context.Context, wakeup *models.WakeupRequest, wakeupErr error,
) error {
	if wakeup.RetryCount < MaxRetryCount {
		return s.scheduleRetry(ctx, wakeup)
	}
	return s.escalateFailure(ctx, wakeup, wakeupErr)
}

// scheduleRetry re-queues the wakeup with exponential backoff + jitter.
func (s *Service) scheduleRetry(ctx context.Context, wakeup *models.WakeupRequest) error {
	delay := retryDelayWithJitter(wakeup.RetryCount)
	retryAt := time.Now().UTC().Add(delay)
	newCount := wakeup.RetryCount + 1

	s.logger.Info("scheduling wakeup retry",
		zap.String("wakeup_id", wakeup.ID),
		zap.Int("retry_count", newCount),
		zap.Duration("delay", delay),
		zap.Time("retry_at", retryAt))

	return s.repo.ScheduleRetry(ctx, wakeup.ID, retryAt, newCount)
}

// escalateFailure marks the wakeup as permanently failed, logs an inbox
// item, and queues an agent_error wakeup for the CEO agent.
func (s *Service) escalateFailure(
	ctx context.Context, wakeup *models.WakeupRequest, wakeupErr error,
) error {
	if err := s.FailWakeup(ctx, wakeup.ID); err != nil {
		return fmt.Errorf("fail wakeup: %w", err)
	}

	agent, err := s.repo.GetAgentInstance(ctx, wakeup.AgentInstanceID)
	if err != nil {
		s.logger.Error("failed to get agent for escalation",
			zap.String("agent_id", wakeup.AgentInstanceID), zap.Error(err))
		return nil
	}

	// Log activity as agent error for inbox visibility.
	errMsg := "unknown"
	if wakeupErr != nil {
		errMsg = wakeupErr.Error()
	}
	s.LogActivity(ctx, agent.WorkspaceID, "system", "scheduler",
		"agent.error", "agent", wakeup.AgentInstanceID,
		mustJSON(map[string]string{
			"wakeup_id": wakeup.ID,
			"reason":    wakeup.Reason,
			"error":     errMsg,
		}))

	s.logger.Warn("wakeup permanently failed after max retries",
		zap.String("wakeup_id", wakeup.ID),
		zap.String("agent", agent.Name),
		zap.Int("retry_count", wakeup.RetryCount))

	// Queue agent_error wakeup for CEO if one exists.
	s.queueCEOAgentError(ctx, agent, wakeup, errMsg)
	return nil
}

// queueCEOAgentError finds the CEO agent in the workspace and queues
// an agent_error wakeup for it.
func (s *Service) queueCEOAgentError(
	ctx context.Context, agent *models.AgentInstance,
	wakeup *models.WakeupRequest, errMsg string,
) {
	ceos, err := s.repo.ListAgentInstancesFiltered(ctx, agent.WorkspaceID,
		sqlite.AgentListFilter{Role: string(models.AgentRoleCEO)})
	if err != nil || len(ceos) == 0 {
		return
	}
	payload := mustJSON(map[string]string{
		"agent_instance_id": wakeup.AgentInstanceID,
		"wakeup_id":         wakeup.ID,
		"error":             errMsg,
	})
	_ = s.QueueWakeup(ctx, ceos[0].ID, WakeupReasonAgentError, payload, "")
}

// retryDelayWithJitter returns the base delay for a given retry index
// plus up to 25% random jitter.
func retryDelayWithJitter(retryIndex int) time.Duration {
	if retryIndex < 0 || retryIndex >= len(retryDelays) {
		retryIndex = len(retryDelays) - 1
	}
	base := retryDelays[retryIndex]
	jitter := time.Duration(rand.Int63n(int64(base / 4)))
	return base + jitter
}
