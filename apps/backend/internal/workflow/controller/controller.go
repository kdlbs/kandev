package controller

import (
	"context"

	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/service"
)

// Controller handles workflow-related requests
type Controller struct {
	svc *service.Service
}

// NewController creates a new workflow controller
func NewController(svc *service.Service) *Controller {
	return &Controller{svc: svc}
}

// Template responses

type ListTemplatesResponse struct {
	Templates []*models.WorkflowTemplate `json:"templates"`
}

type GetTemplateResponse struct {
	Template *models.WorkflowTemplate `json:"template"`
}

func (c *Controller) ListTemplates(ctx context.Context) (*ListTemplatesResponse, error) {
	templates, err := c.svc.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	return &ListTemplatesResponse{Templates: templates}, nil
}

func (c *Controller) GetTemplate(ctx context.Context, id string) (*GetTemplateResponse, error) {
	template, err := c.svc.GetTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return &GetTemplateResponse{Template: template}, nil
}

// Step responses

type ListStepsRequest struct {
	WorkflowID string `json:"workflow_id"`
}

type ListStepsResponse struct {
	Steps []*models.WorkflowStep `json:"steps"`
}

type GetStepResponse struct {
	Step *models.WorkflowStep `json:"step"`
}

type CreateStepsFromTemplateRequest struct {
	WorkflowID string `json:"workflow_id"`
	TemplateID string `json:"template_id"`
}

func (c *Controller) ListStepsByWorkflow(ctx context.Context, req ListStepsRequest) (*ListStepsResponse, error) {
	steps, err := c.svc.ListStepsByWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return nil, err
	}
	return &ListStepsResponse{Steps: steps}, nil
}

func (c *Controller) GetStep(ctx context.Context, id string) (*GetStepResponse, error) {
	step, err := c.svc.GetStep(ctx, id)
	if err != nil {
		return nil, err
	}
	return &GetStepResponse{Step: step}, nil
}

func (c *Controller) CreateStepsFromTemplate(ctx context.Context, req CreateStepsFromTemplateRequest) error {
	return c.svc.CreateStepsFromTemplate(ctx, req.WorkflowID, req.TemplateID)
}

// CreateStepRequest is the request for creating a single workflow step.
type CreateStepRequest struct {
	WorkflowID      string             `json:"workflow_id"`
	Name            string             `json:"name"`
	Position        int                `json:"position"`
	Color           string             `json:"color"`
	Prompt          string             `json:"prompt,omitempty"`
	Events          *models.StepEvents `json:"events,omitempty"`
	AllowManualMove bool               `json:"allow_manual_move"`
	IsStartStep     *bool              `json:"is_start_step,omitempty"`
}

// CreateStep creates a new workflow step.
func (c *Controller) CreateStep(ctx context.Context, req CreateStepRequest) (*GetStepResponse, error) {
	step := &models.WorkflowStep{
		WorkflowID:      req.WorkflowID,
		Name:            req.Name,
		Position:        req.Position,
		Color:           req.Color,
		Prompt:          req.Prompt,
		AllowManualMove: req.AllowManualMove,
	}
	if req.Events != nil {
		step.Events = *req.Events
	}
	if req.IsStartStep != nil {
		step.IsStartStep = *req.IsStartStep
	}
	if err := c.svc.CreateStep(ctx, step); err != nil {
		return nil, err
	}
	return &GetStepResponse{Step: step}, nil
}

// UpdateStepRequest is the request for updating a workflow step.
type UpdateStepRequest struct {
	ID                    string             `json:"id"`
	Name                  *string            `json:"name,omitempty"`
	Position              *int               `json:"position,omitempty"`
	Color                 *string            `json:"color,omitempty"`
	Prompt                *string            `json:"prompt,omitempty"`
	Events                *models.StepEvents `json:"events,omitempty"`
	AllowManualMove       *bool              `json:"allow_manual_move,omitempty"`
	IsStartStep           *bool              `json:"is_start_step,omitempty"`
	AutoArchiveAfterHours *int               `json:"auto_archive_after_hours,omitempty"`
}

// UpdateStep updates an existing workflow step.
func (c *Controller) UpdateStep(ctx context.Context, req UpdateStepRequest) (*GetStepResponse, error) {
	step, err := c.svc.GetStep(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		step.Name = *req.Name
	}
	if req.Position != nil {
		step.Position = *req.Position
	}
	if req.Color != nil {
		step.Color = *req.Color
	}
	if req.Prompt != nil {
		step.Prompt = *req.Prompt
	}
	if req.Events != nil {
		step.Events = *req.Events
	}
	if req.AllowManualMove != nil {
		step.AllowManualMove = *req.AllowManualMove
	}
	if req.IsStartStep != nil {
		step.IsStartStep = *req.IsStartStep
	}
	if req.AutoArchiveAfterHours != nil {
		step.AutoArchiveAfterHours = *req.AutoArchiveAfterHours
	}
	if err := c.svc.UpdateStep(ctx, step); err != nil {
		return nil, err
	}
	return &GetStepResponse{Step: step}, nil
}

// DeleteStep deletes a workflow step.
func (c *Controller) DeleteStep(ctx context.Context, id string) error {
	return c.svc.DeleteStep(ctx, id)
}

// ReorderStepsRequest is the request for reordering workflow steps.
type ReorderStepsRequest struct {
	WorkflowID string   `json:"workflow_id"`
	StepIDs    []string `json:"step_ids"`
}

// ReorderSteps reorders workflow steps for a workflow.
func (c *Controller) ReorderSteps(ctx context.Context, req ReorderStepsRequest) error {
	return c.svc.ReorderSteps(ctx, req.WorkflowID, req.StepIDs)
}

// History responses

type ListHistoryRequest struct {
	SessionID string `json:"session_id"`
}

type ListHistoryResponse struct {
	History []*models.SessionStepHistory `json:"history"`
}

func (c *Controller) ListHistoryBySession(ctx context.Context, req ListHistoryRequest) (*ListHistoryResponse, error) {
	history, err := c.svc.ListHistoryBySession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	return &ListHistoryResponse{History: history}, nil
}

// Export/Import types and methods

// ImportWorkflowsRequest carries import data.
type ImportWorkflowsRequest struct {
	WorkspaceID string                 `json:"workspace_id"`
	Data        *models.WorkflowExport `json:"data"`
}

// ExportWorkflow exports a single workflow.
func (c *Controller) ExportWorkflow(ctx context.Context, workflowID string) (*models.WorkflowExport, error) {
	return c.svc.ExportWorkflow(ctx, workflowID)
}

// ExportWorkflows exports all workflows for a workspace.
func (c *Controller) ExportWorkflows(ctx context.Context, workspaceID string) (*models.WorkflowExport, error) {
	return c.svc.ExportWorkflows(ctx, workspaceID)
}

// ImportWorkflows imports workflows into a workspace.
func (c *Controller) ImportWorkflows(ctx context.Context, req ImportWorkflowsRequest) (*service.ImportResult, error) {
	return c.svc.ImportWorkflows(ctx, req.WorkspaceID, req.Data)
}
