package handlers

import (
	"context"
	"encoding/json"

	githubsvc "github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// GitHubRateLimitService exposes only the local, provider-free snapshot path.
type GitHubRateLimitService interface {
	GetWorkspaceRateLimitSnapshot(
		ctx context.Context,
		workspaceID string,
	) (githubsvc.WorkspaceRateLimitSnapshot, error)
}

type githubRateLimitRequest struct {
	TaskID string `json:"task_id"`
}

// SetGitHubRateLimitService wires the task-scoped GitHub budget snapshot.
func (h *Handlers) SetGitHubRateLimitService(rateLimits GitHubRateLimitService) {
	h.githubRateLimits = rateLimits
}

func (h *Handlers) handleGetGitHubRateLimit(
	ctx context.Context,
	message *ws.Message,
) (*ws.Message, error) {
	if h.githubRateLimits == nil || h.taskSvc == nil {
		return ws.NewError(
			message.ID, message.Action, ws.ErrorCodeInternalError,
			"GitHub rate-limit snapshots are unavailable", nil,
		)
	}
	var request githubRateLimitRequest
	if err := json.Unmarshal(message.Payload, &request); err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeBadRequest, "Invalid payload", nil)
	}
	if request.TaskID == "" {
		return ws.NewError(
			message.ID, message.Action, ws.ErrorCodeValidation,
			"task_id is required", nil,
		)
	}
	task, err := h.taskSvc.GetTask(ctx, request.TaskID)
	if err != nil || task == nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeNotFound, "Task not found", nil)
	}
	snapshot, err := h.githubRateLimits.GetWorkspaceRateLimitSnapshot(ctx, task.WorkspaceID)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(message.ID, message.Action, snapshot)
}
