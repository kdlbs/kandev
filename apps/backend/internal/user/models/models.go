package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettings struct {
	UserID               string    `json:"user_id"`
	WorkspaceID          string    `json:"workspace_id"`
	BoardID              string    `json:"board_id"`
	RepositoryIDs        []string  `json:"repository_ids"`
	InitialSetupComplete bool      `json:"initial_setup_complete"`
	PreferredShell       string    `json:"preferred_shell"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
