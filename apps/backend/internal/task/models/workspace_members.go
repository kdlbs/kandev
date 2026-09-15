package models

import "time"

// WorkspaceMember grants one user access to one workspace with a workspace
// role. It is the exception mechanism next to Workspace.Visibility: it
// populates a private workspace, admits a guest to a single workspace, and
// narrows a member to viewer on an org-visible one. An explicit row always
// outranks the org default, in both directions.
//
// Lives in its own file because models.go is already past the 800-line limit.
type WorkspaceMember struct {
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	UserID      string    `json:"user_id" db:"user_id"`
	Role        string    `json:"role" db:"role"`
	AddedBy     string    `json:"added_by" db:"added_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}
