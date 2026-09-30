package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/prompts/models"
)

var (
	ErrBuiltinPrompt            = errors.New("built-in prompts cannot be modified by agents")
	ErrPromptAgentEditsDisabled = errors.New("shared prompt does not allow agent edits; an operator can enable Allow agent edits in Settings > Prompts")
)

func (s *Service) CreatePromptForAgent(ctx context.Context, name, content string) (*models.Prompt, error) {
	return s.createPrompt(ctx, name, content, true)
}

func (s *Service) UpdatePromptContentForAgent(ctx context.Context, name, content string) (*models.Prompt, error) {
	prompt, err := s.GetPromptByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.updateAgentPrompt(ctx, prompt, nil, &content)
}

func (s *Service) UpdatePromptForAgent(ctx context.Context, id string, name, content *string) (*models.Prompt, error) {
	prompt, err := s.repo.GetPromptByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && prompt == nil) {
		return nil, ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.updateAgentPrompt(ctx, prompt, name, content)
}

func (s *Service) updateAgentPrompt(ctx context.Context, prompt *models.Prompt, name, content *string) (*models.Prompt, error) {
	if prompt.Builtin {
		return nil, ErrBuiltinPrompt
	}
	if !prompt.AllowAgentEdits {
		return nil, ErrPromptAgentEditsDisabled
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
