package azuredevops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists Azure DevOps configuration and health, never credentials.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

const createConfigTableSQL = `
	CREATE TABLE IF NOT EXISTS azure_devops_configs (
		workspace_id TEXT PRIMARY KEY,
		organization_url TEXT NOT NULL,
		default_project_id TEXT NOT NULL DEFAULT '',
		default_project_name TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL DEFAULT 'pat',
		last_checked_at DATETIME,
		last_ok BOOLEAN NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		saved_views TEXT NOT NULL DEFAULT '[]',
		workspace_settings TEXT NOT NULL DEFAULT '{}',
		workspace_settings_version INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`

const createTaskPRTableSQL = `
	CREATE TABLE IF NOT EXISTS azure_devops_task_prs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		organization_url TEXT NOT NULL,
		project_id TEXT NOT NULL,
		azure_repository_id TEXT NOT NULL,
		pull_request_id INTEGER NOT NULL,
		pull_request_url TEXT NOT NULL,
		title TEXT NOT NULL,
		source_branch TEXT NOT NULL,
		target_branch TEXT NOT NULL,
		author_id TEXT NOT NULL,
		author_name TEXT NOT NULL,
		status TEXT NOT NULL,
		review_state TEXT NOT NULL DEFAULT '',
		policy_state TEXT NOT NULL DEFAULT '',
		is_draft BOOLEAN NOT NULL DEFAULT 0,
		last_synced_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, azure_repository_id, pull_request_id)
	);
	CREATE INDEX IF NOT EXISTS idx_azure_devops_task_prs_task_id
	ON azure_devops_task_prs(task_id)`

const createTaskWorkItemTableSQL = `
CREATE TABLE IF NOT EXISTS azure_devops_task_work_items (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	workspace_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	work_item_id INTEGER NOT NULL,
	work_item_url TEXT NOT NULL,
	title TEXT NOT NULL,
	state TEXT NOT NULL,
	type TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(task_id, workspace_id, project_id, work_item_id)
);
CREATE INDEX IF NOT EXISTS idx_azure_devops_task_work_items_task_id ON azure_devops_task_work_items(task_id);
CREATE INDEX IF NOT EXISTS idx_azure_devops_task_work_items_workspace_id ON azure_devops_task_work_items(workspace_id)`

const createWatchTablesSQL = `
CREATE TABLE IF NOT EXISTS azure_devops_work_item_watches (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	workflow_id TEXT NOT NULL DEFAULT '',
	workflow_step_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL,
	wiql TEXT NOT NULL,
	repository_id TEXT NOT NULL DEFAULT '',
	base_branch TEXT NOT NULL DEFAULT '',
	agent_profile_id TEXT NOT NULL DEFAULT '',
	executor_profile_id TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	enabled BOOLEAN NOT NULL DEFAULT 1,
	poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
	cleanup_policy TEXT NOT NULL DEFAULT 'auto',
	max_inflight_tasks INTEGER,
	generation INTEGER NOT NULL DEFAULT 1,
	deleting BOOLEAN NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	last_error_at DATETIME,
	last_polled_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_azure_devops_work_item_watches_workspace
	ON azure_devops_work_item_watches(workspace_id);
CREATE TABLE IF NOT EXISTS azure_devops_pull_request_watches (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	workflow_id TEXT NOT NULL DEFAULT '',
	workflow_step_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL,
	azure_repository_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	creator_id TEXT NOT NULL DEFAULT '',
	reviewer_id TEXT NOT NULL DEFAULT '',
	repository_id TEXT NOT NULL DEFAULT '',
	base_branch TEXT NOT NULL DEFAULT '',
	agent_profile_id TEXT NOT NULL DEFAULT '',
	executor_profile_id TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	enabled BOOLEAN NOT NULL DEFAULT 1,
	poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
	cleanup_policy TEXT NOT NULL DEFAULT 'auto',
	max_inflight_tasks INTEGER,
	generation INTEGER NOT NULL DEFAULT 1,
	deleting BOOLEAN NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	last_error_at DATETIME,
	last_polled_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_azure_devops_pull_request_watches_workspace
	ON azure_devops_pull_request_watches(workspace_id);
CREATE TABLE IF NOT EXISTS azure_devops_work_item_watch_tasks (
	id TEXT PRIMARY KEY,
	watch_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	work_item_id INTEGER NOT NULL,
	work_item_url TEXT NOT NULL,
	task_id TEXT NOT NULL DEFAULT '',
	generation INTEGER NOT NULL,
	created_at DATETIME NOT NULL,
	UNIQUE(watch_id, generation, project_id, work_item_id)
);
CREATE INDEX IF NOT EXISTS idx_azure_devops_work_item_watch_tasks_watch
	ON azure_devops_work_item_watch_tasks(watch_id);
CREATE TABLE IF NOT EXISTS azure_devops_pull_request_watch_tasks (
	id TEXT PRIMARY KEY,
	watch_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	azure_repository_id TEXT NOT NULL,
	pull_request_id INTEGER NOT NULL,
	pull_request_url TEXT NOT NULL,
	task_id TEXT NOT NULL DEFAULT '',
	generation INTEGER NOT NULL,
	created_at DATETIME NOT NULL,
	UNIQUE(watch_id, generation, project_id, azure_repository_id, pull_request_id)
);
CREATE INDEX IF NOT EXISTS idx_azure_devops_pull_request_watch_tasks_watch
	ON azure_devops_pull_request_watch_tasks(watch_id)`

const selectConfigColumns = `workspace_id, organization_url, default_project_id,
	default_project_name, auth_method, last_checked_at, last_ok, last_error,
	created_at, updated_at, saved_views`

// NewStore creates the store and initializes its replay-safe schema.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	if writer == nil {
		return nil, errors.New("azure devops store: writer is required")
	}
	if reader == nil {
		reader = writer
	}
	store := &Store{db: writer, ro: reader}
	if _, err := store.db.Exec(createConfigTableSQL); err != nil {
		return nil, fmt.Errorf("azure devops schema init: %w", err)
	}
	if err := store.ensureSavedViewsColumn(); err != nil {
		return nil, fmt.Errorf("azure devops saved views schema init: %w", err)
	}
	if err := store.ensureWorkspaceSettingsColumn(); err != nil {
		return nil, fmt.Errorf("azure devops workspace settings schema init: %w", err)
	}
	if err := store.ensureWorkspaceSettingsVersionColumn(); err != nil {
		return nil, fmt.Errorf("azure devops workspace settings version schema init: %w", err)
	}
	if _, err := store.db.Exec(createTaskPRTableSQL); err != nil {
		return nil, fmt.Errorf("azure devops task PR schema init: %w", err)
	}
	if _, err := store.db.Exec(createTaskWorkItemTableSQL); err != nil {
		return nil, fmt.Errorf("azure devops task work item schema init: %w", err)
	}
	if _, err := store.db.Exec(createWatchTablesSQL); err != nil {
		return nil, fmt.Errorf("azure devops watcher schema init: %w", err)
	}
	return store, nil
}

func (s *Store) ensureWorkspaceSettingsColumn() error {
	return s.ensureConfigColumn(
		"workspace_settings",
		`ALTER TABLE azure_devops_configs ADD COLUMN workspace_settings TEXT NOT NULL DEFAULT '{}'`,
	)
}

func (s *Store) ensureWorkspaceSettingsVersionColumn() error {
	return s.ensureConfigColumn(
		"workspace_settings_version",
		`ALTER TABLE azure_devops_configs ADD COLUMN workspace_settings_version INTEGER NOT NULL DEFAULT 0`,
	)
}

func (s *Store) ensureSavedViewsColumn() error {
	return s.ensureConfigColumn(
		"saved_views",
		`ALTER TABLE azure_devops_configs ADD COLUMN saved_views TEXT NOT NULL DEFAULT '[]'`,
	)
}

func (s *Store) ensureConfigColumn(column, statement string) error {
	rows, err := s.db.Query(`PRAGMA table_info(azure_devops_configs)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if scanErr := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); scanErr != nil {
			return scanErr
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(statement)
	return err
}

// GetConfig returns a workspace's configuration, or nil when none exists.
func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*Config, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	var cfg Config
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM azure_devops_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates non-secret workspace configuration. Existing
// health fields are preserved until the next explicit authentication probe.
func (s *Store) UpsertConfig(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return errors.New("azure devops store: config is required")
	}
	if err := validateWorkspaceID(cfg.WorkspaceID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO azure_devops_configs (
			workspace_id, organization_url, default_project_id, default_project_name,
			auth_method, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			organization_url = excluded.organization_url,
			default_project_id = excluded.default_project_id,
			default_project_name = excluded.default_project_name,
			auth_method = excluded.auth_method,
			updated_at = excluded.updated_at`,
		cfg.WorkspaceID, cfg.OrganizationURL, cfg.DefaultProjectID,
		cfg.DefaultProjectName, cfg.AuthMethod, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes one workspace's configuration row.
func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM azure_devops_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// ListConfigWorkspaceIDs returns all configured workspace IDs in stable order.
func (s *Store) ListConfigWorkspaceIDs(ctx context.Context) ([]string, error) {
	var workspaceIDs []string
	if err := s.ro.SelectContext(ctx, &workspaceIDs,
		`SELECT workspace_id FROM azure_devops_configs ORDER BY workspace_id`); err != nil {
		return nil, err
	}
	return workspaceIDs, nil
}

// UpdateAuthHealth persists an authentication probe outcome for one workspace.
func (s *Store) UpdateAuthHealth(
	ctx context.Context,
	workspaceID string,
	ok bool,
	errMsg string,
	checkedAt time.Time,
) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE azure_devops_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`, checkedAt, ok, errMsg, workspaceID)
	return err
}

// ResetAuthHealth marks a workspace configuration as not yet checked.
func (s *Store) ResetAuthHealth(ctx context.Context, workspaceID string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE azure_devops_configs
		SET last_checked_at = NULL, last_ok = 0, last_error = ''
		WHERE workspace_id = ?`, workspaceID)
	return err
}

func (s *Store) GetSavedViewsJSON(ctx context.Context, workspaceID string) (string, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return "", err
	}
	var raw string
	err := s.ro.GetContext(ctx, &raw,
		`SELECT saved_views FROM azure_devops_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotConfigured
	}
	return raw, err
}

func (s *Store) PutSavedViewsJSON(ctx context.Context, workspaceID, raw string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE azure_devops_configs SET saved_views = ?, updated_at = ?
		WHERE workspace_id = ?`, raw, time.Now().UTC(), workspaceID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrNotConfigured
	}
	return nil
}

// WorkspaceSettingsSnapshot is the persisted settings payload and its optimistic-lock version.
type WorkspaceSettingsSnapshot struct {
	JSON    string
	Version int64
}

func (s *Store) GetWorkspaceSettingsSnapshot(ctx context.Context, workspaceID string) (WorkspaceSettingsSnapshot, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return WorkspaceSettingsSnapshot{}, err
	}
	var snapshot WorkspaceSettingsSnapshot
	err := s.ro.GetContext(ctx, &snapshot,
		`SELECT workspace_settings AS json, workspace_settings_version AS version
		FROM azure_devops_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceSettingsSnapshot{}, ErrNotConfigured
	}
	return snapshot, err
}

func (s *Store) GetWorkspaceSettingsJSON(ctx context.Context, workspaceID string) (string, error) {
	snapshot, err := s.GetWorkspaceSettingsSnapshot(ctx, workspaceID)
	return snapshot.JSON, err
}

func (s *Store) PutWorkspaceSettingsJSON(ctx context.Context, workspaceID, raw string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE azure_devops_configs
		SET workspace_settings = ?, workspace_settings_version = workspace_settings_version + 1, updated_at = ?
		WHERE workspace_id = ?`, raw, time.Now().UTC(), workspaceID)
	if err != nil {
		return err
	}
	updated, err := workspaceSettingsRowsUpdated(result)
	if err != nil {
		return err
	}
	if !updated {
		return ErrNotConfigured
	}
	return nil
}

// PutWorkspaceSettingsJSONIfVersion saves settings only when the caller's
// snapshot is still current. A false result means another writer won the race.
func (s *Store) PutWorkspaceSettingsJSONIfVersion(ctx context.Context, workspaceID, raw string, version int64) (bool, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE azure_devops_configs
		SET workspace_settings = ?, workspace_settings_version = workspace_settings_version + 1, updated_at = ?
		WHERE workspace_id = ? AND workspace_settings_version = ?`, raw, time.Now().UTC(), workspaceID, version)
	if err != nil {
		return false, err
	}
	return workspaceSettingsRowsUpdated(result)
}

func workspaceSettingsRowsUpdated(result sql.Result) (bool, error) {
	updated, err := result.RowsAffected()
	return updated > 0, err
}
