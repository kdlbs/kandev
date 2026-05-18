package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// staleRunThreshold is how old a run can be before it is considered stale.
const staleRunThreshold = 2 * time.Hour

// evaluateRunStaleness decides whether a run should be cancelled rather
// than executed. Returns (true, reason) when it should be cancelled.
func (si *SchedulerIntegration) evaluateRunStaleness(
	ctx context.Context,
	run *models.Run,
) (bool, string) {
	if run.RetryCount == 0 {
		return false, ""
	}
	if run.RequestedAt.IsZero() {
		return false, ""
	}
	if time.Since(run.RequestedAt) > staleRunThreshold {
		return true, "execution_too_old"
	}
	_ = ctx
	return false, ""
}

// cancelStaleRun marks the run cancelled, logs the event, and releases any checkout.
func (si *SchedulerIntegration) cancelStaleRun(
	ctx context.Context,
	run *models.Run,
	agent *models.AgentInstance,
	reason string,
) {
	si.logger.Info("cancelling stale run",
		zap.String("run_id", run.ID),
		zap.String("agent", agent.Name),
		zap.String("reason", reason),
		zap.Time("requested_at", run.RequestedAt))

	taskID := si.extractTaskID(run.Payload)
	si.releaseCheckoutIfNeeded(ctx, taskID)

	if err := si.svc.repo.CancelRun(ctx, run.ID, reason); err != nil {
		si.logger.Error("failed to cancel stale run",
			zap.String("run_id", run.ID), zap.Error(err))
	} else {
		si.svc.publishRunProcessed(ctx, run.ID, RunStatusCancelled, run)
	}

	si.svc.LogActivityWithRun(ctx, agent.WorkspaceID,
		"scheduler", "office-scheduler",
		"run_cancelled_stale", "run", run.ID,
		mustJSON(map[string]string{
			"agent":    agent.Name,
			"agent_id": agent.ID,
			"reason":   reason,
		}), run.ID, "")
}
