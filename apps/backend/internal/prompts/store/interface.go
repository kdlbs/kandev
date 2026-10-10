package store

import (
	"context"

	"github.com/kandev/kandev/internal/prompts/models"
)

type Repository interface {
	ListPrompts(ctx context.Context) ([]*models.Prompt, error)
	ListPromptsForReferenceExpansion(
		ctx context.Context,
		limit, maxNameBytes, maxContentBytes, maxTotalNameBytes, maxTotalContentBytes int,
	) ([]*models.Prompt, bool, error)
	GetPromptByID(ctx context.Context, id string) (*models.Prompt, error)
	GetPromptByName(ctx context.Context, name string) (*models.Prompt, error)
	CreatePrompt(ctx context.Context, prompt *models.Prompt) error
	UpdatePrompt(ctx context.Context, prompt *models.Prompt) error
	UpdatePromptForAgent(ctx context.Context, prompt *models.Prompt, expectedName string) error
	DeletePrompt(ctx context.Context, id string) error
}
