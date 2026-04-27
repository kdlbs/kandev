package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Task Plan Handlers

// wsCreateTaskPlan creates a new task plan
func (h *TaskHandlers) wsCreateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID     string `json:"task_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
		CreatedBy  string `json:"created_by"`
		AuthorKind string `json:"author_kind"`
		AuthorName string `json:"author_name"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}

	plan, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID:     req.TaskID,
		Title:      req.Title,
		Content:    req.Content,
		CreatedBy:  createdBy,
		AuthorKind: req.AuthorKind,
		AuthorName: req.AuthorName,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsGetTaskPlan retrieves a task plan
func (h *TaskHandlers) wsGetTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	plan, err := h.planService.GetPlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task plan", nil)
	}
	if plan == nil {
		return ws.NewResponse(msg.ID, msg.Action, nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsUpdateTaskPlan updates an existing task plan
func (h *TaskHandlers) wsUpdateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID     string `json:"task_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
		CreatedBy  string `json:"created_by"`
		AuthorKind string `json:"author_kind"`
		AuthorName string `json:"author_name"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}

	plan, err := h.planService.UpdatePlan(ctx, service.UpdatePlanRequest{
		TaskID:     req.TaskID,
		Title:      req.Title,
		Content:    req.Content,
		CreatedBy:  createdBy,
		AuthorKind: req.AuthorKind,
		AuthorName: req.AuthorName,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// wsDeleteTaskPlan deletes a task plan
func (h *TaskHandlers) wsDeleteTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	err := h.planService.DeletePlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

// wsListTaskPlanRevisions returns revision metadata newest-first (no content).
func (h *TaskHandlers) wsListTaskPlanRevisions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	revs, err := h.planService.ListRevisions(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list revisions: "+err.Error(), nil)
	}

	out := make([]*dto.TaskPlanRevisionDTO, 0, len(revs))
	for _, r := range revs {
		out = append(out, dto.TaskPlanRevisionMetaFromModel(r))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"revisions": out})
}

// wsGetTaskPlanRevision returns a single revision with content.
func (h *TaskHandlers) wsGetTaskPlanRevision(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID     string `json:"task_id"`
		RevisionID string `json:"revision_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.RevisionID == "" {
		// Surface missing argument as a 400, matching wsRevertTaskPlan's
		// ErrRevisionIDRequired branch — without this guard, an empty id
		// falls through to the service and returns 404 (lookup miss),
		// which is misleading for the caller.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "revision_id is required", nil)
	}

	rev, err := h.planService.GetRevision(ctx, req.RevisionID)
	if err != nil {
		if errors.Is(err, service.ErrRevisionNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Revision not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get revision: "+err.Error(), nil)
	}
	// Match wsRevertTaskPlan's ownership check so a caller can only read
	// content for revisions belonging to the task they hold a reference to.
	// Optional task_id keeps existing callers compatible while letting the
	// frontend tighten the request shape.
	if req.TaskID != "" && rev.TaskID != req.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Revision does not belong to task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanRevisionFromModel(rev))
}

// wsRevertTaskPlan reverts the plan HEAD to a target revision by appending a new revision.
func (h *TaskHandlers) wsRevertTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID     string `json:"task_id"`
		RevisionID string `json:"revision_id"`
		AuthorName string `json:"author_name"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	rev, err := h.planService.RevertPlan(ctx, service.RevertPlanRequest{
		TaskID:           req.TaskID,
		TargetRevisionID: req.RevisionID,
		AuthorName:       req.AuthorName,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTaskIDRequired):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		case errors.Is(err, service.ErrRevisionIDRequired):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "revision_id is required", nil)
		case errors.Is(err, service.ErrRevisionNotFound):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Revision not found", nil)
		case errors.Is(err, service.ErrRevisionTaskMismatch):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Revision does not belong to task", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to revert plan: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanRevisionFromModel(rev))
}
