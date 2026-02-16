package v1

import "time"

// User represents a user account
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettings struct {
	UserID           string    `json:"user_id"`
	WorkspaceID      string    `json:"workspace_id"`
	KanbanViewMode   string    `json:"kanban_view_mode"`
	WorkflowFilterID string    `json:"workflow_filter_id"`
	RepositoryIDs    []string  `json:"repository_ids"`
	PreferredShell  string    `json:"preferred_shell"`
	DefaultEditorID string    `json:"default_editor_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}
