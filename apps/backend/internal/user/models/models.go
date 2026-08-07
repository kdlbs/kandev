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

func NormalizeMCPTaskAgentProfileDefault(value string) string {
	if value == MCPTaskAgentProfileDefaultWorkspaceDefault {
		return value
	}
	return MCPTaskAgentProfileDefaultCurrentTask
}

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

func NormalizeStartupPage(value string) string {
	if value == StartupPageLastTask {
		return value
	}
	return StartupPageTaskOverview
}

const (
	// RoleAdmin unlocks user management and system settings mutation when
	// authentication is enabled. It does NOT grant visibility into other
	// users' workspaces (hard privacy isolation).
	RoleAdmin = "admin"
	// RoleMember is a regular authenticated user.
	RoleMember = "member"

	// StatusActive marks a user that may log in.
	StatusActive = "active"
	// StatusDisabled marks a user whose access is revoked; all sessions and
	// API tokens are invalidated when this status is set.
	StatusDisabled = "disabled"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserSettings struct {
	UserID                          string                            `json:"user_id"`
	WorkspaceID                     string                            `json:"workspace_id"`
	KanbanViewMode                  string                            `json:"kanban_view_mode"`
	StartupPage                     string                            `json:"startup_page"`
	WorkflowFilterID                string                            `json:"workflow_filter_id"`
	RepositoryIDs                   []string                          `json:"repository_ids"`
	TasksListSort                   string                            `json:"tasks_list_sort"`
	TasksListGroup                  string                            `json:"tasks_list_group"`
	TasksListShowDetails            bool                              `json:"tasks_list_show_details"`
	InitialSetupComplete            bool                              `json:"initial_setup_complete"`
	PreferredShell                  string                            `json:"preferred_shell"`
	DefaultEditorID                 string                            `json:"default_editor_id"`
	EnablePreviewOnClick            bool                              `json:"enable_preview_on_click"`
	ChatSubmitKey                   string                            `json:"chat_submit_key"` // "enter" | "cmd_enter"
	ReviewAutoMarkOnScroll          bool                              `json:"review_auto_mark_on_scroll"`
	ConfirmTaskArchive              bool                              `json:"confirm_task_archive"`
	UnreadDivider                   bool                              `json:"unread_divider"`
	AgentGeneratedTaskTitles        bool                              `json:"agent_generated_task_titles"`
	MCPTaskAgentProfileDefault      string                            `json:"mcp_task_agent_profile_default"`
	ShowAnchoredPromptBar           bool                              `json:"show_anchored_prompt_bar"` // desktop-only sticky last-prompt bar
	ShowScrollToLastPrompt          bool                              `json:"show_scroll_to_last_prompt"`
	ShowScrollToStart               bool                              `json:"show_scroll_to_start"`
	ShowTranscriptAutoScrollControl bool                              `json:"show_transcript_auto_scroll_control"`
	ShowTodoListPanel               bool                              `json:"show_todo_list_panel"`
	ShowReleaseNotification         bool                              `json:"show_release_notification"`
	ReleaseNotesLastSeenVersion     string                            `json:"release_notes_last_seen_version"`
	LspAutoStartLanguages           []string                          `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages         []string                          `json:"lsp_auto_install_languages"`
	LspServerConfigs                map[string]map[string]interface{} `json:"lsp_server_configs"`
	LspStatusLocation               string                            `json:"lsp_status_location"`
	SavedLayouts                    []SavedLayout                     `json:"saved_layouts"`
	SidebarViews                    []SidebarView                     `json:"sidebar_views"`
	SidebarActiveViewID             string                            `json:"sidebar_active_view_id"`
	SidebarDraft                    *SidebarViewDraft                 `json:"sidebar_draft"`
	SidebarTaskPrefs                SidebarTaskPrefs                  `json:"sidebar_task_prefs"`
	TaskCreateLastUsed              TaskCreateLastUsed                `json:"task_create_last_used"`
	JiraSavedViews                  json.RawMessage                   `json:"jira_saved_views"`
	JiraTaskPresets                 json.RawMessage                   `json:"jira_task_presets"`
	GitHubSavedPresets              json.RawMessage                   `json:"github_saved_presets"`
	GitHubDefaultQueryPresets       json.RawMessage                   `json:"github_default_query_presets"`
	GitLabSavedPresets              json.RawMessage                   `json:"gitlab_saved_presets"`
	AzureDevOpsBrowsePreferences    json.RawMessage                   `json:"azure_devops_browse_preferences"`
	DefaultUtilityAgentID           string                            `json:"default_utility_agent_id"` // Default inference agent for utility agents
	DefaultUtilityModel             string                            `json:"default_utility_model"`    // Default model for utility agents
	KeyboardShortcuts               map[string]interface{}            `json:"keyboard_shortcuts"`       // User-configured keyboard shortcut overrides
	TerminalLinkBehavior            string                            `json:"terminal_link_behavior"`   // "new_tab" | "browser_panel"
	TerminalFontFamily              string                            `json:"terminal_font_family"`
	TerminalFontSize                int                               `json:"terminal_font_size"`
	ChangesPanelLayout              string                            `json:"changes_panel_layout"` // "flat" | "tree"
	SystemMetricsDisplay            SystemMetricsDisplaySettings      `json:"system_metrics_display"`
	AppStatusBarOrder               AppStatusBarOrder                 `json:"app_status_bar_order"`
	VoiceMode                       VoiceModeSettings                 `json:"voice_mode"`
	CreatedAt                       time.Time                         `json:"created_at"`
	UpdatedAt                       time.Time                         `json:"updated_at"`
}

type SystemMetricsDisplaySettings struct {
	ShowInTopbar bool `json:"show_in_topbar"`
	Simplified   bool `json:"simplified"`
}

type AppStatusBarOrder struct {
	LeftItemIDs  []string `json:"left_item_ids"`
	RightItemIDs []string `json:"right_item_ids"`
}

// VoiceModeSettings is the per-user configuration surface for the chat
// voice-input feature. Stored as a nested JSON object inside the `users.settings`
// blob — adding fields here does not require a schema migration.
type VoiceModeSettings struct {
	// Enabled gates the whole feature. When false, the mic button is hidden
	// entirely and no voice-related hooks run on the chat input. Defaults to
	// true for new users; pre-existing user rows that have no `enabled` field
	// in their stored JSON are also treated as enabled (see store layer).
	Enabled bool `json:"enabled"`
	// Engine is the user's preferred transcription engine.
	// "auto" | "webSpeech" | "whisperWeb" | "whisperServer". Default "auto".
	Engine string `json:"engine"`
	// Language is the BCP-47 tag or "auto" to use the browser's language.
	// Examples: "en-US", "pt-PT", "ja-JP". Default "auto".
	Language string `json:"language"`
	// Mode controls how the mic button is activated: "toggle" (click to start/stop)
	// or "hold" (push-to-talk). Default "toggle".
	Mode string `json:"mode"`
	// AutoSend submits the chat message immediately after the transcript is inserted.
	AutoSend bool `json:"auto_send"`
	// WhisperWebModel selects the in-browser Whisper model when engine = whisperWeb.
	// "tiny" | "base" | "small". Default "base".
	WhisperWebModel string `json:"whisper_web_model"`
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
	RepositoryID      string `json:"repository_id"`
	Branch            string `json:"branch"`
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
}
