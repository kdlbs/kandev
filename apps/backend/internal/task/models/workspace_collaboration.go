package models

import "time"

type WorkspaceCollaborationAction string

const (
	CollaborationRead    WorkspaceCollaborationAction = "read"
	CollaborationReport  WorkspaceCollaborationAction = "report"
	CollaborationPrompt  WorkspaceCollaborationAction = "prompt"
	CollaborationControl WorkspaceCollaborationAction = "control"
)

// WorkspaceCollaborationGrant narrows one logical conversation's native reach.
// Runtime session replacement does not change its identity.
type WorkspaceCollaborationGrant struct {
	CallerTaskID      string    `json:"caller_task_id" db:"caller_task_id"`
	SourceWorkspaceID string    `json:"source_workspace_id" db:"source_workspace_id"`
	TargetWorkspaceID string    `json:"target_workspace_id" db:"target_workspace_id"`
	Read              bool      `json:"read" db:"can_read"`
	Report            bool      `json:"report" db:"can_report"`
	Prompt            bool      `json:"prompt" db:"can_prompt"`
	Control           bool      `json:"control" db:"can_control"`
	UpdatedBy         string    `json:"updated_by" db:"updated_by"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

func (g WorkspaceCollaborationGrant) Allows(action WorkspaceCollaborationAction) bool {
	switch action {
	case CollaborationRead:
		return g.Read
	case CollaborationReport:
		return g.Report
	case CollaborationPrompt:
		return g.Prompt
	case CollaborationControl:
		return g.Control
	default:
		return false
	}
}
