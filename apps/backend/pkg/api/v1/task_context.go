package v1

import "time"

// TaskContext is the office task-handoffs context envelope returned by
// GET /api/v1/tasks/:id/context (phase 7). It bundles related-task,
// document, and workspace-group projections so both the task detail
// React panel and the prompt builder can hydrate from one call.
//
// IMPORTANT: this DTO never carries document body content. AvailableDocs
// lists keys + titles only; the agent / UI must explicitly fetch a
// document via the existing get_task_document_kandev MCP tool or the
// HTTP document endpoint.
type TaskContext struct {
	Task            TaskRef            `json:"task"`
	Parent          *TaskRef           `json:"parent,omitempty"`
	Children        []TaskRef          `json:"children"`
	Siblings        []TaskRef          `json:"siblings"`
	Blockers        []TaskRef          `json:"blockers"`
	BlockedBy       []TaskRef          `json:"blocked_by"`
	AvailableDocs   []DocumentRef      `json:"available_documents"`
	WorkspaceMode   string             `json:"workspace_mode,omitempty"`   // metadata.workspace.mode
	WorkspaceGroup  *WorkspaceGroupRef `json:"workspace_group,omitempty"`  // resolved from task_workspace_groups
	BlockedReason   string             `json:"blocked_reason,omitempty"`   // "blockers_pending" | "workspace_restoring" | ""
	WorkspaceStatus string             `json:"workspace_status,omitempty"` // "active" | "requires_configuration"
}

// TaskRef is the lightweight projection used in TaskContext relation lists.
type TaskRef struct {
	ID            string   `json:"id"`
	Identifier    string   `json:"identifier,omitempty"`
	Title         string   `json:"title"`
	State         string   `json:"state"`
	WorkspaceID   string   `json:"workspace_id"`
	ParentID      string   `json:"parent_id,omitempty"`
	AssigneeLabel string   `json:"assignee_label,omitempty"`
	DocumentKeys  []string `json:"document_keys,omitempty"`
}

// DocumentRef is a metadata-only projection of a task document. Body
// content is deliberately omitted so the prompt builder can list available
// documents without inlining bodies and the UI can render quick-open links
// without fetching unnecessary data.
type DocumentRef struct {
	TaskRef   TaskRef   `json:"task"`
	Key       string    `json:"key"`
	Title     string    `json:"title,omitempty"`
	Type      string    `json:"type,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// WorkspaceGroupRef projects the workspace group a task belongs to (when
// any). Members are listed so the UI can show "shared with X, Y, Z";
// MaterializedPath / Kind support the "Shared workspace" badge on the
// task detail panel.
type WorkspaceGroupRef struct {
	ID               string    `json:"id"`
	MaterializedPath string    `json:"materialized_path,omitempty"`
	MaterializedKind string    `json:"materialized_kind,omitempty"`
	CleanupStatus    string    `json:"cleanup_status,omitempty"`
	OwnedByKandev    bool      `json:"owned_by_kandev"`
	Members          []TaskRef `json:"members"`
}

// Common BlockedReason / WorkspaceStatus values surfaced through TaskContext.
const (
	TaskBlockedReasonNone               = ""
	TaskBlockedReasonBlockersPending    = "blockers_pending"
	TaskBlockedReasonWorkspaceRestoring = "workspace_restoring"

	TaskWorkspaceStatusActive       = "active"
	TaskWorkspaceStatusRequiresConf = "requires_configuration"
)
