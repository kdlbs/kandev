package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAuthFlowUnavailable means an OAuth/setup state cannot be consumed.
var ErrAuthFlowUnavailable = errors.New("github auth flow is expired, consumed, or missing")

const workspaceConnectionSelect = `
	SELECT workspace_id, source, github_host, COALESCE(login, '') AS login,
		installation_id, COALESCE(installation_account_login, '') AS installation_account_login,
		COALESCE(installation_account_type, '') AS installation_account_type,
		COALESCE(app_registration_id, '') AS app_registration_id,
		status, credential_generation, COALESCE(last_error, '') AS last_error,
		created_at, updated_at
	FROM github_workspace_connections`

const userConnectionSelect = `
	SELECT workspace_id, user_id, app_registration_id, github_user_id, login, status, access_expires_at,
		refresh_expires_at, credential_generation, COALESCE(last_error, '') AS last_error,
		created_at, updated_at
	FROM github_user_connections`

// WorkspaceConnectionHealth summarizes persisted connection state across all
// workspaces, including workspaces with no connection row.
type WorkspaceConnectionHealth struct {
	WorkspaceCount int
	Active         int
	Disconnected   int
	Invalid        int
	Suspended      int
	Revoked        int
}

// GetWorkspaceConnectionHealth returns aggregate workspace-owned GitHub
// health without consulting ambient process credentials.
func (s *Store) GetWorkspaceConnectionHealth(ctx context.Context) (WorkspaceConnectionHealth, error) {
	var health WorkspaceConnectionHealth
	err := s.ro.QueryRowxContext(ctx, `
		SELECT COUNT(w.id),
			COALESCE(SUM(CASE WHEN c.status = 'active' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.workspace_id IS NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status = 'invalid' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status = 'suspended' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN c.status = 'revoked' THEN 1 ELSE 0 END), 0)
		FROM workspaces w
		LEFT JOIN github_workspace_connections c ON c.workspace_id = w.id`).Scan(
		&health.WorkspaceCount,
		&health.Active,
		&health.Disconnected,
		&health.Invalid,
		&health.Suspended,
		&health.Revoked,
	)
	return health, err
}

// GetWorkspaceConnection returns a workspace's automation connection, if any.
func (s *Store) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*WorkspaceConnection, error) {
	var connection WorkspaceConnection
	err := s.ro.GetContext(ctx, &connection,
		s.ro.Rebind(workspaceConnectionSelect+` WHERE workspace_id = ?`), workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &connection, err
}

// ListWorkspaceConnectionsByInstallation returns every workspace bound to an installation.
func (s *Store) ListWorkspaceConnectionsByInstallation(
	ctx context.Context,
	installationID int64,
) ([]*WorkspaceConnection, error) {
	var connections []*WorkspaceConnection
	err := s.ro.SelectContext(ctx, &connections,
		s.ro.Rebind(workspaceConnectionSelect+` WHERE installation_id = ? ORDER BY workspace_id`), installationID)
	return connections, err
}

func (s *Store) ListWorkspaceConnectionsByAppInstallation(
	ctx context.Context,
	registrationID string,
	installationID int64,
) ([]*WorkspaceConnection, error) {
	var connections []*WorkspaceConnection
	err := s.ro.SelectContext(ctx, &connections, s.ro.Rebind(
		workspaceConnectionSelect+`
		WHERE app_registration_id = ? AND installation_id = ? ORDER BY workspace_id`),
		registrationID, installationID)
	return connections, err
}

// UpsertWorkspaceConnection creates or replaces workspace automation metadata.
func (s *Store) UpsertWorkspaceConnection(ctx context.Context, connection *WorkspaceConnection) error {
	if connection == nil {
		return fmt.Errorf("workspace connection is required")
	}
	now := time.Now().UTC()
	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = now
	}
	if connection.CredentialGeneration < 1 {
		connection.CredentialGeneration = 1
	}
	connection.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_workspace_connections (
			workspace_id, source, github_host, login, installation_id,
			installation_account_login, installation_account_type, app_registration_id, status,
			credential_generation, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			source = excluded.source,
			github_host = excluded.github_host,
			login = excluded.login,
			installation_id = excluded.installation_id,
			installation_account_login = excluded.installation_account_login,
			installation_account_type = excluded.installation_account_type,
			app_registration_id = excluded.app_registration_id,
			status = excluded.status,
			credential_generation = excluded.credential_generation,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`),
		connection.WorkspaceID, connection.Source, connection.GitHubHost, nullString(connection.Login),
		connection.InstallationID, nullString(connection.InstallationAccountLogin),
		nullString(connection.InstallationAccountType), nullString(connection.AppRegistrationID), connection.Status,
		connection.CredentialGeneration, nullString(connection.LastError), connection.CreatedAt, connection.UpdatedAt)
	return err
}

// TransitionWorkspaceInstallationConnection applies a webhook lifecycle change
// only while the workspace is still bound to the exact installation state that
// was listed by the webhook handler.
func (s *Store) TransitionWorkspaceInstallationConnection(
	ctx context.Context,
	expected, next *WorkspaceConnection,
) (bool, error) {
	if expected == nil || next == nil || expected.InstallationID == nil ||
		next.WorkspaceID != expected.WorkspaceID {
		return false, fmt.Errorf("expected and next installation connections are required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_workspace_connections
		SET installation_account_login = ?, installation_account_type = ?, status = ?,
			credential_generation = ?, last_error = ?, updated_at = ?
		WHERE workspace_id = ? AND source = ? AND installation_id = ?
			AND app_registration_id = ? AND credential_generation = ? AND status = ?`),
		nullString(next.InstallationAccountLogin), nullString(next.InstallationAccountType), next.Status,
		next.CredentialGeneration, nullString(next.LastError), now,
		expected.WorkspaceID, expected.Source, *expected.InstallationID,
		expected.AppRegistrationID, expected.CredentialGeneration, expected.Status,
	)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if count == 1 {
		next.UpdatedAt = now
	}
	return count == 1, err
}

// DeleteWorkspaceConnection removes workspace automation metadata.
func (s *Store) DeleteWorkspaceConnection(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM github_workspace_connections WHERE workspace_id = ?`), workspaceID)
	return err
}

// GetUserConnection returns a user's personal connection in a workspace, if any.
func (s *Store) GetUserConnection(ctx context.Context, workspaceID, userID string) (*UserConnection, error) {
	var connection UserConnection
	err := s.ro.GetContext(ctx, &connection,
		s.ro.Rebind(userConnectionSelect+` WHERE workspace_id = ? AND user_id = ?`), workspaceID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &connection, err
}

func (s *Store) GetUserConnectionGeneration(ctx context.Context, workspaceID, userID string) (int64, error) {
	var generation int64
	err := s.ro.GetContext(ctx, &generation, s.ro.Rebind(`
		SELECT credential_generation FROM github_user_connection_versions
		WHERE workspace_id = ? AND user_id = ?`), workspaceID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return generation, err
}

// ListUserConnectionsByGitHubUser returns bindings for one verified GitHub user.
func (s *Store) ListUserConnectionsByGitHubUser(
	ctx context.Context,
	githubUserID int64,
) ([]*UserConnection, error) {
	var connections []*UserConnection
	err := s.ro.SelectContext(ctx, &connections,
		s.ro.Rebind(userConnectionSelect+` WHERE github_user_id = ? ORDER BY workspace_id, user_id`), githubUserID)
	return connections, err
}

func (s *Store) ListUserConnectionsByAppGitHubUser(
	ctx context.Context,
	registrationID string,
	githubUserID int64,
) ([]*UserConnection, error) {
	var connections []*UserConnection
	err := s.ro.SelectContext(ctx, &connections, s.ro.Rebind(
		userConnectionSelect+`
		WHERE app_registration_id = ? AND github_user_id = ? ORDER BY workspace_id, user_id`),
		registrationID, githubUserID)
	return connections, err
}

// ListUserConnectionsByWorkspace returns all personal identities owned by a workspace.
func (s *Store) ListUserConnectionsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]*UserConnection, error) {
	var connections []*UserConnection
	err := s.ro.SelectContext(ctx, &connections,
		s.ro.Rebind(userConnectionSelect+` WHERE workspace_id = ? ORDER BY user_id`), workspaceID)
	return connections, err
}

// UpsertUserConnection creates or replaces personal connection metadata.
func (s *Store) UpsertUserConnection(ctx context.Context, connection *UserConnection) error {
	if connection == nil {
		return fmt.Errorf("user connection is required")
	}
	workspace, err := s.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace == nil || workspace.Source != ConnectionSourceGitHubAppInstallation ||
		workspace.AppRegistrationID == "" || workspace.AppRegistrationID != connection.AppRegistrationID {
		return ErrOAuthFlowStale
	}
	now := time.Now().UTC()
	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = now
	}
	if connection.CredentialGeneration < 1 {
		connection.CredentialGeneration = 1
	}
	connection.UpdatedAt = now
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_user_connections (
			workspace_id, user_id, app_registration_id, github_user_id, login, status, access_expires_at,
			refresh_expires_at, credential_generation, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET
			app_registration_id = excluded.app_registration_id,
			github_user_id = excluded.github_user_id,
			login = excluded.login,
			status = excluded.status,
			access_expires_at = excluded.access_expires_at,
			refresh_expires_at = excluded.refresh_expires_at,
			credential_generation = excluded.credential_generation,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`),
		connection.WorkspaceID, connection.UserID, connection.AppRegistrationID,
		connection.GitHubUserID, connection.Login,
		connection.Status, connection.AccessExpiresAt, connection.RefreshExpiresAt,
		connection.CredentialGeneration, nullString(connection.LastError), connection.CreatedAt, connection.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_user_connection_versions (workspace_id, user_id, credential_generation, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET
			credential_generation = MAX(github_user_connection_versions.credential_generation, excluded.credential_generation),
			updated_at = excluded.updated_at`),
		connection.WorkspaceID, connection.UserID, connection.CredentialGeneration, connection.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteUserConnection removes one user's personal workspace connection.
func (s *Store) DeleteUserConnection(ctx context.Context, workspaceID, userID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_user_connection_versions (workspace_id, user_id, credential_generation, updated_at)
		VALUES (?, ?, COALESCE((SELECT credential_generation + 1 FROM github_user_connections
			WHERE workspace_id = ? AND user_id = ?), 1), ?)
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET
			credential_generation = MAX(
				github_user_connection_versions.credential_generation + 1,
				excluded.credential_generation
			), updated_at = excluded.updated_at`),
		workspaceID, userID, workspaceID, userID, time.Now().UTC()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM github_user_connections WHERE workspace_id = ? AND user_id = ?`), workspaceID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteUserConnectionsByWorkspace removes all personal identities owned by a workspace.
func (s *Store) DeleteUserConnectionsByWorkspace(ctx context.Context, workspaceID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_user_connection_versions (workspace_id, user_id, credential_generation, updated_at)
		SELECT workspace_id, user_id, credential_generation + 1, ?
		FROM github_user_connections WHERE workspace_id = ?
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET
			credential_generation = MAX(
				github_user_connection_versions.credential_generation + 1,
				excluded.credential_generation
			), updated_at = excluded.updated_at`), time.Now().UTC(), workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM github_user_connections WHERE workspace_id = ?`), workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteWorkspaceAuthData removes workspace-owned GitHub auth metadata. Secret
// values use the deterministic keys in models.go and must be removed by the
// service in the same higher-level cleanup operation.
func (s *Store) DeleteWorkspaceAuthData(ctx context.Context, workspaceID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM github_app_registration_flows WHERE workspace_id = ?`,
		`DELETE FROM github_auth_flows WHERE workspace_id = ?`,
		`DELETE FROM github_user_connections WHERE workspace_id = ?`,
		`DELETE FROM github_user_connection_versions WHERE workspace_id = ?`,
		`DELETE FROM github_workspace_connections WHERE workspace_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(query), workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteAllMockAuthData resets the mock-only connection state without
// touching GitHub watches or repository fixtures.
func (s *Store) DeleteAllMockAuthData(ctx context.Context) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM github_auth_flows`,
		`DELETE FROM github_user_connections`,
		`DELETE FROM github_user_connection_versions`,
		`DELETE FROM github_workspace_connections`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateAuthFlow persists one short-lived OAuth/setup flow.
func (s *Store) CreateAuthFlow(ctx context.Context, flow *AuthFlow) error {
	if flow == nil {
		return fmt.Errorf("auth flow is required")
	}
	if flow.CreatedAt.IsZero() {
		flow.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_auth_flows (
			state_hash, workspace_id, user_id, app_registration_id, kind, pkce_verifier,
			expected_workspace_source, expected_workspace_generation, expected_installation_id,
			expected_workspace_app_registration_id, expected_personal_generation, expires_at, consumed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		flow.StateHash, flow.WorkspaceID, flow.UserID, flow.AppRegistrationID, flow.Kind, flow.PKCEVerifier,
		flow.ExpectedWorkspaceSource, flow.ExpectedWorkspaceGeneration, flow.ExpectedInstallationID,
		nullString(flow.ExpectedWorkspaceAppRegistrationID),
		flow.ExpectedPersonalGeneration,
		flow.ExpiresAt, flow.ConsumedAt, flow.CreatedAt)
	return err
}

// GetAuthFlow returns auth flow state without consuming it.
func (s *Store) GetAuthFlow(ctx context.Context, stateHash string) (*AuthFlow, error) {
	var flow AuthFlow
	err := s.ro.GetContext(ctx, &flow, s.ro.Rebind(`
		SELECT state_hash, workspace_id, user_id, app_registration_id, kind, pkce_verifier,
			expected_workspace_source, expected_workspace_generation, expected_installation_id,
			COALESCE(expected_workspace_app_registration_id, '') AS expected_workspace_app_registration_id,
			expected_personal_generation, expires_at, consumed_at, created_at
		FROM github_auth_flows WHERE state_hash = ?`), stateHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &flow, err
}

// ConsumeAuthFlow atomically claims one unexpired, unused flow.
func (s *Store) ConsumeAuthFlow(
	ctx context.Context,
	stateHash, registrationID string,
	kind AuthFlowKind,
	now time.Time,
) (*AuthFlow, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_auth_flows SET consumed_at = ?
		WHERE state_hash = ? AND app_registration_id = ? AND kind = ?
			AND consumed_at IS NULL AND expires_at > ?`), now, stateHash, registrationID, kind, now)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrAuthFlowUnavailable
	}
	return s.GetAuthFlow(ctx, stateHash)
}

// RecordWebhookDelivery returns false for a duplicate delivery ID.
func (s *Store) RecordWebhookDelivery(ctx context.Context, delivery *WebhookDelivery) (bool, error) {
	if delivery == nil {
		return false, fmt.Errorf("webhook delivery is required")
	}
	if delivery.Status == "" {
		delivery.Status = WebhookDeliveryStatusReceived
	}
	if delivery.ReceivedAt.IsZero() {
		delivery.ReceivedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_webhook_deliveries (
			app_registration_id, delivery_id, event, status, result, received_at, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_registration_id, delivery_id) DO NOTHING`),
		delivery.AppRegistrationID, delivery.DeliveryID, delivery.Event, delivery.Status, delivery.Result,
		delivery.ReceivedAt, delivery.ProcessedAt)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// ClaimWebhookDelivery inserts a new delivery or atomically reclaims a failed
// or stale in-flight delivery. Only one concurrent caller can acquire a retry.
func (s *Store) ClaimWebhookDelivery(
	ctx context.Context,
	delivery *WebhookDelivery,
	staleBefore time.Time,
) (WebhookDeliveryClaim, error) {
	if delivery == nil {
		return WebhookDeliveryClaim{}, fmt.Errorf("webhook delivery is required")
	}
	if delivery.ReceivedAt.IsZero() {
		delivery.ReceivedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_webhook_deliveries (
			app_registration_id, delivery_id, event, status, result, received_at, processed_at
		) VALUES (?, ?, ?, ?, '', ?, NULL)
		ON CONFLICT(app_registration_id, delivery_id) DO UPDATE SET
			event = excluded.event,
			status = excluded.status,
			result = '',
			received_at = excluded.received_at,
			processed_at = NULL
		WHERE github_webhook_deliveries.status = ?
			OR (github_webhook_deliveries.status = ? AND github_webhook_deliveries.received_at <= ?)`),
		delivery.AppRegistrationID, delivery.DeliveryID, delivery.Event,
		WebhookDeliveryStatusReceived, delivery.ReceivedAt,
		WebhookDeliveryStatusFailed, WebhookDeliveryStatusReceived, staleBefore,
	)
	if err != nil {
		return WebhookDeliveryClaim{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WebhookDeliveryClaim{}, err
	}
	if count == 1 {
		return WebhookDeliveryClaim{Acquired: true, Status: WebhookDeliveryStatusReceived}, nil
	}
	var status WebhookDeliveryStatus
	if err := s.ro.GetContext(ctx, &status, s.ro.Rebind(`
		SELECT status FROM github_webhook_deliveries
		WHERE app_registration_id = ? AND delivery_id = ?`),
		delivery.AppRegistrationID, delivery.DeliveryID); err != nil {
		return WebhookDeliveryClaim{}, err
	}
	return WebhookDeliveryClaim{Status: status}, nil
}

// CompleteWebhookDelivery records the terminal webhook processing result.
func (s *Store) CompleteWebhookDelivery(
	ctx context.Context,
	deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	var registrationID string
	err := s.ro.GetContext(ctx, &registrationID, s.ro.Rebind(`
		SELECT app_registration_id FROM github_webhook_deliveries WHERE delivery_id = ?
		GROUP BY app_registration_id HAVING COUNT(*) = 1`), deliveryID)
	if err != nil {
		return err
	}
	return s.CompleteAppRegistrationWebhookDelivery(
		ctx, registrationID, deliveryID, status, result, processedAt,
	)
}

func (s *Store) CompleteAppRegistrationWebhookDelivery(
	ctx context.Context,
	registrationID, deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_webhook_deliveries
		SET status = ?, result = ?, processed_at = ?
		WHERE app_registration_id = ? AND delivery_id = ?`),
		status, result, processedAt, registrationID, deliveryID)
	return err
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
