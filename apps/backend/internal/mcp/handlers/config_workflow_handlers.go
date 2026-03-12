package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleCreateWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	workflow, err := h.taskSvc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: req.WorkspaceID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to create workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, workflow)
}

func (h *Handlers) handleUpdateWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkflowID  string  `json:"workflow_id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	workflow, err := h.taskSvc.UpdateWorkflow(ctx, req.WorkflowID, &service.UpdateWorkflowRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, workflow)
}

func (h *Handlers) handleDeleteWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workflowID, err := unmarshalStringField(msg.Payload, "workflow_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if workflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	if err := h.taskSvc.DeleteWorkflow(ctx, workflowID); err != nil {
		h.logger.Error("failed to delete workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleCreateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkflowID         string               `json:"workflow_id"`
		Name               string               `json:"name"`
		Position           int                  `json:"position"`
		Color              string               `json:"color"`
		Prompt             string               `json:"prompt"`
		IsStartStep        *bool                `json:"is_start_step"`
		AllowManualMove    *bool                `json:"allow_manual_move"`
		ShowInCommandPanel *bool                `json:"show_in_command_panel"`
		Events             *wfmodels.StepEvents `json:"events"`
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
		WorkflowID:         req.WorkflowID,
		Name:               req.Name,
		Position:           req.Position,
		Color:              req.Color,
		Prompt:             req.Prompt,
		IsStartStep:        req.IsStartStep,
		ShowInCommandPanel: req.ShowInCommandPanel,
		Events:             req.Events,
	}
	if req.AllowManualMove != nil {
		createReq.AllowManualMove = *req.AllowManualMove
	}

	resp, err := h.workflowCtrl.CreateStep(ctx, createReq)
	if err != nil {
		h.logger.Error("failed to create workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workflow step", nil)
	}
	h.publishWorkflowStepEvent(ctx, events.WorkflowStepCreated, resp.Step)
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleUpdateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		StepID                string               `json:"step_id"`
		Name                  *string              `json:"name"`
		Color                 *string              `json:"color"`
		Prompt                *string              `json:"prompt"`
		IsStartStep           *bool                `json:"is_start_step"`
		AllowManualMove       *bool                `json:"allow_manual_move"`
		ShowInCommandPanel    *bool                `json:"show_in_command_panel"`
		AutoArchiveAfterHours *int                 `json:"auto_archive_after_hours"`
		Events                *wfmodels.StepEvents `json:"events"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.StepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "step_id is required", nil)
	}

	updateReq := workflowctrl.UpdateStepRequest{
		ID:                    req.StepID,
		Name:                  req.Name,
		Color:                 req.Color,
		Prompt:                req.Prompt,
		IsStartStep:           req.IsStartStep,
		AllowManualMove:       req.AllowManualMove,
		ShowInCommandPanel:    req.ShowInCommandPanel,
		AutoArchiveAfterHours: req.AutoArchiveAfterHours,
		Events:                req.Events,
	}

	resp, err := h.workflowCtrl.UpdateStep(ctx, updateReq)
	if err != nil {
		h.logger.Error("failed to update workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workflow step", nil)
	}
	h.publishWorkflowStepEvent(ctx, events.WorkflowStepUpdated, resp.Step)
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

	// Fetch step before deleting to get workflow_id for the event
	stepResp, _ := h.workflowCtrl.GetStep(ctx, stepID)

	if err := h.workflowCtrl.DeleteStep(ctx, stepID); err != nil {
		h.logger.Error("failed to delete workflow step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete workflow step", nil)
	}
	if stepResp != nil {
		h.publishWorkflowStepEvent(ctx, events.WorkflowStepDeleted, stepResp.Step)
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

// publishWorkflowStepEvent publishes a workflow step event to the event bus.
func (h *Handlers) publishWorkflowStepEvent(ctx context.Context, eventType string, step *wfmodels.WorkflowStep) {
	if h.eventBus == nil || step == nil {
		return
	}
	data := map[string]interface{}{
		"step": map[string]interface{}{
			"id":                       step.ID,
			"workflow_id":              step.WorkflowID,
			"name":                     step.Name,
			"position":                 step.Position,
			"color":                    step.Color,
			"events":                   step.Events,
			"show_in_command_panel":    step.ShowInCommandPanel,
			"allow_manual_move":        step.AllowManualMove,
			"is_start_step":            step.IsStartStep,
			"auto_archive_after_hours": step.AutoArchiveAfterHours,
		},
	}
	if err := h.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "mcp-handlers", data)); err != nil {
		h.logger.Error("failed to publish workflow step event",
			zap.String("event_type", eventType),
			zap.String("step_id", step.ID),
			zap.Error(err))
	}
}
