package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// MaxRetryCount is the maximum number of retry attempts before marking failed.
const MaxRetryCount = 4

// retryDelays defines the backoff schedule for run retries.
var retryDelays = []time.Duration{
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// HandleRunFailure handles a failed run by scheduling a retry or
// marking it as permanently failed and escalating to the CEO agent.
func (s *Service) HandleRunFailure(
	ctx context.Context, run *models.Run, runErr error,
) error {
	if run.RetryCount < MaxRetryCount {
		errMsg := ""
		if runErr != nil {
			errMsg = runErr.Error()
		}
		if isRateLimitError(errMsg) {
			if resetAt := parseRateLimitResetTime(errMsg, time.Now().UTC()); resetAt != nil {
				return s.scheduleRetryAt(ctx, run, *resetAt, "rate_limit_parsed")
			}
		}
		return s.scheduleRetry(ctx, run)
	}
	return s.escalateFailure(ctx, run, runErr)
}

// scheduleRetry re-queues the run with exponential backoff + jitter.
func (s *Service) scheduleRetry(ctx context.Context, run *models.Run) error {
	if stale, reason := isRetryStale(run); stale {
		return s.cancelRetry(ctx, run, reason)
	}
	delay := retryDelayWithJitter(run.RetryCount)
	retryAt := time.Now().UTC().Add(delay)
	newCount := run.RetryCount + 1

	s.logger.Info("scheduling run retry",
		zap.String("run_id", run.ID),
		zap.Int("retry_count", newCount),
		zap.Duration("delay", delay),
		zap.Time("retry_at", retryAt),
		zap.String("source", "backoff"))

	return s.repo.ScheduleRetry(ctx, run.ID, retryAt, newCount)
}

// scheduleRetryAt re-queues the run for an explicit absolute time,
// logging the source and the parsed reset time for observability.
func (s *Service) scheduleRetryAt(
	ctx context.Context,
	run *models.Run,
	retryAt time.Time,
	source string,
) error {
	if stale, reason := isRetryStale(run); stale {
		return s.cancelRetry(ctx, run, reason)
	}
	newCount := run.RetryCount + 1

	s.logger.Info("scheduling run retry",
		zap.String("run_id", run.ID),
		zap.Int("retry_count", newCount),
		zap.Time("retry_at", retryAt),
		zap.Time("parsed_reset_at", retryAt),
		zap.String("source", source))

	return s.repo.ScheduleRetry(ctx, run.ID, retryAt, newCount)
}

// retryMaxAge is the maximum age of the original run before retries are abandoned.
const retryMaxAge = 24 * time.Hour

// isRetryStale returns (true, reason) when retrying the run would be pointless.
func isRetryStale(run *models.Run) (bool, string) {
	if time.Since(run.RequestedAt) > retryMaxAge {
		return true, "run_too_old"
	}
	return false, ""
}

// cancelRetry cancels a run that is too stale to retry.
func (s *Service) cancelRetry(ctx context.Context, run *models.Run, reason string) error {
	s.logger.Info("cancelling stale run retry",
		zap.String("run_id", run.ID),
		zap.String("reason", reason))
	if err := s.repo.CancelRun(ctx, run.ID, reason); err != nil {
		return err
	}
	s.publishRunProcessed(ctx, run.ID, RunStatusCancelled, run)
	return nil
}

// escalateFailure marks the run as permanently failed, logs an inbox
// item, and queues an agent_error run for the CEO agent.
func (s *Service) escalateFailure(
	ctx context.Context, run *models.Run, runErr error,
) error {
	if err := s.FailRun(ctx, run.ID); err != nil {
		return fmt.Errorf("fail run: %w", err)
	}

	agent, err := s.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		s.logger.Error("failed to get agent for escalation",
			zap.String("agent_id", run.AgentProfileID), zap.Error(err))
		return nil
	}

	// Log activity as agent error for inbox visibility.
	errMsg := "unknown"
	if runErr != nil {
		errMsg = runErr.Error()
	}
	s.LogActivityWithRun(ctx, agent.WorkspaceID, "system", "scheduler",
		"agent.error", "agent", run.AgentProfileID,
		mustJSON(map[string]string{
			"run_id": run.ID,
			"reason": run.Reason,
			"error":  errMsg,
		}), run.ID, "")

	s.logger.Warn("run permanently failed after max retries",
		zap.String("run_id", run.ID),
		zap.String("agent", agent.Name),
		zap.Int("retry_count", run.RetryCount))

	// Queue agent_error run for CEO if one exists.
	s.queueCEOAgentError(ctx, agent, run, errMsg)
	return nil
}

// queueCEOAgentError finds the CEO agent in the workspace and queues
// an agent_error run for it.
func (s *Service) queueCEOAgentError(
	ctx context.Context, agent *models.AgentInstance,
	run *models.Run, errMsg string,
) {
	ceos, err := s.ListAgentInstancesFiltered(ctx, agent.WorkspaceID,
		AgentListFilter{Role: string(models.AgentRoleCEO)})
	if err != nil || len(ceos) == 0 {
		return
	}
	payload := mustJSON(map[string]string{
		"agent_profile_id": run.AgentProfileID,
		"run_id":           run.ID,
		"error":            errMsg,
	})
	_ = s.QueueRun(ctx, ceos[0].ID, RunReasonAgentError, payload, "")
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
