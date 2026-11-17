package models

import v1 "github.com/kandev/kandev/pkg/api/v1"

// TaskStateWrite describes a guarded task-state mutation. A nil guard is
// absent; a pointer to an empty workflow-step ID is an explicit empty-step
// guard.
type TaskStateWrite struct {
	State                  v1.TaskState
	ExpectedWorkflowStepID *string
	ExpectedState          *v1.TaskState
	// AcceptIfAlreadyAtTarget makes a guarded retry a matched no-op when the
	// task has already reached State. ExpectedWorkflowStepID still applies.
	AcceptIfAlreadyAtTarget bool
}

// TaskStateBatchWrite commits a set of task state changes with any related
// checkout releases in one transaction.
type TaskStateBatchWrite struct {
	TaskIDs                []string
	State                  v1.TaskState
	ReleaseCheckoutTaskIDs []string
}

// TaskStateChange is the committed result of a TaskStateWrite. Matched is
// false when a requested guard rejected the write. Matched with Changed
// false records an unchanged state. Task is the snapshot returned by the
// committing transaction when Matched is true.
type TaskStateChange struct {
	Matched  bool
	Changed  bool
	OldState v1.TaskState
	Task     *Task
}
