package azuredevops

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const createOAuthFlowTableSQL = `
	CREATE TABLE IF NOT EXISTS azure_devops_oauth_flows (
		state_hash TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT '',
		organization_url TEXT NOT NULL,
		default_project_id TEXT NOT NULL DEFAULT '',
		default_project_name TEXT NOT NULL DEFAULT '',
		tenant_id TEXT NOT NULL,
		client_id TEXT NOT NULL DEFAULT '',
		pkce_verifier TEXT NOT NULL,
		redirect_url TEXT NOT NULL,
		expires_at {{timestamp}} NOT NULL,
		consumed_at {{timestamp}},
		created_at {{timestamp}} NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_azure_devops_oauth_flows_workspace
		ON azure_devops_oauth_flows(workspace_id)`

// oauthFlow is one pending Microsoft sign-in attempt. Only the SHA-256 hash of
// its state is stored; the PKCE verifier never leaves the server.
type oauthFlow struct {
	StateHash          string     `db:"state_hash"`
	WorkspaceID        string     `db:"workspace_id"`
	UserID             string     `db:"user_id"`
	OrganizationURL    string     `db:"organization_url"`
	DefaultProjectID   string     `db:"default_project_id"`
	DefaultProjectName string     `db:"default_project_name"`
	TenantID           string     `db:"tenant_id"`
	ClientID           string     `db:"client_id"`
	PKCEVerifier       string     `db:"pkce_verifier"`
	RedirectURL        string     `db:"redirect_url"`
	ExpiresAt          time.Time  `db:"expires_at"`
	ConsumedAt         *time.Time `db:"consumed_at"`
	CreatedAt          time.Time  `db:"created_at"`
}

// oauthCredentialState is the non-secret projection of a workspace's
// Microsoft account credential.
type oauthCredentialState struct {
	Account        string
	TenantID       string
	ClientID       string
	RedirectURL    string
	TokenExpiresAt *time.Time
}

func (s *Store) initOAuthSchema() error {
	if _, err := s.db.Exec(schemaSQLForDriver(createOAuthFlowTableSQL, s.db.DriverName())); err != nil {
		return err
	}
	for column, statement := range map[string]string{
		"oauth_account":         `ALTER TABLE azure_devops_configs ADD COLUMN oauth_account TEXT NOT NULL DEFAULT ''`,
		"oauth_tenant_id":       `ALTER TABLE azure_devops_configs ADD COLUMN oauth_tenant_id TEXT NOT NULL DEFAULT ''`,
		"token_expires_at":      `ALTER TABLE azure_devops_configs ADD COLUMN token_expires_at {{timestamp}}`,
		"oauth_reauth_required": `ALTER TABLE azure_devops_configs ADD COLUMN oauth_reauth_required {{boolean}} NOT NULL DEFAULT FALSE`,
		"oauth_client_id":       `ALTER TABLE azure_devops_configs ADD COLUMN oauth_client_id TEXT NOT NULL DEFAULT ''`,
		"oauth_redirect_url":    `ALTER TABLE azure_devops_configs ADD COLUMN oauth_redirect_url TEXT NOT NULL DEFAULT ''`,
	} {
		if err := s.ensureConfigColumn(column, statement); err != nil {
			return err
		}
	}
	return nil
}

// InsertOAuthFlow records a pending sign-in and purges spent attempts.
func (s *Store) InsertOAuthFlow(ctx context.Context, flow *oauthFlow, now time.Time) error {
	if flow == nil {
		return errors.New("azure devops store: oauth flow is required")
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM azure_devops_oauth_flows
		WHERE expires_at <= ? OR consumed_at IS NOT NULL`), now); err != nil {
		return err
	}
	flow.CreatedAt = now
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO azure_devops_oauth_flows (
			state_hash, workspace_id, user_id, organization_url, default_project_id,
			default_project_name, tenant_id, client_id, pkce_verifier, redirect_url, expires_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		flow.StateHash, flow.WorkspaceID, flow.UserID, flow.OrganizationURL, flow.DefaultProjectID,
		flow.DefaultProjectName, flow.TenantID, flow.ClientID, flow.PKCEVerifier, flow.RedirectURL,
		flow.ExpiresAt, flow.CreatedAt)
	return err
}

// ConsumeOAuthFlow claims a live, unconsumed attempt exactly once and returns
// it in the same statement, so a concurrent purge cannot remove the claimed row
// before it is read. It returns nil when the state is unknown, expired, or
// already used.
func (s *Store) ConsumeOAuthFlow(ctx context.Context, stateHash string, now time.Time) (*oauthFlow, error) {
	var flow oauthFlow
	err := s.db.GetContext(ctx, &flow, s.db.Rebind(`
		UPDATE azure_devops_oauth_flows SET consumed_at = ?
		WHERE state_hash = ? AND consumed_at IS NULL AND expires_at > ?
		RETURNING state_hash, workspace_id, user_id, organization_url, default_project_id,
			default_project_name, tenant_id, client_id, pkce_verifier, redirect_url, expires_at,
			consumed_at, created_at`), now, stateHash, now)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &flow, nil
}

// DeleteOAuthFlowsByWorkspace removes every sign-in attempt of a workspace.
func (s *Store) DeleteOAuthFlowsByWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM azure_devops_oauth_flows WHERE workspace_id = ?`), workspaceID)
	return err
}

// SetOAuthCredentialState records a successful sign-in and clears any
// re-sign-in requirement.
func (s *Store) SetOAuthCredentialState(ctx context.Context, workspaceID string, state oauthCredentialState) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs
		SET oauth_account = ?, oauth_tenant_id = ?, oauth_client_id = ?, oauth_redirect_url = ?,
			token_expires_at = ?, oauth_reauth_required = FALSE
		WHERE workspace_id = ?`), state.Account, state.TenantID, state.ClientID, state.RedirectURL,
		state.TokenExpiresAt, workspaceID)
	return err
}

// UpdateOAuthTokenExpiry records the expiry of a renewed access token.
func (s *Store) UpdateOAuthTokenExpiry(ctx context.Context, workspaceID string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs SET token_expires_at = ? WHERE workspace_id = ?`), expiresAt, workspaceID)
	return err
}

// SetOAuthReauthRequired marks whether only a new sign-in can restore access.
func (s *Store) SetOAuthReauthRequired(ctx context.Context, workspaceID string, required bool) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs SET oauth_reauth_required = ? WHERE workspace_id = ?`), required, workspaceID)
	return err
}

// ClearOAuthCredentialState removes the Microsoft account projection.
func (s *Store) ClearOAuthCredentialState(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs
		SET oauth_account = '', oauth_tenant_id = '', oauth_client_id = '', oauth_redirect_url = '',
			token_expires_at = NULL, oauth_reauth_required = FALSE
		WHERE workspace_id = ?`), workspaceID)
	return err
}

// SetOAuthApplication records the Entra application a workspace signs in with.
func (s *Store) SetOAuthApplication(ctx context.Context, workspaceID, clientID, redirectURL string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs SET oauth_client_id = ?, oauth_redirect_url = ? WHERE workspace_id = ?`),
		clientID, redirectURL, workspaceID)
	return err
}

// MarkOAuthSignedOut clears the signed-in identity and token expiry and
// requires a new sign-in, keeping the tenant and application for it.
func (s *Store) MarkOAuthSignedOut(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE azure_devops_configs
		SET oauth_account = '', token_expires_at = NULL, oauth_reauth_required = TRUE
		WHERE workspace_id = ?`), workspaceID)
	return err
}
