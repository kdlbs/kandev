package service

import (
	"context"
	"fmt"
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
) (bool, string, error) {
	if run.RetryCount > 0 && !run.RequestedAt.IsZero() &&
		time.Since(run.RequestedAt) > staleRunThreshold {
		return true, "execution_too_old", nil
	}

	// Engine-emitted runs carry the workflow step that produced them. A task
	// can move to another step while the run waits in the queue, so compare
	// the queued step with the task's current step before launch. Legacy runs
	// do not include workflow_step_id and retain the existing behaviour.
	payload := ParseRunPayload(run.Payload)
	expectedStepID := payload["workflow_step_id"]
	taskID := payload["task_id"]
	if expectedStepID == "" || taskID == "" {
		return false, "", nil
	}
	currentStepID, err := si.svc.repo.GetTaskWorkflowStepID(ctx, taskID)
	if err != nil {
		return false, "", fmt.Errorf("resolve current workflow step for task %s: %w", taskID, err)
	}
	if currentStepID != expectedStepID {
		return true, "workflow_step_changed", nil
	}
	return false, "", nil
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

	si.releaseCheckoutIfNeeded(ctx, run)

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
