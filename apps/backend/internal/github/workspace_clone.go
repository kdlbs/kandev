package github

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// CopyWorkspaceConfigurationTx copies only workspace-owned settings and
// automation connections. The caller copies the PAT through the secret owner
// before committing this same transaction.
func (s *Store) CopyWorkspaceConfigurationTx(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (bool, error) {
	var connection WorkspaceConnection
	query := workspaceConnectionSelect + ` WHERE workspace_id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += ` FOR SHARE`
	}
	err := tx.GetContext(ctx, &connection, tx.Rebind(query), sourceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	hasConnection := err == nil
	if hasConnection {
		switch connection.Source {
		case ConnectionSourcePAT, ConnectionSourceGitHubAppInstallation, ConnectionSourceLegacyShared:
		case ConnectionSourceGHCLI:
			if err := requireGHCLIOperator(ctx); err != nil {
				return false, err
			}
		default:
			return false, repoerrors.ErrWorkspaceCloneConfiguration
		}
	}
	if err := s.copyCloneSettings(ctx, tx, sourceID, targetID); err != nil {
		return false, err
	}
	if !hasConnection {
		return false, nil
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_workspace_connections (workspace_id,source,github_host,login,installation_id,installation_account_login,installation_account_type,app_registration_id,status,credential_generation,last_error,created_at,updated_at)
 SELECT ?,source,github_host,login,installation_id,installation_account_login,installation_account_type,app_registration_id,status,1,NULL,?,? FROM github_workspace_connections WHERE workspace_id = ?`), targetID, now, now, sourceID)
	return connection.Source == ConnectionSourcePAT, err
}

func (s *Store) copyCloneSettings(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_workspace_settings (workspace_id,task_git_credentials_mode,repo_scope_mode,repo_scope_orgs,repo_scope_repos,saved_presets,default_query_presets,created_at,updated_at)
 SELECT ?,task_git_credentials_mode,repo_scope_mode,repo_scope_orgs,repo_scope_repos,saved_presets,default_query_presets,?,? FROM github_workspace_settings WHERE workspace_id = ?`), targetID, now, now, sourceID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_action_presets (workspace_id,pr_presets,issue_presets,updated_at) SELECT ?,pr_presets,issue_presets,? FROM github_action_presets WHERE workspace_id = ?`), targetID, now, sourceID)
	return err
}

// CopyWorkspaceConfigurationTx participates in an already authorized clone.
func (s *Service) CopyWorkspaceConfigurationTx(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errWorkspaceDefaultsUnavailable
	}
	return s.store.CopyWorkspaceConfigurationTx(ctx, tx, sourceID, targetID)
}
