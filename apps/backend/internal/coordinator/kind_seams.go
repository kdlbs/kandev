package coordinator

import (
	"context"
	"errors"
	"time"
)

// Errors the message seam reports; the adapter maps the message queue's own
// errors onto them.
var (
	ErrMessageQueueFull = errors.New("coordinator: message queue is full")
)

// TargetSession is the slice of a task session the resume and message
// executors read.
type TargetSession struct {
	ID                string
	State             string
	HasExecutorRecord bool
	RecoveryPending   bool
}

// TargetTask is the slice of a task the propose validators read.
type TargetTask struct {
	ID             string
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	Origin         string
	ArchivedAt     *time.Time
	Primary        *TargetSession
}

// KindTaskReader reads a task's placement and primary session, and whether a
// session has a live execution. GetTarget returns ErrTaskNotFound for a task
// that does not exist.
type KindTaskReader interface {
	GetTarget(ctx context.Context, taskID string) (*TargetTask, error)
	HasLiveExecution(ctx context.Context, sessionID string) bool
}

// TaskResumer resumes a task session. started is false when the launch was
// deferred (the executor had no room) rather than started.
type TaskResumer interface {
	ResumeTaskSession(ctx context.Context, taskID, sessionID string) (started bool, err error)
}

// TaskMessenger delivers a prompt to a task's session queued behind a running
// turn, the delivery message_task_kandev uses. It returns the session the
// prompt went to, or ErrMessageQueueFull.
type TaskMessenger interface {
	DeliverQueued(ctx context.Context, taskID, sessionID, prompt string) (string, error)
}

// KindDeps are the seams the resume and message kinds use.
type KindDeps struct {
	Tasks     KindTaskReader
	Resumer   TaskResumer
	Messenger TaskMessenger
}

// SetKindDeps wires the resume and message kinds' seams.
func (s *Service) SetKindDeps(deps KindDeps) { s.kindDeps = deps }
