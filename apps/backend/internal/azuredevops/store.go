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

const selectConfigColumns = `workspace_id, organization_url, default_project_id,
	default_project_name, auth_method, last_checked_at, last_ok, last_error,
	created_at, updated_at`

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
	if _, err := store.db.Exec(createTaskPRTableSQL); err != nil {
		return nil, fmt.Errorf("azure devops task PR schema init: %w", err)
	}
	return store, nil
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
