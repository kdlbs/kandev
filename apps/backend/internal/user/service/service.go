package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

// ErrUserSettingsConflict marks an exhausted conditional settings update.
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrValidation           = errors.New("validation error")
	ErrUserSettingsConflict = store.ErrUserSettingsRevisionConflict
)

const (
	changesPanelLayoutFlat = "flat"
	changesPanelLayoutTree = "tree"

	maxUserSettingsCASAttempts = 3
)

type Service struct {
	repo        store.Repository
	eventBus    bus.EventBus
	logger      *logger.Logger
	defaultUser string
}

type UpdateUserSettingsRequest struct {
	WorkspaceID                       *string
	KanbanViewMode                    *string
	StartupPage                       *string
	WorkflowFilterID                  *string
	RepositoryIDs                     *[]string
	TasksListSort                     *string
	TasksListGroup                    *string
	TasksListShowDetails              *bool
	InitialSetupComplete              *bool
	PreferredShell                    *string
	DefaultEditorID                   *string
	EnablePreviewOnClick              *bool
	ChatSubmitKey                     *string
	ReviewAutoMarkOnScroll            *bool
	ConfirmTaskArchive                *bool
	PreventAutoStartAgentOnOpen       *bool
	UnreadDivider                     *bool
	AgentGeneratedTaskTitles          *bool
	MCPTaskAgentProfileDefault        *string
	ShowAnchoredPromptBar             *bool
	ShowScrollToLastPrompt            *bool
	ShowScrollToStart                 *bool
	ShowTranscriptAutoScrollControl   *bool
	ShowTodoListPanel                 *bool
	ShowTodoListPanelOnlyWhenNotEmpty *bool
	ShowReleaseNotification           *bool
	ReleaseNotesLastSeenVersion       *string
	LspAutoStartLanguages             *[]string
	LspAutoInstallLanguages           *[]string
	LspServerConfigs                  *map[string]map[string]interface{}
	LspStatusLocation                 *string
	SavedLayouts                      *[]models.SavedLayout
	SidebarViews                      *[]models.SidebarView
	SidebarActiveViewID               *string
	SidebarDraft                      **models.SidebarViewDraft
	SidebarTaskPrefs                  *models.SidebarTaskPrefs
	TaskCreateLastUsed                *models.TaskCreateLastUsed
	JiraSavedViews                    **json.RawMessage
	JiraTaskPresets                   **json.RawMessage
	GitHubSavedPresets                **json.RawMessage
	GitHubDefaultQueryPresets         **json.RawMessage
	GitLabSavedPresets                **json.RawMessage
	AzureDevOpsBrowsePreferences      **json.RawMessage
	DefaultUtilityAgentID             *string
	DefaultUtilityModel               *string
	DefaultUtilityAgentProfileID      *string
	KeyboardShortcuts                 *map[string]interface{}
	TerminalLinkBehavior              *string
	TerminalFontFamily                *string
	TerminalFontSize                  *int
	ChangesPanelLayout                *string
	LastSeenDisplay                   *string
	SystemMetricsDisplay              *SystemMetricsDisplaySettingsPatch
	AppStatusBarEnabled               *bool
	AppStatusBarOrder                 *models.AppStatusBarOrder
	KanbanHiddenStepIDs               *map[string][]string
	WorkflowIDsWithAutoHideEmptySteps *[]string
}

type SystemMetricsDisplaySettingsPatch struct {
	ShowInTopbar *bool
	Simplified   *bool
}

// NewService builds the user settings service with its repository, event bus,
// and logger.
func NewService(repo store.Repository, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		repo:        repo,
		eventBus:    eventBus,
		logger:      log.WithFields(zap.String("component", "user-service")),
		defaultUser: store.DefaultUserID,
	}
}

// settingsUserID resolves the effective user for settings operations:
// the authenticated identity when present, otherwise the default user.
func (s *Service) settingsUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return s.defaultUser
	}
	return identity.UserID
}

// GetCurrentUser returns the current (or default) user, mapping a missing
// row to ErrUserNotFound.
func (s *Service) GetCurrentUser(ctx context.Context) (*models.User, error) {
	user, err := s.repo.GetUser(ctx, s.settingsUserID(ctx))
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// GetUserSettings returns the current user's persisted settings.
func (s *Service) GetUserSettings(ctx context.Context) (*models.UserSettings, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.settingsUserID(ctx))
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// PreferredShell returns the user's configured shell.
func (s *Service) PreferredShell(ctx context.Context) (string, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.settingsUserID(ctx))
	if err != nil {
		return "", err
	}
	return settings.PreferredShell, nil
}

// GetDefaultUtilitySettings returns the user's default utility agent/model settings.
func (s *Service) GetDefaultUtilitySettings(ctx context.Context) (agentID, model string, err error) {
	settings, err := s.repo.GetUserSettings(ctx, s.settingsUserID(ctx))
	if err != nil {
		return "", "", err
	}
	return settings.DefaultUtilityAgentID, settings.DefaultUtilityModel, nil
}

// GetDefaultUtilityAgentProfileID returns the profile used by new built-in utility actions.
func (s *Service) GetDefaultUtilityAgentProfileID(ctx context.Context) (string, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.settingsUserID(ctx))
	if err != nil {
		return "", err
	}
	return settings.DefaultUtilityAgentProfileID, nil
}

// UpdateUserSettings applies a partial settings patch field by field,
// validates each group, persists the result under expected-revision CAS, and
// publishes the settings update event.
func (s *Service) UpdateUserSettings(ctx context.Context, req *UpdateUserSettingsRequest) (*models.UserSettings, error) {
	var taskCreatePatch *models.TaskCreateLastUsed
	if req.TaskCreateLastUsed != nil && !taskCreateLastUsedPatchEmpty(*req.TaskCreateLastUsed) {
		taskCreatePatch = req.TaskCreateLastUsed
	}
	return s.updateUserSettingsCAS(ctx, func(settings *models.UserSettings) (bool, error) {
		// The shallow copy is safe only while every apply* helper replaces
		// reference fields (slices, maps, and json.RawMessage) instead of
		// mutating them in place. In-place mutation would alias before and make
		// DeepEqual miss the write.
		before := *settings
		if err := applyBasicSettings(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := s.applyChatSubmitKey(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := applyLSPSettings(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := applySavedLayouts(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := applySidebarViews(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := applySidebarViewState(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		if err := applyUserPreferenceBlobs(settings, req); err != nil {
			return false, fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		return !reflect.DeepEqual(*settings, before), nil
	}, taskCreatePatch)
}

// updateUserSettingsCAS applies a full-blob user-settings write under
// expected-revision CAS with bounded retry. apply is evaluated against every
// freshly read model and reports whether it mutated it; the immutable
// taskCreatePatch is passed to every attempt so a retry re-applies the same
// task-create merge against the fresh row. A no-op (apply returned false and
// no patch) writes and publishes nothing and returns the freshly read
// (current) settings. Exactly one event is published per successful write.
func (s *Service) updateUserSettingsCAS(
	ctx context.Context,
	apply func(*models.UserSettings) (bool, error),
	taskCreatePatch *models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	userID := s.settingsUserID(ctx)
	var lastErr error
	for attempt := 0; attempt < maxUserSettingsCASAttempts; attempt++ {
		settings, err := s.repo.GetUserSettings(ctx, userID)
		if err != nil {
			return nil, err
		}
		applied, err := apply(settings)
		if err != nil {
			return nil, err
		}
		if !applied && taskCreatePatch == nil {
			// No-op: report the freshly read (current) settings, write nothing,
			// publish nothing.
			return settings, nil
		}
		updated, err := s.repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, taskCreatePatch, settings.Revision)
		if err == nil {
			s.publishUserSettingsEvent(ctx, updated)
			return updated, nil
		}
		if !errors.Is(err, store.ErrUserSettingsRevisionConflict) {
			return nil, err
		}
		lastErr = err
	}
	s.logger.Warn("user settings update exhausted CAS retries",
		zap.Int("attempts", maxUserSettingsCASAttempts),
		zap.Error(lastErr),
	)
	return nil, fmt.Errorf("user settings update exhausted after %d attempts: %w", maxUserSettingsCASAttempts, lastErr)
}

// RecordTaskCreateLastUsed persists the last task-creation choices (a no-op
// for an empty patch) and publishes the settings update event.
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

// updateTaskCreateLastUsed delegates the patch to the repository.
func (s *Service) updateTaskCreateLastUsed(ctx context.Context, patch models.TaskCreateLastUsed) (*models.UserSettings, error) {
	return s.repo.UpdateTaskCreateLastUsed(ctx, s.settingsUserID(ctx), patch)
}

// taskCreateLastUsedPatchEmpty reports whether every field of the patch is
// unset, i.e. nothing to record.
func taskCreateLastUsedPatchEmpty(patch models.TaskCreateLastUsed) bool {
	return patch.RepositoryID == "" &&
		patch.Branch == "" &&
		patch.AgentProfileID == "" &&
		patch.ExecutorProfileID == "" &&
		len(patch.WorkflowIDsByWorkspace) == 0
}

// applyBasicSettings copies simple (non-validated) fields from req to settings.
func applyBasicSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if err := applyWorkspaceAndTaskListPreferences(settings, req); err != nil {
		return err
	}
	if err := applyTaskActionPreferences(settings, req); err != nil {
		return err
	}
	applyUtilityPreferences(settings, req)
	if err := applyKeyboardShortcuts(settings, req.KeyboardShortcuts); err != nil {
		return err
	}
	if err := applyTerminalLinkBehavior(settings, req.TerminalLinkBehavior); err != nil {
		return err
	}
	if err := applyChangesPanelLayout(settings, req.ChangesPanelLayout); err != nil {
		return err
	}
	if err := applyLastSeenDisplay(settings, req.LastSeenDisplay); err != nil {
		return err
	}
	applySystemMetricsDisplay(settings, req.SystemMetricsDisplay)
	if req.AppStatusBarEnabled != nil {
		settings.AppStatusBarEnabled = *req.AppStatusBarEnabled
	}
	if req.AppStatusBarOrder != nil {
		settings.AppStatusBarOrder = *req.AppStatusBarOrder
	}
	if err := applyTerminalFontPreferences(settings, req); err != nil {
		return err
	}
	return nil
}

// applyWorkspaceAndTaskListPreferences copies workspace, board, and task-list
// fields from the patch, validating the hidden-step map and list sort/group.
func applyWorkspaceAndTaskListPreferences(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.WorkspaceID != nil {
		settings.WorkspaceID = *req.WorkspaceID
	}
	if req.KanbanViewMode != nil {
		settings.KanbanViewMode = *req.KanbanViewMode
	}
	if err := applyStartupPage(settings, req.StartupPage); err != nil {
		return err
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
	if req.TasksListShowDetails != nil {
		settings.TasksListShowDetails = *req.TasksListShowDetails
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
	if req.KanbanHiddenStepIDs != nil {
		if err := validateKanbanHiddenStepIDs(*req.KanbanHiddenStepIDs); err != nil {
			return err
		}
		settings.KanbanHiddenStepIDs = *req.KanbanHiddenStepIDs
	}
	if req.WorkflowIDsWithAutoHideEmptySteps != nil {
		workflowIDs, err := normalizeWorkflowIDsWithAutoHideEmptySteps(
			*req.WorkflowIDsWithAutoHideEmptySteps,
		)
		if err != nil {
			return err
		}
		settings.WorkflowIDsWithAutoHideEmptySteps = workflowIDs
	}
	return nil
}

const (
	maxKanbanHiddenStepWorkflows      = 200
	maxKanbanHiddenStepIDsPerWorkflow = 200
	// maxKanbanHiddenStepIDsTotalBytes matches maxUserPreferenceBlobBytes, the
	// sibling cap for other free-form settings blobs. The count caps above
	// bound shape (how many entries), not size (how long each string is); an
	// attacker who stays under both count limits could otherwise still submit
	// a handful of multi-megabyte ids. This bounds total content regardless
	// of shape, and — unlike an HTTP-only body-size guard — it runs inside
	// validateKanbanHiddenStepIDs, which both the REST and WebSocket update
	// paths call, so it isn't bypassable by whichever transport skips a
	// transport-level guard.
	maxKanbanHiddenStepIDsTotalBytes     = maxUserPreferenceBlobBytes
	maxWorkflowIDsWithAutoHideEmptySteps = 200
)

func normalizeWorkflowIDsWithAutoHideEmptySteps(workflowIDs []string) ([]string, error) {
	unique := make(map[string]struct{}, len(workflowIDs))
	for _, workflowID := range workflowIDs {
		unique[workflowID] = struct{}{}
	}
	if len(unique) > maxWorkflowIDsWithAutoHideEmptySteps {
		return nil, fmt.Errorf(
			"workflow_ids_with_auto_hide_empty_steps: max %d workflow ids allowed",
			maxWorkflowIDsWithAutoHideEmptySteps,
		)
	}
	totalBytes := 0
	normalized := make([]string, 0, len(unique))
	for workflowID := range unique {
		totalBytes += len(workflowID)
		if totalBytes > maxUserPreferenceBlobBytes {
			return nil, fmt.Errorf(
				"workflow_ids_with_auto_hide_empty_steps: max %d bytes allowed",
				maxUserPreferenceBlobBytes,
			)
		}
		normalized = append(normalized, workflowID)
	}
	sort.Strings(normalized)
	return normalized, nil
}

// validateKanbanHiddenStepIDs bounds the per-workflow hidden-step-id map so a
// single settings write cannot grow the users.settings JSON blob unboundedly
// on the shared single-writer SQLite connection.
func validateKanbanHiddenStepIDs(hidden map[string][]string) error {
	if len(hidden) > maxKanbanHiddenStepWorkflows {
		return fmt.Errorf("kanban_hidden_step_ids: max %d workflows allowed", maxKanbanHiddenStepWorkflows)
	}
	totalBytes := 0
	for workflowID, ids := range hidden {
		if len(ids) > maxKanbanHiddenStepIDsPerWorkflow {
			return fmt.Errorf(
				"kanban_hidden_step_ids[%s]: max %d step ids allowed",
				workflowID,
				maxKanbanHiddenStepIDsPerWorkflow,
			)
		}
		totalBytes += len(workflowID)
		for _, id := range ids {
			totalBytes += len(id)
		}
		if totalBytes > maxKanbanHiddenStepIDsTotalBytes {
			return fmt.Errorf("kanban_hidden_step_ids: max %d bytes allowed", maxKanbanHiddenStepIDsTotalBytes)
		}
	}
	return nil
}

// applyStartupPage validates and applies the startup page enum.
func applyStartupPage(settings *models.UserSettings, value *string) error {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	switch v {
	case models.StartupPageTaskOverview, models.StartupPageLastTask:
		settings.StartupPage = v
		return nil
	default:
		return fmt.Errorf("startup_page must be %q or %q", models.StartupPageTaskOverview, models.StartupPageLastTask)
	}
}

// applyTaskActionPreferences copies the task-action preference group
// (archive confirmation, auto-start prevention, transcript display toggles,
// MCP default profile, release notification) from the patch.
func applyTaskActionPreferences(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *req.ReviewAutoMarkOnScroll
	}
	if req.ConfirmTaskArchive != nil {
		settings.ConfirmTaskArchive = *req.ConfirmTaskArchive
	}
	if req.PreventAutoStartAgentOnOpen != nil {
		settings.PreventAutoStartAgentOnOpen = *req.PreventAutoStartAgentOnOpen
	}
	if req.UnreadDivider != nil {
		settings.UnreadDivider = *req.UnreadDivider
	}
	if req.AgentGeneratedTaskTitles != nil {
		settings.AgentGeneratedTaskTitles = *req.AgentGeneratedTaskTitles
	}
	if err := applyMCPTaskAgentProfileDefault(settings, req.MCPTaskAgentProfileDefault); err != nil {
		return err
	}
	if req.ShowAnchoredPromptBar != nil {
		settings.ShowAnchoredPromptBar = *req.ShowAnchoredPromptBar
	}
	if req.ShowScrollToLastPrompt != nil {
		settings.ShowScrollToLastPrompt = *req.ShowScrollToLastPrompt
	}
	if req.ShowScrollToStart != nil {
		settings.ShowScrollToStart = *req.ShowScrollToStart
	}
	if req.ShowTranscriptAutoScrollControl != nil {
		settings.ShowTranscriptAutoScrollControl = *req.ShowTranscriptAutoScrollControl
	}
	if req.ShowTodoListPanel != nil {
		settings.ShowTodoListPanel = *req.ShowTodoListPanel
	}
	if req.ShowTodoListPanelOnlyWhenNotEmpty != nil {
		settings.ShowTodoListPanelOnlyWhenNotEmpty = *req.ShowTodoListPanelOnlyWhenNotEmpty
	}
	if req.ShowReleaseNotification != nil {
		settings.ShowReleaseNotification = *req.ShowReleaseNotification
	}
	return nil
}

// applyUtilityPreferences copies the utility-agent defaults and release-notes
// version from the patch.
func applyUtilityPreferences(settings *models.UserSettings, req *UpdateUserSettingsRequest) {
	if req.ReleaseNotesLastSeenVersion != nil {
		settings.ReleaseNotesLastSeenVersion = *req.ReleaseNotesLastSeenVersion
	}
	if req.DefaultUtilityAgentID != nil {
		settings.DefaultUtilityAgentID = strings.TrimSpace(*req.DefaultUtilityAgentID)
	}
	if req.DefaultUtilityModel != nil {
		settings.DefaultUtilityModel = strings.TrimSpace(*req.DefaultUtilityModel)
	}
	if req.DefaultUtilityAgentProfileID != nil {
		settings.DefaultUtilityAgentProfileID = strings.TrimSpace(*req.DefaultUtilityAgentProfileID)
	}
}

// applyKeyboardShortcuts validates and applies the keyboard shortcut map.
func applyKeyboardShortcuts(settings *models.UserSettings, value *map[string]interface{}) error {
	if value == nil {
		return nil
	}
	if err := validateKeyboardShortcuts(*value); err != nil {
		return err
	}
	settings.KeyboardShortcuts = *value
	return nil
}

// applySystemMetricsDisplay applies the topbar metrics display patch.
func applySystemMetricsDisplay(settings *models.UserSettings, value *SystemMetricsDisplaySettingsPatch) {
	if value == nil {
		return
	}
	if value.ShowInTopbar != nil {
		settings.SystemMetricsDisplay.ShowInTopbar = *value.ShowInTopbar
	}
	if value.Simplified != nil {
		settings.SystemMetricsDisplay.Simplified = *value.Simplified
	}
}

// applyTerminalFontPreferences applies the terminal font family and validates
// the font size range.
func applyTerminalFontPreferences(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.TerminalFontFamily != nil {
		settings.TerminalFontFamily = strings.TrimSpace(*req.TerminalFontFamily)
	}
	if req.TerminalFontSize == nil {
		return nil
	}
	v := *req.TerminalFontSize
	if v != 0 && (v < 8 || v > 24) {
		return errors.New("terminal_font_size must be 0 (default) or between 8 and 24")
	}
	settings.TerminalFontSize = v
	return nil
}

// applyMCPTaskAgentProfileDefault validates and applies the MCP task agent
// profile default enum.
func applyMCPTaskAgentProfileDefault(settings *models.UserSettings, value *string) error {
	if value == nil {
		return nil
	}
	switch *value {
	case models.MCPTaskAgentProfileDefaultCurrentTask, models.MCPTaskAgentProfileDefaultWorkspaceDefault:
		settings.MCPTaskAgentProfileDefault = *value
		return nil
	default:
		return fmt.Errorf("mcp_task_agent_profile_default must be %q or %q",
			models.MCPTaskAgentProfileDefaultCurrentTask,
			models.MCPTaskAgentProfileDefaultWorkspaceDefault,
		)
	}
}

// applyTasksListPreferences validates and applies the task list sort and
// group enums, defaulting empty values.
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

// applyTerminalLinkBehavior validates and applies the terminal link behavior
// enum.
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

// applyChangesPanelLayout validates and applies the changes panel layout
// enum (flat or tree).
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

// applyLastSeenDisplay validates and applies the last-seen display enum
// (absolute or relative).
func applyLastSeenDisplay(settings *models.UserSettings, value *string) error {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v != models.LastSeenDisplayAbsolute && v != models.LastSeenDisplayRelative {
		return errors.New("last_seen_display must be 'absolute' or 'relative'")
	}
	settings.LastSeenDisplay = v
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
		if err := validateLSPAutoInstallLanguages(*req.LspAutoInstallLanguages); err != nil {
			return fmt.Errorf("lsp_auto_install_languages: %w", err)
		}
		settings.LspAutoInstallLanguages = *req.LspAutoInstallLanguages
	}
	if req.LspServerConfigs != nil {
		settings.LspServerConfigs = *req.LspServerConfigs
	}
	if req.LspStatusLocation != nil {
		switch *req.LspStatusLocation {
		case models.LspStatusLocationToolbar, models.LspStatusLocationStatusBar:
			settings.LspStatusLocation = *req.LspStatusLocation
		default:
			return fmt.Errorf(
				"lsp_status_location must be %q or %q",
				models.LspStatusLocationToolbar,
				models.LspStatusLocationStatusBar,
			)
		}
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
	seen := make(map[string]struct{}, len(layouts))
	hasDefault := false
	for i := range layouts {
		if strings.TrimSpace(layouts[i].ID) == "" {
			return errors.New("saved_layouts: layout id must not be empty")
		}
		if strings.TrimSpace(layouts[i].Name) == "" {
			return errors.New("saved_layouts: layout name must not be empty")
		}
		if _, dup := seen[layouts[i].ID]; dup {
			return fmt.Errorf("saved_layouts: duplicate layout id %q", layouts[i].ID)
		}
		seen[layouts[i].ID] = struct{}{}
		if layouts[i].IsDefault {
			if hasDefault {
				return errors.New("saved_layouts: at most one default layout allowed")
			}
			hasDefault = true
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
	if len(views) == 0 && req.SidebarActiveViewID == nil {
		views = store.DefaultSidebarViews()
		settings.SidebarActiveViewID = store.DefaultSidebarViewID
	}
	settings.SidebarViews = views
	return nil
}

// applySidebarViewState validates and applies the active sidebar view id and
// the sidebar draft.
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

// sidebarViewIDExists reports whether a view with the given id is saved.
func sidebarViewIDExists(views []models.SidebarView, id string) bool {
	for _, view := range views {
		if view.ID == id {
			return true
		}
	}
	return false
}

const maxUserPreferenceBlobBytes = 64 * 1024

// applyUserPreferenceBlobs applies each free-form preference blob (sidebar
// task prefs, Jira/GitHub/GitLab/Azure saved views and presets), validating
// size and shape via applyUserPreferenceBlob.
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
	if err := applyUserPreferenceBlob("azure_devops_browse_preferences", req.AzureDevOpsBrowsePreferences, &settings.AzureDevOpsBrowsePreferences); err != nil {
		return err
	}
	return nil
}

// applyUserPreferenceBlob applies one PATCH-distinguished blob: omitted
// leaves the target untouched, explicit null clears it, and a value is
// validated before being stored.
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

// validateUserPreferenceBlob enforces the blob size cap and JSON shape
// (object, array, or null).
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

// publishUserSettingsEvent broadcasts the full settings snapshot on the
// UserSettingsUpdated event bus topic so connected clients stay in sync.
func (s *Service) publishUserSettingsEvent(ctx context.Context, settings *models.UserSettings) {
	if s.eventBus == nil || settings == nil {
		return
	}
	data := map[string]interface{}{
		"user_id":                                  settings.UserID,
		"workspace_id":                             settings.WorkspaceID,
		"kanban_view_mode":                         settings.KanbanViewMode,
		"startup_page":                             models.NormalizeStartupPage(settings.StartupPage),
		"workflow_filter_id":                       settings.WorkflowFilterID,
		"repository_ids":                           settings.RepositoryIDs,
		"tasks_list_sort":                          settings.TasksListSort,
		"tasks_list_group":                         settings.TasksListGroup,
		"tasks_list_show_details":                  settings.TasksListShowDetails,
		"initial_setup_complete":                   settings.InitialSetupComplete,
		"preferred_shell":                          settings.PreferredShell,
		"default_editor_id":                        settings.DefaultEditorID,
		"enable_preview_on_click":                  settings.EnablePreviewOnClick,
		"chat_submit_key":                          settings.ChatSubmitKey,
		"review_auto_mark_on_scroll":               settings.ReviewAutoMarkOnScroll,
		"confirm_task_archive":                     settings.ConfirmTaskArchive,
		"prevent_auto_start_agent_on_open":         settings.PreventAutoStartAgentOnOpen,
		"unread_divider":                           settings.UnreadDivider,
		"agent_generated_task_titles":              settings.AgentGeneratedTaskTitles,
		"mcp_task_agent_profile_default":           models.NormalizeMCPTaskAgentProfileDefault(settings.MCPTaskAgentProfileDefault),
		"show_anchored_prompt_bar":                 settings.ShowAnchoredPromptBar,
		"show_scroll_to_last_prompt":               settings.ShowScrollToLastPrompt,
		"show_scroll_to_start":                     settings.ShowScrollToStart,
		"show_transcript_auto_scroll_control":      settings.ShowTranscriptAutoScrollControl,
		"show_todo_list_panel":                     settings.ShowTodoListPanel,
		"show_todo_list_panel_only_when_not_empty": settings.ShowTodoListPanelOnlyWhenNotEmpty,
		"show_release_notification":                settings.ShowReleaseNotification,
		"release_notes_last_seen_version":          settings.ReleaseNotesLastSeenVersion,
		"lsp_auto_start_languages":                 settings.LspAutoStartLanguages,
		"lsp_auto_install_languages":               settings.LspAutoInstallLanguages,
		"lsp_server_configs":                       settings.LspServerConfigs,
		"lsp_status_location":                      models.NormalizeLspStatusLocation(settings.LspStatusLocation),
		"saved_layouts":                            settings.SavedLayouts,
		"sidebar_views":                            settings.SidebarViews,
		"sidebar_active_view_id":                   settings.SidebarActiveViewID,
		"sidebar_draft":                            settings.SidebarDraft,
		"sidebar_task_prefs":                       settings.SidebarTaskPrefs,
		"task_create_last_used":                    settings.TaskCreateLastUsed,
		"jira_saved_views":                         settings.JiraSavedViews,
		"jira_task_presets":                        settings.JiraTaskPresets,
		"github_saved_presets":                     settings.GitHubSavedPresets,
		"github_default_query_presets":             settings.GitHubDefaultQueryPresets,
		"gitlab_saved_presets":                     settings.GitLabSavedPresets,
		"azure_devops_browse_preferences":          settings.AzureDevOpsBrowsePreferences,
		"default_utility_agent_id":                 settings.DefaultUtilityAgentID,
		"default_utility_model":                    settings.DefaultUtilityModel,
		"default_utility_agent_profile_id":         settings.DefaultUtilityAgentProfileID,
		"keyboard_shortcuts":                       settings.KeyboardShortcuts,
		"terminal_link_behavior":                   settings.TerminalLinkBehavior,
		"terminal_font_family":                     settings.TerminalFontFamily,
		"terminal_font_size":                       settings.TerminalFontSize,
		"changes_panel_layout":                     settings.ChangesPanelLayout,
		"last_seen_display":                        models.NormalizeLastSeenDisplay(settings.LastSeenDisplay),
		"system_metrics_display":                   settings.SystemMetricsDisplay,
		"app_status_bar_enabled":                   settings.AppStatusBarEnabled,
		"app_status_bar_order":                     settings.AppStatusBarOrder,
		"kanban_hidden_step_ids":                   settings.KanbanHiddenStepIDs,
		"workflow_ids_with_auto_hide_empty_steps":  settings.WorkflowIDsWithAutoHideEmptySteps,
		"revision":                                 settings.Revision,
		"updated_at":                               settings.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, events.UserSettingsUpdated, bus.NewEvent(events.UserSettingsUpdated, "user-service", data)); err != nil {
		s.logger.Error("failed to publish user settings event", zap.Error(err))
	}
}

// validateKeyboardShortcuts checks the shortcut map shape: each entry must be
// an object with a non-empty key and boolean-only modifiers.
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

// validateLSPLanguages rejects languages the LSP installer does not support.
func validateLSPLanguages(langs []string) error {
	supported := installer.SupportedLanguages()
	for _, lang := range langs {
		if _, ok := supported[lang]; !ok {
			return fmt.Errorf("unsupported language: %s", lang)
		}
	}
	return nil
}

// validateLSPAutoInstallLanguages rejects languages that are unsupported or
// cannot be auto-installed.
func validateLSPAutoInstallLanguages(langs []string) error {
	if err := validateLSPLanguages(langs); err != nil {
		return err
	}
	for _, lang := range langs {
		if !installer.SupportsAutoInstall(lang) {
			return fmt.Errorf("language %s does not support auto-install", lang)
		}
	}
	return nil
}

// ClearDefaultEditorID clears the saved default editor when it matches the
// given editor id (used when an editor is uninstalled), persisting and
// publishing the change.
func (s *Service) ClearDefaultEditorID(ctx context.Context, editorID string) error {
	if editorID == "" {
		return nil
	}
	_, err := s.updateUserSettingsCAS(ctx, func(settings *models.UserSettings) (bool, error) {
		if settings.DefaultEditorID != editorID {
			return false, nil
		}
		settings.DefaultEditorID = ""
		return true, nil
	}, nil)
	return err
}
