package linear

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists Linear workspace configurations. Secret values are delegated
// to the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("linear schema init: %w", err)
	}
	return s, nil
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
`

// addedColumns lists columns introduced after the initial schema. Kept for
// parity with the Jira store so future column additions follow the same
// migration pattern; empty for now since the table ships fully-formed.
var addedColumns = []struct {
	name string
	sql  string
}{}

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return s.migrateAddedColumns()
}

// migrateAddedColumns applies ALTER TABLE statements for columns introduced
// after the initial schema. Existing databases need these columns backfilled;
// new databases already have them from createTablesSQL and the ALTERs are
// skipped.
func (s *Store) migrateAddedColumns() error {
	if len(addedColumns) == 0 {
		return nil
	}
	existing, err := s.tableColumns("linear_configs")
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

const selectConfigColumns = `workspace_id, auth_method, default_team_key, org_slug,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the Linear config for a workspace, or nil when no row
// exists.
func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*LinearConfig, error) {
	var cfg LinearConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM linear_configs WHERE workspace_id = ?`, workspaceID)
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
// last_* health columns and org_slug are deliberately not touched here; the
// poller owns those and writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *LinearConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO linear_configs (workspace_id, auth_method, default_team_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			auth_method = excluded.auth_method,
			default_team_key = excluded.default_team_key,
			updated_at = excluded.updated_at`,
		cfg.WorkspaceID, cfg.AuthMethod, cfg.DefaultTeamKey, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the Linear config row for a workspace. Secrets must be
// cleared separately by the caller.
func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM linear_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// ListConfiguredWorkspaces returns the IDs of all workspaces that have a
// Linear config row. Used by the auth-health poller.
func (s *Store) ListConfiguredWorkspaces(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT workspace_id FROM linear_configs ORDER BY workspace_id`)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateAuthHealth records the result of a credential probe. orgSlug is
// captured opportunistically from successful probes; pass "" to leave the
// existing slug unchanged. If the workspace row no longer exists, the update
// is a silent no-op.
func (s *Store) UpdateAuthHealth(ctx context.Context, workspaceID string, ok bool, errMsg, orgSlug string, checkedAt time.Time) error {
	if orgSlug != "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE linear_configs
			SET last_checked_at = ?, last_ok = ?, last_error = ?, org_slug = ?
			WHERE workspace_id = ?`,
			checkedAt, ok, errMsg, orgSlug, workspaceID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE linear_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`,
		checkedAt, ok, errMsg, workspaceID)
	return err
}
