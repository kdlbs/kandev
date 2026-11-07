package service

import (
	"context"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

type taskRunCapacityGate interface{ TaskRunCapacityEnabled() bool }

func (si *SchedulerIntegration) taskCapacityContext(ctx context.Context, run *runmodels.Run, taskID string) context.Context {
	if taskID == "" || run == nil || runmodels.PayloadTaskID(run.Payload) != taskID {
		return ctx
	}
	gate, ok := si.svc.runSessionLauncher.(taskRunCapacityGate)
	if !ok || !gate.TaskRunCapacityEnabled() {
		return ctx
	}
	return runmodels.WithTaskCapacityRun(ctx, run.ID, taskID)
}
