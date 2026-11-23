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

	// Sync-only writes. Each is guarded by sync ownership and never touches a
	// manual or built-in prompt.
	CreateSyncedPrompt(ctx context.Context, name, content, sourcePath string) (*models.Prompt, error)
	UpdateSyncedPrompt(ctx context.Context, id, content, sourcePath string) (bool, error)
	DeleteSyncedPrompt(ctx context.Context, id string) (bool, error)
	ReleaseSyncedPrompts(ctx context.Context, opts ReleaseOptions) (int64, error)

	// Ownership transitions, each guarded by the expected current source.
	SavePromptVersions(ctx context.Context, id string, from, to PromptVersions) (bool, error)
	RevertOverride(ctx context.Context, id string) (bool, error)
	AdoptPromptAsOverride(ctx context.Context, seen *models.Prompt, sourcePath, repositoryContent string, conflict bool) (bool, error)
	AdoptPromptAsSynced(ctx context.Context, seen *models.Prompt, content, sourcePath string) (bool, error)
	SetOverrideRepositoryFile(ctx context.Context, id, sourcePath, content string) (bool, error)
	ClearOverrideConflict(ctx context.Context, id string) (bool, error)
	DetachOverride(ctx context.Context, id string) (bool, error)
	RestoreSyncedPrompt(ctx context.Context, id string) (bool, error)
}
