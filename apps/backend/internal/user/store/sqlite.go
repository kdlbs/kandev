package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/user/models"
)

const (
	DefaultUserID        = "default-user"
	DefaultUserEmail     = "default@kandev.local"
	DefaultSidebarViewID = "view-all-tasks"

	defaultChangesPanelLayout = "tree"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

// newSQLiteRepositoryWithDB builds a repository over caller-owned database
// handles (the writer/reader pair is not closed by the repository).
func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

// newSQLiteRepository builds a repository, initializing the schema and the
// default user; when ownsDB is set the writer connection is closed on schema
// failure.
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

// initSchema creates the users table, applies idempotent migrations, and
// seeds the default user.
func (r *sqliteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'admin',
		org_id TEXT NOT NULL DEFAULT '',
		is_operator INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		settings TEXT NOT NULL DEFAULT '{}',
		settings_revision BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	r.runMigrations()

	return r.ensureDefaultUser()
}

// runMigrations evolves existing databases. CREATE TABLE IF NOT EXISTS is a
// no-op on a table that already exists, so every added column must also appear
// here as an idempotent ADD COLUMN (see apps/backend/CLAUDE.md, ADR 0027).
func (r *sqliteRepository) runMigrations() {
	m := db.NewMigrateLogger(r.db, nil)
	m.Apply("users.display_name", "ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''")
	// Default 'admin': the pre-auth singleton default-user becomes the admin
	// when authentication is enabled. Explicit CreateUser calls always set role.
	m.Apply("users.role", "ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'")
	m.Apply("users.status", "ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active'")
	m.Apply("users.settings_revision", "ALTER TABLE users ADD COLUMN settings_revision BIGINT NOT NULL DEFAULT 0")
	// Organizations. Empty means "not yet assigned"; the tenancy migration
	// puts every existing account into the default org on first boot with the
	// feature on. Email stays unique instance-wide rather than per-org, so an
	// address identifies one person and the login form needs no org picker.
	m.Apply("users.org_id", "ALTER TABLE users ADD COLUMN org_id TEXT NOT NULL DEFAULT ''")
	m.Apply("users.org_idx", "CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id)")
	// The instance operator tier. Granted to the first admin by the tenancy
	// migration so an upgraded instance is never left with nobody able to
	// manage organizations.
	m.Apply("users.is_operator", "ALTER TABLE users ADD COLUMN is_operator INTEGER NOT NULL DEFAULT 0")
	// Safe pre-auth: the table only ever held the single default-user row.
	m.Apply("users.email_unique", "CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email)")
}

// ensureDefaultUser inserts the pre-auth default user row when it does not
// exist yet.
func (r *sqliteRepository) ensureDefaultUser() error {
	ctx := context.Background()
	var count int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind("SELECT COUNT(1) FROM users WHERE id = ?"), DefaultUserID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := time.Now().UTC()
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO users (id, email, display_name, role, status, settings, created_at, updated_at)
			VALUES (?, ?, '', ?, ?, '{}', ?, ?)
		`), DefaultUserID, DefaultUserEmail, models.RoleAdmin, models.StatusActive, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// Close releases the writer connection when the repository owns it.
func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

const userColumns = "id, email, display_name, role, status, org_id, is_operator, created_at, updated_at"

// GetUser returns the user row for the given id.
func (r *sqliteRepository) GetUser(ctx context.Context, id string) (*models.User, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+userColumns+`
		FROM users WHERE id = ?
	`), id)
	return scanUser(row)
}

// GetDefaultUser returns the pre-auth default user row.
func (r *sqliteRepository) GetDefaultUser(ctx context.Context) (*models.User, error) {
	return r.GetUser(ctx, DefaultUserID)
}

// GetUserByEmail returns the user row matching the email.
func (r *sqliteRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+userColumns+`
		FROM users WHERE email = ?
	`), email)
	return scanUser(row)
}

// ListUsers returns all users ordered by creation time.
func (r *sqliteRepository) ListUsers(ctx context.Context) ([]*models.User, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT `+userColumns+`
		FROM users ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	users := []*models.User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// DeleteUser removes the user row with the given id.
func (r *sqliteRepository) DeleteUser(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM users WHERE id = ?`), id)
	return err
}

// CreateUser inserts a new user row with an empty settings blob, stamping
// creation timestamps.
func (r *sqliteRepository) CreateUser(ctx context.Context, user *models.User) error {
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO users (id, email, display_name, role, status, org_id, is_operator, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '{}', ?, ?)
	`), user.ID, user.Email, user.DisplayName, user.Role, user.Status, user.OrgID, user.IsOperator,
		user.CreatedAt, user.UpdatedAt)
	return err
}

// UpdateUserProfile sets identity-facing fields. Used by the setup wizard to
// promote the pre-auth default-user row into the admin account (preserving the
// row keeps all existing user settings intact) and by profile edits.
func (r *sqliteRepository) UpdateUserProfile(ctx context.Context, id, email, displayName, role string) (*models.User, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users SET email = ?, display_name = ?, role = ?, updated_at = ?
		WHERE id = ?
	`), email, displayName, role, time.Now().UTC(), id)
	if err != nil {
		return nil, err
	}
	if err := checkUserRowsAffected(result, id); err != nil {
		return nil, err
	}
	return r.getUserFromWriter(ctx, id)
}

// UpdateUserRoleStatus updates a user's role and status, returning the
// refreshed row.
func (r *sqliteRepository) UpdateUserRoleStatus(ctx context.Context, id, role, status string) (*models.User, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users SET role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`), role, status, time.Now().UTC(), id)
	if err != nil {
		return nil, err
	}
	if err := checkUserRowsAffected(result, id); err != nil {
		return nil, err
	}
	return r.getUserFromWriter(ctx, id)
}

// getUserFromWriter reads through the writer connection so callers observe
// their own just-committed mutation (the reader pool may lag under WAL).
func (r *sqliteRepository) getUserFromWriter(ctx context.Context, id string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT `+userColumns+`
		FROM users WHERE id = ?
	`), id)
	return scanUser(row)
}

// checkUserRowsAffected fails when a user mutation affected no rows, i.e.
// the target user does not exist.
func checkUserRowsAffected(result sqlResult, userID string) error {
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

// GetUserSettings reads the user's settings through the reader connection.
func (r *sqliteRepository) GetUserSettings(ctx context.Context, userID string) (*models.UserSettings, error) {
	return r.getUserSettings(ctx, r.ro, userID)
}

// getUserSettings reads settings through an explicit connection.
func (r *sqliteRepository) getUserSettings(ctx context.Context, conn *sqlx.DB, userID string) (*models.UserSettings, error) {
	row := conn.QueryRowContext(ctx, conn.Rebind(`
		SELECT settings, updated_at, settings_revision
		FROM users WHERE id = ?
	`), userID)
	settings, err := scanUserSettings(row, userID)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// UpsertUserSettingsPreservingTaskCreateLastUsed writes the settings blob,
// merging the task_create_last_used patch into the stored JSON rather than
// replacing it, and returns the persisted settings.
func (r *sqliteRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	ctx context.Context,
	settings *models.UserSettings,
	patch *models.TaskCreateLastUsed,
	expectedRevision int64,
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
		return r.upsertUserSettingsPreservingTaskCreateLastUsedPostgres(ctx, settings, settingsPayload, patch, expectedRevision)
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
		SET settings = %s, updated_at = ?, settings_revision = settings_revision + 1
		WHERE id = ? AND settings_revision = ?
		RETURNING settings, updated_at, settings_revision
	`, settingsExpr)
	args = append(args, settings.UpdatedAt, settings.UserID, expectedRevision)
	return r.scanConditionalSettingsUpdate(
		ctx,
		r.db.QueryRowContext(ctx, r.db.Rebind(query), args...),
		settings.UserID,
	)
}

// upsertUserSettingsPreservingTaskCreateLastUsedPostgres writes the settings via a Postgres jsonb_set UPDATE that merges the task_create_last_used patch.
func (r *sqliteRepository) upsertUserSettingsPreservingTaskCreateLastUsedPostgres(
	ctx context.Context,
	settings *models.UserSettings,
	settingsPayload []byte,
	patch *models.TaskCreateLastUsed,
	expectedRevision int64,
) (*models.UserSettings, error) {
	query, patchArgs := buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate(patch)
	args := append([]any{string(settingsPayload)}, patchArgs...)
	args = append(args, settings.UpdatedAt, settings.UserID, expectedRevision)
	return r.scanConditionalSettingsUpdate(
		ctx,
		r.db.QueryRowContext(ctx, r.db.Rebind(query), args...),
		settings.UserID,
	)
}

// UpdateTaskCreateLastUsed merges the last task-creation choices into the
// stored settings JSON and bumps the revision.
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
		SET settings = %s, updated_at = ?, settings_revision = settings_revision + 1
		WHERE id = ?
		RETURNING settings, updated_at, settings_revision
	`
	now := time.Now().UTC()
	args := append([]any{}, patchArgs...)
	args = append(args, now, userID)
	return scanUpdatedUserSettings(
		r.db.QueryRowContext(ctx, r.db.Rebind(fmt.Sprintf(query, settingsExpr)), args...),
		userID,
	)
}

// updateTaskCreateLastUsedPostgres merges the patch via a Postgres jsonb_set
// expression.
func (r *sqliteRepository) updateTaskCreateLastUsedPostgres(ctx context.Context, userID string, patch models.TaskCreateLastUsed) (*models.UserSettings, error) {
	query, args := buildPostgresTaskCreateLastUsedUpdate(patch)
	if len(args) == 0 {
		return r.getUserSettings(ctx, r.db, userID)
	}
	now := time.Now().UTC()
	args = append(args, now, userID)
	return scanUpdatedUserSettings(
		r.db.QueryRowContext(ctx, r.db.Rebind(query), args...),
		userID,
	)
}

// makeTaskCreateLastUsedJSONSetArgs flattens the patch into JSON1 json_set
// path/value arguments, skipping empty fields and unsafe workspace path keys.
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
	workflowIDs := make([]string, 0, len(patch.WorkflowIDsByWorkspace))
	for workspaceID := range patch.WorkflowIDsByWorkspace {
		workflowIDs = append(workflowIDs, workspaceID)
	}
	sort.Strings(workflowIDs)
	for _, workspaceID := range workflowIDs {
		workflowID := patch.WorkflowIDsByWorkspace[workspaceID]
		if !isSafeTaskCreateWorkspacePathKey(workspaceID) || workflowID == "" {
			continue
		}
		// Workspace IDs are generated UUIDs. SQLite JSON1 does not support
		// PostgreSQL-style parameterized path segments, so reject punctuation
		// rather than interpolating a key that could change the JSON path.
		args = append(args,
			"$.task_create_last_used.workflow_ids_by_workspace."+workspaceID,
			workflowID,
		)
	}
	return args
}

// isSafeTaskCreateWorkspacePathKey reports whether a workspace id can be
// embedded in a JSON path segment without punctuation that would change the
// path.
func isSafeTaskCreateWorkspacePathKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

// buildPostgresTaskCreateLastUsedUpdate builds the jsonb_set UPDATE statement
// and its arguments for a task_create_last_used patch.
func buildPostgresTaskCreateLastUsedUpdate(patch models.TaskCreateLastUsed) (string, []any) {
	base := "(CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}'::jsonb ELSE settings::jsonb END)"
	expr := fmt.Sprintf(
		"jsonb_set(%s, '{task_create_last_used}', %s, true)",
		base,
		normalizePostgresTaskCreateLastUsedExpr(base),
	)
	args := []any{}
	expr, args = applyPostgresTaskCreateLastUsedPatch(expr, patch, args)
	query := fmt.Sprintf(`
		UPDATE users
		SET settings = %s::text, updated_at = ?, settings_revision = settings_revision + 1
		WHERE id = ?
		RETURNING settings, updated_at, settings_revision
	`, expr)
	return query, args
}

// buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate builds the
// jsonb_set UPDATE for a full settings write that merges the patch.
func buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate(patch *models.TaskCreateLastUsed) (string, []any) {
	base := "(CASE WHEN settings IS NULL OR settings = 'null' OR settings = '' THEN '{}'::jsonb ELSE settings::jsonb END)"
	expr := fmt.Sprintf(
		"jsonb_set(?::jsonb, '{task_create_last_used}', %s, true)",
		normalizePostgresTaskCreateLastUsedExpr(base),
	)
	args := []any{}
	if patch != nil {
		expr, args = applyPostgresTaskCreateLastUsedPatch(expr, *patch, args)
	}
	query := fmt.Sprintf(`
		UPDATE users
		SET settings = %s::text, updated_at = ?, settings_revision = settings_revision + 1
		WHERE id = ? AND settings_revision = ?
		RETURNING settings, updated_at, settings_revision
	`, expr)
	return query, args
}

// normalizePostgresTaskCreateLastUsedExpr builds the jsonb expression that
// reads (or defaults) the task_create_last_used object and its workflow map.
func normalizePostgresTaskCreateLastUsedExpr(base string) string {
	taskCreate := fmt.Sprintf("COALESCE(%s->'task_create_last_used', '{}'::jsonb)", base)
	workflowMap := fmt.Sprintf(
		"CASE WHEN jsonb_typeof(%s->'workflow_ids_by_workspace') = 'object' THEN %s->'workflow_ids_by_workspace' ELSE '{}'::jsonb END",
		taskCreate,
		taskCreate,
	)
	return fmt.Sprintf(
		"jsonb_set(%s, '{workflow_ids_by_workspace}', %s, true)",
		taskCreate,
		workflowMap,
	)
}

// applyPostgresTaskCreateLastUsedPatch appends jsonb_set calls for each
// non-empty patch field and the corresponding arguments.
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
	workflowIDs := make([]string, 0, len(patch.WorkflowIDsByWorkspace))
	for workspaceID := range patch.WorkflowIDsByWorkspace {
		workflowIDs = append(workflowIDs, workspaceID)
	}
	sort.Strings(workflowIDs)
	for _, workspaceID := range workflowIDs {
		workflowID := patch.WorkflowIDsByWorkspace[workspaceID]
		if workspaceID == "" || workflowID == "" {
			continue
		}
		expr = fmt.Sprintf(
			"jsonb_set(%s, ARRAY['task_create_last_used','workflow_ids_by_workspace',?::text], to_jsonb(?::text), true)",
			expr,
		)
		args = append(args, workspaceID, workflowID)
	}
	return expr, args
}

// marshalUserSettingsPayload serializes the settings model to the stored JSON
// blob, normalizing enums and defaulting nil slices/maps.
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
		"workspace_id":                             settings.WorkspaceID,
		"kanban_view_mode":                         settings.KanbanViewMode,
		"startup_page":                             models.NormalizeStartupPage(settings.StartupPage),
		"workflow_filter_id":                       settings.WorkflowFilterID,
		"repository_ids":                           settings.RepositoryIDs,
		"tasks_list_sort":                          models.NormalizeTasksListSort(settings.TasksListSort),
		"tasks_list_group":                         models.NormalizeTasksListGroup(settings.TasksListGroup),
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
		"lsp_auto_start_languages":                 lspAutoStart,
		"lsp_auto_install_languages":               lspAutoInstall,
		"lsp_server_configs":                       lspServerConfigs,
		"lsp_status_location":                      models.NormalizeLspStatusLocation(settings.LspStatusLocation),
		"saved_layouts":                            savedLayouts,
		"sidebar_views":                            sidebarViews,
		"sidebar_active_view_id":                   settings.SidebarActiveViewID,
		"sidebar_draft":                            settings.SidebarDraft,
		"sidebar_task_prefs":                       sidebarTaskPrefs,
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
		"keyboard_shortcuts":                       keyboardShortcuts,
		"terminal_link_behavior":                   settings.TerminalLinkBehavior,
		"terminal_font_family":                     settings.TerminalFontFamily,
		"terminal_font_size":                       settings.TerminalFontSize,
		"changes_panel_layout":                     settings.ChangesPanelLayout,
		"last_seen_display":                        models.NormalizeLastSeenDisplay(settings.LastSeenDisplay),
		"system_metrics_display":                   settings.SystemMetricsDisplay,
		"app_status_bar_enabled":                   settings.AppStatusBarEnabled,
		"app_status_bar_order":                     normalizeAppStatusBarOrder(settings.AppStatusBarOrder),
		"kanban_hidden_step_ids":                   settings.KanbanHiddenStepIDs,
	})
}

// scanUpdatedUserSettings scans a settings row after an UPDATE, mapping a
// missing row to a user-not-found error.
func scanUpdatedUserSettings(scanner interface{ Scan(dest ...any) error }, userID string) (*models.UserSettings, error) {
	settings, err := scanUserSettings(scanner, userID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return settings, err
}

// scanConditionalSettingsUpdate scans a revision-conditional settings UPDATE,
// mapping a zero-row result to either a missing user (user-not-found) or a
// stale revision (ErrUserSettingsRevisionConflict).
func (r *sqliteRepository) scanConditionalSettingsUpdate(ctx context.Context, scanner interface{ Scan(dest ...any) error }, userID string) (*models.UserSettings, error) {
	settings, err := scanUserSettings(scanner, userID)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, uerr := r.getUserFromWriter(ctx, userID); uerr != nil {
		if errors.Is(uerr, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, uerr
	}
	return nil, ErrUserSettingsRevisionConflict
}

// scanUser scans a single user row into a models.User.
func scanUser(scanner interface{ Scan(dest ...any) error }) (*models.User, error) {
	user := &models.User{}
	if err := scanner.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.Status, &user.OrgID, &user.IsOperator, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, err
	}
	return user, nil
}

// defaultUserSettings returns the baseline settings for a user with no saved
// preferences.
func defaultUserSettings(userID string) *models.UserSettings {
	return &models.UserSettings{
		UserID:                            userID,
		StartupPage:                       models.StartupPageTaskOverview,
		RepositoryIDs:                     []string{},
		TasksListSort:                     models.TasksListSortDefault,
		TasksListGroup:                    models.TasksListGroupDefault,
		ReviewAutoMarkOnScroll:            true,
		ConfirmTaskArchive:                true,
		UnreadDivider:                     false,
		AgentGeneratedTaskTitles:          true,
		MCPTaskAgentProfileDefault:        models.MCPTaskAgentProfileDefaultCurrentTask,
		ShowAnchoredPromptBar:             false,
		ShowScrollToLastPrompt:            true,
		ShowScrollToStart:                 false,
		ShowTranscriptAutoScrollControl:   false,
		ShowTodoListPanel:                 false,
		ShowTodoListPanelOnlyWhenNotEmpty: false,
		ShowReleaseNotification:           true,
		LspAutoStartLanguages:             []string{},
		LspAutoInstallLanguages:           []string{},
		LspServerConfigs:                  map[string]map[string]interface{}{},
		LspStatusLocation:                 models.LspStatusLocationToolbar,
		SavedLayouts:                      []models.SavedLayout{},
		ChatSubmitKey:                     "cmd_enter",
		KeyboardShortcuts:                 map[string]interface{}{},
		TerminalLinkBehavior:              "new_tab",
		ChangesPanelLayout:                defaultChangesPanelLayout,
		LastSeenDisplay:                   models.LastSeenDisplayAbsolute,
		SidebarViews:                      DefaultSidebarViews(),
		SidebarActiveViewID:               DefaultSidebarViewID,
		SidebarTaskPrefs:                  normalizeSidebarTaskPrefs(models.SidebarTaskPrefs{}),
		AppStatusBarEnabled:               false,
		AppStatusBarOrder:                 normalizeAppStatusBarOrder(models.AppStatusBarOrder{}),
		KanbanHiddenStepIDs:               map[string][]string{},
	}
}

// DefaultSidebarViews returns the default single "All tasks" sidebar view.
func DefaultSidebarViews() []models.SidebarView {
	return []models.SidebarView{{
		ID:              DefaultSidebarViewID,
		Name:            "All tasks",
		Filters:         []models.SidebarViewClause{},
		Sort:            models.SidebarViewSort{Key: "state", Direction: "asc"},
		Group:           "repository",
		CollapsedGroups: []string{},
	}}
}

// scanUserSettings scans and decodes the settings row, starting from defaults
// and merging every field present in the stored JSON blob.
func scanUserSettings(scanner interface{ Scan(dest ...any) error }, userID string) (*models.UserSettings, error) {
	settings := defaultUserSettings(userID)
	var settingsRaw string
	if err := scanner.Scan(&settingsRaw, &settings.UpdatedAt, &settings.Revision); err != nil {
		return nil, err
	}
	if settingsRaw == "" || settingsRaw == "{}" {
		return settings, nil
	}
	var payload struct {
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
		ReviewAutoMarkOnScroll            *bool                               `json:"review_auto_mark_on_scroll"`
		ConfirmTaskArchive                *bool                               `json:"confirm_task_archive"`
		PreventAutoStartAgentOnOpen       *bool                               `json:"prevent_auto_start_agent_on_open"`
		UnreadDivider                     *bool                               `json:"unread_divider"`
		AgentGeneratedTaskTitles          *bool                               `json:"agent_generated_task_titles"`
		MCPTaskAgentProfileDefault        string                              `json:"mcp_task_agent_profile_default"`
		ShowAnchoredPromptBar             *bool                               `json:"show_anchored_prompt_bar"`
		ShowScrollToLastPrompt            *bool                               `json:"show_scroll_to_last_prompt"`
		ShowScrollToStart                 *bool                               `json:"show_scroll_to_start"`
		ShowTranscriptAutoScrollControl   *bool                               `json:"show_transcript_auto_scroll_control"`
		ShowTodoListPanel                 *bool                               `json:"show_todo_list_panel"`
		ShowTodoListPanelOnlyWhenNotEmpty *bool                               `json:"show_todo_list_panel_only_when_not_empty"`
		ShowReleaseNotification           *bool                               `json:"show_release_notification"`
		ReleaseNotesLastSeenVersion       string                              `json:"release_notes_last_seen_version"`
		LspAutoStartLanguages             []string                            `json:"lsp_auto_start_languages"`
		LspAutoInstallLanguages           []string                            `json:"lsp_auto_install_languages"`
		LspServerConfigs                  map[string]map[string]interface{}   `json:"lsp_server_configs"`
		LspStatusLocation                 string                              `json:"lsp_status_location"`
		SavedLayouts                      []models.SavedLayout                `json:"saved_layouts"`
		SidebarViews                      json.RawMessage                     `json:"sidebar_views"`
		SidebarActiveViewID               json.RawMessage                     `json:"sidebar_active_view_id"`
		SidebarDraft                      *models.SidebarViewDraft            `json:"sidebar_draft"`
		SidebarTaskPrefs                  models.SidebarTaskPrefs             `json:"sidebar_task_prefs"`
		TaskCreateLastUsed                models.TaskCreateLastUsed           `json:"task_create_last_used"`
		JiraSavedViews                    json.RawMessage                     `json:"jira_saved_views"`
		JiraTaskPresets                   json.RawMessage                     `json:"jira_task_presets"`
		GitHubSavedPresets                json.RawMessage                     `json:"github_saved_presets"`
		GitHubDefaultQueryPresets         json.RawMessage                     `json:"github_default_query_presets"`
		GitLabSavedPresets                json.RawMessage                     `json:"gitlab_saved_presets"`
		AzureDevOpsBrowsePreferences      json.RawMessage                     `json:"azure_devops_browse_preferences"`
		DefaultUtilityAgentID             string                              `json:"default_utility_agent_id"`
		DefaultUtilityModel               string                              `json:"default_utility_model"`
		DefaultUtilityAgentProfileID      string                              `json:"default_utility_agent_profile_id"`
		KeyboardShortcuts                 map[string]interface{}              `json:"keyboard_shortcuts"`
		TerminalLinkBehavior              string                              `json:"terminal_link_behavior"`
		TerminalFontFamily                string                              `json:"terminal_font_family"`
		TerminalFontSize                  int                                 `json:"terminal_font_size"`
		ChangesPanelLayout                string                              `json:"changes_panel_layout"`
		LastSeenDisplay                   json.RawMessage                     `json:"last_seen_display"`
		SystemMetricsDisplay              models.SystemMetricsDisplaySettings `json:"system_metrics_display"`
		AppStatusBarEnabled               *bool                               `json:"app_status_bar_enabled"`
		AppStatusBarOrder                 models.AppStatusBarOrder            `json:"app_status_bar_order"`
		KanbanHiddenStepIDs               json.RawMessage                     `json:"kanban_hidden_step_ids"`
	}
	if err := json.Unmarshal([]byte(settingsRaw), &payload); err != nil {
		return nil, err
	}
	settings.WorkspaceID = payload.WorkspaceID
	settings.KanbanViewMode = payload.KanbanViewMode
	settings.StartupPage = models.NormalizeStartupPage(payload.StartupPage)
	settings.WorkflowFilterID = payload.WorkflowFilterID
	settings.RepositoryIDs = payload.RepositoryIDs
	if settings.RepositoryIDs == nil {
		settings.RepositoryIDs = []string{}
	}
	settings.TasksListSort = models.NormalizeTasksListSort(payload.TasksListSort)
	settings.TasksListGroup = models.NormalizeTasksListGroup(payload.TasksListGroup)
	settings.TasksListShowDetails = payload.TasksListShowDetails
	settings.InitialSetupComplete = payload.InitialSetupComplete
	settings.PreferredShell = payload.PreferredShell
	settings.DefaultEditorID = payload.DefaultEditorID
	settings.EnablePreviewOnClick = payload.EnablePreviewOnClick
	if payload.ChatSubmitKey != "" {
		settings.ChatSubmitKey = payload.ChatSubmitKey
	}
	if payload.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *payload.ReviewAutoMarkOnScroll
	}
	if payload.ConfirmTaskArchive != nil {
		settings.ConfirmTaskArchive = *payload.ConfirmTaskArchive
	}
	if payload.PreventAutoStartAgentOnOpen != nil {
		settings.PreventAutoStartAgentOnOpen = *payload.PreventAutoStartAgentOnOpen
	}
	if payload.UnreadDivider != nil {
		settings.UnreadDivider = *payload.UnreadDivider
	}
	if payload.AgentGeneratedTaskTitles != nil {
		settings.AgentGeneratedTaskTitles = *payload.AgentGeneratedTaskTitles
	}
	settings.MCPTaskAgentProfileDefault = models.NormalizeMCPTaskAgentProfileDefault(payload.MCPTaskAgentProfileDefault)
	if payload.ShowAnchoredPromptBar != nil {
		settings.ShowAnchoredPromptBar = *payload.ShowAnchoredPromptBar
	}
	if payload.ShowScrollToLastPrompt != nil {
		settings.ShowScrollToLastPrompt = *payload.ShowScrollToLastPrompt
	}
	if payload.ShowScrollToStart != nil {
		settings.ShowScrollToStart = *payload.ShowScrollToStart
	}
	if payload.ShowTranscriptAutoScrollControl != nil {
		settings.ShowTranscriptAutoScrollControl = *payload.ShowTranscriptAutoScrollControl
	}
	if payload.ShowTodoListPanel != nil {
		settings.ShowTodoListPanel = *payload.ShowTodoListPanel
	}
	if payload.ShowTodoListPanelOnlyWhenNotEmpty != nil {
		settings.ShowTodoListPanelOnlyWhenNotEmpty = *payload.ShowTodoListPanelOnlyWhenNotEmpty
	}
	if payload.ShowReleaseNotification != nil {
		settings.ShowReleaseNotification = *payload.ShowReleaseNotification
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
	if settings.LspServerConfigs == nil {
		settings.LspServerConfigs = map[string]map[string]interface{}{}
	}
	settings.LspStatusLocation = models.NormalizeLspStatusLocation(payload.LspStatusLocation)
	settings.SavedLayouts = payload.SavedLayouts
	if settings.SavedLayouts == nil {
		settings.SavedLayouts = []models.SavedLayout{}
	}
	if len(payload.SidebarViews) > 0 {
		if err := json.Unmarshal(payload.SidebarViews, &settings.SidebarViews); err != nil {
			return nil, err
		}
		if settings.SidebarViews == nil {
			settings.SidebarViews = []models.SidebarView{}
		}
	}
	if len(payload.SidebarActiveViewID) > 0 {
		var activeViewID *string
		if err := json.Unmarshal(payload.SidebarActiveViewID, &activeViewID); err != nil {
			return nil, err
		}
		if activeViewID == nil {
			settings.SidebarActiveViewID = ""
		} else {
			settings.SidebarActiveViewID = *activeViewID
		}
	}
	settings.SidebarDraft = payload.SidebarDraft
	settings.SidebarTaskPrefs = normalizeSidebarTaskPrefs(payload.SidebarTaskPrefs)
	settings.TaskCreateLastUsed = payload.TaskCreateLastUsed
	settings.JiraSavedViews = payload.JiraSavedViews
	settings.JiraTaskPresets = payload.JiraTaskPresets
	settings.GitHubSavedPresets = payload.GitHubSavedPresets
	settings.GitHubDefaultQueryPresets = payload.GitHubDefaultQueryPresets
	settings.GitLabSavedPresets = payload.GitLabSavedPresets
	settings.AzureDevOpsBrowsePreferences = payload.AzureDevOpsBrowsePreferences
	settings.DefaultUtilityAgentID = payload.DefaultUtilityAgentID
	settings.DefaultUtilityModel = payload.DefaultUtilityModel
	settings.DefaultUtilityAgentProfileID = payload.DefaultUtilityAgentProfileID
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
	settings.SystemMetricsDisplay = payload.SystemMetricsDisplay
	if payload.AppStatusBarEnabled != nil {
		settings.AppStatusBarEnabled = *payload.AppStatusBarEnabled
	}
	settings.AppStatusBarOrder = normalizeAppStatusBarOrder(payload.AppStatusBarOrder)
	if payload.ChangesPanelLayout == "flat" {
		settings.ChangesPanelLayout = "flat"
	} else {
		settings.ChangesPanelLayout = defaultChangesPanelLayout
	}
	settings.LastSeenDisplay = normalizeLastSeenDisplayStored(payload.LastSeenDisplay)
	settings.KanbanHiddenStepIDs = decodeKanbanHiddenStepIDs(payload.KanbanHiddenStepIDs)
	return settings, nil
}

// normalizeLastSeenDisplayStored maps a stored JSON value to the canonical
// display mode: only a string exactly equal to "relative" is accepted. Every
// other JSON type (number, object, boolean, null) and every other string
// coerces to "absolute", so a hand-edited blob can never fail the settings
// read or produce a broken column.
func normalizeLastSeenDisplayStored(raw json.RawMessage) string {
	if len(raw) == 0 {
		return models.LastSeenDisplayAbsolute
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return models.LastSeenDisplayAbsolute
	}
	return models.NormalizeLastSeenDisplay(value)
}

// decodeKanbanHiddenStepIDs parses the persisted per-workflow hidden-step-id
// map, defaulting to an empty map on a missing or malformed value instead of
// failing the whole settings read. Unlike most structured settings fields,
// this one is decoded as json.RawMessage in the payload struct specifically
// so a corrupt value here can't block every other setting from loading — the
// spec requires this field to fall back to "nothing hidden" on corruption,
// not to take the rest of the user's settings down with it.
func decodeKanbanHiddenStepIDs(raw json.RawMessage) map[string][]string {
	if len(raw) == 0 {
		return map[string][]string{}
	}
	var decoded map[string][]string
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded == nil {
		return map[string][]string{}
	}
	return decoded
}

// normalizeSidebarTaskPrefs defaults nil sidebar task pref collections so the
// stored JSON never carries null.
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

// normalizeAppStatusBarOrder defaults nil status bar item lists so the stored
// JSON never carries null.
func normalizeAppStatusBarOrder(order models.AppStatusBarOrder) models.AppStatusBarOrder {
	if order.LeftItemIDs == nil {
		order.LeftItemIDs = []string{}
	}
	if order.RightItemIDs == nil {
		order.RightItemIDs = []string{}
	}
	return order
}
