package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflow/stepevents"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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

func (h *Handlers) handleExportWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workflowID, err := unmarshalStringField(msg.Payload, "workflow_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if workflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	export, err := h.workflowSvc.ExportWorkflow(ctx, workflowID)
	if err != nil {
		h.logger.Error("failed to export workflow", zap.Error(err))
		return h.workflowScopeError(msg, err, "Failed to export workflow")
	}
	return ws.NewResponse(msg.ID, msg.Action, export)
}

// handleImportWorkflow imports one or more workflows into a workspace from a
// portable document. The document is the same YAML/JSON envelope produced by
// the export endpoint; YAML parsing accepts JSON too, matching the HTTP
// /workspaces/{id}/workflows/import contract.
func (h *Handlers) handleImportWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Document    string `json:"document"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.Document == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "document is required", nil)
	}
	// Mirror the 1 MB cap the HTTP import endpoint enforces — the WS tunnel
	// permits much larger frames, so bound the document before parsing it.
	const maxImportSize = 1 << 20
	if len(req.Document) > maxImportSize {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "document exceeds 1MB limit", nil)
	}

	var export wfmodels.WorkflowExport
	if err := yaml.Unmarshal([]byte(req.Document), &export); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid document: "+err.Error(), nil)
	}
	// Validate the document up front: a malformed envelope (wrong type/version,
	// missing names, duplicate positions, bad move_to_step refs) is a client
	// error, and surfacing the detail is what lets the calling agent fix its
	// document. Past this point any ImportWorkflows error is a server-side DB
	// failure, so it gets a generic internal error without leaking internals —
	// matching the sibling workflow handlers.
	if err := export.Validate(); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid workflow document: "+err.Error(), nil)
	}

	result, err := h.workflowSvc.ImportWorkflows(ctx, req.WorkspaceID, &export)
	if err != nil {
		h.logger.Error("failed to import workflows", zap.Error(err))
		return h.workflowScopeError(msg, err, "Failed to import workflows")
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) handleCreateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkflowID                 string               `json:"workflow_id"`
		Name                       string               `json:"name"`
		Position                   int                  `json:"position"`
		Color                      string               `json:"color"`
		Prompt                     string               `json:"prompt"`
		IsStartStep                *bool                `json:"is_start_step"`
		AllowManualMove            *bool                `json:"allow_manual_move"`
		ShowInCommandPanel         *bool                `json:"show_in_command_panel"`
		AutoAdvanceRequiresSignal  *bool                `json:"auto_advance_requires_signal"`
		CancelTriggersTurnComplete *bool                `json:"cancel_triggers_turn_complete"`
		WIPLimit                   *int                 `json:"wip_limit"`
		PullFromStepID             *string              `json:"pull_from_step_id"`
		Events                     *wfmodels.StepEvents `json:"events"`
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
		WorkflowID:                 req.WorkflowID,
		Name:                       req.Name,
		Position:                   req.Position,
		Color:                      req.Color,
		Prompt:                     req.Prompt,
		IsStartStep:                req.IsStartStep,
		ShowInCommandPanel:         req.ShowInCommandPanel,
		AutoAdvanceRequiresSignal:  req.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: req.CancelTriggersTurnComplete,
		WIPLimit:                   req.WIPLimit,
		PullFromStepID:             req.PullFromStepID,
		Events:                     req.Events,
	}
	if req.AllowManualMove != nil {
		createReq.AllowManualMove = *req.AllowManualMove
	}

	resp, err := h.workflowCtrl.CreateStep(ctx, createReq)
	if err != nil {
		h.logger.Error("failed to create workflow step", zap.Error(err))
		return h.workflowScopeError(msg, err, "Failed to create workflow step")
	}
	h.publishWorkflowStepEvents(ctx, events.WorkflowStepUpdated, resp.DemotedStartSteps)
	h.publishWorkflowStepEvent(ctx, events.WorkflowStepCreated, resp.Step)
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *Handlers) handleUpdateWorkflowStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		StepID                     string               `json:"step_id"`
		Name                       *string              `json:"name"`
		Color                      *string              `json:"color"`
		Prompt                     *string              `json:"prompt"`
		IsStartStep                *bool                `json:"is_start_step"`
		AllowManualMove            *bool                `json:"allow_manual_move"`
		ShowInCommandPanel         *bool                `json:"show_in_command_panel"`
		AutoArchiveAfterHours      *int                 `json:"auto_archive_after_hours"`
		AutoAdvanceRequiresSignal  *bool                `json:"auto_advance_requires_signal"`
		CancelTriggersTurnComplete *bool                `json:"cancel_triggers_turn_complete"`
		WIPLimit                   *int                 `json:"wip_limit"`
		PullFromStepID             *string              `json:"pull_from_step_id"`
		Events                     *wfmodels.StepEvents `json:"events"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.StepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "step_id is required", nil)
	}

	updateReq := workflowctrl.UpdateStepRequest{
		ID:                         req.StepID,
		Name:                       req.Name,
		Color:                      req.Color,
		Prompt:                     req.Prompt,
		IsStartStep:                req.IsStartStep,
		AllowManualMove:            req.AllowManualMove,
		ShowInCommandPanel:         req.ShowInCommandPanel,
		AutoArchiveAfterHours:      req.AutoArchiveAfterHours,
		AutoAdvanceRequiresSignal:  req.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: req.CancelTriggersTurnComplete,
		WIPLimit:                   req.WIPLimit,
		PullFromStepID:             req.PullFromStepID,
		Events:                     req.Events,
	}

	resp, err := h.workflowCtrl.UpdateStep(ctx, updateReq)
	if err != nil {
		h.logger.Error("failed to update workflow step", zap.Error(err))
		return h.workflowScopeError(msg, err, "Failed to update workflow step")
	}
	h.publishWorkflowStepEvents(ctx, events.WorkflowStepUpdated, resp.DemotedStartSteps)
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
		return h.workflowScopeError(msg, err, "Failed to delete workflow step")
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
		return h.workflowScopeError(msg, err, "Failed to reorder workflow steps")
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

// workflowScopeError answers a workflow-surface failure. A resource the
// caller may not see, and one that does not exist, share the single not-found
// reply the workflow package produces for both; anything else is a genuine
// server failure. Without this, an agent that named a workflow, step or task
// it cannot reach was told the backend broke.
func (h *Handlers) workflowScopeError(msg *ws.Message, err error, clientErrMsg string) (*ws.Message, error) {
	if workflowsvc.IsNotFound(err) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, clientErrMsg, nil)
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
}

// workflowStepEventSource labels workflow-step events published by this surface.
const workflowStepEventSource = "mcp-handlers"

// workflowStepEvents returns the shared workflow-step publisher for this
// surface. Unlike the REST handlers, this Handlers value is assembled field by
// field — NewHandlers plus the Set* wiring, and tests build it as a struct
// literal — so eventBus is not final at construction. Building the publisher
// at publish time reads whatever bus is actually wired; the allocation is
// negligible next to the bus dispatch.
func (h *Handlers) workflowStepEvents() *stepevents.Publisher {
	return stepevents.NewPublisher(h.eventBus, workflowStepEventSource, h.logger)
}

// publishWorkflowStepEvent publishes a workflow step event to the event bus.
func (h *Handlers) publishWorkflowStepEvent(ctx context.Context, eventType string, step *wfmodels.WorkflowStep) {
	h.workflowStepEvents().Publish(ctx, eventType, step)
}

func (h *Handlers) publishWorkflowStepEvents(ctx context.Context, eventType string, steps []*wfmodels.WorkflowStep) {
	h.workflowStepEvents().PublishAll(ctx, eventType, steps)
}
