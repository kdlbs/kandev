package models

import "time"

type TaskResourceCleanupTrigger string

const (
	TaskResourceCleanupTriggerArchive         TaskResourceCleanupTrigger = "archive"
	TaskResourceCleanupTriggerDelete          TaskResourceCleanupTrigger = "delete"
	TaskResourceCleanupTriggerCascadeArchive  TaskResourceCleanupTrigger = "cascade_archive"
	TaskResourceCleanupTriggerCascadeDelete   TaskResourceCleanupTrigger = "cascade_delete"
	TaskResourceCleanupTriggerWorkspaceDelete TaskResourceCleanupTrigger = "workspace_delete"
	TaskResourceCleanupTriggerQuickChatExpire TaskResourceCleanupTrigger = "quick_chat_expire"
	TaskResourceCleanupTriggerReconcile       TaskResourceCleanupTrigger = "reconcile"
)

type TaskResourceCleanupState string

const (
	TaskResourceCleanupStatePrepared  TaskResourceCleanupState = "prepared"
	TaskResourceCleanupStatePending   TaskResourceCleanupState = "pending"
	TaskResourceCleanupStateRunning   TaskResourceCleanupState = "running"
	TaskResourceCleanupStateRetryWait TaskResourceCleanupState = "retry_wait"
	TaskResourceCleanupStateSucceeded TaskResourceCleanupState = "succeeded"
	TaskResourceCleanupStateCancelled TaskResourceCleanupState = "cancelled"
)

// TaskResourceCleanupJob is a durable task-lifecycle cleanup intent. TaskID is
// deliberately not foreign-keyed: delete cleanup must outlive task/session
// cascades and use ResourceSnapshot as its recovery inventory.
type TaskResourceCleanupJob struct {
	ID               string
	OperationID      string
	TaskID           string
	Trigger          TaskResourceCleanupTrigger
	State            TaskResourceCleanupState
	ResourceSnapshot string
	Attempts         int
	NextAttemptAt    *time.Time
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}

func (j *TaskResourceCleanupJob) IsArchive() bool {
	return j != nil && (j.Trigger == TaskResourceCleanupTriggerArchive ||
		j.Trigger == TaskResourceCleanupTriggerCascadeArchive)
}
