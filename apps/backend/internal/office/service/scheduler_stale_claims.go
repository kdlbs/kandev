package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// TaskRunExecutionProbe reports whether a task-bound run's agent execution
// is still alive. The orchestrator task starter implements it; a starter
// without it keeps the plain stale-claim requeue.
type TaskRunExecutionProbe interface {
	TaskRunExecutionLive(ctx context.Context, taskID, agentProfileID, sessionID string) (bool, error)
}

// renewLiveStaleClaims keeps a stale task-bound claim while its execution is
// still alive. Requeueing such a run starts a second launch for the same
// task session, which fails because the first execution still owns it. The
// run stays claimed and settles through the normal completion path when the
// execution ends; a later sweep requeues it if the execution is gone.
func (si *SchedulerIntegration) renewLiveStaleClaims(ctx context.Context, cutoff, now time.Time) {
	probe, ok := si.svc.taskStarter.(TaskRunExecutionProbe)
	if !ok {
		return
	}
	runs, err := si.svc.repo.ListStaleClaimedTaskRuns(ctx, cutoff)
	if err != nil {
		if ctx.Err() == nil {
			si.logger.Warn("list stale claimed task runs failed", zap.Error(err))
		}
		return
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			return
		}
		live, err := probe.TaskRunExecutionLive(ctx, run.TaskID, run.AgentProfileID, run.SessionID)
		if err != nil {
			si.logger.Warn("stale claim execution probe failed",
				zap.String("run_id", run.ID), zap.String("task_id", run.TaskID), zap.Error(err))
			continue
		}
		if !live {
			continue
		}
		renewed, err := si.svc.repo.RenewStaleClaim(ctx, run.ID, cutoff, now)
		if err != nil {
			si.logger.Warn("renew stale claim failed", zap.String("run_id", run.ID), zap.Error(err))
			continue
		}
		if renewed {
			si.logger.Info("kept stale claim of a live execution",
				zap.String("run_id", run.ID), zap.String("task_id", run.TaskID))
		}
	}
}
