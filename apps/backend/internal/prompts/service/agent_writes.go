package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/prompts/models"
)

var (
	ErrBuiltinPrompt = errors.New("built-in prompts cannot be modified by agents")
)

func (s *Service) CreatePromptForAgent(ctx context.Context, name, content string) (*models.Prompt, error) {
	return s.CreatePrompt(ctx, name, content)
}

func (s *Service) UpdatePromptContentForAgent(ctx context.Context, name, content string) (*models.Prompt, error) {
	prompt, err := s.GetPromptByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.updateAgentPrompt(ctx, prompt, nil, &content)
}

func (s *Service) UpdatePromptForAgent(ctx context.Context, id string, name, content *string) (*models.Prompt, error) {
	prompt, err := s.GetPromptByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.updateAgentPrompt(ctx, prompt, name, content)
}

func (s *Service) updateAgentPrompt(ctx context.Context, prompt *models.Prompt, name, content *string) (*models.Prompt, error) {
	if prompt.Builtin {
		return nil, ErrBuiltinPrompt
	}
	expectedName := prompt.Name
	if err := s.applyPromptFields(ctx, prompt, name, content); err != nil {
		return nil, err
	}
	if err := s.repo.UpdatePromptForAgent(ctx, prompt, expectedName); err != nil {
		return nil, translateNameConflict(err)
	}
	s.publishChanged(ctx)
	return prompt, nil
}
