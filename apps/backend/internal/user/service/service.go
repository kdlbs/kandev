package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrValidation   = errors.New("validation error")
)

const (
	changesPanelLayoutFlat = "flat"
	changesPanelLayoutTree = "tree"
)

type Service struct {
	repo        store.Repository
	eventBus    bus.EventBus
	logger      *logger.Logger
	defaultUser string
}

type UpdateUserSettingsRequest struct {
	WorkspaceID                 *string
	KanbanViewMode              *string
	WorkflowFilterID            *string
	RepositoryIDs               *[]string
	TasksListSort               *string
	TasksListGroup              *string
	InitialSetupComplete        *bool
	PreferredShell              *string
	DefaultEditorID             *string
	EnablePreviewOnClick        *bool
	ChatSubmitKey               *string
	ReviewAutoMarkOnScroll      *bool
	ConfirmTaskArchive          *bool
	ShowReleaseNotification     *bool
	ReleaseNotesLastSeenVersion *string
	LspAutoStartLanguages       *[]string
	LspAutoInstallLanguages     *[]string
	LspServerConfigs            *map[string]map[string]interface{}
	SavedLayouts                *[]models.SavedLayout
	SidebarViews                *[]models.SidebarView
	SidebarActiveViewID         *string
	SidebarDraft                **models.SidebarViewDraft
	SidebarTaskPrefs            *models.SidebarTaskPrefs
	TaskCreateLastUsed          *models.TaskCreateLastUsed
	JiraSavedViews              **json.RawMessage
	JiraTaskPresets             **json.RawMessage
	GitHubSavedPresets          **json.RawMessage
	GitHubDefaultQueryPresets   **json.RawMessage
	GitLabSavedPresets          **json.RawMessage
	DefaultUtilityAgentID       *string
	DefaultUtilityModel         *string
	KeyboardShortcuts           *map[string]interface{}
	TerminalLinkBehavior        *string
	TerminalFontFamily          *string
	TerminalFontSize            *int
	ChangesPanelLayout          *string
	SystemMetricsDisplay        *models.SystemMetricsDisplaySettings
	VoiceMode                   *models.VoiceModeSettings
}

func NewService(repo store.Repository, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		repo:        repo,
		eventBus:    eventBus,
		logger:      log.WithFields(zap.String("component", "user-service")),
		defaultUser: store.DefaultUserID,
	}
}

func (s *Service) GetCurrentUser(ctx context.Context) (*models.User, error) {
	user, err := s.repo.GetUser(ctx, s.defaultUser)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *Service) GetUserSettings(ctx context.Context) (*models.UserSettings, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Service) PreferredShell(ctx context.Context) (string, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return "", err
	}
	return settings.PreferredShell, nil
}

// GetDefaultUtilitySettings returns the user's default utility agent/model settings.
func (s *Service) GetDefaultUtilitySettings(ctx context.Context) (agentID, model string, err error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return "", "", err
	}
	return settings.DefaultUtilityAgentID, settings.DefaultUtilityModel, nil
}

func (s *Service) UpdateUserSettings(ctx context.Context, req *UpdateUserSettingsRequest) (*models.UserSettings, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return nil, err
	}
	if err := applyBasicSettings(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := s.applyChatSubmitKey(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applyLSPSettings(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applySavedLayouts(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applySidebarViews(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applySidebarViewState(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applyUserPreferenceBlobs(settings, req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if err := applyVoiceMode(settings, req.VoiceMode); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	settings.UpdatedAt = time.Now().UTC()
	var taskCreatePatch *models.TaskCreateLastUsed
	if req.TaskCreateLastUsed != nil && !taskCreateLastUsedPatchEmpty(*req.TaskCreateLastUsed) {
		taskCreatePatch = req.TaskCreateLastUsed
	}
	settings, err = s.repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, taskCreatePatch)
	if err != nil {
		return nil, err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return settings, nil
}

func (s *Service) RecordTaskCreateLastUsed(ctx context.Context, patch models.TaskCreateLastUsed) error {
	if taskCreateLastUsedPatchEmpty(patch) {
		return nil
	}
	settings, err := s.updateTaskCreateLastUsed(ctx, patch)
	if err != nil {
		return err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return nil
}

func (s *Service) updateTaskCreateLastUsed(ctx context.Context, patch models.TaskCreateLastUsed) (*models.UserSettings, error) {
	return s.repo.UpdateTaskCreateLastUsed(ctx, s.defaultUser, patch)
}

func taskCreateLastUsedPatchEmpty(patch models.TaskCreateLastUsed) bool {
	return patch.RepositoryID == "" &&
		patch.Branch == "" &&
		patch.AgentProfileID == "" &&
		patch.ExecutorProfileID == ""
}

// applyBasicSettings copies simple (non-validated) fields from req to settings.
func applyBasicSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.WorkspaceID != nil {
		settings.WorkspaceID = *req.WorkspaceID
	}
	if req.KanbanViewMode != nil {
		settings.KanbanViewMode = *req.KanbanViewMode
	}
	if req.WorkflowFilterID != nil {
		settings.WorkflowFilterID = *req.WorkflowFilterID
	}
	if req.RepositoryIDs != nil {
		settings.RepositoryIDs = *req.RepositoryIDs
	}
	if err := applyTasksListPreferences(settings, req.TasksListSort, req.TasksListGroup); err != nil {
		return err
	}
	if req.InitialSetupComplete != nil {
		settings.InitialSetupComplete = *req.InitialSetupComplete
	}
	if req.PreferredShell != nil {
		settings.PreferredShell = strings.TrimSpace(*req.PreferredShell)
	}
	if req.DefaultEditorID != nil {
		settings.DefaultEditorID = strings.TrimSpace(*req.DefaultEditorID)
	}
	if req.EnablePreviewOnClick != nil {
		settings.EnablePreviewOnClick = *req.EnablePreviewOnClick
	}
	if req.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *req.ReviewAutoMarkOnScroll
	}
	if req.ConfirmTaskArchive != nil {
		settings.ConfirmTaskArchive = *req.ConfirmTaskArchive
	}
	if req.ShowReleaseNotification != nil {
		settings.ShowReleaseNotification = *req.ShowReleaseNotification
	}
	if req.ReleaseNotesLastSeenVersion != nil {
		settings.ReleaseNotesLastSeenVersion = *req.ReleaseNotesLastSeenVersion
	}
	if req.DefaultUtilityAgentID != nil {
		settings.DefaultUtilityAgentID = strings.TrimSpace(*req.DefaultUtilityAgentID)
	}
	if req.DefaultUtilityModel != nil {
		settings.DefaultUtilityModel = strings.TrimSpace(*req.DefaultUtilityModel)
	}
	if req.KeyboardShortcuts != nil {
		if err := validateKeyboardShortcuts(*req.KeyboardShortcuts); err != nil {
			return err
		}
		settings.KeyboardShortcuts = *req.KeyboardShortcuts
	}
	if err := applyTerminalLinkBehavior(settings, req.TerminalLinkBehavior); err != nil {
		return err
	}
	if err := applyChangesPanelLayout(settings, req.ChangesPanelLayout); err != nil {
		return err
	}
	if req.SystemMetricsDisplay != nil {
		settings.SystemMetricsDisplay = *req.SystemMetricsDisplay
	}
	if req.TerminalFontFamily != nil {
		settings.TerminalFontFamily = strings.TrimSpace(*req.TerminalFontFamily)
	}
	if req.TerminalFontSize != nil {
		v := *req.TerminalFontSize
		if v != 0 && (v < 8 || v > 24) {
			return errors.New("terminal_font_size must be 0 (default) or between 8 and 24")
		}
		settings.TerminalFontSize = v
	}
	return nil
}

func applyTasksListPreferences(settings *models.UserSettings, sortValue, groupValue *string) error {
	if sortValue != nil {
		v := strings.TrimSpace(*sortValue)
		if v == "" {
			v = models.TasksListSortDefault
		}
		if !models.IsValidTasksListSort(v) {
			return fmt.Errorf("tasks_list_sort must be one of %s", strings.Join(models.TasksListSortValues(), ", "))
		}
		settings.TasksListSort = v
	}
	if groupValue != nil {
		v := strings.TrimSpace(*groupValue)
		if v == "" {
			v = models.TasksListGroupDefault
		}
		if !models.IsValidTasksListGroup(v) {
			return fmt.Errorf("tasks_list_group must be one of %s", strings.Join(models.TasksListGroupValues(), ", "))
		}
		settings.TasksListGroup = v
	}
	return nil
}

func applyTerminalLinkBehavior(settings *models.UserSettings, value *string) error {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v != "new_tab" && v != "browser_panel" {
		return errors.New("terminal_link_behavior must be 'new_tab' or 'browser_panel'")
	}
	settings.TerminalLinkBehavior = v
	return nil
}

func applyChangesPanelLayout(settings *models.UserSettings, value *string) error {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v != changesPanelLayoutFlat && v != changesPanelLayoutTree {
		return errors.New("changes_panel_layout must be 'flat' or 'tree'")
	}
	settings.ChangesPanelLayout = v
	return nil
}

var (
	validVoiceEngines = map[string]struct{}{
		"auto":          {},
		"webSpeech":     {},
		"whisperWeb":    {},
		"whisperServer": {},
	}
	validVoiceModes = map[string]struct{}{
		"toggle": {},
		"hold":   {},
	}
	validWhisperWebModels = map[string]struct{}{
		"tiny":  {},
		"base":  {},
		"small": {},
	}
)

// applyVoiceMode validates the inbound voice-mode settings and merges them
// onto the user record. Each sub-field is validated independently so a
// partial update (e.g. just `engine`) still works.
//
// `enabled` and `auto_send` are plain bools — every PATCH carries them. The
// settings UI always sends the full VoiceMode object so partial updates that
// would otherwise zero these are not a real concern.
func applyVoiceMode(settings *models.UserSettings, value *models.VoiceModeSettings) error {
	if value == nil {
		return nil
	}
	current := settings.VoiceMode
	if current.Engine == "" {
		current.Engine = "auto"
	}
	if value.Engine != "" {
		if _, ok := validVoiceEngines[value.Engine]; !ok {
			return errors.New("voice_mode.engine must be 'auto', 'webSpeech', 'whisperWeb', or 'whisperServer'")
		}
		current.Engine = value.Engine
	}
	if value.Language != "" {
		current.Language = strings.TrimSpace(value.Language)
	}
	if value.Mode != "" {
		if _, ok := validVoiceModes[value.Mode]; !ok {
			return errors.New("voice_mode.mode must be 'toggle' or 'hold'")
		}
		current.Mode = value.Mode
	}
	if value.WhisperWebModel != "" {
		if _, ok := validWhisperWebModels[value.WhisperWebModel]; !ok {
			return errors.New("voice_mode.whisper_web_model must be 'tiny', 'base', or 'small'")
		}
		current.WhisperWebModel = value.WhisperWebModel
	}
	current.AutoSend = value.AutoSend
	current.Enabled = value.Enabled
	settings.VoiceMode = current
	return nil
}

// applyChatSubmitKey validates and applies the chat_submit_key setting.
func (s *Service) applyChatSubmitKey(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.ChatSubmitKey == nil {
		return nil
	}
	key := strings.TrimSpace(*req.ChatSubmitKey)
	if key != "enter" && key != "cmd_enter" {
		return errors.New("chat_submit_key must be 'enter' or 'cmd_enter'")
	}
	s.logger.Info("[Settings] Setting ChatSubmitKey", zap.String("value", key))
	settings.ChatSubmitKey = key
	return nil
}

// applyLSPSettings validates and applies LSP-related settings.
func applyLSPSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.LspAutoStartLanguages != nil {
		if err := validateLSPLanguages(*req.LspAutoStartLanguages); err != nil {
			return fmt.Errorf("lsp_auto_start_languages: %w", err)
		}
		settings.LspAutoStartLanguages = *req.LspAutoStartLanguages
	}
	if req.LspAutoInstallLanguages != nil {
		if err := validateLSPLanguages(*req.LspAutoInstallLanguages); err != nil {
			return fmt.Errorf("lsp_auto_install_languages: %w", err)
		}
		settings.LspAutoInstallLanguages = *req.LspAutoInstallLanguages
	}
	if req.LspServerConfigs != nil {
		settings.LspServerConfigs = *req.LspServerConfigs
	}
	return nil
}

const maxSavedLayouts = 20

// applySavedLayouts validates and applies the saved_layouts setting.
func applySavedLayouts(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SavedLayouts == nil {
		return nil
	}
	layouts := *req.SavedLayouts
	if len(layouts) > maxSavedLayouts {
		return fmt.Errorf("saved_layouts: max %d layouts allowed", maxSavedLayouts)
	}
	for i := range layouts {
		if strings.TrimSpace(layouts[i].Name) == "" {
			return errors.New("saved_layouts: layout name must not be empty")
		}
	}
	settings.SavedLayouts = layouts
	return nil
}

const maxSidebarViews = 50

// applySidebarViews validates and applies the sidebar_views setting.
func applySidebarViews(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarViews == nil {
		return nil
	}
	views := *req.SidebarViews
	if len(views) > maxSidebarViews {
		return fmt.Errorf("sidebar_views: max %d views allowed", maxSidebarViews)
	}
	seen := make(map[string]struct{}, len(views))
	for i := range views {
		if strings.TrimSpace(views[i].ID) == "" {
			return errors.New("sidebar_views: view id must not be empty")
		}
		if strings.TrimSpace(views[i].Name) == "" {
			return errors.New("sidebar_views: view name must not be empty")
		}
		if _, dup := seen[views[i].ID]; dup {
			return fmt.Errorf("sidebar_views: duplicate view id %q", views[i].ID)
		}
		seen[views[i].ID] = struct{}{}
	}
	settings.SidebarViews = views
	return nil
}

func applySidebarViewState(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarActiveViewID != nil {
		activeViewID := strings.TrimSpace(*req.SidebarActiveViewID)
		if activeViewID == "" {
			return errors.New("sidebar_active_view_id must not be empty")
		}
		if !sidebarViewIDExists(settings.SidebarViews, activeViewID) {
			return fmt.Errorf("sidebar_active_view_id %q does not match any saved view", activeViewID)
		}
		settings.SidebarActiveViewID = activeViewID
	}
	if req.SidebarDraft != nil {
		settings.SidebarDraft = *req.SidebarDraft
	}
	return nil
}

func sidebarViewIDExists(views []models.SidebarView, id string) bool {
	for _, view := range views {
		if view.ID == id {
			return true
		}
	}
	return false
}

const maxUserPreferenceBlobBytes = 64 * 1024

func applyUserPreferenceBlobs(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarTaskPrefs != nil {
		settings.SidebarTaskPrefs = *req.SidebarTaskPrefs
	}
	if err := applyUserPreferenceBlob("jira_saved_views", req.JiraSavedViews, &settings.JiraSavedViews); err != nil {
		return err
	}
	if err := applyUserPreferenceBlob("jira_task_presets", req.JiraTaskPresets, &settings.JiraTaskPresets); err != nil {
		return err
	}
	if err := applyUserPreferenceBlob("github_saved_presets", req.GitHubSavedPresets, &settings.GitHubSavedPresets); err != nil {
		return err
	}
	if err := applyUserPreferenceBlob("github_default_query_presets", req.GitHubDefaultQueryPresets, &settings.GitHubDefaultQueryPresets); err != nil {
		return err
	}
	if err := applyUserPreferenceBlob("gitlab_saved_presets", req.GitLabSavedPresets, &settings.GitLabSavedPresets); err != nil {
		return err
	}
	return nil
}

func applyUserPreferenceBlob(field string, value **json.RawMessage, target *json.RawMessage) error {
	if value == nil {
		return nil
	}
	if *value == nil {
		*target = nil
		return nil
	}
	if err := validateUserPreferenceBlob(field, **value); err != nil {
		return err
	}
	*target = **value
	return nil
}

func validateUserPreferenceBlob(field string, value json.RawMessage) error {
	if len(value) > maxUserPreferenceBlobBytes {
		return fmt.Errorf("%s: max %d bytes allowed", field, maxUserPreferenceBlobBytes)
	}
	var decoded interface{}
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("%s: must be valid JSON", field)
	}
	switch decoded.(type) {
	case nil, []interface{}, map[string]interface{}:
		return nil
	default:
		return fmt.Errorf("%s: must be a JSON object, array, or null", field)
	}
}

func (s *Service) publishUserSettingsEvent(ctx context.Context, settings *models.UserSettings) {
	if s.eventBus == nil || settings == nil {
		return
	}
	data := map[string]interface{}{
		"user_id":                         settings.UserID,
		"workspace_id":                    settings.WorkspaceID,
		"kanban_view_mode":                settings.KanbanViewMode,
		"workflow_filter_id":              settings.WorkflowFilterID,
		"repository_ids":                  settings.RepositoryIDs,
		"tasks_list_sort":                 settings.TasksListSort,
		"tasks_list_group":                settings.TasksListGroup,
		"initial_setup_complete":          settings.InitialSetupComplete,
		"preferred_shell":                 settings.PreferredShell,
		"default_editor_id":               settings.DefaultEditorID,
		"enable_preview_on_click":         settings.EnablePreviewOnClick,
		"chat_submit_key":                 settings.ChatSubmitKey,
		"review_auto_mark_on_scroll":      settings.ReviewAutoMarkOnScroll,
		"confirm_task_archive":            settings.ConfirmTaskArchive,
		"show_release_notification":       settings.ShowReleaseNotification,
		"release_notes_last_seen_version": settings.ReleaseNotesLastSeenVersion,
		"lsp_auto_start_languages":        settings.LspAutoStartLanguages,
		"lsp_auto_install_languages":      settings.LspAutoInstallLanguages,
		"lsp_server_configs":              settings.LspServerConfigs,
		"saved_layouts":                   settings.SavedLayouts,
		"sidebar_views":                   settings.SidebarViews,
		"sidebar_active_view_id":          settings.SidebarActiveViewID,
		"sidebar_draft":                   settings.SidebarDraft,
		"sidebar_task_prefs":              settings.SidebarTaskPrefs,
		"task_create_last_used":           settings.TaskCreateLastUsed,
		"jira_saved_views":                settings.JiraSavedViews,
		"jira_task_presets":               settings.JiraTaskPresets,
		"github_saved_presets":            settings.GitHubSavedPresets,
		"github_default_query_presets":    settings.GitHubDefaultQueryPresets,
		"gitlab_saved_presets":            settings.GitLabSavedPresets,
		"default_utility_agent_id":        settings.DefaultUtilityAgentID,
		"default_utility_model":           settings.DefaultUtilityModel,
		"keyboard_shortcuts":              settings.KeyboardShortcuts,
		"terminal_link_behavior":          settings.TerminalLinkBehavior,
		"terminal_font_family":            settings.TerminalFontFamily,
		"terminal_font_size":              settings.TerminalFontSize,
		"changes_panel_layout":            settings.ChangesPanelLayout,
		"system_metrics_display":          settings.SystemMetricsDisplay,
		"voice_mode":                      settings.VoiceMode,
		"updated_at":                      settings.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, events.UserSettingsUpdated, bus.NewEvent(events.UserSettingsUpdated, "user-service", data)); err != nil {
		s.logger.Error("failed to publish user settings event", zap.Error(err))
	}
}

func validateKeyboardShortcuts(shortcuts map[string]interface{}) error {
	for name, raw := range shortcuts {
		shortcut, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("keyboard_shortcuts.%s must be an object", name)
		}
		key, ok := shortcut["key"].(string)
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("keyboard_shortcuts.%s.key must be a non-empty string", name)
		}
		if modsRaw, exists := shortcut["modifiers"]; exists {
			mods, ok := modsRaw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("keyboard_shortcuts.%s.modifiers must be an object", name)
			}
			for mod, v := range mods {
				if _, ok := v.(bool); !ok {
					return fmt.Errorf("keyboard_shortcuts.%s.modifiers.%s must be a boolean", name, mod)
				}
			}
		}
	}
	return nil
}

func validateLSPLanguages(langs []string) error {
	supported := installer.SupportedLanguages()
	for _, lang := range langs {
		if _, ok := supported[lang]; !ok {
			return fmt.Errorf("unsupported language: %s", lang)
		}
	}
	return nil
}

func (s *Service) ClearDefaultEditorID(ctx context.Context, editorID string) error {
	if editorID == "" {
		return nil
	}
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return err
	}
	if settings.DefaultEditorID != editorID {
		return nil
	}
	settings.DefaultEditorID = ""
	settings.UpdatedAt = time.Now().UTC()
	settings, err = s.repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, nil)
	if err != nil {
		return err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return nil
}
