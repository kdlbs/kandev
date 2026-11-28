package models

import "time"

// WorkspaceInventoryRepairFence is server-read lifecycle state, not launch authority.
type WorkspaceInventoryRepairFence struct {
	TaskUpdatedAt    time.Time `json:"task_updated_at"`
	WorkflowStepID   string    `json:"workflow_step_id"`
	SessionUpdatedAt time.Time `json:"session_updated_at"`
	SessionState     string    `json:"session_state"`
	RouteKnown       bool      `json:"route_known"`
	RouteGeneration  int64     `json:"route_generation"`
	RouteState       string    `json:"route_state"`
	RouteUpdatedAt   time.Time `json:"route_updated_at"`
}

func (f *WorkspaceInventoryRepairFence) Equal(other *WorkspaceInventoryRepairFence) bool {
	if f == nil || other == nil {
		return f == other
	}
	return f.TaskUpdatedAt.Equal(other.TaskUpdatedAt) && f.WorkflowStepID == other.WorkflowStepID &&
		f.SessionUpdatedAt.Equal(other.SessionUpdatedAt) && f.SessionState == other.SessionState &&
		f.RouteKnown == other.RouteKnown && f.RouteGeneration == other.RouteGeneration &&
		f.RouteState == other.RouteState && f.RouteUpdatedAt.Equal(other.RouteUpdatedAt)
}
