package store

import (
	"context"

	"github.com/kandev/kandev/internal/user/models"
)

type Repository interface {
	GetUser(ctx context.Context, id string) (*models.User, error)
	GetDefaultUser(ctx context.Context) (*models.User, error)
	GetUserSettings(ctx context.Context, userID string) (*models.UserSettings, error)
	UpsertUserSettingsPreservingTaskCreateLastUsed(ctx context.Context, settings *models.UserSettings, patch *models.TaskCreateLastUsed) (*models.UserSettings, error)
	UpdateTaskCreateLastUsed(ctx context.Context, userID string, patch models.TaskCreateLastUsed) (*models.UserSettings, error)
	Close() error
}
