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
	UserID                      string                              `json:"user_id"`
	WorkspaceID                 string                              `json:"workspace_id"`
	KanbanViewMode              string                              `json:"kanban_view_mode"`
	WorkflowFilterID            string                              `json:"workflow_filter_id"`
	RepositoryIDs               []string                            `json:"repository_ids"`
	TasksListSort               string                              `json:"tasks_list_sort"`
	TasksListGroup              string                              `json:"tasks_list_group"`
	InitialSetupComplete        bool                                `json:"initial_setup_complete"`
	PreferredShell              string                              `json:"preferred_shell"`
	DefaultEditorID             string                              `json:"default_editor_id"`
	EnablePreviewOnClick        bool                                `json:"enable_preview_on_click"`
	ChatSubmitKey               string                              `json:"chat_submit_key"`
	ReviewAutoMarkOnScroll      bool                                `json:"review_auto_mark_on_scroll"`
	ConfirmTaskArchive          bool                                `json:"confirm_task_archive"`
	MCPTaskAgentProfileDefault  string                              `json:"mcp_task_agent_profile_default"`
	ShowReleaseNotification     bool                                `json:"show_release_notification"`
	ReleaseNotesLastSeenVersion string                              `json:"release_notes_last_seen_version"`
	LspAutoStartLanguages       []string                            `json:"lsp_auto_start_languages"`
	LspAutoInstallLanguages     []string                            `json:"lsp_auto_install_languages"`
	LspServerConfigs            map[string]map[string]interface{}   `json:"lsp_server_configs,omitempty"`
	SavedLayouts                []models.SavedLayout                `json:"saved_layouts"`
	SidebarViews                []models.SidebarView                `json:"sidebar_views"`
	SidebarActiveViewID         string                              `json:"sidebar_active_view_id"`
	SidebarDraft                *models.SidebarViewDraft            `json:"sidebar_draft"`
	SidebarTaskPrefs            models.SidebarTaskPrefs             `json:"sidebar_task_prefs"`
	TaskCreateLastUsed          models.TaskCreateLastUsed           `json:"task_create_last_used"`
	JiraSavedViews              json.RawMessage                     `json:"jira_saved_views,omitempty"`
	JiraTaskPresets             json.RawMessage                     `json:"jira_task_presets,omitempty"`
	GitHubSavedPresets          json.RawMessage                     `json:"github_saved_presets,omitempty"`
	GitHubDefaultQueryPresets   json.RawMessage                     `json:"github_default_query_presets,omitempty"`
	GitLabSavedPresets          json.RawMessage                     `json:"gitlab_saved_presets,omitempty"`
	DefaultUtilityAgentID       string                              `json:"default_utility_agent_id"`
	DefaultUtilityModel         string                              `json:"default_utility_model"`
	UtilityAgentProfileID       string                              `json:"utility_agent_profile_id"`
	KeyboardShortcuts           map[string]interface{}              `json:"keyboard_shortcuts,omitempty"`
	TerminalLinkBehavior        string                              `json:"terminal_link_behavior"`
	TerminalFontFamily          string                              `json:"terminal_font_family"`
	TerminalFontSize            int                                 `json:"terminal_font_size"`
	ChangesPanelLayout          string                              `json:"changes_panel_layout"`
	SystemMetricsDisplay        models.SystemMetricsDisplaySettings `json:"system_metrics_display"`
	VoiceMode                   models.VoiceModeSettings            `json:"voice_mode"`
	UpdatedAt                   string                              `json:"updated_at"`
}

type UserResponse struct {
	User     UserDTO         `json:"user"`
	Settings UserSettingsDTO `json:"settings"`
}

type UserSettingsResponse struct {
	Settings     UserSettingsDTO `json:"settings"`
	ShellOptions []ShellOption   `json:"shell_options"`
}

type ShellOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type UpdateUserSettingsRequest struct {
	WorkspaceID                 *string                              `json:"workspace_id,omitempty"`
	KanbanViewMode              *string                              `json:"kanban_view_mode,omitempty"`
	WorkflowFilterID            *string                              `json:"workflow_filter_id,omitempty"`
	RepositoryIDs               *[]string                            `json:"repository_ids,omitempty"`
	TasksListSort               *string                              `json:"tasks_list_sort,omitempty"`
	TasksListGroup              *string                              `json:"tasks_list_group,omitempty"`
	InitialSetupComplete        *bool                                `json:"initial_setup_complete,omitempty"`
	PreferredShell              *string                              `json:"preferred_shell,omitempty"`
	DefaultEditorID             *string                              `json:"default_editor_id,omitempty"`
	EnablePreviewOnClick        *bool                                `json:"enable_preview_on_click,omitempty"`
	ChatSubmitKey               *string                              `json:"chat_submit_key,omitempty"`
	ReviewAutoMarkOnScroll      *bool                                `json:"review_auto_mark_on_scroll,omitempty"`
	ConfirmTaskArchive          *bool                                `json:"confirm_task_archive,omitempty"`
	MCPTaskAgentProfileDefault  *string                              `json:"mcp_task_agent_profile_default,omitempty"`
	ShowReleaseNotification     *bool                                `json:"show_release_notification,omitempty"`
	ReleaseNotesLastSeenVersion *string                              `json:"release_notes_last_seen_version,omitempty"`
	LspAutoStartLanguages       *[]string                            `json:"lsp_auto_start_languages,omitempty"`
	LspAutoInstallLanguages     *[]string                            `json:"lsp_auto_install_languages,omitempty"`
	LspServerConfigs            *map[string]map[string]interface{}   `json:"lsp_server_configs,omitempty"`
	SavedLayouts                *[]models.SavedLayout                `json:"saved_layouts,omitempty"`
	SidebarViews                *[]models.SidebarView                `json:"sidebar_views,omitempty"`
	SidebarActiveViewID         *string                              `json:"sidebar_active_view_id,omitempty"`
	SidebarDraft                NullableSidebarDraft                 `json:"sidebar_draft,omitempty"`
	SidebarTaskPrefs            *models.SidebarTaskPrefs             `json:"sidebar_task_prefs,omitempty"`
	TaskCreateLastUsed          *models.TaskCreateLastUsed           `json:"task_create_last_used,omitempty"`
	JiraSavedViews              NullableRawMessage                   `json:"jira_saved_views,omitempty"`
	JiraTaskPresets             NullableRawMessage                   `json:"jira_task_presets,omitempty"`
	GitHubSavedPresets          NullableRawMessage                   `json:"github_saved_presets,omitempty"`
	GitHubDefaultQueryPresets   NullableRawMessage                   `json:"github_default_query_presets,omitempty"`
	GitLabSavedPresets          NullableRawMessage                   `json:"gitlab_saved_presets,omitempty"`
	DefaultUtilityAgentID       *string                              `json:"default_utility_agent_id,omitempty"`
	DefaultUtilityModel         *string                              `json:"default_utility_model,omitempty"`
	UtilityAgentProfileID       *string                              `json:"utility_agent_profile_id,omitempty"`
	KeyboardShortcuts           *map[string]interface{}              `json:"keyboard_shortcuts,omitempty"`
	TerminalLinkBehavior        *string                              `json:"terminal_link_behavior,omitempty"`
	TerminalFontFamily          *string                              `json:"terminal_font_family,omitempty"`
	TerminalFontSize            *int                                 `json:"terminal_font_size,omitempty"`
	ChangesPanelLayout          *string                              `json:"changes_panel_layout,omitempty"`
	SystemMetricsDisplay        *models.SystemMetricsDisplaySettings `json:"system_metrics_display,omitempty"`
	VoiceMode                   *models.VoiceModeSettings            `json:"voice_mode,omitempty"`
}

// NullableSidebarDraft preserves the JSON PATCH distinction between an omitted
// sidebar_draft field and an explicit null value. Prefer JSON decoding or
// NewNullableSidebarDraft; the zero value intentionally means "field omitted".
type NullableSidebarDraft struct {
	Set   bool
	Value *models.SidebarViewDraft
}

func NewNullableSidebarDraft(value *models.SidebarViewDraft) NullableSidebarDraft {
	return NullableSidebarDraft{Set: true, Value: value}
}

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

func (n NullableRawMessage) ServiceValue() **json.RawMessage {
	if !n.Set {
		return nil
	}
	return &n.Value
}

func FromUser(user *models.User) UserDTO {
	return UserDTO{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func FromUserSettings(settings *models.UserSettings) UserSettingsDTO {
	return UserSettingsDTO{
		UserID:                      settings.UserID,
		WorkspaceID:                 settings.WorkspaceID,
		KanbanViewMode:              settings.KanbanViewMode,
		WorkflowFilterID:            settings.WorkflowFilterID,
		RepositoryIDs:               settings.RepositoryIDs,
		TasksListSort:               settings.TasksListSort,
		TasksListGroup:              settings.TasksListGroup,
		InitialSetupComplete:        settings.InitialSetupComplete,
		PreferredShell:              settings.PreferredShell,
		DefaultEditorID:             settings.DefaultEditorID,
		EnablePreviewOnClick:        settings.EnablePreviewOnClick,
		ChatSubmitKey:               settings.ChatSubmitKey,
		ReviewAutoMarkOnScroll:      settings.ReviewAutoMarkOnScroll,
		ConfirmTaskArchive:          settings.ConfirmTaskArchive,
		MCPTaskAgentProfileDefault:  models.NormalizeMCPTaskAgentProfileDefault(settings.MCPTaskAgentProfileDefault),
		ShowReleaseNotification:     settings.ShowReleaseNotification,
		ReleaseNotesLastSeenVersion: settings.ReleaseNotesLastSeenVersion,
		LspAutoStartLanguages:       settings.LspAutoStartLanguages,
		LspAutoInstallLanguages:     settings.LspAutoInstallLanguages,
		LspServerConfigs:            settings.LspServerConfigs,
		SavedLayouts:                settings.SavedLayouts,
		SidebarViews:                settings.SidebarViews,
		SidebarActiveViewID:         settings.SidebarActiveViewID,
		SidebarDraft:                settings.SidebarDraft,
		SidebarTaskPrefs:            settings.SidebarTaskPrefs,
		TaskCreateLastUsed:          settings.TaskCreateLastUsed,
		JiraSavedViews:              settings.JiraSavedViews,
		JiraTaskPresets:             settings.JiraTaskPresets,
		GitHubSavedPresets:          settings.GitHubSavedPresets,
		GitHubDefaultQueryPresets:   settings.GitHubDefaultQueryPresets,
		GitLabSavedPresets:          settings.GitLabSavedPresets,
		DefaultUtilityAgentID:       settings.DefaultUtilityAgentID,
		DefaultUtilityModel:         settings.DefaultUtilityModel,
		UtilityAgentProfileID:       settings.UtilityAgentProfileID,
		KeyboardShortcuts:           settings.KeyboardShortcuts,
		TerminalLinkBehavior:        settings.TerminalLinkBehavior,
		TerminalFontFamily:          settings.TerminalFontFamily,
		TerminalFontSize:            settings.TerminalFontSize,
		ChangesPanelLayout:          settings.ChangesPanelLayout,
		SystemMetricsDisplay:        settings.SystemMetricsDisplay,
		VoiceMode:                   settings.VoiceMode,
		UpdatedAt:                   settings.UpdatedAt.Format(time.RFC3339),
	}
}
