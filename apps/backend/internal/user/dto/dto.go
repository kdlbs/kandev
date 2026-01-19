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
	UserID               string   `json:"user_id"`
	WorkspaceID          string   `json:"workspace_id"`
	BoardID              string   `json:"board_id"`
	RepositoryIDs        []string `json:"repository_ids"`
	InitialSetupComplete bool     `json:"initial_setup_complete"`
	PreferredShell       string   `json:"preferred_shell"`
	DefaultEditorID      string   `json:"default_editor_id"`
	UpdatedAt            string   `json:"updated_at"`
}

type UserResponse struct {
	User     UserDTO         `json:"user"`
	Settings UserSettingsDTO `json:"settings"`
}

type UserSettingsResponse struct {
	Settings     UserSettingsDTO `json:"settings"`
	ShellOptions []ShellOption   `json:"shell_options"`
}

type ShellOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type UpdateUserSettingsRequest struct {
	WorkspaceID          *string   `json:"workspace_id,omitempty"`
	BoardID              *string   `json:"board_id,omitempty"`
	RepositoryIDs        *[]string `json:"repository_ids,omitempty"`
	InitialSetupComplete *bool     `json:"initial_setup_complete,omitempty"`
	PreferredShell       *string   `json:"preferred_shell,omitempty"`
	DefaultEditorID      *string   `json:"default_editor_id,omitempty"`
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
		UserID:               settings.UserID,
		WorkspaceID:          settings.WorkspaceID,
		BoardID:              settings.BoardID,
		RepositoryIDs:        settings.RepositoryIDs,
		InitialSetupComplete: settings.InitialSetupComplete,
		PreferredShell:       settings.PreferredShell,
		DefaultEditorID:      settings.DefaultEditorID,
		UpdatedAt:            settings.UpdatedAt.Format(time.RFC3339),
	}
}
