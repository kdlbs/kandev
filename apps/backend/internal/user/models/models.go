package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettings struct {
	UserID                 string    `json:"user_id"`
	WorkspaceID            string    `json:"workspace_id"`
	KanbanViewMode         string    `json:"kanban_view_mode"`
	WorkflowFilterID       string    `json:"workflow_filter_id"`
	RepositoryIDs          []string  `json:"repository_ids"`
	InitialSetupComplete   bool      `json:"initial_setup_complete"`
	PreferredShell         string    `json:"preferred_shell"`
	DefaultEditorID        string    `json:"default_editor_id"`
	EnablePreviewOnClick   bool      `json:"enable_preview_on_click"`
	ChatSubmitKey          string    `json:"chat_submit_key"` // "enter" | "cmd_enter"
	ReviewAutoMarkOnScroll  bool      `json:"review_auto_mark_on_scroll"`
	LspAutoStartLanguages   []string                          `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages []string                          `json:"lsp_auto_install_languages"`
	LspServerConfigs        map[string]map[string]interface{} `json:"lsp_server_configs"`
	SavedLayouts            []SavedLayout                     `json:"saved_layouts"`
	CreatedAt               time.Time                         `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// SavedLayout represents a user-saved dockview layout configuration.
type SavedLayout struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"is_default"`
	Layout    json.RawMessage `json:"layout"`
	CreatedAt string          `json:"created_at"`
}
