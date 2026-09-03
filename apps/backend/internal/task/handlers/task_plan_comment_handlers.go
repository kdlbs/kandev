package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/planws"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *TaskHandlers) wsListTaskPlanComments(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req planws.TaskIDRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPlanCommentPayload(msg, err)
	}
	snapshot, err := h.planService.ListPlanComments(ctx, req.TaskID)
	if err != nil {
		return planws.PlanCommentError(msg, err, dto.TaskPlanCommentSnapshotFromModel(snapshot))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanCommentSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsCreateTaskPlanComment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID       string `json:"task_id"`
		PlanID       string `json:"plan_id"`
		ID           string `json:"id"`
		Body         string `json:"body"`
		SelectedText string `json:"selected_text"`
		AnchorFrom   int    `json:"anchor_from"`
		AnchorTo     int    `json:"anchor_to"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPlanCommentPayload(msg, err)
	}
	snapshot, err := h.planService.CreatePlanComment(ctx, service.CreatePlanCommentRequest{
		TaskID: req.TaskID, PlanID: req.PlanID, ID: req.ID, Body: req.Body,
		SelectedText: req.SelectedText, AnchorFrom: req.AnchorFrom, AnchorTo: req.AnchorTo,
	})
	if err != nil {
		return planws.PlanCommentError(msg, err, dto.TaskPlanCommentSnapshotFromModel(snapshot))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanCommentSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsUpdateTaskPlanComment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		PlanID          string `json:"plan_id"`
		ID              string `json:"id"`
		Body            string `json:"body"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPlanCommentPayload(msg, err)
	}
	snapshot, err := h.planService.UpdatePlanComment(ctx, service.UpdatePlanCommentRequest{
		TaskID: req.TaskID, PlanID: req.PlanID, ID: req.ID,
		Body: req.Body, ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		return planws.PlanCommentError(msg, err, dto.TaskPlanCommentSnapshotFromModel(snapshot))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanCommentSnapshotFromModel(snapshot))
}

func (h *TaskHandlers) wsDeleteTaskPlanComment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		PlanID          string `json:"plan_id"`
		ID              string `json:"id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return invalidPlanCommentPayload(msg, err)
	}
	snapshot, err := h.planService.DeletePlanComment(ctx, service.DeletePlanCommentRequest{
		TaskID: req.TaskID, PlanID: req.PlanID, ID: req.ID, ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		return planws.PlanCommentError(msg, err, dto.TaskPlanCommentSnapshotFromModel(snapshot))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanCommentSnapshotFromModel(snapshot))
}

func invalidPlanCommentPayload(msg *ws.Message, err error) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
}
