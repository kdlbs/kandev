package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type configExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

const configSelectColumns = `workspace_id, host, auth_method, username,
	last_ok, last_error, last_checked_at, revision, created_at, updated_at`

// GetConfigForWorkspace returns one workspace's config or nil when absent.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*GitLabConfig, error) {
	var cfg GitLabConfig
	err := s.ro.GetContext(ctx, &cfg, `SELECT `+configSelectColumns+`
		FROM gitlab_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfigForWorkspace writes connection metadata without touching health.
func (s *Store) UpsertConfigForWorkspace(ctx context.Context, workspaceID string, cfg *GitLabConfig) error {
	now := time.Now().UTC()
	return upsertConfigForWorkspace(ctx, s.db, workspaceID, cfg, now)
}

func upsertConfigForWorkspace(ctx context.Context, execer configExecer, workspaceID string, cfg *GitLabConfig, now time.Time) error {
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.WorkspaceID = workspaceID
	cfg.UpdatedAt = now
	err := execer.QueryRowxContext(ctx, `
		INSERT INTO gitlab_configs (
			workspace_id, host, auth_method, username, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			host = excluded.host,
			auth_method = excluded.auth_method,
			username = excluded.username,
			updated_at = excluded.updated_at,
			revision = gitlab_configs.revision + 1
		RETURNING revision`,
		workspaceID, cfg.Host, cfg.AuthMethod, cfg.Username, cfg.CreatedAt, cfg.UpdatedAt).
		Scan(&cfg.Revision)
	return err
}

// SaveConfigForWorkspace atomically replaces metadata and records the
// successful credential probe. A failure in either statement leaves the
// previous row untouched.
func (s *Store) SaveConfigForWorkspace(ctx context.Context, workspaceID string, cfg *GitLabConfig) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	checkedAt := time.Now().UTC()
	if err := upsertConfigForWorkspace(ctx, tx, workspaceID, cfg, checkedAt); err != nil {
		return err
	}
	updated, err := updateConfigHealthForRevision(ctx, tx, workspaceID, cfg.Username, true, "", checkedAt, cfg.Revision)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("gitlab config revision changed during save")
	}
	return tx.Commit()
}

// DeleteConfigForWorkspace removes only the selected workspace connection.
func (s *Store) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// ListConfigWorkspaceIDs returns every workspace with a configured connection.
func (s *Store) ListConfigWorkspaceIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids, `SELECT workspace_id FROM gitlab_configs ORDER BY workspace_id`)
	return ids, err
}

// UpdateConfigHealthForRevision writes a probe result only when the connection
// identity is unchanged from the revision used to construct the client.
func (s *Store) UpdateConfigHealthForRevision(ctx context.Context, workspaceID, username string, ok bool, errMsg string, checkedAt time.Time, revision int64) (bool, error) {
	return updateConfigHealthForRevision(ctx, s.db, workspaceID, username, ok, errMsg, checkedAt, revision)
}

func updateConfigHealthForRevision(ctx context.Context, execer configExecer, workspaceID, username string, ok bool, errMsg string, checkedAt time.Time, revision int64) (bool, error) {
	result, err := execer.ExecContext(ctx, `
		UPDATE gitlab_configs
		SET username = ?, last_ok = ?, last_error = ?, last_checked_at = ?, updated_at = ?
		WHERE workspace_id = ? AND revision = ?`,
		username, ok, errMsg, checkedAt, checkedAt, workspaceID, revision)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// RestoreConfigForWorkspace restores an exact metadata snapshot after an
// external secret-store operation fails. It is only used as compensation.
func (s *Store) RestoreConfigForWorkspace(ctx context.Context, workspaceID string, cfg *GitLabConfig) error {
	if cfg == nil {
		return s.DeleteConfigForWorkspace(ctx, workspaceID)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_configs (
			workspace_id, host, auth_method, username, last_ok, last_error,
			last_checked_at, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			host = excluded.host,
			auth_method = excluded.auth_method,
			username = excluded.username,
			last_ok = excluded.last_ok,
			last_error = excluded.last_error,
			last_checked_at = excluded.last_checked_at,
			revision = excluded.revision,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`,
		workspaceID, cfg.Host, cfg.AuthMethod, cfg.Username, cfg.LastOK, cfg.LastError,
		cfg.LastCheckedAt, cfg.Revision, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// WorkspaceIDForTask resolves the authoritative workspace for a task-backed
// operation.
func (s *Store) WorkspaceIDForTask(ctx context.Context, taskID string) (string, error) {
	var workspaceID string
	err := s.ro.GetContext(ctx, &workspaceID, `SELECT workspace_id FROM tasks WHERE id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotConfigured
	}
	return workspaceID, err
}
