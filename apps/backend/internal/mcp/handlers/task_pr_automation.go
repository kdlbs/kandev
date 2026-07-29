package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// TaskPRAutomationService is the task-bound GitHub automation surface exposed
// to agents. The MCP server binds the task ID; agents cannot mutate another
// task through these tools.
type TaskPRAutomationService interface {
	GetTaskCIOptionsResponse(ctx context.Context, taskID string) (*github.TaskCIOptionsResponse, error)
	UpdateTaskCIOptions(ctx context.Context, taskID string, patch github.TaskCIOptionsPatch) (*github.TaskCIOptionsResponse, error)
}

func (h *Handlers) SetTaskPRAutomationService(automation TaskPRAutomationService) {
	h.taskPRAutomation = automation
}

func (h *Handlers) handleGetTaskPRAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if h.taskPRAutomation == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "GitHub PR automation is not available", nil)
	}
	options, err := h.taskPRAutomation.GetTaskCIOptionsResponse(ctx, req.TaskID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get PR automation options: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, options)
}

func (h *Handlers) handleUpdateTaskPRAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(msg.Payload, &fields); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if hasLifecyclePromptOverride(fields) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "lifecycle prompt overrides are not supported", nil)
	}
	var req struct {
		TaskID                  string  `json:"task_id"`
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
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	patch := github.TaskCIOptionsPatch{
		AutoFixEnabled:          req.AutoFixEnabled,
		AutoMergeEnabled:        req.AutoMergeEnabled,
		AutoFixPromptOverride:   req.AutoFixPromptOverride,
		PromptOnReviewRequested: req.PromptOnReviewRequested,
		PromptOnMerged:          req.PromptOnMerged,
		PromptOnClosed:          req.PromptOnClosed,
	}
	if !patch.HasAny() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "at least one PR automation option is required", nil)
	}
	if h.taskPRAutomation == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "GitHub PR automation is not available", nil)
	}
	options, err := h.taskPRAutomation.UpdateTaskCIOptions(ctx, req.TaskID, patch)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update PR automation options: "+err.Error(), nil)
	}
	if h.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskCIOptionsUpdated, "mcp", options)
		if err := h.eventBus.Publish(ctx, events.GitHubTaskCIOptionsUpdated, event); err != nil {
			h.logger.Warn("failed to publish MCP PR automation options update", zap.Error(err))
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, options)
}

func hasLifecyclePromptOverride(fields map[string]json.RawMessage) bool {
	for _, field := range []string{"review_prompt_override", "merged_prompt_override", "closed_prompt_override"} {
		if _, ok := fields[field]; ok {
			return true
		}
	}
	return false
}
