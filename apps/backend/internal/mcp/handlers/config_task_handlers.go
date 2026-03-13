package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		Position       int    `json:"position"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}

	result, err := h.taskSvc.MoveTask(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position)
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to move task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(result.Task))
}

func (h *Handlers) handleDeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.taskSvc.DeleteTask(ctx, taskID); err != nil {
		h.logger.Error("failed to delete task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleArchiveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.taskSvc.ArchiveTask(ctx, taskID); err != nil {
		h.logger.Error("failed to archive task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to archive task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleUpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}
	state := v1.TaskState(req.State)
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateScheduling,
		v1.TaskStateInProgress, v1.TaskStateReview, v1.TaskStateBlocked,
		v1.TaskStateWaitingForInput, v1.TaskStateCompleted,
		v1.TaskStateFailed, v1.TaskStateCancelled:
		// valid
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task state: "+req.State, nil)
	}

	task, err := h.taskSvc.UpdateTaskState(ctx, req.TaskID, state)
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}
