package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
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

// TaskPRAutoFixOutcomeService accepts the server-resolved disposition of the
// current GitHub PR auto-fix turn. The MCP server supplies task and session
// identity from its bound execution, not from tool arguments.
type TaskPRAutoFixOutcomeService interface {
	ReportTaskPRAutoFixOutcome(ctx context.Context, taskID, sessionID, outcome, summary string) error
}

func (h *Handlers) SetTaskPRAutomationService(automation TaskPRAutomationService) {
	h.taskPRAutomation = automation
}

func (h *Handlers) SetTaskPRAutoFixOutcomeService(outcomes TaskPRAutoFixOutcomeService) {
	h.taskPRAutoFixOutcome = outcomes
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
		RepositoryID            *string `json:"repository_id"`
		PRNumber                *int    `json:"pr_number"`
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
		RepositoryID:            req.RepositoryID,
		PRNumber:                req.PRNumber,
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
		if errors.Is(err, github.ErrTaskPRNotLinked) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
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

func (h *Handlers) handleReportTaskPRAutoFixOutcome(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Outcome   string `json:"outcome"`
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Outcome = strings.TrimSpace(req.Outcome)
	req.Summary = strings.TrimSpace(req.Summary)
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "MCP caller identity is unavailable", nil)
	}
	if (req.TaskID != "" && req.TaskID != principal.CallerTaskID) ||
		(req.SessionID != "" && req.SessionID != principal.CallerSessionID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task_id and session_id must match the current MCP caller", nil)
	}
	req.TaskID = principal.CallerTaskID
	req.SessionID = principal.CallerSessionID
	if req.Outcome != string(github.TaskCIAutoFixOutcomeActionTaken) &&
		req.Outcome != string(github.TaskCIAutoFixOutcomeNonActionable) &&
		req.Outcome != string(github.TaskCIAutoFixOutcomeBlocked) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "outcome must be action_taken, non_actionable, or blocked", nil)
	}
	if req.Summary == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "summary is required", nil)
	}
	if h.taskPRAutoFixOutcome == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "GitHub PR auto-fix outcome reporting is not available", nil)
	}
	if err := h.taskPRAutoFixOutcome.ReportTaskPRAutoFixOutcome(
		ctx, req.TaskID, req.SessionID, req.Outcome, req.Summary,
	); err != nil {
		switch {
		case errors.Is(err, github.ErrTaskCIAutoFixAttemptNotFound),
			errors.Is(err, github.ErrTaskCIAutoFixOutcomeInvalid):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		default:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to record PR auto-fix outcome: "+err.Error(), nil)
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]string{
		"status":  "recorded",
		"outcome": req.Outcome,
	})
}

func hasLifecyclePromptOverride(fields map[string]json.RawMessage) bool {
	for _, field := range []string{"review_prompt_override", "merged_prompt_override", "closed_prompt_override"} {
		if _, ok := fields[field]; ok {
			return true
		}
	}
	return false
}
