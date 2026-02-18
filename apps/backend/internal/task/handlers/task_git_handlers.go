package handlers

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Git Snapshot and Commit Handlers

type wsGetGitSnapshotsRequest struct {
	SessionID string `json:"session_id"`
	Limit     int    `json:"limit,omitempty"`
}

func (h *TaskHandlers) wsGetGitSnapshots(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetGitSnapshotsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	snapshots, err := h.controller.GetGitSnapshots(ctx, req.SessionID, req.Limit)
	if err != nil {
		h.logger.Error("failed to get git snapshots", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get git snapshots", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": req.SessionID,
		"snapshots":  snapshots,
	})
}

func (h *TaskHandlers) wsGetSessionData(
	ctx context.Context, msg *ws.Message,
	logErrMsg, clientErrMsg, resultKey string,
	fn func(ctx context.Context, sessionID string) (any, error),
) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	result, err := fn(ctx, req.SessionID)
	if err != nil {
		h.logger.Error(logErrMsg, zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"session_id": req.SessionID,
		resultKey:    result,
	})
}

func (h *TaskHandlers) wsGetSessionCommits(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsGetSessionData(ctx, msg,
		"failed to get session commits", "Failed to get session commits", "commits",
		func(ctx context.Context, sessionID string) (any, error) {
			return h.controller.GetSessionCommits(ctx, sessionID)
		})
}

type wsGetCumulativeDiffRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsGetCumulativeDiff(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetCumulativeDiffRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	diff, err := h.controller.GetCumulativeDiff(ctx, req.SessionID)
	if err != nil {
		h.logger.Error("failed to get cumulative diff", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get cumulative diff", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"cumulative_diff": diff,
	})
}

// Session File Review Handlers

func (h *TaskHandlers) wsGetSessionFileReviews(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsGetSessionData(ctx, msg,
		"failed to get session file reviews", "Failed to get session file reviews", "reviews",
		func(ctx context.Context, sessionID string) (any, error) {
			return h.repo.GetSessionFileReviews(ctx, sessionID)
		})
}

type wsUpdateSessionFileReviewRequest struct {
	SessionID string `json:"session_id"`
	FilePath  string `json:"file_path"`
	Reviewed  bool   `json:"reviewed"`
	DiffHash  string `json:"diff_hash"`
}

func (h *TaskHandlers) wsUpdateSessionFileReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateSessionFileReviewRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.FilePath == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "file_path is required", nil)
	}

	review := &models.SessionFileReview{
		SessionID: req.SessionID,
		FilePath:  req.FilePath,
		Reviewed:  req.Reviewed,
		DiffHash:  req.DiffHash,
	}
	if req.Reviewed {
		now := time.Now().UTC()
		review.ReviewedAt = &now
	}

	if err := h.repo.UpsertSessionFileReview(ctx, review); err != nil {
		h.logger.Error("failed to update session file review", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update session file review", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"review":  review,
	})
}

type wsResetSessionFileReviewsRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsResetSessionFileReviews(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsResetSessionFileReviewsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	if err := h.repo.DeleteSessionFileReviews(ctx, req.SessionID); err != nil {
		h.logger.Error("failed to reset session file reviews", zap.Error(err), zap.String("session_id", req.SessionID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reset session file reviews", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}
