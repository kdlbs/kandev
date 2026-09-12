package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ReorderStepTasksResult is the whole-step, both-bands view every reorder
// carrier reports: the success body, the step_changed conflict body, and the
// task.reordered event all carry this same shape
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.19, .25).
type ReorderStepTasksResult struct {
	WorkflowStepID string
	Revision       int64
	Tasks          []*models.Task
}

const (
	reorderPayloadPositionKey    = "position"
	reorderPayloadWorkspaceIDKey = "workspace_id"
)

// reorderRepository is the narrow capability ReorderStepTasks needs from the
// task repository, following the same runtime-asserted narrow-interface
// pattern as workflowQueuedTaskPromoter.
type reorderRepository interface {
	ReorderStepTasks(ctx context.Context, stepID, band string, orderedTaskIDs []string) ([]*models.Task, int64, error)
}

// ReorderStepTasks rewrites stepID's named band to orderedTaskIDs
// (REQ-TASKS-KANBAN-TASK-REORDERING-001). A caller authorized to move a task
// in a workflow is authorized to reorder that workflow's steps
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.23). On the step_changed conflict the
// returned result still carries the authoritative order the caller
// reconciles to; on any other error the result is nil.
func (s *Service) ReorderStepTasks(ctx context.Context, stepID, band string, orderedTaskIDs []string) (*ReorderStepTasksResult, error) {
	if s.workflowStepGetter == nil {
		return nil, fmt.Errorf("workflow step getter not configured")
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkflowID(ctx, step.WorkflowID); err != nil {
		return nil, err
	}
	reorderer, ok := s.tasks.(reorderRepository)
	if !ok {
		return nil, fmt.Errorf("task repository does not support reordering")
	}

	tasks, revision, err := reorderer.ReorderStepTasks(ctx, stepID, band, orderedTaskIDs)
	if err != nil {
		if errors.Is(err, repoerrors.ErrStepChanged) {
			return &ReorderStepTasksResult{WorkflowStepID: stepID, Revision: revision, Tasks: tasks}, err
		}
		return nil, err
	}

	result := &ReorderStepTasksResult{WorkflowStepID: stepID, Revision: revision, Tasks: tasks}
	s.publishTaskReordered(ctx, stepID, band, revision, tasks)
	return result, nil
}

// publishTaskReordered emits task.reordered
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.16), deliberately not task.moved
// (.21) and not one task.updated per task (design's "## Reorder contract" ·
// Publication). workspace_id is not part of the client-visible payload
// contract but is required for Hub.BroadcastToWorkspace to scope the event
// to the step's own workspace rather than falling back to a global
// broadcast; every task in tasks shares stepID's workspace, so the first
// entry's is authoritative. tasks is always non-empty here: the repository
// only reaches a committed reorder with at least the named band's submitted
// ids present.
func (s *Service) publishTaskReordered(ctx context.Context, stepID, band string, revision int64, tasks []*models.Task) {
	if s.eventBus == nil || len(tasks) == 0 {
		return
	}
	taskEntries := make([]map[string]interface{}, len(tasks))
	for i, task := range tasks {
		taskEntries[i] = map[string]interface{}{"id": task.ID, reorderPayloadPositionKey: task.Position}
	}
	data := map[string]interface{}{
		reorderPayloadWorkspaceIDKey: tasks[0].WorkspaceID,
		"workflow_step_id":           stepID,
		"band":                       band,
		"revision":                   revision,
		"tasks":                      taskEntries,
	}
	event := bus.NewEvent(events.TaskReordered, "task-service", data)
	if err := s.eventBus.Publish(ctx, events.TaskReordered, event); err != nil {
		s.logger.Error("failed to publish task.reordered event",
			zap.String("workflow_step_id", stepID),
			zap.Error(err))
	}
}
