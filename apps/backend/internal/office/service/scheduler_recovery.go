package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// maxRecoveryPerTick caps the number of unstarted tasks recovered in one tick.
const maxRecoveryPerTick = 5

// OfficeRecoveryHandler drives Office-only unstarted-task maintenance from
// the shared cron loop. Generic queued-run dispatch remains owned by the
// runs scheduler.
type OfficeRecoveryHandler struct {
	scheduler *SchedulerIntegration
	logger    *logger.Logger
}

// NewOfficeRecoveryHandler constructs the Office maintenance handler for a
// scheduler integration.
func NewOfficeRecoveryHandler(si *SchedulerIntegration) *OfficeRecoveryHandler {
	return &OfficeRecoveryHandler{
		scheduler: si,
		logger:    si.svc.logger.WithFields(zap.String("component", "office-recovery")),
	}
}

// Name implements scheduler/cron.Handler.
func (h *OfficeRecoveryHandler) Name() string { return "office_recovery" }

// Tick implements scheduler/cron.Handler. The adoption check prevents an
// Office-enabled but Kanban-only installation from scanning task rows.
func (h *OfficeRecoveryHandler) Tick(ctx context.Context) error {
	adopted, err := h.scheduler.svc.repo.HasOfficeAdoption(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("check Office adoption: %w", err)
	}
	if !adopted {
		return nil
	}
	h.scheduler.recoverUnstartedTasks(ctx, h.logger)
	return nil
}

// recoverUnstartedTasks queues a task_assigned run for authoritative Office
// TODO tasks that were never picked up, guarded by the lookback window.
func (si *SchedulerIntegration) recoverUnstartedTasks(ctx context.Context, log *logger.Logger) {
	lookbackHours := si.svc.GetRecoveryLookbackHours()

	tasks, err := si.svc.repo.ListUnstartedTasks(ctx, lookbackHours, maxRecoveryPerTick)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("recovery sweep: list unstarted tasks failed", zap.Error(err))
		return
	}

	for _, t := range tasks {
		log.Info("recovery sweep: re-queueing unstarted task",
			zap.String("task_id", t.ID),
			zap.String("agent_profile_id", t.AssigneeAgentProfileID))

		payload := mustJSON(map[string]string{"task_id": t.ID})
		if err := si.svc.QueueRun(ctx, t.AssigneeAgentProfileID,
			RunReasonTaskAssigned, payload, ""); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("recovery sweep: queue run failed",
				zap.String("task_id", t.ID), zap.Error(err))
		}
	}
}
