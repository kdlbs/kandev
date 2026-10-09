package models

import "encoding/json"

// SidebarWorkspaceState is a personal saved-view collection for one workspace.
type SidebarWorkspaceState struct {
	Views        []SidebarView     `json:"views"`
	ActiveViewID string            `json:"active_view_id"`
	Draft        *SidebarViewDraft `json:"draft"`
}

// SidebarWorkspacePatch replaces fields in exactly one personal workspace view collection.
type SidebarWorkspacePatch struct {
	WorkspaceID  string          `json:"workspace_id"`
	Views        *[]SidebarView  `json:"views,omitempty"`
	ActiveViewID *string         `json:"active_view_id,omitempty"`
	Draft        json.RawMessage `json:"draft,omitempty"`
}
