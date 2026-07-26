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

// AccountRepository is the account-management surface consumed by
// internal/auth. It is intentionally separate from Repository so existing
// Repository fakes/tests are unaffected by the auth feature; *sqliteRepository
// implements both.
type AccountRepository interface {
	GetUser(ctx context.Context, id string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	ListUsers(ctx context.Context) ([]*models.User, error)
	CreateUser(ctx context.Context, user *models.User) error
	UpdateUserProfile(ctx context.Context, id, email, displayName, role string) (*models.User, error)
	UpdateUserRoleStatus(ctx context.Context, id, role, status string) (*models.User, error)
	// DeleteUser removes a user row by id. Used to roll back a just-created
	// account when a follow-up step (e.g. linking its login identity) fails, so
	// no account is left without a usable login. Deleting a missing id is not an
	// error.
	DeleteUser(ctx context.Context, id string) error
}

var _ AccountRepository = (*sqliteRepository)(nil)
