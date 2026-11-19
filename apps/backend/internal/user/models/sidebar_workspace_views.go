package models

import "encoding/json"

// SidebarWorkspaceState is a personal saved-view collection for one workspace.
type SidebarWorkspaceState struct {
	Views        []SidebarView     `json:"views"`
	ActiveViewID string            `json:"active_view_id"`
	Draft        *SidebarViewDraft `json:"draft"`
}

// SidebarWorkspacePatch updates exactly one personal workspace view collection.
type SidebarWorkspacePatch struct {
	WorkspaceID   string          `json:"workspace_id"`
	Views         *[]SidebarView  `json:"views,omitempty"`
	ViewsUpsert   []SidebarView   `json:"views_upsert,omitempty"`
	ViewIDsDelete []string        `json:"view_ids_delete,omitempty"`
	ViewOrder     *[]string       `json:"view_order,omitempty"`
	ActiveViewID  *string         `json:"active_view_id,omitempty"`
	Draft         json.RawMessage `json:"draft,omitempty"`
}
