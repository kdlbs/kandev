package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Placement values accepted by reorder_child_tasks_kandev.
const (
	// childReorderPlacementInPlace permutes the named children inside the
	// slots they already occupy, so every other task keeps its position.
	childReorderPlacementInPlace = "in_place"
	// childReorderPlacementFront moves the named children, in the given order,
	// to the head of their band; the remaining tasks keep their relative order.
	childReorderPlacementFront = "front"

	// Band discriminators of the existing step reorder contract.
	childReorderBandAdmitted = "admitted"
	childReorderBandQueued   = "queued"
)

// ChildTaskReorderService is the task-service surface reorder_child_tasks_kandev
// needs. The MCP layer owns the direct-parent authorization; the step reorder
// itself, its workflow-scope check, and the task.reordered event stay in the
// shared step reorder path with transactional child-relation checks.
type ChildTaskReorderService interface {
	GetTask(ctx context.Context, taskID string) (*models.Task, error)
	ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error)
	ReorderChildStepTasks(context.Context, string, string, []string, models.ChildTaskOrderScope) (*service.ReorderStepTasksResult, error)
}

type reorderChildTasksRequest struct {
	OrderedTaskIDs []string `json:"ordered_task_ids"`
	Placement      string   `json:"placement"`
	SenderTaskID   string   `json:"sender_task_id"`
}

type childReorderFailure struct {
	code    string
	message string
}

func (f *childReorderFailure) response(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, f.code, f.message, nil)
}

// childReorderTarget is the step and band every named child shares.
type childReorderTarget struct {
	workflowID  string
	stepID      string
	band        string
	parentID    string
	workspaceID string
}

func (h *Handlers) handleReorderChildTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, failure := parseReorderChildTasksRequest(msg)
	if failure != nil {
		return failure.response(msg)
	}
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task reordering is not configured", nil)
	}
	principal, trusted := mcpscope.PrincipalFromContext(ctx)
	if !trusted || principal.Surface != mcpprofile.SurfaceKanbanTask || principal.CallerTaskID != req.SenderTaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "trusted task sender is required", nil)
	}
	reorderer := ChildTaskReorderService(h.taskSvc)
	target, failure := resolveChildReorderTarget(ctx, reorderer, req)
	if failure != nil {
		return failure.response(msg)
	}

	if principal.WorkspaceID != target.workspaceID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "caller workspace does not match parent", nil)
	}
	stepTasks, err := reorderer.ListTasks(ctx, target.workflowID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to read the step's task order", nil)
	}
	result, failure := applyChildReorder(ctx, reorderer, target, req, stepTasks)
	if failure != nil {
		return failure.response(msg)
	}
	return ws.NewResponse(msg.ID, msg.Action, childReorderResponse(target, req.Placement, result))
}

func parseReorderChildTasksRequest(msg *ws.Message) (reorderChildTasksRequest, *childReorderFailure) {
	var req reorderChildTasksRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return req, &childReorderFailure{ws.ErrorCodeBadRequest, "Invalid payload: " + err.Error()}
	}
	req.SenderTaskID = strings.TrimSpace(req.SenderTaskID)
	if req.SenderTaskID == "" {
		return req, &childReorderFailure{ws.ErrorCodeValidation,
			"sender_task_id is required (the calling agent's MCP server must supply this)"}
	}
	req.Placement = strings.TrimSpace(req.Placement)
	if req.Placement == "" {
		req.Placement = childReorderPlacementInPlace
	}
	if req.Placement != childReorderPlacementInPlace && req.Placement != childReorderPlacementFront {
		return req, &childReorderFailure{ws.ErrorCodeValidation, `placement must be "in_place" or "front"`}
	}
	if len(req.OrderedTaskIDs) == 0 {
		return req, &childReorderFailure{ws.ErrorCodeValidation, "ordered_task_ids must name at least one task"}
	}
	seen := make(map[string]struct{}, len(req.OrderedTaskIDs))
	for i, id := range req.OrderedTaskIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return req, &childReorderFailure{ws.ErrorCodeValidation, "ordered_task_ids must not contain empty IDs"}
		}
		if _, dup := seen[id]; dup {
			return req, &childReorderFailure{ws.ErrorCodeValidation, "ordered_task_ids must not repeat a task: " + id}
		}
		seen[id] = struct{}{}
		req.OrderedTaskIDs[i] = id
	}
	return req, nil
}

// resolveChildReorderTarget loads the sender and every named task, requires
// each one to be the sender's direct child in the same workspace, and requires
// them all to share one workflow step and one band of that step.
func resolveChildReorderTarget(
	ctx context.Context, reorderer ChildTaskReorderService, req reorderChildTasksRequest,
) (childReorderTarget, *childReorderFailure) {
	sender, failure := lookupChildReorderTask(ctx, reorderer, req.SenderTaskID, "sender")
	if failure != nil {
		return childReorderTarget{}, failure
	}
	var target childReorderTarget
	for i, id := range req.OrderedTaskIDs {
		task, failure := lookupChildReorderTask(ctx, reorderer, id, "task "+id)
		if failure != nil {
			return childReorderTarget{}, failure
		}
		if !canDirectParentAccess(sender, task) {
			return childReorderTarget{}, &childReorderFailure{ws.ErrorCodeForbidden,
				"only a task's direct parent in the same workspace can reorder it: " + id}
		}
		if task.WorkflowStepID == "" {
			return childReorderTarget{}, &childReorderFailure{ws.ErrorCodeValidation,
				"task is not on a workflow step: " + id}
		}
		current := childReorderTarget{
			workflowID:  task.WorkflowID,
			stepID:      task.WorkflowStepID,
			band:        childReorderBand(task, task.WorkflowStepID),
			parentID:    sender.ID,
			workspaceID: sender.WorkspaceID,
		}
		if i == 0 {
			target = current
			continue
		}
		if current.stepID != target.stepID || current.workflowID != target.workflowID {
			return childReorderTarget{}, &childReorderFailure{ws.ErrorCodeValidation,
				"all ordered_task_ids must be in the same workflow step; " + id + " is in a different step"}
		}
		if current.band != target.band {
			return childReorderTarget{}, &childReorderFailure{ws.ErrorCodeValidation,
				"all ordered_task_ids must be in the same band (admitted or queued) of the step; " + id + " is not"}
		}
	}
	return target, nil
}

func lookupChildReorderTask(ctx context.Context, reorderer ChildTaskReorderService, taskID, role string) (*models.Task, *childReorderFailure) {
	task, err := reorderer.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, &childReorderFailure{ws.ErrorCodeNotFound, role + " not found"}
		}
		return nil, &childReorderFailure{ws.ErrorCodeInternalError, "failed to look up " + role}
	}
	if task == nil {
		return nil, &childReorderFailure{ws.ErrorCodeNotFound, role + " not found"}
	}
	return task, nil
}

// applyChildReorder submits the whole band in its new order. The step reorder
// contract requires the band's complete current membership, so a concurrent
// membership change surfaces as ErrStepChanged. A fresh band snapshot is
// validated and retried once; a second conflict is reported to the caller.
func applyChildReorder(
	ctx context.Context,
	reorderer ChildTaskReorderService,
	target childReorderTarget,
	req reorderChildTasksRequest,
	stepTasks []*models.Task,
) (*service.ReorderStepTasksResult, *childReorderFailure) {
	for attempt := 0; ; attempt++ {
		bandIDs, failure := childReorderBandOrder(stepTasks, target, req.OrderedTaskIDs, req.Placement)
		if failure != nil {
			return nil, failure
		}
		result, err := reorderer.ReorderChildStepTasks(ctx, target.stepID, target.band, bandIDs, models.ChildTaskOrderScope{
			ParentTaskID: target.parentID, WorkspaceID: target.workspaceID, TaskIDs: req.OrderedTaskIDs,
		})
		if err == nil {
			return result, nil
		}
		if errors.Is(err, repoerrors.ErrStepChanged) && attempt == 0 {
			stepTasks, err = reorderer.ListTasks(ctx, target.workflowID)
			if err != nil {
				return nil, &childReorderFailure{ws.ErrorCodeInternalError, "failed to refresh the step's task order"}
			}
			continue
		}
		return nil, childReorderServiceFailure(err)
	}
}

func childReorderServiceFailure(err error) *childReorderFailure {
	switch {
	case errors.Is(err, repoerrors.ErrTaskParentMismatch):
		return &childReorderFailure{ws.ErrorCodeForbidden, "a named task is no longer your direct child in this workspace"}
	case errors.Is(err, repoerrors.ErrStepChanged):
		return &childReorderFailure{ws.ErrorCodeConflict,
			"the step's task order changed while reordering; read it again and retry"}
	case errors.Is(err, repoerrors.ErrInvalidReorder):
		return &childReorderFailure{ws.ErrorCodeValidation, "the reorder request was rejected as invalid"}
	case service.IsForbidden(err):
		return &childReorderFailure{ws.ErrorCodeForbidden, "task write access is required to reorder tasks"}
	default:
		return &childReorderFailure{ws.ErrorCodeInternalError, "failed to reorder tasks"}
	}
}

func childReorderResponse(
	target childReorderTarget, placement string, result *service.ReorderStepTasksResult,
) map[string]interface{} {
	tasks := make([]map[string]interface{}, 0, len(result.Tasks))
	for _, task := range result.Tasks {
		if childReorderBand(task, target.stepID) != target.band {
			continue
		}
		tasks = append(tasks, map[string]interface{}{
			"id":       task.ID,
			"title":    task.Title,
			"position": task.Position,
		})
	}
	return map[string]interface{}{
		"workflow_step_id": target.stepID,
		"band":             target.band,
		"placement":        placement,
		"revision":         result.Revision,
		"tasks":            tasks,
	}
}
