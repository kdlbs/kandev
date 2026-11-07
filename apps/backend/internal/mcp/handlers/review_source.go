package handlers

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/authz"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) handleSetReviewSource(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID       string `json:"task_id"`
		WorktreePath string `json:"worktree_path"`
		BaseRef      string `json:"base_ref"`
		HeadRef      string `json:"head_ref"`
		Clear        bool   `json:"clear"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	// Authorize the target before examining a caller-supplied local path.
	if err := h.taskSvc.AuthorizeTaskScope(ctx, req.TaskID, authz.ScopeTaskWrite); err != nil {
		return reviewLaunchError(msg, err)
	}
	var value any
	if req.Clear {
		if req.WorktreePath != "" || req.BaseRef != "" || req.HeadRef != "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "clear cannot be combined with source fields", nil)
		}
	} else {
		source := review.Source{WorktreePath: req.WorktreePath, BaseRef: req.BaseRef, HeadRef: req.HeadRef}
		if err := review.ValidateSource(ctx, source); err != nil {
			return reviewLaunchError(msg, err)
		}
		value = source
	}
	task, err := h.taskSvc.UpdateTaskMetadata(ctx, req.TaskID, map[string]any{review.SourceMetadataKey: value})
	if err != nil {
		return reviewLaunchError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"task_id": task.ID, "review_source": task.Metadata[review.SourceMetadataKey]})
}

func (h *Handlers) manualReviewOrigin(ctx context.Context, taskID, sessionID string) *review.OriginIdentity {
	writer := h.manualReviewWriter(ctx, taskID, sessionID)
	if writer == nil || (writer.AgentProfileID == "" && writer.ExecutionProfileID == "") {
		return nil
	}
	return review.OriginFromSession(writer)
}

func (h *Handlers) manualReviewWriter(ctx context.Context, taskID, sessionID string) *models.TaskSession {
	if sessionID != "" {
		return h.manualReviewNamedWriter(ctx, taskID, sessionID)
	}
	if h.taskSvc == nil {
		return nil
	}
	sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, session := range sessions {
		if session != nil && session.IsPrimary {
			return session
		}
	}
	return nil
}

func (h *Handlers) manualReviewNamedWriter(ctx context.Context, taskID, sessionID string) *models.TaskSession {
	if h.sessionRepo == nil {
		return nil
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil
	}
	return session
}
