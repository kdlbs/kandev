package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// MaxRetryCount is the maximum number of retry attempts before marking failed.
const MaxRetryCount = 4

// mustJSON marshals v to JSON string, returning "{}" on error. Used by
// scheduler activity log emitters to attach structured details to a
// run/activity row.
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// retryDelays defines the backoff schedule for run retries.
var retryDelays = []time.Duration{
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// HandleRunFailure handles a failed run by scheduling a retry or
// marking it as permanently failed and escalating to the CEO agent.
func (ss *SchedulerService) HandleRunFailure(
	ctx context.Context, run *models.Run, runErr error,
) error {
	if run.RetryCount < MaxRetryCount {
		return ss.scheduleRetry(ctx, run)
	}
	return ss.escalateFailure(ctx, run, runErr)
}

// scheduleRetry re-queues the run with exponential backoff + jitter.
func (ss *SchedulerService) scheduleRetry(ctx context.Context, run *models.Run) error {
	delay := retryDelayWithJitter(run.RetryCount)
	retryAt := time.Now().UTC().Add(delay)
	newCount := run.RetryCount + 1

	ss.logger.Info("scheduling run retry",
		zap.String("run_id", run.ID),
		zap.Int("retry_count", newCount),
		zap.Duration("delay", delay),
		zap.Time("retry_at", retryAt))

	return ss.repo.ScheduleRetry(ctx, run.ID, retryAt, newCount)
}

// escalateFailure marks the run as permanently failed, logs an inbox
// item, and queues an agent_error run for the CEO agent.
func (ss *SchedulerService) escalateFailure(
	ctx context.Context, run *models.Run, runErr error,
) error {
	if err := ss.FailRun(ctx, run.ID); err != nil {
		return fmt.Errorf("fail run: %w", err)
	}

	agent, err := ss.svc.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		ss.logger.Error("failed to get agent for escalation",
			zap.String("agent_id", run.AgentProfileID), zap.Error(err))
		return nil
	}

	// Log activity as agent error for inbox visibility.
	errMsg := "unknown"
	if runErr != nil {
		errMsg = runErr.Error()
	}
	ss.svc.LogActivityWithRun(ctx, agent.WorkspaceID, "system", "scheduler",
		"agent.error", "agent", run.AgentProfileID,
		mustJSON(map[string]string{
			"run_id": run.ID,
			"reason": run.Reason,
			"error":  errMsg,
		}), run.ID, "")

	ss.logger.Warn("run permanently failed after max retries",
		zap.String("run_id", run.ID),
		zap.String("agent", agent.Name),
		zap.Int("retry_count", run.RetryCount))

	// Queue agent_error run for CEO if one exists.
	ss.queueCEOAgentError(ctx, agent, run, errMsg)
	return nil
}

// queueCEOAgentError finds the CEO agent in the workspace and queues
// an agent_error run for it.
func (ss *SchedulerService) queueCEOAgentError(
	ctx context.Context, agent *models.AgentInstance,
	run *models.Run, errMsg string,
) {
	ceos, err := ss.svc.ListAgentInstancesFiltered(ctx, agent.WorkspaceID,
		service.AgentListFilter{Role: string(models.AgentRoleCEO)})
	if err != nil || len(ceos) == 0 {
		return
	}
	payload := mustJSON(map[string]string{
		"agent_profile_id": run.AgentProfileID,
		"run_id":           run.ID,
		"error":            errMsg,
	})
	_ = ss.QueueRun(ctx, ceos[0].ID, RunReasonAgentError, payload, "")
}

// retryDelayWithJitter returns the base delay for a given retry index
// plus up to 25% random jitter.
func retryDelayWithJitter(retryIndex int) time.Duration {
	if retryIndex < 0 || retryIndex >= len(retryDelays) {
		retryIndex = len(retryDelays) - 1
	}
	base := retryDelays[retryIndex]
	jitter := time.Duration(rand.Int63n(int64(base / 4))) //nolint:gosec
	return base + jitter
}
