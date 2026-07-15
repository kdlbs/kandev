package workflowsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
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
		updated_at DATETIME NOT NULL
	);
`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return s.addPollEnabledColumn()
}

// addPollEnabledColumn brings databases created before the poll toggle up to
// the current schema. Idempotent — the column lookup avoids the "duplicate
// column name" error on installs that already have it.
func (s *Store) addPollEnabledColumn() error {
	rows, err := s.ro.Query(`PRAGMA table_info(workflow_sync_configs)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "poll_enabled" {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE workflow_sync_configs ADD COLUMN poll_enabled INTEGER NOT NULL DEFAULT 1`)
	return err
}

const configSelectColumns = `workspace_id, repo_owner, repo_name, branch, path, interval_seconds,
	poll_enabled, last_synced_at, last_ok, last_error, last_warnings, last_hash, created_at, updated_at`

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
		&cfg.RepoOwner,
		&cfg.RepoName,
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
			workspace_id, repo_owner, repo_name, branch, path, interval_seconds, poll_enabled,
			last_synced_at, last_ok, last_error, last_warnings, last_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, 0, '', '[]', '', ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			repo_owner = excluded.repo_owner,
			repo_name = excluded.repo_name,
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
	`), workspaceID, req.RepoOwner, req.RepoName, req.Branch, req.Path, req.IntervalSeconds, boolToInt(req.PollEnabled != nil && *req.PollEnabled), now, now)
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
