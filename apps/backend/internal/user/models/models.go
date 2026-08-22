package models

import (
	"encoding/json"
	"time"
)

const (
	MCPTaskAgentProfileDefaultCurrentTask      = "current_task"
	MCPTaskAgentProfileDefaultWorkspaceDefault = "workspace_default"
	LspStatusLocationToolbar                   = "toolbar"
	LspStatusLocationStatusBar                 = "status_bar"
)

// NormalizeMCPTaskAgentProfileDefault returns the canonical MCP task agent
// profile default: workspace_default is accepted as-is, anything else is
// coerced to current_task.
func NormalizeMCPTaskAgentProfileDefault(value string) string {
	if value == MCPTaskAgentProfileDefaultWorkspaceDefault {
		return value
	}
	return MCPTaskAgentProfileDefaultCurrentTask
}

// NormalizeLspStatusLocation returns the canonical LSP status location:
// status_bar is accepted as-is, anything else is coerced to toolbar.
func NormalizeLspStatusLocation(value string) string {
	if value == LspStatusLocationStatusBar {
		return value
	}
	return LspStatusLocationToolbar
}

const (
	StartupPageTaskOverview = "task_overview"
	StartupPageLastTask     = "last_task"
)

// NormalizeStartupPage returns the canonical startup page: last_task is
// accepted as-is, anything else is coerced to task_overview.
func NormalizeStartupPage(value string) string {
	if value == StartupPageLastTask {
		return value
	}
	return StartupPageTaskOverview
}

const (
	LastSeenDisplayAbsolute = "absolute"
	LastSeenDisplayRelative = "relative"
)

// NormalizeLastSeenDisplay returns the canonical last-seen display mode:
// relative is accepted as-is, anything else is coerced to absolute.
func NormalizeLastSeenDisplay(value string) string {
	if value == LastSeenDisplayRelative {
		return value
	}
	return LastSeenDisplayAbsolute
}

const (
	// RoleAdmin unlocks user management and system settings mutation when
	// authentication is enabled. It does NOT grant visibility into other
	// users' workspaces (hard privacy isolation).
	RoleAdmin = "admin"
	// RoleMember is a regular authenticated user.
	RoleMember = "member"
	// RoleGuest reaches only workspaces it is an explicit member of. It is
	// never assigned by migration; an admin grants it deliberately.
	RoleGuest = "guest"

	// StatusActive marks a user that may log in.
	StatusActive = "active"
	// StatusDisabled marks a user whose access is revoked; all sessions and
	// API tokens are invalidated when this status is set.
	StatusDisabled = "disabled"
)

// NormalizeRole returns the canonical org role. Admin and guest are accepted
// as-is; anything else (including empty and unknown values) becomes member,
// which is the role every pre-guest account already had.
func NormalizeRole(value string) string {
	switch value {
	case RoleAdmin:
		return RoleAdmin
	case RoleGuest:
		return RoleGuest
	default:
		return RoleMember
	}
}

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	// OrgID is the user's tenant. Empty means the account predates
	// organizations or the tenancy migration has not run.
	OrgID string `json:"org_id,omitempty"`
	// IsOperator marks the instance operator tier: managing organizations,
	// feature toggles and instance configuration. It grants NO access inside
	// any organization, including the operator's own.
	IsOperator bool      `json:"is_operator,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type UserSettings struct {
	UserID                            string                            `json:"user_id"`
	WorkspaceID                       string                            `json:"workspace_id"`
	KanbanViewMode                    string                            `json:"kanban_view_mode"`
	StartupPage                       string                            `json:"startup_page"`
	WorkflowFilterID                  string                            `json:"workflow_filter_id"`
	RepositoryIDs                     []string                          `json:"repository_ids"`
	TasksListSort                     string                            `json:"tasks_list_sort"`
	TasksListGroup                    string                            `json:"tasks_list_group"`
	TasksListShowDetails              bool                              `json:"tasks_list_show_details"`
	InitialSetupComplete              bool                              `json:"initial_setup_complete"`
	PreferredShell                    string                            `json:"preferred_shell"`
	DefaultEditorID                   string                            `json:"default_editor_id"`
	EnablePreviewOnClick              bool                              `json:"enable_preview_on_click"`
	ChatSubmitKey                     string                            `json:"chat_submit_key"` // "enter" | "cmd_enter"
	ReviewAutoMarkOnScroll            bool                              `json:"review_auto_mark_on_scroll"`
	ConfirmTaskArchive                bool                              `json:"confirm_task_archive"`
	PreventAutoStartAgentOnOpen       bool                              `json:"prevent_auto_start_agent_on_open"`
	UnreadDivider                     bool                              `json:"unread_divider"`
	AgentGeneratedTaskTitles          bool                              `json:"agent_generated_task_titles"`
	MCPTaskAgentProfileDefault        string                            `json:"mcp_task_agent_profile_default"`
	ShowAnchoredPromptBar             bool                              `json:"show_anchored_prompt_bar"` // desktop-only sticky last-prompt bar
	ShowScrollToLastPrompt            bool                              `json:"show_scroll_to_last_prompt"`
	ShowScrollToStart                 bool                              `json:"show_scroll_to_start"`
	ShowTranscriptAutoScrollControl   bool                              `json:"show_transcript_auto_scroll_control"`
	ShowTodoListPanel                 bool                              `json:"show_todo_list_panel"`
	ShowTodoListPanelOnlyWhenNotEmpty bool                              `json:"show_todo_list_panel_only_when_not_empty"`
	ShowReleaseNotification           bool                              `json:"show_release_notification"`
	ReleaseNotesLastSeenVersion       string                            `json:"release_notes_last_seen_version"`
	LspAutoStartLanguages             []string                          `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages           []string                          `json:"lsp_auto_install_languages"`
	LspServerConfigs                  map[string]map[string]interface{} `json:"lsp_server_configs"`
	LspStatusLocation                 string                            `json:"lsp_status_location"`
	SavedLayouts                      []SavedLayout                     `json:"saved_layouts"`
	SidebarViews                      []SidebarView                     `json:"sidebar_views"`
	SidebarActiveViewID               string                            `json:"sidebar_active_view_id"`
	SidebarDraft                      *SidebarViewDraft                 `json:"sidebar_draft"`
	SidebarTaskPrefs                  SidebarTaskPrefs                  `json:"sidebar_task_prefs"`
	TaskCreateLastUsed                TaskCreateLastUsed                `json:"task_create_last_used"`
	JiraSavedViews                    json.RawMessage                   `json:"jira_saved_views"`
	JiraTaskPresets                   json.RawMessage                   `json:"jira_task_presets"`
	GitHubSavedPresets                json.RawMessage                   `json:"github_saved_presets"`
	GitHubDefaultQueryPresets         json.RawMessage                   `json:"github_default_query_presets"`
	GitLabSavedPresets                json.RawMessage                   `json:"gitlab_saved_presets"`
	AzureDevOpsBrowsePreferences      json.RawMessage                   `json:"azure_devops_browse_preferences"`
	DefaultUtilityAgentID             string                            `json:"default_utility_agent_id"` // Default inference agent for utility agents
	DefaultUtilityModel               string                            `json:"default_utility_model"`    // Default model for utility agents
	DefaultUtilityAgentProfileID      string                            `json:"default_utility_agent_profile_id"`
	KeyboardShortcuts                 map[string]interface{}            `json:"keyboard_shortcuts"`     // User-configured keyboard shortcut overrides
	TerminalLinkBehavior              string                            `json:"terminal_link_behavior"` // "new_tab" | "browser_panel"
	TerminalFontFamily                string                            `json:"terminal_font_family"`
	TerminalFontSize                  int                               `json:"terminal_font_size"`
	ChangesPanelLayout                string                            `json:"changes_panel_layout"` // "flat" | "tree"
	LastSeenDisplay                   string                            `json:"last_seen_display"`    // "absolute" | "relative"
	SystemMetricsDisplay              SystemMetricsDisplaySettings      `json:"system_metrics_display"`
	AppStatusBarEnabled               bool                              `json:"app_status_bar_enabled"`
	AppStatusBarOrder                 AppStatusBarOrder                 `json:"app_status_bar_order"`
	KanbanHiddenStepIDs               map[string][]string               `json:"kanban_hidden_step_ids"`
	Revision                          int64                             `json:"revision"`
	CreatedAt                         time.Time                         `json:"created_at"`
	UpdatedAt                         time.Time                         `json:"updated_at"`
}

type SystemMetricsDisplaySettings struct {
	ShowInTopbar bool `json:"show_in_topbar"`
	Simplified   bool `json:"simplified"`
}

type AppStatusBarOrder struct {
	LeftItemIDs  []string `json:"left_item_ids"`
	RightItemIDs []string `json:"right_item_ids"`
}

// SavedLayout represents a user-saved dockview layout configuration.
type SavedLayout struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"is_default"`
	Layout    json.RawMessage `json:"layout"`
	CreatedAt string          `json:"created_at"`
}

// SidebarView represents a user-saved sidebar filter/sort/group preset.
// The payload is stored and returned as-is; the server does not interpret
// clause values beyond passing them through.
type SidebarView struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Filters         []SidebarViewClause `json:"filters"`
	Sort            SidebarViewSort     `json:"sort"`
	Group           string              `json:"group"`
	CollapsedGroups []string            `json:"collapsed_groups"`
}

type SidebarViewClause struct {
	ID        string          `json:"id"`
	Dimension string          `json:"dimension"`
	Op        string          `json:"op"`
	Value     json.RawMessage `json:"value"`
}

type SidebarViewSort struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type SidebarViewDraft struct {
	BaseViewID string              `json:"base_view_id"`
	Filters    []SidebarViewClause `json:"filters"`
	Sort       SidebarViewSort     `json:"sort"`
	Group      string              `json:"group"`
}

type SidebarTaskPrefs struct {
	PinnedTaskIDs          []string            `json:"pinned_task_ids"`
	OrderedTaskIDs         []string            `json:"ordered_task_ids"`
	SubtaskOrderByParentID map[string][]string `json:"subtask_order_by_parent_id"`
}

type TaskCreateLastUsed struct {
	RepositoryID           string            `json:"repository_id"`
	Branch                 string            `json:"branch"`
	AgentProfileID         string            `json:"agent_profile_id"`
	ExecutorProfileID      string            `json:"executor_profile_id"`
	WorkflowIDsByWorkspace map[string]string `json:"workflow_ids_by_workspace"`
}
