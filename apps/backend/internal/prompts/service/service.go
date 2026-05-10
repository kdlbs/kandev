package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

var (
	ErrPromptNotFound      = errors.New("prompt not found")
	ErrInvalidPrompt       = errors.New("invalid prompt")
	ErrPromptAlreadyExists = errors.New("prompt with this name already exists")
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
	if err := s.assertNameAvailable(ctx, name, ""); err != nil {
		return nil, err
	}
	prompt := &models.Prompt{
		Name:    name,
		Content: content,
	}
	if err := s.repo.CreatePrompt(ctx, prompt); err != nil {
		return nil, translateNameConflict(err)
	}
	return prompt, nil
}

// translateNameConflict closes the TOCTOU window between assertNameAvailable
// and the write: the SQLite UNIQUE index on custom_prompts.name is the only
// authoritative guard, and a concurrent write that loses the race surfaces a
// "UNIQUE constraint failed" driver error which would otherwise fall through
// to a generic 500.
func translateNameConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrPromptAlreadyExists
	}
	return err
}

// assertNameAvailable returns ErrPromptAlreadyExists if a different prompt with
// the given name already exists. excludeID lets callers exclude the prompt
// being updated so unchanged-name saves do not falsely trip.
func (s *Service) assertNameAvailable(ctx context.Context, name, excludeID string) error {
	existing, err := s.repo.GetPromptByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if existing.ID == excludeID {
		return nil
	}
	return ErrPromptAlreadyExists
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
		if trimmed != prompt.Name {
			if err := s.assertNameAvailable(ctx, trimmed, prompt.ID); err != nil {
				return nil, err
			}
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
		return nil, translateNameConflict(err)
	}
	return prompt, nil
}

func (s *Service) DeletePrompt(ctx context.Context, promptID string) error {
	if promptID == "" {
		return ErrInvalidPrompt
	}
	return s.repo.DeletePrompt(ctx, promptID)
}
