package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
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
	Operation string
	TaskID    string
	Link      TaskChangeLink
	Old       *TaskChangeLink
}

const (
	taskChangeOperationLink    = "link"
	taskChangeOperationUnlink  = "unlink"
	taskChangeOperationReplace = "replace"
)

// TaskChangeLinkMutationResult is the structured result of a neutral change
// request operation. StateKnown is false when the provider mutation or the
// final association read cannot establish the active set.
type TaskChangeLinkMutationResult struct {
	TaskID         string           `json:"task_id"`
	Links          []TaskChangeLink `json:"links"`
	OperationError string           `json:"operation_error,omitempty"`
	RollbackError  string           `json:"rollback_error,omitempty"`
	StateKnown     bool             `json:"state_known"`
}

// TaskChangeLinkMutationService is an optional richer seam for the neutral
// management tool. Legacy provider-specific actions continue to use the three
// methods on TaskChangeLinkService.
type TaskChangeLinkMutationService interface {
	ManageTaskChangeRequest(context.Context, TaskChangeLinkRequest) (TaskChangeLinkMutationResult, error)
}

type taskChangeRequestPayload struct {
	Operation       string `json:"operation"`
	TaskID          string `json:"task_id"`
	CallerTaskID    string `json:"caller_task_id"`
	Provider        string `json:"provider"`
	RepositoryID    string `json:"repository_id"`
	Number          int    `json:"number"`
	OldProvider     string `json:"old_provider"`
	OldRepositoryID string `json:"old_repository_id"`
	OldNumber       int    `json:"old_number"`
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

func (h *Handlers) handleManageTaskChangeRequest(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	payload, errResp, err := h.parseTaskChangeRequestPayload(ctx, msg)
	if errResp != nil || err != nil {
		return errResp, err
	}
	if h.taskChangeLinks == nil {
		return taskChangeLinksUnavailable(msg)
	}
	request := TaskChangeLinkRequest{
		Operation: payload.Operation,
		TaskID:    payload.TaskID,
		Link: TaskChangeLink{
			Provider: payload.Provider, RepositoryID: payload.RepositoryID, Number: payload.Number,
		},
	}
	if payload.Operation == taskChangeOperationReplace {
		request.Old = &TaskChangeLink{
			Provider: payload.OldProvider, RepositoryID: payload.OldRepositoryID, Number: payload.OldNumber,
		}
	}
	if mutationService, ok := h.taskChangeLinks.(TaskChangeLinkMutationService); ok {
		result, err := mutationService.ManageTaskChangeRequest(ctx, request)
		if err != nil {
			return taskChangeLinkMutationError(msg, result, err)
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	}
	var links []TaskChangeLink
	switch payload.Operation {
	case taskChangeOperationLink:
		links, err = h.taskChangeLinks.LinkTaskChange(ctx, request)
	case taskChangeOperationUnlink:
		links, err = h.taskChangeLinks.UnlinkTaskChange(ctx, request)
	case taskChangeOperationReplace:
		links, err = h.taskChangeLinks.ReplaceTaskChange(ctx, request)
	}
	if err != nil {
		return taskChangeLinkMutationError(msg, TaskChangeLinkMutationResult{
			TaskID: payload.TaskID, StateKnown: false,
		}, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{keyTaskID: payload.TaskID, "links": links})
}

func taskChangeLinkMutationError(msg *ws.Message, result TaskChangeLinkMutationResult, err error) (*ws.Message, error) {
	if result.OperationError == "" && err != nil {
		result.OperationError = "task change request operation failed"
	}
	details := map[string]interface{}{
		keyTaskID:     result.TaskID,
		"links":       result.Links,
		"state_known": result.StateKnown,
	}
	if result.OperationError != "" {
		details["operation_error"] = result.OperationError
	}
	if result.RollbackError != "" {
		details["rollback_error"] = result.RollbackError
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "failed to manage task change request", details)
}

func (h *Handlers) parseTaskChangeRequestPayload(ctx context.Context, msg *ws.Message) (taskChangeRequestPayload, *ws.Message, error) {
	fields, err := taskChangeRequestFields(msg.Payload)
	if err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return taskChangeRequestPayload{}, response, responseErr
	}
	if err := rejectUnknownTaskChangeRequestFields(fields); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		return taskChangeRequestPayload{}, response, responseErr
	}
	var payload taskChangeRequestPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return taskChangeRequestPayload{}, response, responseErr
	}
	link, response, responseErr := validateTaskChangeRequestOperation(msg, fields, payload)
	if response != nil || responseErr != nil {
		return taskChangeRequestPayload{}, response, responseErr
	}
	targetID, response, responseErr := h.authorizeTaskChangeRequest(ctx, msg, payload)
	if response != nil || responseErr != nil {
		return taskChangeRequestPayload{}, response, responseErr
	}
	return taskChangeRequestPayload{
		Operation:       payload.Operation,
		TaskID:          targetID,
		CallerTaskID:    payload.CallerTaskID,
		Provider:        link.Provider,
		RepositoryID:    link.RepositoryID,
		Number:          link.Number,
		OldProvider:     strings.ToLower(strings.TrimSpace(payload.OldProvider)),
		OldRepositoryID: payload.OldRepositoryID,
		OldNumber:       payload.OldNumber,
	}, nil, nil
}

func validateTaskChangeRequestOperation(
	msg *ws.Message, fields map[string]json.RawMessage, payload taskChangeRequestPayload,
) (TaskChangeLink, *ws.Message, error) {
	if payload.Operation != taskChangeOperationLink &&
		payload.Operation != taskChangeOperationUnlink &&
		payload.Operation != taskChangeOperationReplace {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "operation must be link, unlink, or replace", nil)
		return TaskChangeLink{}, response, responseErr
	}
	link, err := validateTaskChangeLink(payload.Provider, payload.RepositoryID, payload.Number)
	if err != nil || strings.TrimSpace(payload.TaskID) == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, provider, repository_id, and positive number are required", nil)
		return TaskChangeLink{}, response, responseErr
	}
	if payload.Operation == taskChangeOperationReplace {
		if _, err := validateTaskChangeLink(payload.OldProvider, payload.OldRepositoryID, payload.OldNumber); err != nil {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "old_provider, old_repository_id, and positive old_number are required", nil)
			return TaskChangeLink{}, response, responseErr
		}
		if link.Provider != strings.ToLower(strings.TrimSpace(payload.OldProvider)) {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "replacement must use the same provider", nil)
			return TaskChangeLink{}, response, responseErr
		}
	} else if hasTaskChangeRequestOldField(fields) {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "old identity fields are only valid for replace", nil)
		return TaskChangeLink{}, response, responseErr
	}
	return link, nil, nil
}

func (h *Handlers) authorizeTaskChangeRequest(
	ctx context.Context, msg *ws.Message, payload taskChangeRequestPayload,
) (string, *ws.Message, error) {
	if h.taskSvc == nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not available", nil)
		return "", response, responseErr
	}
	target, targetErr := h.taskSvc.GetTask(ctx, payload.TaskID)
	if targetErr != nil || target == nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
		return "", response, responseErr
	}
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	if !hasPrincipal || strings.TrimSpace(principal.WorkspaceID) == "" || strings.TrimSpace(payload.CallerTaskID) == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "trusted MCP caller identity is required", nil)
		return "", response, responseErr
	}
	if payload.CallerTaskID != principal.CallerTaskID || target.WorkspaceID != principal.WorkspaceID {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task is outside the caller workspace", nil)
		return "", response, responseErr
	}
	return target.ID, nil, nil
}

func taskChangeRequestFields(payload json.RawMessage) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("object payload is required")
	}
	return fields, nil
}

func rejectUnknownTaskChangeRequestFields(fields map[string]json.RawMessage) error {
	for field := range fields {
		switch field {
		case "operation", keyTaskID, "caller_task_id", "provider", "repository_id", "number", "old_provider", "old_repository_id", "old_number":
		default:
			return fmt.Errorf("unknown task change request field %q", field)
		}
	}
	return nil
}

func hasTaskChangeRequestOldField(fields map[string]json.RawMessage) bool {
	for _, field := range []string{"old_provider", "old_repository_id", "old_number"} {
		if _, ok := fields[field]; ok {
			return true
		}
	}
	return false
}

func taskChangeLinksUnavailable(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task change request link management is not available", nil)
}

func (h *Handlers) applyTaskChangeLink(ctx context.Context, msg *ws.Message, req TaskChangeLinkRequest, apply func(context.Context, TaskChangeLinkRequest) ([]TaskChangeLink, error)) (*ws.Message, error) {
	links, err := apply(ctx, req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{keyTaskID: req.TaskID, "links": links})
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
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	if !hasPrincipal || strings.TrimSpace(principal.WorkspaceID) == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "trusted MCP caller identity is required", nil)
		return TaskChangeLinkRequest{}, response, responseErr
	}
	if payload.CallerTaskID != principal.CallerTaskID || target.WorkspaceID != principal.WorkspaceID {
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
