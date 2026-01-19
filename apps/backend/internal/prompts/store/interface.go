package store

import (
	"context"

	"github.com/kandev/kandev/internal/prompts/models"
)

type Repository interface {
	ListPrompts(ctx context.Context) ([]*models.Prompt, error)
	GetPromptByID(ctx context.Context, id string) (*models.Prompt, error)
	GetPromptByName(ctx context.Context, name string) (*models.Prompt, error)
	CreatePrompt(ctx context.Context, prompt *models.Prompt) error
	UpdatePrompt(ctx context.Context, prompt *models.Prompt) error
	DeletePrompt(ctx context.Context, id string) error
}
