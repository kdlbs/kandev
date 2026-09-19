package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/planws"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// handleEditTaskPlan applies one exact, unique agent edit to a task plan.
func (h *Handlers) handleEditTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string  `json:"task_id"`
		ExpectedVersion string  `json:"expected_version"`
		OldText         string  `json:"old_text"`
		NewText         *string `json:"new_text"`
		AllowTruncation bool    `json:"allow_truncation"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.NewText == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"new_text is required; use an empty string to delete the match", nil)
	}

	result, err := h.planService.EditPlan(ctx, service.ExactPlanEditRequest{
		TaskID:          req.TaskID,
		ExpectedVersion: req.ExpectedVersion,
		OldText:         req.OldText,
		NewText:         *req.NewText,
		AllowTruncation: req.AllowTruncation,
	})
	if err != nil {
		return planws.EditError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, planWritePayload(
		dto.TaskPlanFromModel(result.Plan), result.Plan.WriteVersion, "", result.PriorRevisionNumber,
	))
}
