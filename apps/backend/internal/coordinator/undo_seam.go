package coordinator

import (
	"context"
	"errors"
	"time"
)

// Errors the task service seam reports. The backend adapter maps the task
// and workflow services' own errors onto these.
var (
	ErrTaskNotFound        = errors.New("coordinator: task not found")
	ErrTaskAlreadyArchived = errors.New("coordinator: task already archived")
	ErrStepNotFound        = errors.New("coordinator: workflow step not found")
	ErrWIPLimitExceeded    = errors.New("coordinator: step is at its work-in-progress limit")
	ErrMoveConflict        = errors.New("coordinator: task moved concurrently")
)

// UndoTask is the slice of a task undo reads.
type UndoTask struct {
	Identifier     string
	ArchivedAt     *time.Time
	WorkflowID     string
	WorkflowStepID string
}

// UndoStep is the slice of a workflow step undo reads.
type UndoStep struct {
	Name             string
	WorkflowID       string
	AutoStart        bool
	CompletesOnEnter bool
}

// UndoMoveOptions carries the move options undo sets.
type UndoMoveOptions struct {
	ExpectedWorkflowID string
	SkipStepPrompt     bool
}

// UndoMoveResult is what a move committed: whether the destination step
// admitted the task, and the step the task left in the move's own write
// transaction (empty when the task service reports none).
type UndoMoveResult struct {
	Admitted   bool
	FromStepID string
}

// UndoTaskService is the one task-service seam undo (and the move executor)
// use. Implementations return the sentinel errors above.
type UndoTaskService interface {
	ArchiveTask(ctx context.Context, id string) error
	GetTask(ctx context.Context, id string) (*UndoTask, error)
	MoveTaskWithOptions(ctx context.Context, id, workflowID, stepID string, position int, opts UndoMoveOptions) (UndoMoveResult, error)
	GetStep(ctx context.Context, stepID string) (*UndoStep, error)
	// ListSteps reads every step of a workflow, the graph StartsAgentOnEnter walks.
	ListSteps(ctx context.Context, workflowID string) ([]StepNode, error)
	HasActiveSession(ctx context.Context, taskID string) (bool, error)
}
