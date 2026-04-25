package jira

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists Jira workspace configurations. Secret values are delegated to
// the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("jira schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS jira_configs (
		workspace_id TEXT PRIMARY KEY,
		site_url TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL,
		default_project_key TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
`

// addedColumns lists columns introduced after the initial schema. SQLite has no
// portable `ADD COLUMN IF NOT EXISTS`, so we ask sqlite_master for the existing
// CREATE TABLE statement and only ALTER when the column name is missing.
var addedColumns = []struct {
	name string
	sql  string
}{
	{"last_checked_at", "ALTER TABLE jira_configs ADD COLUMN last_checked_at DATETIME"},
	{"last_ok", "ALTER TABLE jira_configs ADD COLUMN last_ok INTEGER NOT NULL DEFAULT 0"},
	{"last_error", "ALTER TABLE jira_configs ADD COLUMN last_error TEXT NOT NULL DEFAULT ''"},
}

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return s.migrateAddedColumns()
}

// migrateAddedColumns applies ALTER TABLE statements for columns introduced
// after the initial schema. Existing databases created with the old CREATE
// TABLE need these columns backfilled; new databases already have them from
// createTablesSQL and the ALTERs are skipped.
func (s *Store) migrateAddedColumns() error {
	existing, err := s.tableColumns("jira_configs")
	if err != nil {
		return err
	}
	for _, col := range addedColumns {
		if _, ok := existing[col.name]; ok {
			continue
		}
		if _, err := s.db.Exec(col.sql); err != nil {
			return fmt.Errorf("add column %s: %w", col.name, err)
		}
	}
	return nil
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

const selectConfigColumns = `workspace_id, site_url, email, auth_method, default_project_key,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the Jira config for a workspace, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*JiraConfig, error) {
	var cfg JiraConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM jira_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the config row for a workspace. It never
// touches the secret store — callers must persist the token separately. The
// last_* health columns are deliberately not touched here; the poller owns
// those and writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *JiraConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jira_configs (workspace_id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			site_url = excluded.site_url,
			email = excluded.email,
			auth_method = excluded.auth_method,
			default_project_key = excluded.default_project_key,
			updated_at = excluded.updated_at`,
		cfg.WorkspaceID, cfg.SiteURL, cfg.Email, cfg.AuthMethod, cfg.DefaultProjectKey, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the Jira config row for a workspace. Secrets must be
// cleared separately by the caller.
func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jira_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// ListConfiguredWorkspaces returns the IDs of all workspaces that have a Jira
// config row. Used by the auth-health poller to know which workspaces to probe.
func (s *Store) ListConfiguredWorkspaces(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT workspace_id FROM jira_configs ORDER BY workspace_id`)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateAuthHealth records the result of a credential probe for a workspace.
// errMsg is the empty string when ok is true. If the workspace row no longer
// exists (e.g. the user removed the config concurrently with the poller), the
// update is a silent no-op rather than an error.
func (s *Store) UpdateAuthHealth(ctx context.Context, workspaceID string, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jira_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`,
		checkedAt, ok, errMsg, workspaceID)
	return err
}
