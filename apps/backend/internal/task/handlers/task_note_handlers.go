package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// wsGetTaskNote retrieves a task note.
func (h *TaskHandlers) wsGetTaskNote(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return getTaskModelResponse(ctx, msg, h.noteService.GetNote, dto.TaskNoteFromModel, "Failed to get task note")
}

// wsUpsertTaskNote creates or replaces a task note.
func (h *TaskHandlers) wsUpsertTaskNote(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Content   string `json:"content"`
		UpdatedBy string `json:"updated_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	note, err := h.noteService.UpsertNote(ctx, req.TaskID, req.Content, req.UpdatedBy)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task note: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskNoteFromModel(note))
}

// wsDeleteTaskNote deletes a task note.
func (h *TaskHandlers) wsDeleteTaskNote(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if err := h.noteService.DeleteNote(ctx, req.TaskID); err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskNoteNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task note not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task note: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{responseKeySuccess: true})
}
