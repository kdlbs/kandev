package dto

import v1 "github.com/kandev/kandev/pkg/api/v1"

// TaskGraphDTO is the GET /api/v1/tasks/:id/graph response.
type TaskGraphDTO struct {
	Mode           string             `json:"mode"`
	AvailableModes []string           `json:"available_modes"`
	FocusTaskID    string             `json:"focus_task_id"`
	Root           *TaskGraphRootDTO  `json:"root,omitempty"`
	Nodes          []TaskGraphNodeDTO `json:"nodes"`
	Edges          []TaskGraphEdgeDTO `json:"edges"`
	Truncated      bool               `json:"truncated"`
}

// TaskGraphRootDTO names the subtree root in subtasks mode.
type TaskGraphRootDTO struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// TaskGraphNodeDTO is one displayed task.
type TaskGraphNodeDTO struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	State         v1.TaskState `json:"state"`
	ParentID      string       `json:"parent_id,omitempty"`
	ParentTitle   string       `json:"parent_title,omitempty"`
	Depth         int          `json:"depth"`
	Role          string       `json:"role"`
	Archived      bool         `json:"archived"`
	Blocked       bool         `json:"blocked"`
	BlockedReason string       `json:"blocked_reason,omitempty"`
}

// TaskGraphEdgeDTO points from predecessor to dependent.
type TaskGraphEdgeDTO struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Status string `json:"status,omitempty"`
}
