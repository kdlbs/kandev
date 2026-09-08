package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TaskChangeLink identifies an active GitHub pull request or GitLab merge
// request. RepositoryID and Number deliberately travel together so same-number
// changes in forks and canonical repositories remain distinct.
type TaskChangeLink struct {
	Provider     string `json:"provider"`
	RepositoryID string `json:"repository_id"`
	Number       int    `json:"number"`
}

// TaskChangeLinkRequest is one provider-neutral change-request operation.
type TaskChangeLinkRequest struct {
	TaskID string
	Link   TaskChangeLink
	Old    *TaskChangeLink
}

// TaskChangeLinkService owns provider dispatch and returns the resulting
// active links after each mutation.
type TaskChangeLinkService interface {
	LinkTaskChange(context.Context, TaskChangeLinkRequest) ([]TaskChangeLink, error)
	UnlinkTaskChange(context.Context, TaskChangeLinkRequest) ([]TaskChangeLink, error)
	ReplaceTaskChange(context.Context, TaskChangeLinkRequest) ([]TaskChangeLink, error)
}

func (h *Handlers) handleLinkTaskPR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, response, err := h.taskChangeLinkRequest(ctx, msg, false)
	if response != nil || err != nil {
		return response, err
	}
	if h.taskChangeLinks == nil {
		return taskChangeLinksUnavailable(msg)
	}
	return h.applyTaskChangeLink(ctx, msg, req, h.taskChangeLinks.LinkTaskChange)
}

func (h *Handlers) handleUnlinkTaskPR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, response, err := h.taskChangeLinkRequest(ctx, msg, false)
	if response != nil || err != nil {
		return response, err
	}
	if h.taskChangeLinks == nil {
		return taskChangeLinksUnavailable(msg)
	}
	return h.applyTaskChangeLink(ctx, msg, req, h.taskChangeLinks.UnlinkTaskChange)
}

func (h *Handlers) handleReplaceTaskPR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, response, err := h.taskChangeLinkRequest(ctx, msg, true)
	if response != nil || err != nil {
		return response, err
	}
	if h.taskChangeLinks == nil {
		return taskChangeLinksUnavailable(msg)
	}
	return h.applyTaskChangeLink(ctx, msg, req, h.taskChangeLinks.ReplaceTaskChange)
}

func taskChangeLinksUnavailable(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task PR link management is not available", nil)
}

func (h *Handlers) applyTaskChangeLink(ctx context.Context, msg *ws.Message, req TaskChangeLinkRequest, apply func(context.Context, TaskChangeLinkRequest) ([]TaskChangeLink, error)) (*ws.Message, error) {
	links, err := apply(ctx, req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"task_id": req.TaskID, "links": links})
}

func (h *Handlers) taskChangeLinkRequest(ctx context.Context, msg *ws.Message, replace bool) (TaskChangeLinkRequest, *ws.Message, error) {
	var payload struct {
		TaskID          string `json:"task_id"`
		CallerTaskID    string `json:"caller_task_id"`
		Provider        string `json:"provider"`
		RepositoryID    string `json:"repository_id"`
		Number          int    `json:"number"`
		OldProvider     string `json:"old_provider"`
		OldRepositoryID string `json:"old_repository_id"`
		OldNumber       int    `json:"old_number"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	link, err := validateTaskChangeLink(payload.Provider, payload.RepositoryID, payload.Number)
	if err != nil || strings.TrimSpace(payload.TaskID) == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, provider, repository_id, and positive number are required", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	if h.taskSvc == nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not available", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	target, targetErr := h.taskSvc.GetTask(ctx, payload.TaskID)
	if targetErr != nil || target == nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	if strings.TrimSpace(payload.CallerTaskID) == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "caller_task_id is required", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	caller, callerErr := h.taskSvc.GetTask(ctx, payload.CallerTaskID)
	if callerErr != nil || caller == nil || caller.WorkspaceID != target.WorkspaceID {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task is outside the caller workspace", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	req := TaskChangeLinkRequest{TaskID: target.ID, Link: link}
	if replace {
		old, oldErr := validateTaskChangeLink(payload.OldProvider, payload.OldRepositoryID, payload.OldNumber)
		if oldErr != nil {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "old_provider, old_repository_id, and positive old_number are required", nil)
			return TaskChangeLinkRequest{}, response, responseErr
		}
		req.Old = &old
	}
	return req, nil, nil
}

func validateTaskChangeLink(provider, repositoryID string, number int) (TaskChangeLink, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if (provider != "github" && provider != "gitlab") || strings.TrimSpace(repositoryID) == "" || number <= 0 {
		return TaskChangeLink{}, errors.New("invalid task change identity")
	}
	return TaskChangeLink{Provider: provider, RepositoryID: repositoryID, Number: number}, nil
}
