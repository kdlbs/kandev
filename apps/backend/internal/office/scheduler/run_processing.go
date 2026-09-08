package scheduler

import (
	"context"
	"database/sql"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// ClaimNextRun atomically claims the next eligible run from the queue.
// Returns nil, nil if no run is available.
func (ss *SchedulerService) ClaimNextRun(ctx context.Context) (*models.Run, error) {
	req, err := ss.repo.ClaimNextEligibleRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ss.logger.Info("run claimed",
		zap.String("id", req.ID),
		zap.String("agent", req.AgentProfileID),
		zap.String("reason", req.Reason))
	return req, nil
}

// FinishRun marks a claimed run as finished. outcome is written as NULL:
// this dormant parallel copy of the service-layer FinishRun has no
// production caller and therefore no established semantic label for what
// happened, so it is not bucketed as processed (docs/specs/
// task-delivery-ledger/spec.md, "Office run outcome").
func (ss *SchedulerService) FinishRun(ctx context.Context, id string) error {
	return ss.repo.FinishRun(ctx, id, RunStatusFinished, nil)
}

// FailRun marks a claimed run as failed. outcome is written as NULL, same
// as the service-layer FailRun: a failed run is bucketed on status alone.
func (ss *SchedulerService) FailRun(ctx context.Context, id string) error {
	return ss.repo.FinishRun(ctx, id, RunStatusFailed, nil)
}

// ProcessRunGuard checks if the agent is still eligible to be woken.
// Returns true if the run should proceed, false if it should be skipped.
func (ss *SchedulerService) ProcessRunGuard(ctx context.Context, run *models.Run) (bool, error) {
	agent, err := ss.svc.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		return false, err
	}
	switch agent.Status {
	case models.AgentStatusPaused, models.AgentStatusStopped, models.AgentStatusPendingApproval:
		ss.logger.Info("run skipped (agent not active)",
			zap.String("run_id", run.ID),
			zap.String("agent_status", string(agent.Status)))
		return false, nil
	}
	return true, nil
}
