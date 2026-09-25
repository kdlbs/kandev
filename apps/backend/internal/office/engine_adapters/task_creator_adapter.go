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
	// OfficeCarrierMetadata carries the task-boundary causation carrier
	// set (AC-OFFICE-RUN-CAUSATION-001.5/.18/.24) forward from the parent
	// task, when CarrierResolver resolved one; nil otherwise. Forwarded
	// verbatim to taskservice.ChildTaskSpec.OfficeCarrierMetadata.
	OfficeCarrierMetadata map[string]interface{}
}

// CarrierResolver resolves the task-boundary causation carrier
// (AC-OFFICE-RUN-CAUSATION-001.5/.18) for a task id, so a child task
// created from it (the create_child_task workflow step action) can carry
// the same causation lineage forward instead of silently rooting at
// depth 0. The implementation prefers the run currently claimed against
// the task id by causingAgentProfileID (the agent executing this action's
// turn) so depth advances one hop per create_child_task call while
// staying unambiguous when more than one agent holds a claimed run on the
// same task; it falls back to the task's own already-resolved carrier
// when causingAgentProfileID is empty or holds no claimed run on the
// task. Implemented in production by *office/service.Service. Optional:
// nil means create_child_task never carries a carrier (pre-existing
// behaviour).
type CarrierResolver interface {
	TaskBoundaryCarrierMetadata(ctx context.Context, taskID, causingAgentProfileID string) map[string]interface{}
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
	Carrier     CarrierResolver
}

// NewTaskCreatorAdapter wires the kanban tasks repo (for the parent row
// lookup) and the kanban task service (for the actual create). carrier is
// optional: nil (via SetCarrierResolver being left uncalled) means a
// created child task never carries a causation carrier.
func NewTaskCreatorAdapter(parentRepo ParentTaskRepo, taskSvc ChildTaskCreator) *TaskCreatorAdapter {
	return &TaskCreatorAdapter{
		ParentRepo:  parentRepo,
		TaskService: taskSvc,
	}
}

// SetCarrierResolver wires the office service's task-boundary carrier
// resolver after construction, breaking the construction-order cycle
// between the task creator adapter (built early, before the office
// service exists) and the office service itself.
func (a *TaskCreatorAdapter) SetCarrierResolver(carrier CarrierResolver) {
	a.Carrier = carrier
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
	var carrierMetadata map[string]interface{}
	if a.Carrier != nil {
		carrierMetadata = a.Carrier.TaskBoundaryCarrierMetadata(ctx, parentTaskID, spec.CausingAgentProfileID)
	}
	taskID, err := a.TaskService.CreateChildTask(ctx, parent, ChildTaskCreateSpec{
		Title:                 spec.Title,
		Description:           spec.Description,
		WorkflowID:            spec.WorkflowID,
		StepID:                spec.StepID,
		AgentProfileID:        spec.AgentProfileID,
		OfficeCarrierMetadata: carrierMetadata,
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
