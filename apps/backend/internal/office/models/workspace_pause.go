package models

import "time"

// WorkspacePause is the durable record that makes an Office workspace
// paused. A workspace is paused exactly when it has an unreleased row
// (ReleasedAt == nil) in office_workspace_pauses; the partial unique index
// on workspace_id enforces at most one per workspace.
type WorkspacePause struct {
	ID             string     `json:"id" db:"id"`
	WorkspaceID    string     `json:"workspace_id" db:"workspace_id"`
	Reason         string     `json:"reason" db:"reason"`
	CreatedBy      string     `json:"created_by" db:"created_by"`
	CreatedByKind  string     `json:"created_by_kind" db:"created_by_kind"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	ReleasedAt     *time.Time `json:"released_at,omitempty" db:"released_at"`
	ReleasedBy     string     `json:"released_by,omitempty" db:"released_by"`
	ReleasedReason string     `json:"released_reason,omitempty" db:"released_reason"`
}
