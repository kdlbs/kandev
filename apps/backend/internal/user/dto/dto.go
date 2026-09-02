package dto

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/user/models"
)

type UserDTO struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettingsDTO struct {
	UserID                            string                              `json:"user_id"`
	WorkspaceID                       string                              `json:"workspace_id"`
	KanbanViewMode                    string                              `json:"kanban_view_mode"`
	StartupPage                       string                              `json:"startup_page"`
	WorkflowFilterID                  string                              `json:"workflow_filter_id"`
	RepositoryIDs                     []string                            `json:"repository_ids"`
	TasksListSort                     string                              `json:"tasks_list_sort"`
	TasksListGroup                    string                              `json:"tasks_list_group"`
	TasksListShowDetails              bool                                `json:"tasks_list_show_details"`
	InitialSetupComplete              bool                                `json:"initial_setup_complete"`
	PreferredShell                    string                              `json:"preferred_shell"`
	DefaultEditorID                   string                              `json:"default_editor_id"`
	EnablePreviewOnClick              bool                                `json:"enable_preview_on_click"`
	ChatSubmitKey                     string                              `json:"chat_submit_key"`
	ReviewAutoMarkOnScroll            bool                                `json:"review_auto_mark_on_scroll"`
	ConfirmTaskArchive                bool                                `json:"confirm_task_archive"`
	PreventAutoStartAgentOnOpen       bool                                `json:"prevent_auto_start_agent_on_open"`
	UnreadDivider                     bool                                `json:"unread_divider"`
	AgentGeneratedTaskTitles          bool                                `json:"agent_generated_task_titles"`
	MCPTaskAgentProfileDefault        string                              `json:"mcp_task_agent_profile_default"`
	ShowAnchoredPromptBar             bool                                `json:"show_anchored_prompt_bar"`
	ShowScrollToLastPrompt            bool                                `json:"show_scroll_to_last_prompt"`
	ShowScrollToStart                 bool                                `json:"show_scroll_to_start"`
	ShowTranscriptAutoScrollControl   bool                                `json:"show_transcript_auto_scroll_control"`
	ShowTodoListPanel                 bool                                `json:"show_todo_list_panel"`
	ShowTodoListPanelOnlyWhenNotEmpty bool                                `json:"show_todo_list_panel_only_when_not_empty"`
	ShowReleaseNotification           bool                                `json:"show_release_notification"`
	ReleaseNotesLastSeenVersion       string                              `json:"release_notes_last_seen_version"`
	LspAutoStartLanguages             []string                            `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages           []string                            `json:"lsp_auto_install_languages"`
	LspStatusHiddenLanguages          []string                            `json:"lsp_status_hidden_languages"`
	LspServerConfigs                  map[string]map[string]interface{}   `json:"lsp_server_configs,omitempty"`
	LspStatusLocation                 string                              `json:"lsp_status_location"`
	SavedLayouts                      []models.SavedLayout                `json:"saved_layouts"`
	SidebarViews                      []models.SidebarView                `json:"sidebar_views"`
	SidebarActiveViewID               string                              `json:"sidebar_active_view_id"`
	SidebarDraft                      *models.SidebarViewDraft            `json:"sidebar_draft"`
	SidebarTaskPrefs                  models.SidebarTaskPrefs             `json:"sidebar_task_prefs"`
	TaskCreateLastUsed                models.TaskCreateLastUsed           `json:"task_create_last_used"`
	JiraSavedViews                    json.RawMessage                     `json:"jira_saved_views,omitempty"`
	JiraTaskPresets                   json.RawMessage                     `json:"jira_task_presets,omitempty"`
	GitHubSavedPresets                json.RawMessage                     `json:"github_saved_presets,omitempty"`
	GitHubDefaultQueryPresets         json.RawMessage                     `json:"github_default_query_presets,omitempty"`
	GitLabSavedPresets                json.RawMessage                     `json:"gitlab_saved_presets,omitempty"`
	AzureDevOpsBrowsePreferences      json.RawMessage                     `json:"azure_devops_browse_preferences,omitempty"`
	DefaultUtilityAgentID             string                              `json:"default_utility_agent_id"`
	DefaultUtilityModel               string                              `json:"default_utility_model"`
	DefaultUtilityAgentProfileID      string                              `json:"default_utility_agent_profile_id"`
	KeyboardShortcuts                 map[string]interface{}              `json:"keyboard_shortcuts,omitempty"`
	TerminalLinkBehavior              string                              `json:"terminal_link_behavior"`
	TerminalFontFamily                string                              `json:"terminal_font_family"`
	TerminalFontSize                  int                                 `json:"terminal_font_size"`
	ChangesPanelLayout                string                              `json:"changes_panel_layout"`
	LastSeenDisplay                   string                              `json:"last_seen_display"`
	SystemMetricsDisplay              models.SystemMetricsDisplaySettings `json:"system_metrics_display"`
	AppStatusBarEnabled               bool                                `json:"app_status_bar_enabled"`
	AppStatusBarOrder                 models.AppStatusBarOrder            `json:"app_status_bar_order"`
	QuickChatTabOrderByWorkspace      map[string][]string                 `json:"quick_chat_tab_order_by_workspace"`
	KanbanHiddenStepIDs               map[string][]string                 `json:"kanban_hidden_step_ids"`
	WorkflowIDsWithAutoHideEmptySteps []string                            `json:"workflow_ids_with_auto_hide_empty_steps"`
	Revision                          int64                               `json:"revision"`
	UpdatedAt                         string                              `json:"updated_at"`
}

type UserResponse struct {
	User     UserDTO         `json:"user"`
	Settings UserSettingsDTO `json:"settings"`
}

type UserSettingsResponse struct {
	Settings     UserSettingsDTO `json:"settings"`
	ShellOptions []ShellOption   `json:"shell_options"`
}

type AgentProfileRecentUseDTO struct {
	Context    models.AgentProfileRecentUseContext `json:"context"`
	ProfileIDs []string                            `json:"profile_ids"`
	Revision   int64                               `json:"revision"`
	UpdatedAt  string                              `json:"updated_at"`
}

// FromAgentProfileRecentUse maps the persisted context history to its API
// representation without exposing the owning user id.
func FromAgentProfileRecentUse(record *models.AgentProfileRecentUse) AgentProfileRecentUseDTO {
	if record == nil {
		return AgentProfileRecentUseDTO{}
	}
	return AgentProfileRecentUseDTO{
		Context:    record.Context,
		ProfileIDs: append([]string{}, record.ProfileIDs...),
		Revision:   record.Revision,
		UpdatedAt:  record.UpdatedAt.Format(time.RFC3339),
	}
}

type RecordAgentProfileRecentUseRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
}

type ShellOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type UpdateUserSettingsRequest struct {
	WorkspaceID                       *string                            `json:"workspace_id,omitempty"`
	KanbanViewMode                    *string                            `json:"kanban_view_mode,omitempty"`
	StartupPage                       *string                            `json:"startup_page,omitempty"`
	WorkflowFilterID                  *string                            `json:"workflow_filter_id,omitempty"`
	RepositoryIDs                     *[]string                          `json:"repository_ids,omitempty"`
	TasksListSort                     *string                            `json:"tasks_list_sort,omitempty"`
	TasksListGroup                    *string                            `json:"tasks_list_group,omitempty"`
	TasksListShowDetails              *bool                              `json:"tasks_list_show_details,omitempty"`
	InitialSetupComplete              *bool                              `json:"initial_setup_complete,omitempty"`
	PreferredShell                    *string                            `json:"preferred_shell,omitempty"`
	DefaultEditorID                   *string                            `json:"default_editor_id,omitempty"`
	EnablePreviewOnClick              *bool                              `json:"enable_preview_on_click,omitempty"`
	ChatSubmitKey                     *string                            `json:"chat_submit_key,omitempty"`
	ReviewAutoMarkOnScroll            *bool                              `json:"review_auto_mark_on_scroll,omitempty"`
	ConfirmTaskArchive                *bool                              `json:"confirm_task_archive,omitempty"`
	PreventAutoStartAgentOnOpen       *bool                              `json:"prevent_auto_start_agent_on_open,omitempty"`
	UnreadDivider                     *bool                              `json:"unread_divider,omitempty"`
	AgentGeneratedTaskTitles          *bool                              `json:"agent_generated_task_titles,omitempty"`
	MCPTaskAgentProfileDefault        *string                            `json:"mcp_task_agent_profile_default,omitempty"`
	ShowAnchoredPromptBar             *bool                              `json:"show_anchored_prompt_bar,omitempty"`
	ShowScrollToLastPrompt            *bool                              `json:"show_scroll_to_last_prompt,omitempty"`
	ShowScrollToStart                 *bool                              `json:"show_scroll_to_start,omitempty"`
	ShowTranscriptAutoScrollControl   *bool                              `json:"show_transcript_auto_scroll_control,omitempty"`
	ShowTodoListPanel                 *bool                              `json:"show_todo_list_panel,omitempty"`
	ShowTodoListPanelOnlyWhenNotEmpty *bool                              `json:"show_todo_list_panel_only_when_not_empty,omitempty"`
	ShowReleaseNotification           *bool                              `json:"show_release_notification,omitempty"`
	ReleaseNotesLastSeenVersion       *string                            `json:"release_notes_last_seen_version,omitempty"`
	LspAutoStartLanguages             *[]string                          `json:"lsp_auto_start_languages,omitempty"`
	LspAutoInstallLanguages           *[]string                          `json:"lsp_auto_install_languages,omitempty"`
	LspStatusHiddenLanguages          *[]string                          `json:"lsp_status_hidden_languages,omitempty"`
	LspServerConfigs                  *map[string]map[string]interface{} `json:"lsp_server_configs,omitempty"`
	LspStatusLocation                 *string                            `json:"lsp_status_location,omitempty"`
	SavedLayouts                      *[]models.SavedLayout              `json:"saved_layouts,omitempty"`
	SidebarViews                      *[]models.SidebarView              `json:"sidebar_views,omitempty"`
	SidebarActiveViewID               *string                            `json:"sidebar_active_view_id,omitempty"`
	SidebarDraft                      NullableSidebarDraft               `json:"sidebar_draft,omitempty"`
	SidebarTaskPrefs                  *models.SidebarTaskPrefs           `json:"sidebar_task_prefs,omitempty"`
	TaskCreateLastUsed                *models.TaskCreateLastUsed         `json:"task_create_last_used,omitempty"`
	JiraSavedViews                    NullableRawMessage                 `json:"jira_saved_views,omitempty"`
	JiraTaskPresets                   NullableRawMessage                 `json:"jira_task_presets,omitempty"`
	GitHubSavedPresets                NullableRawMessage                 `json:"github_saved_presets,omitempty"`
	GitHubDefaultQueryPresets         NullableRawMessage                 `json:"github_default_query_presets,omitempty"`
	GitLabSavedPresets                NullableRawMessage                 `json:"gitlab_saved_presets,omitempty"`
	AzureDevOpsBrowsePreferences      NullableRawMessage                 `json:"azure_devops_browse_preferences,omitempty"`
	DefaultUtilityAgentID             *string                            `json:"default_utility_agent_id,omitempty"`
	DefaultUtilityModel               *string                            `json:"default_utility_model,omitempty"`
	DefaultUtilityAgentProfileID      *string                            `json:"default_utility_agent_profile_id,omitempty"`
	KeyboardShortcuts                 *map[string]interface{}            `json:"keyboard_shortcuts,omitempty"`
	TerminalLinkBehavior              *string                            `json:"terminal_link_behavior,omitempty"`
	TerminalFontFamily                *string                            `json:"terminal_font_family,omitempty"`
	TerminalFontSize                  *int                               `json:"terminal_font_size,omitempty"`
	ChangesPanelLayout                *string                            `json:"changes_panel_layout,omitempty"`
	LastSeenDisplay                   *string                            `json:"last_seen_display,omitempty"`
	SystemMetricsDisplay              *SystemMetricsDisplaySettingsPatch `json:"system_metrics_display,omitempty"`
	AppStatusBarEnabled               *bool                              `json:"app_status_bar_enabled,omitempty"`
	AppStatusBarOrder                 *models.AppStatusBarOrder          `json:"app_status_bar_order,omitempty"`
	QuickChatTabOrderByWorkspace      *map[string][]string               `json:"quick_chat_tab_order_by_workspace,omitempty"`
	KanbanHiddenStepIDs               *map[string][]string               `json:"kanban_hidden_step_ids,omitempty"`
	WorkflowIDsWithAutoHideEmptySteps *[]string                          `json:"workflow_ids_with_auto_hide_empty_steps,omitempty"`
}

type SystemMetricsDisplaySettingsPatch struct {
	ShowInTopbar *bool `json:"show_in_topbar,omitempty"`
	Simplified   *bool `json:"simplified,omitempty"`
}

// NullableSidebarDraft preserves the JSON PATCH distinction between an omitted
// sidebar_draft field and an explicit null value. Prefer JSON decoding or
// NewNullableSidebarDraft; the zero value intentionally means "field omitted".
type NullableSidebarDraft struct {
	Set   bool
	Value *models.SidebarViewDraft
}

// NewNullableSidebarDraft wraps a draft value as explicitly-set.
func NewNullableSidebarDraft(value *models.SidebarViewDraft) NullableSidebarDraft {
	return NullableSidebarDraft{Set: true, Value: value}
}

// UnmarshalJSON decodes an explicit null as "set with nil value" so the
// PATCH distinction survives JSON decoding.
func (n *NullableSidebarDraft) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var value models.SidebarViewDraft
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

// ServiceValue returns a **SidebarViewDraft suitable for the service layer:
// nil when the field was omitted, otherwise a pointer to the value (possibly
// nil when explicitly cleared).
func (n NullableSidebarDraft) ServiceValue() **models.SidebarViewDraft {
	if !n.Set {
		return nil
	}
	return &n.Value
}

// NullableRawMessage preserves PATCH semantics for raw JSON preference blobs:
// omitted means "leave unchanged"; explicit null means "clear".
type NullableRawMessage struct {
	Set   bool
	Value *json.RawMessage
}

// UnmarshalJSON decodes an explicit null as "set with nil value" so the
// PATCH distinction survives JSON decoding.
func (n *NullableRawMessage) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var value json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

// ServiceValue returns a **json.RawMessage suitable for the service layer:
// nil when the field was omitted, otherwise a pointer to the value (possibly
// nil when explicitly cleared).
func (n NullableRawMessage) ServiceValue() **json.RawMessage {
	if !n.Set {
		return nil
	}
	return &n.Value
}

// FromUser maps a user model to its API DTO.
func FromUser(user *models.User) UserDTO {
	return UserDTO{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// FromUserSettings maps a settings model to its API DTO, normalizing enum
// fields (startup page, MCP default, LSP location) to canonical values.
func FromUserSettings(settings *models.UserSettings) UserSettingsDTO {
	return UserSettingsDTO{
		UserID:                            settings.UserID,
		WorkspaceID:                       settings.WorkspaceID,
		KanbanViewMode:                    settings.KanbanViewMode,
		StartupPage:                       models.NormalizeStartupPage(settings.StartupPage),
		WorkflowFilterID:                  settings.WorkflowFilterID,
		RepositoryIDs:                     settings.RepositoryIDs,
		TasksListSort:                     settings.TasksListSort,
		TasksListGroup:                    settings.TasksListGroup,
		TasksListShowDetails:              settings.TasksListShowDetails,
		InitialSetupComplete:              settings.InitialSetupComplete,
		PreferredShell:                    settings.PreferredShell,
		DefaultEditorID:                   settings.DefaultEditorID,
		EnablePreviewOnClick:              settings.EnablePreviewOnClick,
		ChatSubmitKey:                     settings.ChatSubmitKey,
		ReviewAutoMarkOnScroll:            settings.ReviewAutoMarkOnScroll,
		ConfirmTaskArchive:                settings.ConfirmTaskArchive,
		PreventAutoStartAgentOnOpen:       settings.PreventAutoStartAgentOnOpen,
		UnreadDivider:                     settings.UnreadDivider,
		AgentGeneratedTaskTitles:          settings.AgentGeneratedTaskTitles,
		MCPTaskAgentProfileDefault:        models.NormalizeMCPTaskAgentProfileDefault(settings.MCPTaskAgentProfileDefault),
		ShowAnchoredPromptBar:             settings.ShowAnchoredPromptBar,
		ShowScrollToLastPrompt:            settings.ShowScrollToLastPrompt,
		ShowScrollToStart:                 settings.ShowScrollToStart,
		ShowTranscriptAutoScrollControl:   settings.ShowTranscriptAutoScrollControl,
		ShowTodoListPanel:                 settings.ShowTodoListPanel,
		ShowTodoListPanelOnlyWhenNotEmpty: settings.ShowTodoListPanelOnlyWhenNotEmpty,
		ShowReleaseNotification:           settings.ShowReleaseNotification,
		ReleaseNotesLastSeenVersion:       settings.ReleaseNotesLastSeenVersion,
		LspAutoStartLanguages:             settings.LspAutoStartLanguages,
		LspAutoInstallLanguages:           settings.LspAutoInstallLanguages,
		LspStatusHiddenLanguages:          settings.LspStatusHiddenLanguages,
		LspServerConfigs:                  settings.LspServerConfigs,
		LspStatusLocation:                 models.NormalizeLspStatusLocation(settings.LspStatusLocation),
		SavedLayouts:                      settings.SavedLayouts,
		SidebarViews:                      settings.SidebarViews,
		SidebarActiveViewID:               settings.SidebarActiveViewID,
		SidebarDraft:                      settings.SidebarDraft,
		SidebarTaskPrefs:                  settings.SidebarTaskPrefs,
		TaskCreateLastUsed:                settings.TaskCreateLastUsed,
		JiraSavedViews:                    settings.JiraSavedViews,
		JiraTaskPresets:                   settings.JiraTaskPresets,
		GitHubSavedPresets:                settings.GitHubSavedPresets,
		GitHubDefaultQueryPresets:         settings.GitHubDefaultQueryPresets,
		GitLabSavedPresets:                settings.GitLabSavedPresets,
		AzureDevOpsBrowsePreferences:      settings.AzureDevOpsBrowsePreferences,
		DefaultUtilityAgentID:             settings.DefaultUtilityAgentID,
		DefaultUtilityModel:               settings.DefaultUtilityModel,
		DefaultUtilityAgentProfileID:      settings.DefaultUtilityAgentProfileID,
		KeyboardShortcuts:                 settings.KeyboardShortcuts,
		TerminalLinkBehavior:              settings.TerminalLinkBehavior,
		TerminalFontFamily:                settings.TerminalFontFamily,
		TerminalFontSize:                  settings.TerminalFontSize,
		ChangesPanelLayout:                settings.ChangesPanelLayout,
		LastSeenDisplay:                   models.NormalizeLastSeenDisplay(settings.LastSeenDisplay),
		SystemMetricsDisplay:              settings.SystemMetricsDisplay,
		AppStatusBarEnabled:               settings.AppStatusBarEnabled,
		AppStatusBarOrder:                 settings.AppStatusBarOrder,
		QuickChatTabOrderByWorkspace:      settings.QuickChatTabOrderByWorkspace,
		KanbanHiddenStepIDs:               settings.KanbanHiddenStepIDs,
		WorkflowIDsWithAutoHideEmptySteps: append([]string{}, settings.WorkflowIDsWithAutoHideEmptySteps...),
		Revision:                          settings.Revision,
		UpdatedAt:                         settings.UpdatedAt.Format(time.RFC3339),
	}
}
