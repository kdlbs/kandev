package models

import "time"

// TaskLifecycleReceipt is durable, workspace-bound evidence of the last
// lifecycle transition. It deliberately has no foreign key to tasks: a delete
// receipt must survive the task row it describes.
type TaskLifecycleReceipt struct {
	TaskID         string
	WorkspaceID    string
	Identifier     string
	Title          string
	WorkflowID     string
	WorkflowStepID string
	State          string
	Disposition    string
	ChangedAt      time.Time
	Revision       int64
	Provenance     string
}
