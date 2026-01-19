package service

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

var (
	ErrPromptNotFound = errors.New("prompt not found")
	ErrInvalidPrompt  = errors.New("invalid prompt")
)

type Service struct {
	repo promptstore.Repository
}

func NewService(repo promptstore.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	return s.repo.ListPrompts(ctx)
}

func (s *Service) CreatePrompt(ctx context.Context, name, content string) (*models.Prompt, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if name == "" || content == "" {
		return nil, ErrInvalidPrompt
	}
	prompt := &models.Prompt{
		Name:    name,
		Content: content,
	}
	if err := s.repo.CreatePrompt(ctx, prompt); err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *Service) UpdatePrompt(ctx context.Context, promptID string, name *string, content *string) (*models.Prompt, error) {
	prompt, err := s.repo.GetPromptByID(ctx, promptID)
	if err != nil {
		return nil, ErrPromptNotFound
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, ErrInvalidPrompt
		}
		prompt.Name = trimmed
	}
	if content != nil {
		trimmed := strings.TrimSpace(*content)
		if trimmed == "" {
			return nil, ErrInvalidPrompt
		}
		prompt.Content = trimmed
	}
	if err := s.repo.UpdatePrompt(ctx, prompt); err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *Service) DeletePrompt(ctx context.Context, promptID string) error {
	if promptID == "" {
		return ErrInvalidPrompt
	}
	return s.repo.DeletePrompt(ctx, promptID)
}
