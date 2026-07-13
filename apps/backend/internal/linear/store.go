package linear

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/integrations/workspacescope"
)

// Store persists workspace-scoped Linear configuration. The secret API key is
// delegated to the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	defaultWorkspace workspacescope.DefaultResolver

	// migratedToWorkspace records the workspace_id that received a legacy
	// singleton row during initSchema. Provider reads this to migrate the
	// singleton secret to the workspace-scoped key. Empty when no migration ran.
	migratedToWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("linear schema init: %w", err)
	}
	return s, nil
}

// MigratedFromWorkspace is kept for older provider call sites. It now returns
// the workspace_id that received a legacy singleton row during migration.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedToWorkspace
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS linear_configs (
		workspace_id TEXT PRIMARY KEY,
		auth_method TEXT NOT NULL,
		default_team_key TEXT NOT NULL DEFAULT '',
		org_slug TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS linear_issue_watches (
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
		-- Cap on concurrent open watcher-created tasks for this watch.
		-- NULL = uncapped. Positive integer = cap. Values <= 0 are rejected at
		-- the API layer. See docs/specs/throttle-watcher-fanout/.
		max_inflight_tasks INTEGER DEFAULT 5,
		-- Dispatch order for matched issues under the in-flight cap.
		-- '' = Linear default order (updatedAt asc). See linear/issue_sort.go.
		sort_by TEXT NOT NULL DEFAULT '',
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_linear_issue_watches_workspace
		ON linear_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS linear_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_identifier TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_identifier),
		FOREIGN KEY(issue_watch_id) REFERENCES linear_issue_watches(id) ON DELETE CASCADE
	);
`

// singletonID is the synthetic primary key used by the legacy install-wide
// linear_configs table.
const singletonID = "singleton"

func (s *Store) initSchema() error {
	if err := s.migrateLegacySingletonTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addConfigColumns(); err != nil {
		return err
	}
	if err := s.addMaxInflightTasksColumn(); err != nil {
		return err
	}
	if err := s.addIssueWatchSortByColumn(); err != nil {
		return err
	}
	if err := s.addIssueWatchLastErrorColumns(); err != nil {
		return err
	}
	if err := s.addIssueWatchRepositoryColumns(); err != nil {
		return err
	}
	return nil
}

// addConfigColumns brings old per-workspace tables up to the current
// workspace-scoped shape.
func (s *Store) addConfigColumns() error {
	cols, err := s.tableColumns("linear_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["org_slug"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_configs ADD COLUMN org_slug TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add org_slug column: %w", err)
		}
	}
	if _, ok := cols["last_checked_at"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_configs ADD COLUMN last_checked_at DATETIME`); err != nil {
			return fmt.Errorf("add last_checked_at column: %w", err)
		}
	}
	if _, ok := cols["last_ok"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_configs ADD COLUMN last_ok INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add last_ok column: %w", err)
		}
	}
	if _, ok := cols["last_error"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_configs ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add last_error column: %w", err)
		}
	}
	return nil
}

// addIssueWatchRepositoryColumns brings older databases up to the current
// schema by appending repository_id / base_branch to linear_issue_watches when
// missing. Both backfill to ” (unbound), so existing repo-less watches keep
// their behaviour. Fresh installs hit the column-already-present branch since
// createTablesSQL declares both columns. Idempotent — column lookup before each
// ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchRepositoryColumns() error {
	cols, err := s.tableColumns("linear_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["repository_id"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add repository_id column: %w", err)
		}
	}
	if _, ok := cols["base_branch"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add base_branch column: %w", err)
		}
	}
	return nil
}

// addMaxInflightTasksColumn brings older databases up to the current schema by
// adding the max_inflight_tasks column to linear_issue_watches when missing.
// Existing rows backfill to the default (5). A fresh install hits the
// column-already-present branch since createTablesSQL declares the column.
func (s *Store) addMaxInflightTasksColumn() error {
	cols, err := s.tableColumns("linear_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["max_inflight_tasks"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN max_inflight_tasks INTEGER DEFAULT 5`); err != nil {
		return fmt.Errorf("add max_inflight_tasks column: %w", err)
	}
	return nil
}

// addIssueWatchSortByColumn brings older databases up to the current schema by
// adding the sort_by column to linear_issue_watches when missing. Existing
// rows backfill to an empty string (Linear default order). Fresh installs hit the
// column-already-present branch since createTablesSQL declares it.
func (s *Store) addIssueWatchSortByColumn() error {
	cols, err := s.tableColumns("linear_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["sort_by"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN sort_by TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add sort_by column: %w", err)
	}
	return nil
}

// addIssueWatchLastErrorColumns brings older databases up to the current
// schema by appending last_error / last_error_at to linear_issue_watches when
// missing. Fresh installs hit the column-already-present branch since
// createTablesSQL declares both columns. Idempotent — column lookup before
// each ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchLastErrorColumns() error {
	cols, err := s.tableColumns("linear_issue_watches")
	if err != nil {
		return err
	}
	if _, ok := cols["last_error"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add last_error column: %w", err)
		}
	}
	if _, ok := cols["last_error_at"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE linear_issue_watches ADD COLUMN last_error_at DATETIME`); err != nil {
			return fmt.Errorf("add last_error_at column: %w", err)
		}
	}
	return nil
}

// migrateLegacySingletonTable detects the install-wide singleton schema and
// rewrites it into the workspace-scoped shape. Picks the active/default
// workspace as the target so startup is deterministic.
func (s *Store) migrateLegacySingletonTable() error {
	cols, err := s.tableColumns("linear_configs")
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

	healthCols := healthColumnsPresent(cols)
	_, hasOrgSlug := cols["org_slug"]
	selectCols := "auth_method, default_team_key"
	if hasOrgSlug {
		selectCols += ", org_slug"
	} else {
		selectCols += ", '' AS org_slug"
	}
	if healthCols {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	var authMethod, defaultTeamKey, orgSlug, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT `+selectCols+` FROM linear_configs WHERE id = ? LIMIT 1`, singletonID)
	switch err := row.Scan(&authMethod, &defaultTeamKey, &orgSlug,
		&lastCheckedAt, &lastOk, &lastError, &createdAt, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`DROP TABLE linear_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE linear_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE linear_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			default_team_key TEXT NOT NULL DEFAULT '',
			org_slug TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO linear_configs (workspace_id, auth_method, default_team_key, org_slug,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		targetWorkspace, authMethod.String, defaultTeamKey.String, orgSlug.String,
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

// healthColumnsPresent reports whether the legacy linear_configs table has the
// auth-health columns that were added in a later release. When all three are
// missing we fall back to NULL/zero defaults rather than crashing on the
// SELECT. Mirrors the helper of the same name in jira/store.go.
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

const selectConfigColumns = `workspace_id, auth_method, default_team_key, org_slug,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the default workspace Linear config, or nil when no row
// exists. New code should call GetConfigForWorkspace.
func (s *Store) GetConfig(ctx context.Context) (*LinearConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns the Linear config for a workspace, or nil when
// no row exists.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*LinearConfig, error) {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	var cfg LinearConfig
	err = s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM linear_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the default workspace config row. The last_* health
// columns and org_slug are deliberately not touched here; the poller owns
// those and writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *LinearConfig) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpsertConfigForWorkspace(ctx, workspaceID, cfg)
}

// UpsertConfigForWorkspace inserts or updates a workspace config row.
func (s *Store) UpsertConfigForWorkspace(ctx context.Context, workspaceID string, cfg *LinearConfig) error {
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
		INSERT INTO linear_configs (workspace_id, auth_method, default_team_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			auth_method = excluded.auth_method,
			default_team_key = excluded.default_team_key,
			updated_at = excluded.updated_at`,
		workspaceID, cfg.AuthMethod, cfg.DefaultTeamKey, cfg.CreatedAt, cfg.UpdatedAt)
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
	_, err = s.db.ExecContext(ctx, `DELETE FROM linear_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// HasConfig reports whether any config row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM linear_configs`)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// HasConfigForWorkspace reports whether a config row exists for one workspace.
func (s *Store) HasConfigForWorkspace(ctx context.Context, workspaceID string) (bool, error) {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return false, err
	}
	var present int
	err = s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM linear_configs WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// ListConfigWorkspaceIDs returns every workspace with a saved Linear config.
func (s *Store) ListConfigWorkspaceIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids, `SELECT workspace_id FROM linear_configs ORDER BY workspace_id`); err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateAuthHealth records the result of a credential probe. orgSlug is
// captured opportunistically from successful probes; pass "" to leave the
// existing slug unchanged.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg, orgSlug string, checkedAt time.Time) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpdateAuthHealthForWorkspace(ctx, workspaceID, ok, errMsg, orgSlug, checkedAt)
}

// UpdateAuthHealthForWorkspace records the result of a credential probe for a
// single workspace.
func (s *Store) UpdateAuthHealthForWorkspace(ctx context.Context, workspaceID string, ok bool, errMsg, orgSlug string, checkedAt time.Time) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	if orgSlug != "" {
		_, err = s.db.ExecContext(ctx, `
			UPDATE linear_configs
			SET last_checked_at = ?, last_ok = ?, last_error = ?, org_slug = ?
			WHERE workspace_id = ?`,
			checkedAt, ok, errMsg, orgSlug, workspaceID)
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE linear_configs
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
