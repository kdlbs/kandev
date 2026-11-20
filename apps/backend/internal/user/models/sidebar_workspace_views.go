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
	WorkspaceID          string              `json:"workspace_id"`
	Views                *[]SidebarView      `json:"views,omitempty"`
	ViewsCreate          []SidebarView       `json:"views_create,omitempty"`
	ViewsUpsert          []SidebarView       `json:"views_upsert,omitempty"`
	ViewsUpdate          []SidebarViewUpdate `json:"views_update,omitempty"`
	ViewIDsDelete        []string            `json:"view_ids_delete,omitempty"`
	ViewOrder            *[]string           `json:"view_order,omitempty"`
	ViewOrderBase        *[]string           `json:"view_order_base,omitempty"`
	ActiveViewID         *string             `json:"active_view_id,omitempty"`
	Draft                json.RawMessage     `json:"draft,omitempty"`
	DraftUpdate          *SidebarDraftUpdate `json:"draft_update,omitempty"`
	SelectPluginGrouping *SidebarView        `json:"select_plugin_grouping,omitempty"`
	SaveDraft            *SidebarDraftSave   `json:"save_draft,omitempty"`
}

// SidebarViewUpdate carries only the fields edited by one user operation.
type SidebarViewUpdate struct {
	ID                      string          `json:"id"`
	Name                    *string         `json:"name,omitempty"`
	GeneratedPluginGrouping json.RawMessage `json:"generated_plugin_grouping,omitempty"`
	SidebarPresentationUpdate
}

type SidebarPresentationUpdate struct {
	Filters     *[]SidebarViewClause     `json:"filters,omitempty"`
	Sort        *SidebarViewSort         `json:"sort,omitempty"`
	Group       *string                  `json:"group,omitempty"`
	GroupIndent *bool                    `json:"group_indent,omitempty"`
	TaskRow     json.RawMessage          `json:"task_row,omitempty"`
	Disclosure  *SidebarDisclosureUpdate `json:"disclosure,omitempty"`
}

type SidebarDraftUpdate struct {
	BaseViewID      string `json:"base_view_id"`
	ExpectedGroup   string `json:"expected_group"`
	Clear           bool   `json:"clear,omitempty"`
	RequireExisting bool   `json:"require_existing,omitempty"`
	SidebarPresentationUpdate
}

type SidebarDisclosureUpdate struct {
	Group        string    `json:"group"`
	Add          []string  `json:"add,omitempty"`
	Remove       []string  `json:"remove,omitempty"`
	Replace      *[]string `json:"replace,omitempty"`
	Canonicalize bool      `json:"canonicalize,omitempty"`
}

type SidebarDraftSave struct {
	BaseViewID    string `json:"base_view_id"`
	ExpectedGroup string `json:"expected_group"`
	TargetViewID  string `json:"target_view_id"`
	Name          string `json:"name,omitempty"`
}
