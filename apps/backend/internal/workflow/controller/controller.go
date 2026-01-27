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
	BoardID string `json:"board_id"`
}

type ListStepsResponse struct {
	Steps []*models.WorkflowStep `json:"steps"`
}

type GetStepResponse struct {
	Step *models.WorkflowStep `json:"step"`
}

type CreateStepsFromTemplateRequest struct {
	BoardID    string `json:"board_id"`
	TemplateID string `json:"template_id"`
}

func (c *Controller) ListStepsByBoard(ctx context.Context, req ListStepsRequest) (*ListStepsResponse, error) {
	steps, err := c.svc.ListStepsByBoard(ctx, req.BoardID)
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
	return c.svc.CreateStepsFromTemplate(ctx, req.BoardID, req.TemplateID)
}

// CreateStepRequest is the request for creating a single workflow step.
type CreateStepRequest struct {
	BoardID         string  `json:"board_id"`
	Name            string  `json:"name"`
	StepType        string  `json:"step_type"`
	Position        int     `json:"position"`
	Color           string  `json:"color"`
	AutoStartAgent  bool    `json:"auto_start_agent"`
	PlanMode        bool    `json:"plan_mode"`
	RequireApproval bool    `json:"require_approval"`
	PromptPrefix    string  `json:"prompt_prefix"`
	PromptSuffix    string  `json:"prompt_suffix"`
	AllowManualMove bool    `json:"allow_manual_move"`
}

// CreateStep creates a new workflow step.
func (c *Controller) CreateStep(ctx context.Context, req CreateStepRequest) (*GetStepResponse, error) {
	step := &models.WorkflowStep{
		BoardID:         req.BoardID,
		Name:            req.Name,
		StepType:        models.StepType(req.StepType),
		Position:        req.Position,
		Color:           req.Color,
		AutoStartAgent:  req.AutoStartAgent,
		PlanMode:        req.PlanMode,
		RequireApproval: req.RequireApproval,
		PromptPrefix:    req.PromptPrefix,
		PromptSuffix:    req.PromptSuffix,
		AllowManualMove: req.AllowManualMove,
	}
	if err := c.svc.CreateStep(ctx, step); err != nil {
		return nil, err
	}
	return &GetStepResponse{Step: step}, nil
}

// UpdateStepRequest is the request for updating a workflow step.
type UpdateStepRequest struct {
	ID              string  `json:"id"`
	Name            *string `json:"name,omitempty"`
	StepType        *string `json:"step_type,omitempty"`
	Position        *int    `json:"position,omitempty"`
	Color           *string `json:"color,omitempty"`
	AutoStartAgent  *bool   `json:"auto_start_agent,omitempty"`
	PlanMode        *bool   `json:"plan_mode,omitempty"`
	RequireApproval *bool   `json:"require_approval,omitempty"`
	PromptPrefix    *string `json:"prompt_prefix,omitempty"`
	PromptSuffix    *string `json:"prompt_suffix,omitempty"`
	AllowManualMove *bool   `json:"allow_manual_move,omitempty"`
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
	if req.StepType != nil {
		step.StepType = models.StepType(*req.StepType)
	}
	if req.Position != nil {
		step.Position = *req.Position
	}
	if req.Color != nil {
		step.Color = *req.Color
	}
	if req.AutoStartAgent != nil {
		step.AutoStartAgent = *req.AutoStartAgent
	}
	if req.PlanMode != nil {
		step.PlanMode = *req.PlanMode
	}
	if req.RequireApproval != nil {
		step.RequireApproval = *req.RequireApproval
	}
	if req.PromptPrefix != nil {
		step.PromptPrefix = *req.PromptPrefix
	}
	if req.PromptSuffix != nil {
		step.PromptSuffix = *req.PromptSuffix
	}
	if req.AllowManualMove != nil {
		step.AllowManualMove = *req.AllowManualMove
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
	BoardID string   `json:"board_id"`
	StepIDs []string `json:"step_ids"`
}

// ReorderSteps reorders workflow steps for a board.
func (c *Controller) ReorderSteps(ctx context.Context, req ReorderStepsRequest) error {
	return c.svc.ReorderSteps(ctx, req.BoardID, req.StepIDs)
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

