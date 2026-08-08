package workflowsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// Store persists workspace-scoped workflow sync configuration.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("workflowsync schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS workflow_sync_configs (
		workspace_id TEXT PRIMARY KEY,
		repo_owner TEXT NOT NULL,
		repo_name TEXT NOT NULL,
		branch TEXT NOT NULL DEFAULT 'main',
		path TEXT NOT NULL DEFAULT '',
		interval_seconds INTEGER NOT NULL DEFAULT 300,
		poll_enabled INTEGER NOT NULL DEFAULT 1,
		last_synced_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_warnings TEXT NOT NULL DEFAULT '[]',
		last_hash TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		provider TEXT NOT NULL DEFAULT 'github',
		project_path TEXT NOT NULL DEFAULT ''
	);
`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addPollEnabledColumn(); err != nil {
		return err
	}
	return s.addProviderColumns()
}

// addProviderColumns brings databases created before GitLab sync up to the
// current schema. The 'github' default is what every pre-existing row
// implicitly was. Idempotent and race-safe, matching addPollEnabledColumn.
func (s *Store) addProviderColumns() error {
	statements := []string{
		`ALTER TABLE workflow_sync_configs ADD COLUMN provider TEXT NOT NULL DEFAULT 'github'`,
		`ALTER TABLE workflow_sync_configs ADD COLUMN project_path TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

// addPollEnabledColumn brings databases created before the poll toggle up to
// the current schema. Idempotent and race-safe: the ALTER always runs and a
// "duplicate column name" failure is swallowed via the shared helper.
func (s *Store) addPollEnabledColumn() error {
	_, err := s.db.Exec(`ALTER TABLE workflow_sync_configs ADD COLUMN poll_enabled INTEGER NOT NULL DEFAULT 1`)
	if err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	return nil
}

const configSelectColumns = `workspace_id, provider, repo_owner, repo_name, project_path, branch, path,
	interval_seconds, poll_enabled, last_synced_at, last_ok, last_error, last_warnings, last_hash,
	created_at, updated_at`

type configScanner interface {
	Scan(dest ...interface{}) error
}

func scanConfig(row configScanner) (*Config, error) {
	cfg := &Config{}
	var lastOk, pollEnabled int
	var lastSyncedAt sql.NullTime
	var warningsJSON string
	if err := row.Scan(
		&cfg.WorkspaceID,
		&cfg.Provider,
		&cfg.RepoOwner,
		&cfg.RepoName,
		&cfg.ProjectPath,
		&cfg.Branch,
		&cfg.Path,
		&cfg.IntervalSeconds,
		&pollEnabled,
		&lastSyncedAt,
		&lastOk,
		&cfg.LastError,
		&warningsJSON,
		&cfg.LastHash,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	); err != nil {
		return nil, err
	}
	cfg.LastOk = lastOk != 0
	cfg.PollEnabled = pollEnabled != 0
	// A row written before the provider column existed carries the implicit
	// GitHub meaning. The migration default covers the normal path; this
	// guards any row that still reads back empty.
	if cfg.Provider == "" {
		cfg.Provider = ProviderGitHub
	}
	if lastSyncedAt.Valid {
		t := lastSyncedAt.Time
		cfg.LastSyncedAt = &t
	}
	if warningsJSON != "" {
		// Corrupt JSON degrades to no warnings rather than failing the read.
		_ = json.Unmarshal([]byte(warningsJSON), &cfg.LastWarnings)
	}
	return cfg, nil
}

// GetConfigForWorkspace returns the config for a workspace, or (nil, nil)
// when none is stored.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+configSelectColumns+` FROM workflow_sync_configs WHERE workspace_id = ?
	`), workspaceID)
	cfg, err := scanConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// ListConfigs returns every stored config, for the background poller.
func (s *Store) ListConfigs(ctx context.Context) ([]*Config, error) {
	rows, err := s.ro.QueryContext(ctx, `SELECT `+configSelectColumns+` FROM workflow_sync_configs ORDER BY workspace_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var configs []*Config
	for rows.Next() {
		cfg, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// UpsertConfigForWorkspace creates or replaces a workspace's config. The sync
// status columns are reset so the next sync re-fetches and re-applies.
func (s *Store) UpsertConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*Config, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO workflow_sync_configs (
			workspace_id, provider, repo_owner, repo_name, project_path, branch, path,
			interval_seconds, poll_enabled,
			last_synced_at, last_ok, last_error, last_warnings, last_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 0, '', '[]', '', ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			provider = excluded.provider,
			repo_owner = excluded.repo_owner,
			repo_name = excluded.repo_name,
			project_path = excluded.project_path,
			branch = excluded.branch,
			path = excluded.path,
			interval_seconds = excluded.interval_seconds,
			poll_enabled = excluded.poll_enabled,
			last_synced_at = NULL,
			last_ok = 0,
			last_error = '',
			last_warnings = '[]',
			last_hash = '',
			updated_at = excluded.updated_at
	`), workspaceID, req.Provider, req.RepoOwner, req.RepoName, req.ProjectPath, req.Branch, req.Path,
		req.IntervalSeconds, boolToInt(req.PollEnabled != nil && *req.PollEnabled), now, now)
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// RecordSyncStatus persists the outcome of a sync attempt.
func (s *Store) RecordSyncStatus(ctx context.Context, workspaceID string, ok bool, errMsg string, warnings []string, hash string, at time.Time) error {
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		warningsJSON = []byte("[]")
	}
	okInt := 0
	if ok {
		okInt = 1
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE workflow_sync_configs
		SET last_synced_at = ?, last_ok = ?, last_error = ?, last_warnings = ?, last_hash = ?, updated_at = ?
		WHERE workspace_id = ?
	`), at, okInt, errMsg, string(warningsJSON), hash, at, workspaceID)
	return err
}

// DeleteConfigForWorkspace removes a workspace's config. Deleting a missing
// config is a no-op.
func (s *Store) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM workflow_sync_configs WHERE workspace_id = ?`), workspaceID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
