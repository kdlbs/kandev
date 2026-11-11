package controller

import (
	"context"

	"github.com/kandev/kandev/internal/prompts/dto"
	"github.com/kandev/kandev/internal/prompts/service"
)

type Controller struct {
	service *service.Service
}

func NewController(svc *service.Service) *Controller {
	return &Controller{service: svc}
}

func (c *Controller) ListPrompts(ctx context.Context) (dto.PromptsResponse, error) {
	prompts, err := c.service.ListPrompts(ctx)
	if err != nil {
		return dto.PromptsResponse{}, err
	}
	result := make([]dto.PromptDTO, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt == nil {
			continue
		}
		result = append(result, dto.FromPrompt(prompt))
	}
	return dto.PromptsResponse{Prompts: result}, nil
}

func (c *Controller) CreatePrompt(ctx context.Context, req dto.CreatePromptRequest) (dto.PromptDTO, error) {
	prompt, err := c.service.CreatePrompt(ctx, req.Name, req.Content)
	if err != nil {
		return dto.PromptDTO{}, err
	}
	return dto.FromPrompt(prompt), nil
}

func (c *Controller) UpdatePrompt(ctx context.Context, promptID string, req dto.UpdatePromptRequest) (dto.PromptDTO, error) {
	prompt, err := c.service.UpdatePrompt(ctx, promptID, req.Name, req.Content)
	if err != nil {
		return dto.PromptDTO{}, err
	}
	return dto.FromPrompt(prompt), nil
}

func (c *Controller) DeletePrompt(ctx context.Context, promptID string) error {
	return c.service.DeletePrompt(ctx, promptID)
}
