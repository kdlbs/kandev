package dto

import (
	"time"

	"github.com/kandev/kandev/internal/user/models"
)

type UserDTO struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettingsDTO struct {
	UserID        string   `json:"user_id"`
	WorkspaceID   string   `json:"workspace_id"`
	BoardID       string   `json:"board_id"`
	RepositoryIDs []string `json:"repository_ids"`
	UpdatedAt     string   `json:"updated_at"`
}

type UserResponse struct {
	User     UserDTO        `json:"user"`
	Settings UserSettingsDTO `json:"settings"`
}

type UserSettingsResponse struct {
	Settings UserSettingsDTO `json:"settings"`
}

type UpdateUserSettingsRequest struct {
	WorkspaceID   *string  `json:"workspace_id,omitempty"`
	BoardID       *string  `json:"board_id,omitempty"`
	RepositoryIDs *[]string `json:"repository_ids,omitempty"`
}

func FromUser(user *models.User) UserDTO {
	return UserDTO{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func FromUserSettings(settings *models.UserSettings) UserSettingsDTO {
	return UserSettingsDTO{
		UserID:        settings.UserID,
		WorkspaceID:   settings.WorkspaceID,
		BoardID:       settings.BoardID,
		RepositoryIDs: settings.RepositoryIDs,
		UpdatedAt:     settings.UpdatedAt.Format(time.RFC3339),
	}
}
