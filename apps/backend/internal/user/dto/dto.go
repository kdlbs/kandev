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
	UserID                  string                            `json:"user_id"`
	WorkspaceID             string                            `json:"workspace_id"`
	KanbanViewMode          string                            `json:"kanban_view_mode"`
	WorkflowFilterID        string                            `json:"workflow_filter_id"`
	RepositoryIDs           []string                          `json:"repository_ids"`
	InitialSetupComplete    bool                              `json:"initial_setup_complete"`
	PreferredShell          string                            `json:"preferred_shell"`
	DefaultEditorID         string                            `json:"default_editor_id"`
	EnablePreviewOnClick    bool                              `json:"enable_preview_on_click"`
	ChatSubmitKey           string                            `json:"chat_submit_key"`
	ReviewAutoMarkOnScroll  bool                              `json:"review_auto_mark_on_scroll"`
	LspAutoStartLanguages   []string                          `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages []string                          `json:"lsp_auto_install_languages"`
	LspServerConfigs        map[string]map[string]interface{} `json:"lsp_server_configs,omitempty"`
	SavedLayouts            []models.SavedLayout              `json:"saved_layouts"`
	UpdatedAt               string                            `json:"updated_at"`
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
	WorkspaceID             *string                            `json:"workspace_id,omitempty"`
	KanbanViewMode          *string                            `json:"kanban_view_mode,omitempty"`
	WorkflowFilterID        *string                            `json:"workflow_filter_id,omitempty"`
	RepositoryIDs           *[]string                          `json:"repository_ids,omitempty"`
	InitialSetupComplete    *bool                              `json:"initial_setup_complete,omitempty"`
	PreferredShell          *string                            `json:"preferred_shell,omitempty"`
	DefaultEditorID         *string                            `json:"default_editor_id,omitempty"`
	EnablePreviewOnClick    *bool                              `json:"enable_preview_on_click,omitempty"`
	ChatSubmitKey           *string                            `json:"chat_submit_key,omitempty"`
	ReviewAutoMarkOnScroll  *bool                              `json:"review_auto_mark_on_scroll,omitempty"`
	LspAutoStartLanguages   *[]string                          `json:"lsp_auto_start_languages,omitempty"`
	LspAutoInstallLanguages *[]string                          `json:"lsp_auto_install_languages,omitempty"`
	LspServerConfigs        *map[string]map[string]interface{} `json:"lsp_server_configs,omitempty"`
	SavedLayouts            *[]models.SavedLayout              `json:"saved_layouts,omitempty"`
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
		UserID:                  settings.UserID,
		WorkspaceID:             settings.WorkspaceID,
		KanbanViewMode:          settings.KanbanViewMode,
		WorkflowFilterID:        settings.WorkflowFilterID,
		RepositoryIDs:           settings.RepositoryIDs,
		InitialSetupComplete:    settings.InitialSetupComplete,
		PreferredShell:          settings.PreferredShell,
		DefaultEditorID:         settings.DefaultEditorID,
		EnablePreviewOnClick:    settings.EnablePreviewOnClick,
		ChatSubmitKey:           settings.ChatSubmitKey,
		ReviewAutoMarkOnScroll:  settings.ReviewAutoMarkOnScroll,
		LspAutoStartLanguages:   settings.LspAutoStartLanguages,
		LspAutoInstallLanguages: settings.LspAutoInstallLanguages,
		LspServerConfigs:        settings.LspServerConfigs,
		SavedLayouts:            settings.SavedLayouts,
		UpdatedAt:               settings.UpdatedAt.Format(time.RFC3339),
	}
}
