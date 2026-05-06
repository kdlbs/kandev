package scheduler

import (
	"context"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// DashboardApprovalAdapter implements dashboard.ApprovalReactivityQueuer
// by translating dashboard-package run descriptors into scheduler
// RunContext values and calling QueueRunCtx.
//
// Lives in the scheduler package so the dashboard package doesn't
// import scheduler types.
type DashboardApprovalAdapter struct {
	scheduler *SchedulerService
}

// NewDashboardApprovalAdapter wraps a SchedulerService for the
// dashboard's ApprovalReactivityQueuer interface.
func NewDashboardApprovalAdapter(s *SchedulerService) *DashboardApprovalAdapter {
	return &DashboardApprovalAdapter{scheduler: s}
}

// QueueApprovalRuns queues each ApprovalRun via QueueRunCtx.
// Failures are best-effort: we log inside QueueRunCtx and skip the
// failing entry; the rest still queue.
func (a *DashboardApprovalAdapter) QueueApprovalRuns(
	ctx context.Context, runs []dashboard.ApprovalRun,
) error {
	if a.scheduler == nil {
		return nil
	}
	for _, w := range runs {
		if w.AgentID == "" || w.Reason == "" {
			continue
		}
		c := RunContext{
			Reason:          w.Reason,
			TaskID:          w.TaskID,
			WorkspaceID:     w.WorkspaceID,
			ActorID:         w.ActorID,
			ActorType:       w.ActorType,
			Role:            w.Role,
			DecisionComment: w.DecisionComment,
		}
		if err := a.scheduler.QueueRunCtx(ctx, w.AgentID, c); err != nil {
			a.scheduler.logger.Warn("approval run failed: " + err.Error())
		}
	}
	return nil
}

// Compile-time check.
var _ dashboard.ApprovalReactivityQueuer = (*DashboardApprovalAdapter)(nil)
