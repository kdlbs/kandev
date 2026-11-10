package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) addEnterpriseAuthFlowColumns() error {
	columns, err := s.tableColumns("github_auth_flows")
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"github_host", "TEXT NOT NULL DEFAULT 'github.com'"},
		{"oauth_policy", "TEXT NOT NULL DEFAULT 'pkce_s256'"},
		{"redirect_uri", "TEXT NOT NULL DEFAULT ''"},
		{"app_credential_generation", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if _, exists := columns[column.name]; exists {
			continue
		}
		if _, err := s.db.Exec("ALTER TABLE github_auth_flows ADD COLUMN " + column.name + " " + column.definition); err != nil {
			return fmt.Errorf("add GitHub authorization %s: %w", column.name, err)
		}
	}
	for _, table := range []string{"github_app_registration_flows", "github_app_import_preparations"} {
		columns, err := s.tableColumns(table)
		if err != nil {
			return err
		}
		if _, exists := columns["github_host"]; !exists {
			if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN github_host TEXT NOT NULL DEFAULT 'github.com'"); err != nil {
				return err
			}
		}
	}
	settingsColumns, err := s.tableColumns("github_workspace_settings")
	if err != nil {
		return err
	}
	if _, exists := settingsColumns["github_host"]; !exists {
		if _, err := s.db.Exec("ALTER TABLE github_workspace_settings ADD COLUMN github_host TEXT NOT NULL DEFAULT 'github.com'"); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceWorkspaceHostConnection commits host authority and its dependent settings together.
func (s *Store) ReplaceWorkspaceHostConnection(ctx context.Context, existing, replacement *WorkspaceConnection) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Disconnect preserves settings, so connection absence does not imply a
	// public-host workspace. Read retained authority inside the same transaction.
	retainedHost := defaultGitHubHost
	err = tx.GetContext(ctx, &retainedHost, s.db.Rebind("SELECT github_host FROM github_workspace_settings WHERE workspace_id = ?"), replacement.WorkspaceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := s.upsertWorkspaceConnection(ctx, tx, replacement); err != nil {
		return err
	}
	newHost := defaultAuthFlowHost(replacement.GitHubHost)
	if defaultAuthFlowHost(retainedHost) == newHost && (existing == nil || defaultAuthFlowHost(existing.GitHubHost) == newHost) {
		return tx.Commit()
	}
	now := time.Now().UTC()
	for _, table := range []string{"github_auth_flows", "github_app_registration_flows", "github_app_import_preparations"} {
		if _, err := tx.ExecContext(ctx, s.db.Rebind("UPDATE "+table+" SET consumed_at = ? WHERE workspace_id = ? AND consumed_at IS NULL"), now, replacement.WorkspaceID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM github_action_presets WHERE workspace_id = ?`), replacement.WorkspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO github_workspace_settings (workspace_id, github_host, task_git_credentials_mode,repo_scope_mode,repo_scope_orgs,repo_scope_repos,saved_presets,default_query_presets,created_at,updated_at)
 VALUES (?, ?, 'managed','repos','[]','[]','[]',NULL,?,?) ON CONFLICT(workspace_id) DO UPDATE SET github_host=excluded.github_host,repo_scope_mode='repos',repo_scope_orgs='[]',repo_scope_repos='[]',saved_presets='[]',default_query_presets=NULL,updated_at=excluded.updated_at`), replacement.WorkspaceID, replacement.GitHubHost, now, now); err != nil {
		return err
	}
	return tx.Commit()
}
