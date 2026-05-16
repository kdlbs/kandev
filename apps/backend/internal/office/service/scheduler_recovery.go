package service

import (
	"context"

	"go.uber.org/zap"
)

// maxRecoveryPerTick caps the number of unstarted tasks recovered in one tick.
const maxRecoveryPerTick = 5

// recoverUnstartedTasks queues a task_assigned run for TODO tasks that were
// never picked up by the scheduler, guarded by the recovery lookback window.
func (si *SchedulerIntegration) recoverUnstartedTasks(ctx context.Context) {
	lookbackHours := si.svc.GetRecoveryLookbackHours()

	tasks, err := si.svc.repo.ListUnstartedTasks(ctx, lookbackHours, maxRecoveryPerTick)
	if err != nil {
		si.logger.Error("recovery sweep: list unstarted tasks failed", zap.Error(err))
		return
	}

	for _, t := range tasks {
		si.logger.Info("recovery sweep: re-queueing unstarted task",
			zap.String("task_id", t.ID),
			zap.String("agent_profile_id", t.AssigneeAgentProfileID))

		payload := mustJSON(map[string]string{"task_id": t.ID})
		if err := si.svc.QueueRun(ctx, t.AssigneeAgentProfileID,
			RunReasonTaskAssigned, payload, ""); err != nil {
			si.logger.Error("recovery sweep: queue run failed",
				zap.String("task_id", t.ID), zap.Error(err))
		}
	}
}
