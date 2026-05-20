package scheduler

import (
	"context"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// DashboardReactivityAdapter implements dashboard.ReactivityApplier by
// translating the dashboard-package types into scheduler.TaskMutation
// and calling SchedulerService.ApplyTaskMutation.
//
// Lives in the scheduler package so dashboard doesn't have to import
// scheduler types — it only sees the small interface declared in
// dashboard.ReactivityApplier.
type DashboardReactivityAdapter struct {
	scheduler *SchedulerService
}

// NewDashboardReactivityAdapter wraps a SchedulerService for the
// dashboard's ReactivityApplier interface.
func NewDashboardReactivityAdapter(s *SchedulerService) *DashboardReactivityAdapter {
	return &DashboardReactivityAdapter{scheduler: s}
}

// ApplyTaskMutation builds a TaskSnapshot from the office repo and runs
// the reactivity pipeline.
func (a *DashboardReactivityAdapter) ApplyTaskMutation(
	ctx context.Context,
	taskID string,
	preStatus string,
	change dashboard.TaskReactivityChange,
) (*dashboard.TaskReactivityResult, error) {
	if a.scheduler == nil {
		return &dashboard.TaskReactivityResult{}, nil
	}

	// Fetch the task search row (has assignee, parent, workspace).
	row, err := a.scheduler.repo.GetTaskByID(ctx, taskID)
	if err != nil || row == nil {
		return &dashboard.TaskReactivityResult{}, nil
	}
	snap := &TaskSnapshot{
		ID:                     row.ID,
		WorkspaceID:            row.WorkspaceID,
		State:                  preStatus, // pre-update DB state captured by caller
		AssigneeAgentProfileID: row.AssigneeAgentProfileID,
		ParentID:               row.ParentID,
	}
	if snap.State == "" {
		// Fallback: use the row's status (post-update if caller didn't capture preStatus).
		snap.State = row.Status
	}
	// If the caller captured the pre-update assignee (e.g. for handoff
	// detection), prefer that over the post-update value the row carries.
	if change.PrevAssigneeID != "" || (change.NewAssigneeID != nil && row.AssigneeAgentProfileID == *change.NewAssigneeID) {
		snap.AssigneeAgentProfileID = change.PrevAssigneeID
	}

	mutation := convertChangeToMutation(change)
	res, err := a.scheduler.ApplyTaskMutation(ctx, snap, mutation)
	if err != nil {
		return nil, err
	}

	out := &dashboard.TaskReactivityResult{}
	if res != nil {
		out.InterruptSessionID = res.InterruptSessionID
	}
	return out, nil
}

// convertChangeToMutation translates the dashboard-package types into
// the scheduler-package TaskMutation. Kept thin to avoid coupling the
// two packages.
func convertChangeToMutation(c dashboard.TaskReactivityChange) TaskMutation {
	out := TaskMutation{
		NewStatus:     c.NewStatus,
		NewAssigneeID: c.NewAssigneeID,
		ReopenIntent:  c.ReopenIntent,
		ResumeIntent:  c.ResumeIntent,
		ActorID:       c.ActorID,
		ActorType:     c.ActorType,
	}
	if c.Comment != nil {
		out.Comment = &MutationComment{
			ID:         c.Comment.ID,
			Body:       c.Comment.Body,
			AuthorType: c.Comment.AuthorType,
			AuthorID:   c.Comment.AuthorID,
		}
	}
	return out
}

// Compile-time check that the adapter implements the interface.
var _ dashboard.ReactivityApplier = (*DashboardReactivityAdapter)(nil)
