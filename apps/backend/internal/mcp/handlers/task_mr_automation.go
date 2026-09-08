package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// TaskMRAutomationService is the task-bound GitLab automation surface
// exposed to agents. The MCP server binds the task ID; agents cannot mutate
// another task through these tools. Mirrors TaskPRAutomationService.
type TaskMRAutomationService interface {
	GetTaskMRAutomationResponse(ctx context.Context, taskID string) (*gitlab.TaskMRAutomationResponse, error)
	UpdateTaskMRAutomationOptions(ctx context.Context, taskID string, patch gitlab.TaskMRAutomationPatch) (*gitlab.TaskMRAutomationResponse, error)
}

func (h *Handlers) SetTaskMRAutomationService(automation TaskMRAutomationService) {
	h.taskMRAutomation = automation
}

// mrAutomationTaskIDPayload decodes the {task_id} envelope shared by both MR
// automation handlers, returning a ready-to-return error response when the
// payload is malformed or task_id is missing.
func mrAutomationTaskIDPayload(msg *ws.Message) (string, *ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		resp, respErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return "", resp, respErr
	}
	if req.TaskID == "" {
		resp, respErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		return "", resp, respErr
	}
	return req.TaskID, nil, nil
}

func (h *Handlers) handleGetTaskMRAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, errResp, err := mrAutomationTaskIDPayload(msg)
	if errResp != nil || err != nil {
		return errResp, err
	}
	if h.taskMRAutomation == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "GitLab MR automation is not available", nil)
	}
	options, err := h.taskMRAutomation.GetTaskMRAutomationResponse(ctx, taskID)
	if err != nil {
		return h.mrAutomationErrorResponse(msg, taskID, "get task MR automation failed", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, options)
}

// mrAutomationErrorResponse logs the underlying cause server-side and returns
// a stable, sanitized client message — a database/GitLab-client error must
// not reach the MCP caller verbatim. A denied task-visibility check (opt-in
// auth) is reported as not-found, matching the "foreign workspace is
// indistinguishable from nonexistent" convention used elsewhere.
func (h *Handlers) mrAutomationErrorResponse(msg *ws.Message, taskID, logMsg string, err error) (*ws.Message, error) {
	if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	}
	// Naming an MR that isn't linked to this task (or naming one only
	// partially) is a caller mistake — report it as such instead of burying
	// it in a generic internal error.
	if errors.Is(err, gitlab.ErrTaskMRNotLinked) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	h.logger.Error(logMsg, zap.String("task_id", taskID), zap.Error(err))
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to process MR automation request", nil)
}

func (h *Handlers) handleUpdateTaskMRAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(msg.Payload, &fields); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if hasLifecyclePromptOverride(fields) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "lifecycle prompt overrides are not supported", nil)
	}
	taskID, errResp, err := mrAutomationTaskIDPayload(msg)
	if errResp != nil || err != nil {
		return errResp, err
	}
	// repository_id/project_path/mr_iid optionally scope the switch changes
	// to one linked MR; omitting all three applies them to every linked MR,
	// which is what an agent with no MR identity to send gets.
	var req struct {
		RepositoryID            *string `json:"repository_id"`
		ProjectPath             *string `json:"project_path"`
		MRIID                   *int    `json:"mr_iid"`
		AutoFixEnabled          *bool   `json:"auto_fix_enabled"`
		AutoMergeEnabled        *bool   `json:"auto_merge_enabled"`
		AutoFixPromptOverride   *string `json:"auto_fix_prompt_override"`
		PromptOnReviewRequested *bool   `json:"prompt_on_review_requested"`
		PromptOnMerged          *bool   `json:"prompt_on_merged"`
		PromptOnClosed          *bool   `json:"prompt_on_closed"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	patch := gitlab.TaskMRAutomationPatch{
		RepositoryID:            req.RepositoryID,
		ProjectPath:             req.ProjectPath,
		MRIID:                   req.MRIID,
		AutoFixEnabled:          req.AutoFixEnabled,
		AutoMergeEnabled:        req.AutoMergeEnabled,
		AutoFixPromptOverride:   req.AutoFixPromptOverride,
		PromptOnReviewRequested: req.PromptOnReviewRequested,
		PromptOnMerged:          req.PromptOnMerged,
		PromptOnClosed:          req.PromptOnClosed,
	}
	if !patch.HasAny() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "at least one MR automation option is required", nil)
	}
	if h.taskMRAutomation == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "GitLab MR automation is not available", nil)
	}
	options, err := h.taskMRAutomation.UpdateTaskMRAutomationOptions(ctx, taskID, patch)
	if err != nil {
		return h.mrAutomationErrorResponse(msg, taskID, "update task MR automation failed", err)
	}
	if h.eventBus != nil {
		event := bus.NewEvent(events.GitLabTaskMROptionsUpdated, "mcp", options)
		if err := h.eventBus.Publish(ctx, events.GitLabTaskMROptionsUpdated, event); err != nil {
			h.logger.Warn("failed to publish MCP MR automation options update", zap.Error(err))
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, options)
}
