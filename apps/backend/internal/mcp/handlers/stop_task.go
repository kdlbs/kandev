package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const stopTaskStatusKey = "status"

type stopTaskRequest struct {
	TaskID       string `json:"task_id"`
	SenderTaskID string `json:"sender_task_id"`
}

type stopTaskFailure struct {
	response *ws.Message
	err      error
}

func (h *Handlers) handleStopTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, validationError := parseStopTaskRequest(msg)
	if validationError != nil {
		return validationError.response, validationError.err
	}
	if h.stopTaskGetter == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task lookup is not configured", nil)
	}

	sender, lookupError := h.lookupStopTask(ctx, msg, req.SenderTaskID, "sender")
	if lookupError != nil {
		return lookupError.response, lookupError.err
	}

	target, lookupError := h.lookupStopTask(ctx, msg, req.TaskID, "target")
	if lookupError != nil {
		return lookupError.response, lookupError.err
	}

	if !canStopTask(sender, target) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"only a task's direct parent in the same workspace can stop it", nil)
	}
	if h.taskStopper == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task stop is not configured", nil)
	}

	result, err := h.taskStopper.StopTaskForCoordinator(ctx, target.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to stop target task", nil)
	}
	switch result.Status {
	case orchestrator.CoordinatorTaskStopStatusStopped, orchestrator.CoordinatorTaskStopStatusNotRunning:
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task stop returned an invalid status", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		keyTaskID:         target.ID,
		stopTaskStatusKey: result.Status,
	})
}

func parseStopTaskRequest(msg *ws.Message) (stopTaskRequest, *stopTaskFailure) {
	var req stopTaskRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return req, newStopTaskFailure(msg, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error())
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.SenderTaskID = strings.TrimSpace(req.SenderTaskID)
	if req.TaskID == "" {
		return req, newStopTaskFailure(msg, ws.ErrorCodeValidation, "task_id is required")
	}
	if req.SenderTaskID == "" {
		return req, newStopTaskFailure(msg, ws.ErrorCodeValidation,
			"sender_task_id is required (the calling agent's MCP server must supply this)")
	}
	return req, nil
}

func (h *Handlers) lookupStopTask(
	ctx context.Context,
	msg *ws.Message,
	taskID string,
	role string,
) (*models.Task, *stopTaskFailure) {
	task, err := h.stopTaskGetter(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return nil, newStopTaskFailure(msg, ws.ErrorCodeNotFound, role+" task not found")
		}
		return nil, newStopTaskFailure(msg, ws.ErrorCodeInternalError, "failed to look up "+role+" task")
	}
	if task == nil {
		return nil, newStopTaskFailure(msg, ws.ErrorCodeNotFound, role+" task not found")
	}
	return task, nil
}

func newStopTaskFailure(msg *ws.Message, code, message string) *stopTaskFailure {
	response, err := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return &stopTaskFailure{response: response, err: err}
}

func canStopTask(sender, target *models.Task) bool {
	return sender.WorkspaceID != "" &&
		target.WorkspaceID != "" &&
		target.ParentID != "" &&
		target.ParentID == sender.ID &&
		target.WorkspaceID == sender.WorkspaceID
}
