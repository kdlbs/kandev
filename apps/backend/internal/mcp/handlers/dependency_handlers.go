package handlers

import (
	"context"
	"encoding/json"
	"errors"

	ws "github.com/kandev/kandev/pkg/websocket"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// dependencyMutationPayload is the wire shape for both dependency tools.
type dependencyMutationPayload struct {
	TaskID          string `json:"task_id"`
	DependsOnTaskID string `json:"depends_on_task_id"`
}

// handleAddTaskDependency backs add_task_dependency_kandev.
func (h *Handlers) handleAddTaskDependency(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errMsg := decodeDependencyMutation(msg)
	if errMsg != nil {
		return errMsg, nil
	}
	err := h.taskSvc.AddDependency(ctx, req.TaskID, req.DependsOnTaskID)
	if err != nil {
		var cycle *service.CycleError
		if errors.As(err, &cycle) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, cycle.Error(),
				map[string]interface{}{"cycle": cycle.Path})
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return h.dependencyResult(ctx, msg, req.TaskID)
}

// handleRemoveTaskDependency backs remove_task_dependency_kandev. Removing an
// absent edge succeeds.
func (h *Handlers) handleRemoveTaskDependency(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errMsg := decodeDependencyMutation(msg)
	if errMsg != nil {
		return errMsg, nil
	}
	if err := h.taskSvc.RemoveDependency(ctx, req.TaskID, req.DependsOnTaskID); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return h.dependencyResult(ctx, msg, req.TaskID)
}

func decodeDependencyMutation(msg *ws.Message) (*dependencyMutationPayload, *ws.Message) {
	var req dependencyMutationPayload
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		errMsg, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return nil, errMsg
	}
	if req.TaskID == "" || req.DependsOnTaskID == "" {
		errMsg, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"task_id and depends_on_task_id are required", nil)
		return nil, errMsg
	}
	return &req, nil
}

// dependencyResult returns the task's resulting dependency projection so the
// agent sees the effect of its own call without a follow-up read.
func (h *Handlers) dependencyResult(ctx context.Context, msg *ws.Message, taskID string) (*ws.Message, error) {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{keyTaskID: taskID})
	}
	views := h.taskSvc.BuildDependencyViews(ctx, []*models.Task{task})
	view := views[taskID]
	dependsOn := make([]string, 0, len(view.DependsOn))
	for _, ref := range view.DependsOn {
		dependsOn = append(dependsOn, ref.ID)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		keyTaskID:        taskID,
		"blocked":        view.Blocked,
		"blocked_reason": view.BlockedReason,
		"depends_on":     dependsOn,
	})
}
