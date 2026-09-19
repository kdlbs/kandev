package models

const (
	SidebarLayoutVersion = 1

	SidebarLayoutNodeBuiltin   = "builtin"
	SidebarLayoutNodePlugin    = "plugin"
	SidebarLayoutNodeShortcuts = "shortcuts"

	SidebarShortcutDestination = "destination"
	SidebarShortcutHostAction  = "host_action"
	SidebarShortcutCanvas      = "canvas"
	SidebarShortcutAutomation  = "automation"
)

// SidebarLayout is the portable navigation composition for one workspace.
// Resource names, icons, and actions are resolved from the live catalog; only
// stable identities and the user's ordering choices are persisted.
type SidebarLayout struct {
	Version  int                 `json:"version"`
	Revision int64               `json:"revision"`
	Nodes    []SidebarLayoutNode `json:"nodes"`
}

type SidebarLayoutNode struct {
	ID            string            `json:"id"`
	Kind          string            `json:"kind"`
	Visible       bool              `json:"visible"`
	DestinationID string            `json:"destination_id,omitempty"`
	Name          string            `json:"name,omitempty"`
	Shortcuts     []SidebarShortcut `json:"shortcuts,omitempty"`
}

type SidebarShortcut struct {
	ID     string                `json:"id"`
	Target SidebarShortcutTarget `json:"target"`
}

type SidebarShortcutTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// SidebarLayoutPatch replaces or resets one workspace layout under its own
// revision while the surrounding user settings row is protected by CAS.
type SidebarLayoutPatch struct {
	WorkspaceID      string         `json:"workspace_id"`
	ExpectedRevision int64          `json:"expected_revision"`
	Layout           *SidebarLayout `json:"layout"`
}

// DefaultSidebarLayout returns the canonical layout used when a workspace has
// no saved customization. A fresh value is returned on every call so callers
// can safely edit it before persisting.
func DefaultSidebarLayout() SidebarLayout {
	return SidebarLayout{
		Version:  SidebarLayoutVersion,
		Revision: 0,
		Nodes: []SidebarLayoutNode{
			{ID: "home", Kind: SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "home"},
			{ID: "new-task", Kind: SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "new_task"},
			{ID: "automations", Kind: SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "automations"},
			{ID: "canvases", Kind: SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "canvases"},
			{ID: "integrations", Kind: SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "integrations"},
		},
	}
}
