package engine_adapters

import (
	"context"
	"fmt"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// ParentTaskRepo captures the subset of the kanban tasks repo the
// TaskCreatorAdapter needs to resolve a parent task's workspace + workflow
// defaults. Implemented in production by *tasksqlite.Repository.
type ParentTaskRepo interface {
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
}

// ChildTaskCreator is the action-side surface the adapter needs from the
// kanban task service. Returns the new task's id (or a non-nil error).
//
// Implementations MUST set parent_id to parent.ID and persist the new row
// with the supplied workflow + step + assignee. The adapter expects the
// service-side CreateTask to fall back to parent's workflow when WorkflowID
// is empty and to resolve the workflow's first runnable step when StepID
// is empty — matching the existing CreateTask semantics.
type ChildTaskCreator interface {
	CreateChildTask(ctx context.Context, parent *taskmodels.Task, spec ChildTaskCreateSpec) (taskID string, err error)
}

// ChildTaskCreateSpec is the typed payload ChildTaskCreator receives.
// Fields mirror engine.ChildTaskSpec but stay decoupled so the task
// service does not need to import the engine package directly.
type ChildTaskCreateSpec struct {
	Title          string
	Description    string
	WorkflowID     string
	StepID         string
	AgentProfileID string
}

// TaskCreatorAdapter implements engine.TaskCreator. Given a parent task id
// and a ChildTaskSpec, it loads the parent task row and asks the kanban
// task service to create the child.
//
// The adapter intentionally does no agent assignment fallback for blank
// AgentProfileID — the task service inherits from the parent during
// CreateTask. That keeps the office and CLI delegation paths consistent.
type TaskCreatorAdapter struct {
	ParentRepo  ParentTaskRepo
	TaskService ChildTaskCreator
}

// NewTaskCreatorAdapter wires the kanban tasks repo (for the parent row
// lookup) and the kanban task service (for the actual create).
func NewTaskCreatorAdapter(parentRepo ParentTaskRepo, taskSvc ChildTaskCreator) *TaskCreatorAdapter {
	return &TaskCreatorAdapter{
		ParentRepo:  parentRepo,
		TaskService: taskSvc,
	}
}

// CreateChildTask satisfies engine.TaskCreator.
func (a *TaskCreatorAdapter) CreateChildTask(
	ctx context.Context, parentTaskID string, spec engine.ChildTaskSpec,
) (string, error) {
	if parentTaskID == "" {
		return "", fmt.Errorf("parent_task_id is required")
	}
	if a.TaskService == nil {
		return "", fmt.Errorf("task service not configured for child task creation")
	}
	parent, err := a.loadParent(ctx, parentTaskID)
	if err != nil {
		return "", err
	}
	taskID, err := a.TaskService.CreateChildTask(ctx, parent, ChildTaskCreateSpec{
		Title:          spec.Title,
		Description:    spec.Description,
		WorkflowID:     spec.WorkflowID,
		StepID:         spec.StepID,
		AgentProfileID: spec.AgentProfileID,
	})
	if err != nil {
		return "", fmt.Errorf("create child task: %w", err)
	}
	return taskID, nil
}

func (a *TaskCreatorAdapter) loadParent(ctx context.Context, parentTaskID string) (*taskmodels.Task, error) {
	if a.ParentRepo == nil {
		return nil, fmt.Errorf("parent task repo not configured")
	}
	parent, err := a.ParentRepo.GetTask(ctx, parentTaskID)
	if err != nil {
		return nil, fmt.Errorf("get parent task %s: %w", parentTaskID, err)
	}
	if parent == nil {
		return nil, fmt.Errorf("parent task %s not found", parentTaskID)
	}
	return parent, nil
}

// Compile-time interface assertion.
var _ engine.TaskCreator = (*TaskCreatorAdapter)(nil)
