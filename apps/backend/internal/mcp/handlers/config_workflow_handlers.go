package handlers

import (
	"context"
	"encoding/json"

	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleCreateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkflowID  string `json:"workflow_id"`
		Name        string `json:"name"`
		Position    int    `json:"position"`
		Color       string `json:"color"`
		Prompt      string `json:"prompt"`
		IsStartStep *bool  `json:"is_start_step"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	createReq := workflowctrl.CreateStepRequest{
		WorkflowID:  req.WorkflowID,
		Name:        req.Name,
		Position:    req.Position,
		Color:       req.Color,
		Prompt:      req.Prompt,
		IsStartStep: req.IsStartStep,
	}

	resp, err := h.workflowCtrl.CreateStep(ctx, createReq)
	if err != nil {
		h.logger.Error("failed to create workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workflow step", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleUpdateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		StepID      string  `json:"step_id"`
		Name        *string `json:"name"`
		Color       *string `json:"color"`
		Prompt      *string `json:"prompt"`
		IsStartStep *bool   `json:"is_start_step"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.StepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "step_id is required", nil)
	}

	updateReq := workflowctrl.UpdateStepRequest{
		ID:          req.StepID,
		Name:        req.Name,
		Color:       req.Color,
		Prompt:      req.Prompt,
		IsStartStep: req.IsStartStep,
	}

	resp, err := h.workflowCtrl.UpdateStep(ctx, updateReq)
	if err != nil {
		h.logger.Error("failed to update workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workflow step", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleDeleteWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	stepID, err := unmarshalStringField(msg.Payload, "step_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if stepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "step_id is required", nil)
	}

	if err := h.workflowCtrl.DeleteStep(ctx, stepID); err != nil {
		h.logger.Error("failed to delete workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete workflow step", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleReorderWorkflowSteps(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkflowID string   `json:"workflow_id"`
		StepIDs    []string `json:"step_ids"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if len(req.StepIDs) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "step_ids is required", nil)
	}

	if err := h.workflowCtrl.ReorderSteps(ctx, workflowctrl.ReorderStepsRequest{
		WorkflowID: req.WorkflowID,
		StepIDs:    req.StepIDs,
	}); err != nil {
		h.logger.Error("failed to reorder workflow steps", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reorder workflow steps", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}
