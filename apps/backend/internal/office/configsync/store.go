package configsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists workspace-scoped config sync configuration and the
// ownership manifest that tracks which Office rows a sync currently manages.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("configsync schema init: %w", err)
	}
	return s, nil
}

// Writer exposes the underlying writable handle so the sync-owned skill
// writer (skillwriter.go) can run its guarded UPDATE against the same
// connection pool other Office repository writes use.
func (s *Store) Writer() *sqlx.DB { return s.db }

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS office_config_sync_configs (
		workspace_id TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT 'github',
		repo_owner TEXT NOT NULL DEFAULT '',
		repo_name TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT 'main',
		path TEXT NOT NULL DEFAULT '',
		interval_seconds INTEGER NOT NULL DEFAULT 300,
		poll_enabled INTEGER NOT NULL DEFAULT 1,
		last_synced_at TIMESTAMP,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_warnings TEXT NOT NULL DEFAULT '[]',
		last_hash TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS office_config_sync_manifest (
		workspace_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		entity_key TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		source_path TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (workspace_id, kind, entity_key)
	);
	CREATE INDEX IF NOT EXISTS idx_office_config_sync_manifest_entity
		ON office_config_sync_manifest (workspace_id, entity_id);
`

func (s *Store) initSchema() error {
	_, err := s.db.Exec(createTablesSQL)
	return err
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
	if lastSyncedAt.Valid {
		t := lastSyncedAt.Time
		cfg.LastSyncedAt = &t
	}
	if warningsJSON != "" {
		_ = json.Unmarshal([]byte(warningsJSON), &cfg.LastWarnings)
	}
	return cfg, nil
}

// GetConfigForWorkspace returns the config for a workspace, or (nil, nil)
// when none is stored.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT `+configSelectColumns+` FROM office_config_sync_configs WHERE workspace_id = ?
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
	rows, err := s.ro.QueryContext(ctx, `SELECT `+configSelectColumns+` FROM office_config_sync_configs ORDER BY workspace_id`)
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
		INSERT INTO office_config_sync_configs (
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
	`), workspaceID, req.Provider, req.RepoOwner, req.RepoName, req.ProjectPath, req.Branch, *req.Path,
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
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE office_config_sync_configs
		SET last_synced_at = ?, last_ok = ?, last_error = ?, last_warnings = ?, last_hash = ?, updated_at = ?
		WHERE workspace_id = ?
	`), at, boolToInt(ok), errMsg, string(warningsJSON), hash, at, workspaceID)
	return err
}

// DeleteConfigForWorkspace removes a workspace's config and its ownership
// manifest. Deleting a missing config is a no-op.
func (s *Store) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM office_config_sync_configs WHERE workspace_id = ?`), workspaceID); err != nil {
		return err
	}
	return s.DeleteManifestForWorkspace(ctx, workspaceID)
}

// ManifestEntry records that a specific Office row (identified by kind +
// entity_key, e.g. kind="agent", entity_key=agent name) is owned by config
// sync, plus which source file it was last materialized from. Membership in
// this table IS the "managed" flag: there is no separate ownership column on
// the underlying agent/skill/project/routine row.
type ManifestEntry struct {
	WorkspaceID string    `db:"workspace_id"`
	Kind        string    `db:"kind"`
	EntityKey   string    `db:"entity_key"`
	EntityID    string    `db:"entity_id"`
	SourcePath  string    `db:"source_path"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ListManifest returns every manifest row for a workspace.
func (s *Store) ListManifest(ctx context.Context, workspaceID string) ([]ManifestEntry, error) {
	var entries []ManifestEntry
	err := s.ro.SelectContext(ctx, &entries, s.ro.Rebind(`
		SELECT workspace_id, kind, entity_key, entity_id, source_path, updated_at
		FROM office_config_sync_manifest WHERE workspace_id = ?
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// UpsertManifestEntry records or refreshes ownership of one entity.
func (s *Store) UpsertManifestEntry(ctx context.Context, workspaceID, kind, entityKey, entityID, sourcePath string) error {
	return s.upsertManifestEntry(ctx, s.db, workspaceID, kind, entityKey, entityID, sourcePath)
}

// UpsertManifestEntryTx is UpsertManifestEntry scoped to a caller-owned
// transaction, letting reconcile write an entity and its ownership manifest
// row atomically (AC-OFFICE-CONFIG-SYNC-003.14). The transaction must have
// been started against the same writer connection pool this Store and the
// office repository share (see Writer()).
func (s *Store) UpsertManifestEntryTx(ctx context.Context, tx *sqlx.Tx, workspaceID, kind, entityKey, entityID, sourcePath string) error {
	return s.upsertManifestEntry(ctx, tx, workspaceID, kind, entityKey, entityID, sourcePath)
}

func (s *Store) upsertManifestEntry(
	ctx context.Context, ext sqlx.ExtContext, workspaceID, kind, entityKey, entityID, sourcePath string,
) error {
	now := time.Now().UTC()
	_, err := ext.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO office_config_sync_manifest (workspace_id, kind, entity_key, entity_id, source_path, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, kind, entity_key) DO UPDATE SET
			entity_id = excluded.entity_id,
			source_path = excluded.source_path,
			updated_at = excluded.updated_at
	`), workspaceID, kind, entityKey, entityID, sourcePath, now)
	return err
}

// DeleteManifestEntry removes ownership tracking for one entity. Deleting a
// missing entry is a no-op.
func (s *Store) DeleteManifestEntry(ctx context.Context, workspaceID, kind, entityKey string) error {
	return s.deleteManifestEntry(ctx, s.db, workspaceID, kind, entityKey)
}

// DeleteManifestEntryTx is DeleteManifestEntry scoped to a caller-owned
// transaction.
func (s *Store) DeleteManifestEntryTx(ctx context.Context, tx *sqlx.Tx, workspaceID, kind, entityKey string) error {
	return s.deleteManifestEntry(ctx, tx, workspaceID, kind, entityKey)
}

func (s *Store) deleteManifestEntry(ctx context.Context, ext sqlx.ExtContext, workspaceID, kind, entityKey string) error {
	_, err := ext.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM office_config_sync_manifest WHERE workspace_id = ? AND kind = ? AND entity_key = ?
	`), workspaceID, kind, entityKey)
	return err
}

// DeleteManifestForWorkspace removes every manifest row for a workspace.
func (s *Store) DeleteManifestForWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM office_config_sync_manifest WHERE workspace_id = ?`), workspaceID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
