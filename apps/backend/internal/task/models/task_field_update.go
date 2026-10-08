package models

import v1 "github.com/kandev/kandev/pkg/api/v1"

// TaskFieldUpdate retains request presence until the current task row is locked.
type TaskFieldUpdate struct {
	Title          *string
	Description    *string
	Priority       *string
	State          *v1.TaskState
	WorkflowStepID *string
	Position       *int
	ParentID       *string
	AssigneeUserID *string
	Metadata       map[string]interface{}
}

// TaskFieldUpdateResult captures mutation provenance before postcommit reads.
type TaskFieldUpdateResult struct {
	Task                *Task
	PriorState          v1.TaskState
	PriorWorkflowStepID string
	ParentChanged       bool
}
