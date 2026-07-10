package sentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/integrations/workspacescope"
)

// Store persists workspace-scoped Sentry configuration. The secret token is
// delegated to the shared encrypted secret store and not stored here.
type Store struct {
	db                  *sqlx.DB
	ro                  *sqlx.DB
	defaultWorkspace    workspacescope.DefaultResolver
	migratedToWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("sentry schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS sentry_configs (
		workspace_id TEXT PRIMARY KEY,
		auth_method TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT 'https://sentry.io',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sentry_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		-- Optional repository binding. Empty = unbound (repo-less task, the
		-- historical behaviour). When set, watcher-created tasks launch in an
		-- isolated worktree of this repo cut from base_branch.
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_workspace
		ON sentry_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS sentry_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_short_id TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_short_id),
		FOREIGN KEY(issue_watch_id) REFERENCES sentry_issue_watches(id) ON DELETE CASCADE
	);
`

// singletonID is the synthetic primary key used by the legacy install-wide
// sentry_configs table.
const singletonID = "singleton"

// MigratedFromWorkspace is kept for parity with Jira/Linear provider
// migrations. It returns the workspace_id that received a legacy singleton row.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedToWorkspace
}

// initSchema creates the integration tables when absent and applies the
// additive column migrations that bring older databases to the current schema.
func (s *Store) initSchema() error {
	if err := s.migrateLegacySingletonTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addConfigURLColumn(); err != nil {
		return err
	}
	if err := s.addMaxInflightTasksColumn(); err != nil {
		return err
	}
	if err := s.addIssueWatchLastErrorColumns(); err != nil {
		return err
	}
	return s.addIssueWatchRepositoryColumns()
}

func (s *Store) migrateLegacySingletonTable() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if !isLegacySingletonConfig(cols) {
		return nil
	}
	targetWorkspace, err := workspacescope.ResolveMigrationTarget(s.db)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	selectCols := "auth_method"
	if _, ok := cols["url"]; ok {
		selectCols += ", url"
	} else {
		selectCols += ", 'https://sentry.io' AS url"
	}
	if healthColumnsPresent(cols) {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	var authMethod, rawURL, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT `+selectCols+` FROM sentry_configs WHERE id = ? LIMIT 1`, singletonID)
	switch err := row.Scan(&authMethod, &rawURL, &lastCheckedAt, &lastOk, &lastError, &createdAt, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE sentry_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT 'https://sentry.io',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO sentry_configs (workspace_id, auth_method, url,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		targetWorkspace, authMethod.String, rawURL.String,
		nullableTime(lastCheckedAt), lastOk.Int64, lastError.String,
		nullableTime(createdAt), nullableTime(updatedAt)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedToWorkspace = targetWorkspace
	return nil
}

func isLegacySingletonConfig(cols map[string]struct{}) bool {
	if len(cols) == 0 {
		return false
	}
	if _, hasWorkspace := cols["workspace_id"]; hasWorkspace {
		return false
	}
	_, hasID := cols["id"]
	return hasID
}

func healthColumnsPresent(cols map[string]struct{}) bool {
	for _, name := range []string{"last_checked_at", "last_ok", "last_error"} {
		if _, ok := cols[name]; !ok {
			return false
		}
	}
	return true
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// addIssueWatchRepositoryColumns brings older databases up to the current
// schema by appending repository_id / base_branch to sentry_issue_watches when
// missing. Both backfill to ” (unbound), so existing repo-less watches keep
// their behaviour. Fresh installs hit the column-already-present branch since
// createTablesSQL declares both columns. Idempotent — column lookup before each
// ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchRepositoryColumns() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["repository_id"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add repository_id column: %w", err)
		}
	}
	if _, ok := cols["base_branch"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add base_branch column: %w", err)
		}
	}
	return nil
}

// addConfigURLColumn brings older databases up to the current schema by adding
// the url column to sentry_configs when missing. Existing rows — all SaaS
// installs, since this predates self-hosted support — backfill to the
// sentry.io default (mirrors DefaultSentryURL). A fresh install hits the
// column-already-present branch since createTablesSQL declares the column.
func (s *Store) addConfigURLColumn() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["url"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_configs ADD COLUMN url TEXT NOT NULL DEFAULT 'https://sentry.io'`); err != nil {
		return fmt.Errorf("add url column: %w", err)
	}
	return nil
}

// addMaxInflightTasksColumn brings older databases up to the current schema by
// adding the max_inflight_tasks column to sentry_issue_watches when missing.
// Existing rows backfill to the default (5). A fresh install hits the
// column-already-present branch since createTablesSQL declares the column.
func (s *Store) addMaxInflightTasksColumn() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["max_inflight_tasks"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN max_inflight_tasks INTEGER DEFAULT 5`); err != nil {
		return fmt.Errorf("add max_inflight_tasks column: %w", err)
	}
	return nil
}

// addIssueWatchLastErrorColumns brings older databases up to the current
// schema by appending last_error / last_error_at to sentry_issue_watches when
// missing. Fresh installs hit the column-already-present branch since
// createTablesSQL declares both columns. Idempotent — column lookup before
// each ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchLastErrorColumns() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["last_error"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add last_error column: %w", err)
		}
	}
	if _, ok := cols["last_error_at"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error_at DATETIME`); err != nil {
			return fmt.Errorf("add last_error_at column: %w", err)
		}
	}
	return nil
}

// tableColumns returns the set of column names for a table via PRAGMA
// table_info, used by the lightweight ADD COLUMN migrations above.
func (s *Store) tableColumns(table string) (map[string]struct{}, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = struct{}{}
	}
	return cols, rows.Err()
}

const selectConfigColumns = `workspace_id, auth_method, url,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the default workspace Sentry config, or nil when no row
// exists. New code should call GetConfigForWorkspace.
func (s *Store) GetConfig(ctx context.Context) (*SentryConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns the Sentry config for a workspace, or nil when
// no row exists.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*SentryConfig, error) {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	var cfg SentryConfig
	err = s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM sentry_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the default workspace config row. The last_* health
// columns are owned by the poller and written via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *SentryConfig) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpsertConfigForWorkspace(ctx, workspaceID, cfg)
}

// UpsertConfigForWorkspace inserts or updates a workspace config row.
func (s *Store) UpsertConfigForWorkspace(ctx context.Context, workspaceID string, cfg *SentryConfig) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sentry_configs (workspace_id, auth_method, url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			auth_method = excluded.auth_method,
			url = excluded.url,
			updated_at = excluded.updated_at`,
		workspaceID, cfg.AuthMethod, cfg.URL, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the default workspace config row.
func (s *Store) DeleteConfig(ctx context.Context) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes the workspace config row.
func (s *Store) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM sentry_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// HasConfig reports whether any config row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM sentry_configs`)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// ListConfigWorkspaceIDs returns every workspace with a saved Sentry config.
func (s *Store) ListConfigWorkspaceIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids, `SELECT workspace_id FROM sentry_configs ORDER BY workspace_id`); err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateAuthHealth records the result of a credential probe.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg string, checkedAt time.Time) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpdateAuthHealthForWorkspace(ctx, workspaceID, ok, errMsg, checkedAt)
}

// UpdateAuthHealthForWorkspace records the result of a credential probe for a
// single workspace config row.
func (s *Store) UpdateAuthHealthForWorkspace(ctx context.Context, workspaceID string, ok bool, errMsg string, checkedAt time.Time) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE sentry_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`,
		checkedAt, ok, errMsg, workspaceID)
	return err
}

func (s *Store) defaultWorkspaceID() (string, error) {
	return s.defaultWorkspace.Resolve(s.db)
}

func (s *Store) resolveWorkspaceID(workspaceID string) (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}
	return s.defaultWorkspaceID()
}
