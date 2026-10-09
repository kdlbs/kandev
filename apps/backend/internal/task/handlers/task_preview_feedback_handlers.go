package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *TaskHandlers) wsListTaskPreviewFeedback(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPreviewFeedbackPayload(msg)
	}
	snapshot, err := h.planService.ListPreviewFeedback(ctx, req.TaskID)
	if err != nil {
		return previewFeedbackError(msg, err, snapshot)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPreviewFeedbackSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsCreateTaskPreviewFeedback(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID                 string                               `json:"task_id"`
		ID                     string                               `json:"id"`
		Kind                   models.TaskPreviewFeedbackKind       `json:"kind"`
		Comment                string                               `json:"comment"`
		SourceKind             models.TaskPreviewFeedbackSourceKind `json:"source_kind"`
		SourceSessionID        string                               `json:"source_session_id"`
		SourceLabel            string                               `json:"source_label"`
		SourcePath             string                               `json:"source_path"`
		PageRoute              string                               `json:"page_route"`
		PageTitle              string                               `json:"page_title"`
		SelectedText           string                               `json:"selected_text"`
		TextAnchor             json.RawMessage                      `json:"text_anchor"`
		ElementSnapshot        json.RawMessage                      `json:"element_snapshot"`
		CaptureRect            json.RawMessage                      `json:"capture_rect"`
		ScreenshotAttachmentID string                               `json:"screenshot_attachment_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPreviewFeedbackPayload(msg)
	}
	snapshot, err := h.planService.CreatePreviewFeedback(ctx, &models.TaskPreviewFeedback{
		ID: req.ID, TaskID: req.TaskID, Kind: req.Kind, Comment: req.Comment,
		SourceKind: req.SourceKind, SourceSessionID: req.SourceSessionID,
		SourceLabel: req.SourceLabel, SourcePath: req.SourcePath,
		PageRoute: req.PageRoute, PageTitle: req.PageTitle,
		SelectedText: req.SelectedText, TextAnchor: req.TextAnchor,
		ElementSnapshot: req.ElementSnapshot, CaptureRect: req.CaptureRect,
		ScreenshotAttachmentID: req.ScreenshotAttachmentID,
	})
	if err != nil {
		return previewFeedbackError(msg, err, snapshot)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPreviewFeedbackSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsUpdateTaskPreviewFeedback(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		ID              string `json:"id"`
		Comment         string `json:"comment"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPreviewFeedbackPayload(msg)
	}
	snapshot, err := h.planService.UpdatePreviewFeedback(
		ctx, req.TaskID, req.ID, req.Comment, req.ExpectedVersion,
	)
	if err != nil {
		return previewFeedbackError(msg, err, snapshot)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPreviewFeedbackSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsDeleteTaskPreviewFeedback(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		ID              string `json:"id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPreviewFeedbackPayload(msg)
	}
	snapshot, err := h.planService.DeletePreviewFeedback(ctx, req.TaskID, req.ID, req.ExpectedVersion)
	if err != nil {
		return previewFeedbackError(msg, err, snapshot)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPreviewFeedbackSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsClearTaskPreviewFeedback(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID           string `json:"task_id"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPreviewFeedbackPayload(msg)
	}
	snapshot, err := h.planService.ClearPreviewFeedback(ctx, req.TaskID, req.ExpectedRevision)
	if err != nil {
		return previewFeedbackError(msg, err, snapshot)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPreviewFeedbackSnapshotFromModel(snapshot))
}

func invalidPreviewFeedbackPayload(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload", nil)
}

func previewFeedbackError(
	msg *ws.Message,
	err error,
	snapshot *models.TaskPreviewFeedbackSnapshot,
) (*ws.Message, error) {
	if errors.Is(err, service.ErrTaskPreviewFeedbackChanged) {
		details := map[string]interface{}{}
		if snapshot != nil {
			details["snapshot"] = dto.TaskPreviewFeedbackSnapshotFromModel(snapshot)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodePreviewFeedbackChanged,
			"Task preview feedback changed", details)
	}
	code := ws.ErrorCodeValidation
	message := "Invalid preview feedback"
	switch {
	case errors.Is(err, repository.ErrTaskNotFound):
		code, message = ws.ErrorCodeNotFound, "Task not found"
	case errors.Is(err, models.ErrAttachmentClaimConflict), errors.Is(err, models.ErrAttachmentForbidden):
		code, message = ws.ErrorCodeConflict, "Screenshot attachment is unavailable"
	case errors.Is(err, previewfeedback.ErrTooManyItems), errors.Is(err, previewfeedback.ErrTooManyScreenshots),
		errors.Is(err, previewfeedback.ErrCollectionTooLarge):
		message = "Task preview feedback collection is too large"
	case errors.Is(err, previewfeedback.ErrCommentRequired):
		message = "Preview feedback comment is required"
	case errors.Is(err, previewfeedback.ErrCommentTooLarge), errors.Is(err, previewfeedback.ErrCaptureTooLarge),
		errors.Is(err, previewfeedback.ErrSourceIdentityLarge):
		message = "Preview feedback is too large"
	case errors.Is(err, service.ErrTaskIDRequired), errors.Is(err, service.ErrPreviewFeedbackIDRequired),
		errors.Is(err, service.ErrPreviewFeedbackIDInvalid), errors.Is(err, service.ErrPreviewFeedbackVersionRequired),
		errors.Is(err, service.ErrPreviewFeedbackRevisionInvalid), errors.Is(err, previewfeedback.ErrCaptureInvalid):
	default:
		code, message = ws.ErrorCodeInternalError, "Failed to update task preview feedback"
	}
	return ws.NewError(msg.ID, msg.Action, code, message, nil)
}
