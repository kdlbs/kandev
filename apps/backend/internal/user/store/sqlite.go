package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/user/models"
)

const (
	DefaultUserID    = "default-user"
	DefaultUserEmail = "default@kandev.local"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

func newSQLiteRepository(writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		settings TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	return r.ensureDefaultUser()
}

func (r *sqliteRepository) ensureDefaultUser() error {
	ctx := context.Background()
	var count int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind("SELECT COUNT(1) FROM users WHERE id = ?"), DefaultUserID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := time.Now().UTC()
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO users (id, email, settings, created_at, updated_at)
			VALUES (?, ?, '{}', ?, ?)
		`), DefaultUserID, DefaultUserEmail, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) GetUser(ctx context.Context, id string) (*models.User, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, email, created_at, updated_at
		FROM users WHERE id = ?
	`), id)
	return scanUser(row)
}

func (r *sqliteRepository) GetDefaultUser(ctx context.Context) (*models.User, error) {
	return r.GetUser(ctx, DefaultUserID)
}

func (r *sqliteRepository) GetUserSettings(ctx context.Context, userID string) (*models.UserSettings, error) {
	return r.getUserSettings(ctx, r.ro, userID)
}

func (r *sqliteRepository) getUserSettings(ctx context.Context, conn *sqlx.DB, userID string) (*models.UserSettings, error) {
	row := conn.QueryRowContext(ctx, conn.Rebind(`
		SELECT settings, updated_at
		FROM users WHERE id = ?
	`), userID)
	settings, err := scanUserSettings(row, userID)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *sqliteRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	ctx context.Context,
	settings *models.UserSettings,
	patch *models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	settings.UpdatedAt = time.Now().UTC()
	if settings.CreatedAt.IsZero() {
		settings.CreatedAt = settings.UpdatedAt
	}
	settingsPayload, err := marshalUserSettingsPayload(settings)
	if err != nil {
		return nil, err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.upsertUserSettingsPreservingTaskCreateLastUsedPostgres(ctx, settings, settingsPayload, patch)
	}
	settingsExpr := `json_set(
		json(?),
		'$.task_create_last_used',
		json(COALESCE(json_extract(
			CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}' ELSE settings END,
			'$.task_create_last_used'
		), '{}'))
	)`
	args := []any{string(settingsPayload)}
	if patch != nil {
		patchArgs := makeTaskCreateLastUsedJSONSetArgs(*patch)
		if len(patchArgs) > 0 {
			placeholders := strings.TrimSuffix(strings.Repeat("?, ?, ", len(patchArgs)/2), ", ")
			settingsExpr = fmt.Sprintf("json_set(%s, %s)", settingsExpr, placeholders)
			args = append(args, patchArgs...)
		}
	}
	query := fmt.Sprintf(`
		UPDATE users
		SET settings = %s, updated_at = ?
		WHERE id = ?
	`, settingsExpr)
	args = append(args, settings.UpdatedAt, settings.UserID)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	if err := checkUserSettingsRowsAffected(result, settings.UserID); err != nil {
		return nil, err
	}
	return r.getUserSettings(ctx, r.db, settings.UserID)
}

func (r *sqliteRepository) upsertUserSettingsPreservingTaskCreateLastUsedPostgres(
	ctx context.Context,
	settings *models.UserSettings,
	settingsPayload []byte,
	patch *models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	query, patchArgs := buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate(patch)
	args := append([]any{string(settingsPayload)}, patchArgs...)
	args = append(args, settings.UpdatedAt, settings.UserID)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	if err := checkUserSettingsRowsAffected(result, settings.UserID); err != nil {
		return nil, err
	}
	return r.getUserSettings(ctx, r.db, settings.UserID)
}

func (r *sqliteRepository) UpdateTaskCreateLastUsed(ctx context.Context, userID string, patch models.TaskCreateLastUsed) (*models.UserSettings, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.updateTaskCreateLastUsedPostgres(ctx, userID, patch)
	}
	patchArgs := makeTaskCreateLastUsedJSONSetArgs(patch)
	if len(patchArgs) == 0 {
		return r.getUserSettings(ctx, r.db, userID)
	}
	base := "CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}' ELSE settings END"
	settingsExpr := fmt.Sprintf(
		"json_set(%s, '$.task_create_last_used', json(COALESCE(json_extract(%s, '$.task_create_last_used'), '{}')))",
		base,
		base,
	)
	placeholders := strings.TrimSuffix(strings.Repeat("?, ?, ", len(patchArgs)/2), ", ")
	settingsExpr = fmt.Sprintf("json_set(%s, %s)", settingsExpr, placeholders)
	query := `
		UPDATE users
		SET settings = %s, updated_at = ?
		WHERE id = ?
	`
	now := time.Now().UTC()
	args := append([]any{}, patchArgs...)
	args = append(args, now, userID)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(fmt.Sprintf(query, settingsExpr)), args...)
	if err != nil {
		return nil, err
	}
	if err := checkUserSettingsRowsAffected(result, userID); err != nil {
		return nil, err
	}
	return r.getUserSettings(ctx, r.db, userID)
}

func (r *sqliteRepository) updateTaskCreateLastUsedPostgres(ctx context.Context, userID string, patch models.TaskCreateLastUsed) (*models.UserSettings, error) {
	query, args := buildPostgresTaskCreateLastUsedUpdate(patch)
	if len(args) == 0 {
		return r.getUserSettings(ctx, r.db, userID)
	}
	now := time.Now().UTC()
	args = append(args, now, userID)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	if err := checkUserSettingsRowsAffected(result, userID); err != nil {
		return nil, err
	}
	return r.getUserSettings(ctx, r.db, userID)
}

func makeTaskCreateLastUsedJSONSetArgs(patch models.TaskCreateLastUsed) []any {
	args := []any{}
	if patch.RepositoryID != "" {
		args = append(args, "$.task_create_last_used.repository_id", patch.RepositoryID)
	}
	if patch.Branch != "" || patch.RepositoryID != "" {
		args = append(args, "$.task_create_last_used.branch", patch.Branch)
	}
	if patch.AgentProfileID != "" {
		args = append(args, "$.task_create_last_used.agent_profile_id", patch.AgentProfileID)
	}
	if patch.ExecutorProfileID != "" {
		args = append(args, "$.task_create_last_used.executor_profile_id", patch.ExecutorProfileID)
	}
	return args
}

func buildPostgresTaskCreateLastUsedUpdate(patch models.TaskCreateLastUsed) (string, []any) {
	base := "(CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}'::jsonb ELSE settings::jsonb END)"
	expr := fmt.Sprintf("jsonb_set(%s, '{task_create_last_used}', COALESCE(%s->'task_create_last_used', '{}'::jsonb), true)", base, base)
	args := []any{}
	expr, args = applyPostgresTaskCreateLastUsedPatch(expr, patch, args)
	query := fmt.Sprintf(`
		UPDATE users
		SET settings = %s::text, updated_at = ?
		WHERE id = ?
	`, expr)
	return query, args
}

func buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate(patch *models.TaskCreateLastUsed) (string, []any) {
	base := "(CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}'::jsonb ELSE settings::jsonb END)"
	expr := fmt.Sprintf("jsonb_set(?::jsonb, '{task_create_last_used}', COALESCE(%s->'task_create_last_used', '{}'::jsonb), true)", base)
	args := []any{}
	if patch != nil {
		expr, args = applyPostgresTaskCreateLastUsedPatch(expr, *patch, args)
	}
	query := fmt.Sprintf(`
		UPDATE users
		SET settings = %s::text, updated_at = ?
		WHERE id = ?
	`, expr)
	return query, args
}

func applyPostgresTaskCreateLastUsedPatch(expr string, patch models.TaskCreateLastUsed, args []any) (string, []any) {
	if patch.RepositoryID != "" {
		expr = fmt.Sprintf("jsonb_set(%s, '{task_create_last_used,repository_id}', to_jsonb(?::text), true)", expr)
		args = append(args, patch.RepositoryID)
	}
	if patch.Branch != "" || patch.RepositoryID != "" {
		expr = fmt.Sprintf("jsonb_set(%s, '{task_create_last_used,branch}', to_jsonb(?::text), true)", expr)
		args = append(args, patch.Branch)
	}
	if patch.AgentProfileID != "" {
		expr = fmt.Sprintf("jsonb_set(%s, '{task_create_last_used,agent_profile_id}', to_jsonb(?::text), true)", expr)
		args = append(args, patch.AgentProfileID)
	}
	if patch.ExecutorProfileID != "" {
		expr = fmt.Sprintf("jsonb_set(%s, '{task_create_last_used,executor_profile_id}', to_jsonb(?::text), true)", expr)
		args = append(args, patch.ExecutorProfileID)
	}
	return expr, args
}

func marshalUserSettingsPayload(settings *models.UserSettings) ([]byte, error) {
	lspAutoStart := settings.LspAutoStartLanguages
	if lspAutoStart == nil {
		lspAutoStart = []string{}
	}
	lspAutoInstall := settings.LspAutoInstallLanguages
	if lspAutoInstall == nil {
		lspAutoInstall = []string{}
	}
	lspServerConfigs := settings.LspServerConfigs
	if lspServerConfigs == nil {
		lspServerConfigs = map[string]map[string]interface{}{}
	}
	savedLayouts := settings.SavedLayouts
	if savedLayouts == nil {
		savedLayouts = []models.SavedLayout{}
	}
	sidebarViews := settings.SidebarViews
	if sidebarViews == nil {
		sidebarViews = []models.SidebarView{}
	}
	sidebarTaskPrefs := normalizeSidebarTaskPrefs(settings.SidebarTaskPrefs)
	keyboardShortcuts := settings.KeyboardShortcuts
	if keyboardShortcuts == nil {
		keyboardShortcuts = map[string]interface{}{}
	}
	return json.Marshal(map[string]interface{}{
		"workspace_id":                    settings.WorkspaceID,
		"kanban_view_mode":                settings.KanbanViewMode,
		"workflow_filter_id":              settings.WorkflowFilterID,
		"repository_ids":                  settings.RepositoryIDs,
		"tasks_list_sort":                 models.NormalizeTasksListSort(settings.TasksListSort),
		"tasks_list_group":                models.NormalizeTasksListGroup(settings.TasksListGroup),
		"initial_setup_complete":          settings.InitialSetupComplete,
		"preferred_shell":                 settings.PreferredShell,
		"default_editor_id":               settings.DefaultEditorID,
		"enable_preview_on_click":         settings.EnablePreviewOnClick,
		"chat_submit_key":                 settings.ChatSubmitKey,
		"review_auto_mark_on_scroll":      settings.ReviewAutoMarkOnScroll,
		"confirm_task_archive":            settings.ConfirmTaskArchive,
		"mcp_task_agent_profile_default":  models.NormalizeMCPTaskAgentProfileDefault(settings.MCPTaskAgentProfileDefault),
		"show_release_notification":       settings.ShowReleaseNotification,
		"release_notes_last_seen_version": settings.ReleaseNotesLastSeenVersion,
		"lsp_auto_start_languages":        lspAutoStart,
		"lsp_auto_install_languages":      lspAutoInstall,
		"lsp_server_configs":              lspServerConfigs,
		"saved_layouts":                   savedLayouts,
		"sidebar_views":                   sidebarViews,
		"sidebar_active_view_id":          settings.SidebarActiveViewID,
		"sidebar_draft":                   settings.SidebarDraft,
		"sidebar_task_prefs":              sidebarTaskPrefs,
		"task_create_last_used":           settings.TaskCreateLastUsed,
		"jira_saved_views":                settings.JiraSavedViews,
		"jira_task_presets":               settings.JiraTaskPresets,
		"github_saved_presets":            settings.GitHubSavedPresets,
		"github_default_query_presets":    settings.GitHubDefaultQueryPresets,
		"gitlab_saved_presets":            settings.GitLabSavedPresets,
		"default_utility_agent_id":        settings.DefaultUtilityAgentID,
		"default_utility_model":           settings.DefaultUtilityModel,
		"utility_agent_profile_id":        settings.UtilityAgentProfileID,
		"keyboard_shortcuts":              keyboardShortcuts,
		"terminal_link_behavior":          settings.TerminalLinkBehavior,
		"terminal_font_family":            settings.TerminalFontFamily,
		"terminal_font_size":              settings.TerminalFontSize,
		"changes_panel_layout":            settings.ChangesPanelLayout,
		"system_metrics_display":          settings.SystemMetricsDisplay,
		"voice_mode":                      settings.VoiceMode,
	})
}

func checkUserSettingsRowsAffected(result sqlResult, userID string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", userID)
	}
	return nil
}

type sqlResult interface {
	RowsAffected() (int64, error)
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (*models.User, error) {
	user := &models.User{}
	if err := scanner.Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, err
	}
	return user, nil
}

// defaultVoiceModeSettings returns the baseline VoiceMode configuration for
// users with no saved preferences. Mirrored on the frontend; keep in sync.
func defaultVoiceModeSettings() models.VoiceModeSettings {
	return models.VoiceModeSettings{
		Enabled:         true,
		Engine:          "auto",
		Language:        "auto",
		Mode:            "toggle",
		AutoSend:        false,
		WhisperWebModel: "base",
	}
}

// storedVoiceMode is the on-disk JSON shape — uses *bool for `enabled` so we
// can distinguish "absent" (older rows written before the toggle existed —
// must default to true) from "explicitly false" (user disabled the feature).
type storedVoiceMode struct {
	Enabled         *bool  `json:"enabled"`
	Engine          string `json:"engine"`
	Language        string `json:"language"`
	Mode            string `json:"mode"`
	AutoSend        bool   `json:"auto_send"`
	WhisperWebModel string `json:"whisper_web_model"`
}

// mergeVoiceModeDefaults fills in zero/missing fields on a stored VoiceMode
// payload so older user rows (written before VoiceMode existed) still produce
// usable settings instead of empty strings the frontend would reject.
func mergeVoiceModeDefaults(stored *storedVoiceMode) models.VoiceModeSettings {
	out := defaultVoiceModeSettings()
	if stored == nil {
		return out
	}
	if stored.Enabled != nil {
		out.Enabled = *stored.Enabled
	}
	if stored.Engine != "" {
		out.Engine = stored.Engine
	}
	if stored.Language != "" {
		out.Language = stored.Language
	}
	if stored.Mode != "" {
		out.Mode = stored.Mode
	}
	if stored.WhisperWebModel != "" {
		out.WhisperWebModel = stored.WhisperWebModel
	}
	out.AutoSend = stored.AutoSend
	return out
}

func scanUserSettings(scanner interface{ Scan(dest ...any) error }, userID string) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	var settingsRaw string
	if err := scanner.Scan(&settingsRaw, &settings.UpdatedAt); err != nil {
		return nil, err
	}
	settings.UserID = userID
	if settingsRaw == "" || settingsRaw == "{}" {
		settings.RepositoryIDs = []string{}
		settings.TasksListSort = models.TasksListSortDefault
		settings.TasksListGroup = models.TasksListGroupDefault
		settings.ShowReleaseNotification = true
		settings.ReviewAutoMarkOnScroll = true
		settings.ConfirmTaskArchive = true
		settings.MCPTaskAgentProfileDefault = models.MCPTaskAgentProfileDefaultCurrentTask
		settings.ChatSubmitKey = "cmd_enter"
		settings.KeyboardShortcuts = map[string]interface{}{}
		settings.TerminalLinkBehavior = "new_tab"
		settings.ChangesPanelLayout = "tree"
		settings.SidebarViews = []models.SidebarView{}
		settings.SidebarTaskPrefs = normalizeSidebarTaskPrefs(models.SidebarTaskPrefs{})
		settings.VoiceMode = defaultVoiceModeSettings()
		return settings, nil
	}
	var payload struct {
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
		ReviewAutoMarkOnScroll      *bool                               `json:"review_auto_mark_on_scroll"`
		ConfirmTaskArchive          *bool                               `json:"confirm_task_archive"`
		MCPTaskAgentProfileDefault  string                              `json:"mcp_task_agent_profile_default"`
		ShowReleaseNotification     *bool                               `json:"show_release_notification"`
		ReleaseNotesLastSeenVersion string                              `json:"release_notes_last_seen_version"`
		LspAutoStartLanguages       []string                            `json:"lsp_auto_start_languages"`
		LspAutoInstallLanguages     []string                            `json:"lsp_auto_install_languages"`
		LspServerConfigs            map[string]map[string]interface{}   `json:"lsp_server_configs"`
		SavedLayouts                []models.SavedLayout                `json:"saved_layouts"`
		SidebarViews                []models.SidebarView                `json:"sidebar_views"`
		SidebarActiveViewID         string                              `json:"sidebar_active_view_id"`
		SidebarDraft                *models.SidebarViewDraft            `json:"sidebar_draft"`
		SidebarTaskPrefs            models.SidebarTaskPrefs             `json:"sidebar_task_prefs"`
		TaskCreateLastUsed          models.TaskCreateLastUsed           `json:"task_create_last_used"`
		JiraSavedViews              json.RawMessage                     `json:"jira_saved_views"`
		JiraTaskPresets             json.RawMessage                     `json:"jira_task_presets"`
		GitHubSavedPresets          json.RawMessage                     `json:"github_saved_presets"`
		GitHubDefaultQueryPresets   json.RawMessage                     `json:"github_default_query_presets"`
		GitLabSavedPresets          json.RawMessage                     `json:"gitlab_saved_presets"`
		DefaultUtilityAgentID       string                              `json:"default_utility_agent_id"`
		DefaultUtilityModel         string                              `json:"default_utility_model"`
		UtilityAgentProfileID       string                              `json:"utility_agent_profile_id"`
		KeyboardShortcuts           map[string]interface{}              `json:"keyboard_shortcuts"`
		TerminalLinkBehavior        string                              `json:"terminal_link_behavior"`
		TerminalFontFamily          string                              `json:"terminal_font_family"`
		TerminalFontSize            int                                 `json:"terminal_font_size"`
		ChangesPanelLayout          string                              `json:"changes_panel_layout"`
		SystemMetricsDisplay        models.SystemMetricsDisplaySettings `json:"system_metrics_display"`
		VoiceMode                   *storedVoiceMode                    `json:"voice_mode"`
	}
	if err := json.Unmarshal([]byte(settingsRaw), &payload); err != nil {
		return nil, err
	}
	settings.WorkspaceID = payload.WorkspaceID
	settings.KanbanViewMode = payload.KanbanViewMode
	settings.WorkflowFilterID = payload.WorkflowFilterID
	settings.RepositoryIDs = payload.RepositoryIDs
	settings.TasksListSort = models.NormalizeTasksListSort(payload.TasksListSort)
	settings.TasksListGroup = models.NormalizeTasksListGroup(payload.TasksListGroup)
	settings.InitialSetupComplete = payload.InitialSetupComplete
	settings.PreferredShell = payload.PreferredShell
	settings.DefaultEditorID = payload.DefaultEditorID
	settings.EnablePreviewOnClick = payload.EnablePreviewOnClick
	settings.ChatSubmitKey = payload.ChatSubmitKey
	// Default to "cmd_enter" if empty
	if settings.ChatSubmitKey == "" {
		settings.ChatSubmitKey = "cmd_enter"
	}
	// Default to true when not set
	if payload.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *payload.ReviewAutoMarkOnScroll
	} else {
		settings.ReviewAutoMarkOnScroll = true
	}
	if payload.ConfirmTaskArchive != nil {
		settings.ConfirmTaskArchive = *payload.ConfirmTaskArchive
	} else {
		settings.ConfirmTaskArchive = true
	}
	settings.MCPTaskAgentProfileDefault = models.NormalizeMCPTaskAgentProfileDefault(payload.MCPTaskAgentProfileDefault)
	if payload.ShowReleaseNotification != nil {
		settings.ShowReleaseNotification = *payload.ShowReleaseNotification
	} else {
		settings.ShowReleaseNotification = true
	}
	settings.ReleaseNotesLastSeenVersion = payload.ReleaseNotesLastSeenVersion
	settings.LspAutoStartLanguages = payload.LspAutoStartLanguages
	if settings.LspAutoStartLanguages == nil {
		settings.LspAutoStartLanguages = []string{}
	}
	settings.LspAutoInstallLanguages = payload.LspAutoInstallLanguages
	if settings.LspAutoInstallLanguages == nil {
		settings.LspAutoInstallLanguages = []string{}
	}
	settings.LspServerConfigs = payload.LspServerConfigs
	settings.SavedLayouts = payload.SavedLayouts
	if settings.SavedLayouts == nil {
		settings.SavedLayouts = []models.SavedLayout{}
	}
	settings.SidebarViews = payload.SidebarViews
	if settings.SidebarViews == nil {
		settings.SidebarViews = []models.SidebarView{}
	}
	settings.SidebarActiveViewID = payload.SidebarActiveViewID
	settings.SidebarDraft = payload.SidebarDraft
	settings.SidebarTaskPrefs = normalizeSidebarTaskPrefs(payload.SidebarTaskPrefs)
	settings.TaskCreateLastUsed = payload.TaskCreateLastUsed
	settings.JiraSavedViews = payload.JiraSavedViews
	settings.JiraTaskPresets = payload.JiraTaskPresets
	settings.GitHubSavedPresets = payload.GitHubSavedPresets
	settings.GitHubDefaultQueryPresets = payload.GitHubDefaultQueryPresets
	settings.GitLabSavedPresets = payload.GitLabSavedPresets
	settings.DefaultUtilityAgentID = payload.DefaultUtilityAgentID
	settings.DefaultUtilityModel = payload.DefaultUtilityModel
	settings.UtilityAgentProfileID = payload.UtilityAgentProfileID
	settings.KeyboardShortcuts = payload.KeyboardShortcuts
	if settings.KeyboardShortcuts == nil {
		settings.KeyboardShortcuts = map[string]interface{}{}
	}
	if payload.TerminalLinkBehavior != "" {
		settings.TerminalLinkBehavior = payload.TerminalLinkBehavior
	} else {
		settings.TerminalLinkBehavior = "new_tab"
	}
	settings.TerminalFontFamily = payload.TerminalFontFamily
	settings.TerminalFontSize = payload.TerminalFontSize
	settings.VoiceMode = mergeVoiceModeDefaults(payload.VoiceMode)
	settings.SystemMetricsDisplay = payload.SystemMetricsDisplay
	if payload.ChangesPanelLayout == "flat" {
		settings.ChangesPanelLayout = "flat"
	} else {
		settings.ChangesPanelLayout = "tree"
	}
	return settings, nil
}

func normalizeSidebarTaskPrefs(prefs models.SidebarTaskPrefs) models.SidebarTaskPrefs {
	if prefs.PinnedTaskIDs == nil {
		prefs.PinnedTaskIDs = []string{}
	}
	if prefs.OrderedTaskIDs == nil {
		prefs.OrderedTaskIDs = []string{}
	}
	if prefs.SubtaskOrderByParentID == nil {
		prefs.SubtaskOrderByParentID = map[string][]string{}
	}
	return prefs
}
