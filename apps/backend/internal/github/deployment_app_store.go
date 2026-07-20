package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	deploymentAppSingletonID             = 1
	deploymentAppRegistrationFlowHeadID  = 1
	DeploymentAppCredentialsSecretID     = "github:deployment-app:credentials"
	DeploymentAppCredentialsSecretPrefix = DeploymentAppCredentialsSecretID + ":"
	DeploymentAppCredentialBundleVersion = 1
	deploymentAppCredentialsSecretName   = "GitHub deployment App credentials"
)

// ErrDeploymentAppFlowUnavailable means registration state is missing, expired, or consumed.
var ErrDeploymentAppFlowUnavailable = errors.New("deployment GitHub App flow is expired, consumed, or missing")

// ErrDeploymentAppInUse prevents replacing or deleting the deployment App
// while a workspace remains bound to one of its installations.
var ErrDeploymentAppInUse = errors.New("deployment GitHub App is used by a workspace")

// DeploymentAppCredentials is the generated secret material kept outside the metadata store.
type DeploymentAppCredentials struct {
	PrivateKey    string `json:"private_key"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
}

// DeploymentAppCredentialBundle keeps all generated secrets in one encrypted generation.
type DeploymentAppCredentialBundle struct {
	Version     int                      `json:"version"`
	Generation  int64                    `json:"generation"`
	Credentials DeploymentAppCredentials `json:"credentials"`
}

// DeploymentAppRepository switches an atomic metadata pointer between
// immutable encrypted credential generations.
type DeploymentAppRepository struct {
	store   *Store
	secrets ConnectionSecretStore
}

func NewDeploymentAppRepository(store *Store, secrets ConnectionSecretStore) *DeploymentAppRepository {
	return &DeploymentAppRepository{store: store, secrets: secrets}
}

const deploymentAppRegistrationSelect = `
	SELECT github_host, app_id, client_id, slug, owner_login, owner_type,
		public_base_url, credential_generation, credential_secret_id, webhook_status, last_webhook_at,
		COALESCE(last_error, '') AS last_error, created_at, updated_at
	FROM github_app_registration WHERE singleton_id = ?`

// GetDeploymentAppRegistration returns the managed deployment registration, if present.
func (s *Store) GetDeploymentAppRegistration(ctx context.Context) (*DeploymentAppRegistration, error) {
	var registration DeploymentAppRegistration
	err := s.ro.GetContext(ctx, &registration,
		s.ro.Rebind(deploymentAppRegistrationSelect), deploymentAppSingletonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &registration, err
}

// UpsertDeploymentAppRegistration creates or replaces singleton non-secret metadata.
func (s *Store) UpsertDeploymentAppRegistration(
	ctx context.Context,
	registration *DeploymentAppRegistration,
) error {
	if registration == nil {
		return fmt.Errorf("deployment App registration is required")
	}
	now := time.Now().UTC()
	if registration.CreatedAt.IsZero() {
		registration.CreatedAt = now
	}
	registration.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_app_registration (
			singleton_id, github_host, app_id, client_id, slug, owner_login, owner_type,
			public_base_url, credential_generation, credential_secret_id, webhook_status,
			last_webhook_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(singleton_id) DO UPDATE SET
			github_host = excluded.github_host,
			app_id = excluded.app_id,
			client_id = excluded.client_id,
			slug = excluded.slug,
			owner_login = excluded.owner_login,
			owner_type = excluded.owner_type,
			public_base_url = excluded.public_base_url,
			credential_generation = excluded.credential_generation,
			credential_secret_id = excluded.credential_secret_id,
			webhook_status = excluded.webhook_status,
			last_webhook_at = excluded.last_webhook_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`),
		deploymentAppSingletonID, registration.GitHubHost, registration.AppID,
		registration.ClientID, registration.Slug, registration.OwnerLogin, registration.OwnerType,
		registration.PublicBaseURL, registration.CredentialGeneration, registration.CredentialSecretID,
		registration.WebhookStatus, registration.LastWebhookAt, nullString(registration.LastError),
		registration.CreatedAt, registration.UpdatedAt,
	)
	return err
}

// DeleteDeploymentAppRegistration removes singleton non-secret metadata.
func (s *Store) DeleteDeploymentAppRegistration(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM github_app_registration WHERE singleton_id = ?`), deploymentAppSingletonID)
	return err
}

// CountDeploymentAppWorkspaceBindings returns workspaces using App installation auth.
func (s *Store) CountDeploymentAppWorkspaceBindings(ctx context.Context) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM github_workspace_connections WHERE source = ?`),
		ConnectionSourceGitHubAppInstallation)
	return count, err
}

// SaveManagedRegistration replaces the complete managed credential generation.
func (r *DeploymentAppRepository) SaveManagedRegistration(
	ctx context.Context,
	registration *DeploymentAppRegistration,
	credentials DeploymentAppCredentials,
) error {
	if r == nil || r.store == nil || r.secrets == nil || registration == nil {
		return errors.New("deployment App repository is not configured")
	}
	if err := validateDeploymentAppCredentials(credentials); err != nil {
		return err
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()

	previous, err := r.store.GetDeploymentAppRegistration(ctx)
	if err != nil {
		return err
	}
	bindings, err := r.store.CountDeploymentAppWorkspaceBindings(ctx)
	if err != nil {
		return err
	}
	if bindings > 0 {
		return ErrDeploymentAppInUse
	}
	if previous != nil {
		if registration.CredentialGeneration != previous.CredentialGeneration+1 {
			return errors.New("deployment App credential generation is not the next generation")
		}
	} else if registration.CredentialGeneration != 1 {
		return errors.New("initial deployment App credential generation must be 1")
	}
	return r.saveManagedRegistrationLocked(ctx, previous, registration, credentials)
}

func (r *DeploymentAppRepository) saveManagedRegistrationLocked(
	ctx context.Context,
	previous *DeploymentAppRegistration,
	registration *DeploymentAppRegistration,
	credentials DeploymentAppCredentials,
) error {
	secretID := deploymentAppCredentialSecretID(registration.CredentialGeneration)
	bundle := DeploymentAppCredentialBundle{
		Version: DeploymentAppCredentialBundleVersion, Generation: registration.CredentialGeneration,
		Credentials: credentials,
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode deployment App credential bundle: %w", err)
	}
	if err := r.secrets.Set(ctx, secretID,
		deploymentAppCredentialsSecretName, string(encoded)); err != nil {
		return fmt.Errorf("store deployment App credential bundle: %w", err)
	}
	next := *registration
	next.CredentialSecretID = secretID
	if err := r.store.UpsertDeploymentAppRegistration(ctx, &next); err != nil {
		return err
	}
	*registration = next
	if previous != nil && previous.CredentialSecretID != "" && previous.CredentialSecretID != secretID {
		_ = r.secrets.Delete(context.WithoutCancel(ctx), previous.CredentialSecretID)
	}
	return nil
}

// LoadManagedRegistration loads one complete managed generation.
func (r *DeploymentAppRepository) LoadManagedRegistration(
	ctx context.Context,
) (*DeploymentAppRegistration, DeploymentAppCredentials, error) {
	if r == nil || r.store == nil || r.secrets == nil {
		return nil, DeploymentAppCredentials{}, errors.New("deployment App repository is not configured")
	}
	registration, err := r.store.GetDeploymentAppRegistration(ctx)
	if err != nil || registration == nil {
		return registration, DeploymentAppCredentials{}, err
	}
	if registration.CredentialSecretID == "" {
		return registration, DeploymentAppCredentials{}, errors.New("deployment App credential pointer is missing")
	}
	encoded, err := r.secrets.Reveal(ctx, registration.CredentialSecretID)
	if err != nil {
		return registration, DeploymentAppCredentials{}, fmt.Errorf("load deployment App credential bundle: %w", err)
	}
	bundle, err := decodeDeploymentAppCredentialBundle(encoded)
	if err != nil {
		return registration, DeploymentAppCredentials{}, err
	}
	if bundle.Generation != registration.CredentialGeneration {
		return registration, DeploymentAppCredentials{}, errors.New("deployment App credential generation mismatch")
	}
	return registration, bundle.Credentials, nil
}

// DeleteManagedRegistration removes managed state only when no workspace uses it.
func (r *DeploymentAppRepository) DeleteManagedRegistration(ctx context.Context) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return errors.New("deployment App repository is not configured")
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()
	bindings, err := r.store.CountDeploymentAppWorkspaceBindings(ctx)
	if err != nil {
		return err
	}
	if bindings > 0 {
		return ErrDeploymentAppInUse
	}
	registration, err := r.store.GetDeploymentAppRegistration(ctx)
	if err != nil {
		return err
	}
	if err := r.store.DeleteDeploymentAppRegistration(ctx); err != nil {
		return err
	}
	if registration != nil && registration.CredentialSecretID != "" {
		_ = r.secrets.Delete(context.WithoutCancel(ctx), registration.CredentialSecretID)
	}
	return nil
}

// CleanupOrphanedCredentialBundles removes inactive generation-addressed bundles.
func (r *DeploymentAppRepository) CleanupOrphanedCredentialBundles(ctx context.Context) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return errors.New("deployment App repository is not configured")
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()
	registration, err := r.store.GetDeploymentAppRegistration(ctx)
	if err != nil {
		return err
	}
	activeID := ""
	if registration != nil {
		activeID = registration.CredentialSecretID
	}
	ids, err := r.secrets.ListIDs(ctx)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, id := range ids {
		if !isDeploymentAppCredentialSecretID(id) || id == activeID {
			continue
		}
		if err := r.secrets.Delete(ctx, id); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func deploymentAppCredentialSecretID(generation int64) string {
	return fmt.Sprintf("%sg%d:%s", DeploymentAppCredentialsSecretPrefix, generation, uuid.NewString())
}

func isDeploymentAppCredentialSecretID(id string) bool {
	return id == DeploymentAppCredentialsSecretID || strings.HasPrefix(id, DeploymentAppCredentialsSecretPrefix)
}

func decodeDeploymentAppCredentialBundle(encoded string) (DeploymentAppCredentialBundle, error) {
	var bundle DeploymentAppCredentialBundle
	if err := json.Unmarshal([]byte(encoded), &bundle); err != nil {
		return bundle, errors.New("deployment App credential bundle is invalid")
	}
	if bundle.Version != DeploymentAppCredentialBundleVersion || bundle.Generation < 1 {
		return bundle, errors.New("deployment App credential bundle version is unsupported")
	}
	if err := validateDeploymentAppCredentials(bundle.Credentials); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func validateDeploymentAppCredentials(credentials DeploymentAppCredentials) error {
	if strings.TrimSpace(credentials.PrivateKey) == "" || credentials.ClientSecret == "" ||
		credentials.WebhookSecret == "" {
		return errors.New("deployment App credentials are incomplete")
	}
	return nil
}

// CreateDeploymentAppRegistrationFlow atomically supersedes every prior flow
// and persists the new manifest registration head.
func (s *Store) CreateDeploymentAppRegistrationFlow(
	ctx context.Context,
	flow *DeploymentAppRegistrationFlow,
) error {
	if flow == nil {
		return fmt.Errorf("deployment App registration flow is required")
	}
	if flow.CreatedAt.IsZero() {
		flow.CreatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_app_registration_flows SET consumed_at = ?
		WHERE consumed_at IS NULL`), flow.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_app_registration_flows (
			state_hash, operator_user_id, owner_type, owner_login, public_base_url,
			manifest_revision, expires_at, consumed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		flow.StateHash, flow.OperatorUserID, flow.OwnerType, flow.OwnerLogin,
		flow.PublicBaseURL, flow.ManifestRevision, flow.ExpiresAt, flow.ConsumedAt, flow.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_app_registration_flow_head (singleton_id, state_hash, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(singleton_id) DO UPDATE SET
			state_hash = excluded.state_hash, updated_at = excluded.updated_at`),
		deploymentAppRegistrationFlowHeadID, flow.StateHash, flow.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// GetDeploymentAppRegistrationFlow returns registration state without consuming it.
func (s *Store) GetDeploymentAppRegistrationFlow(
	ctx context.Context,
	stateHash string,
) (*DeploymentAppRegistrationFlow, error) {
	var flow DeploymentAppRegistrationFlow
	err := s.ro.GetContext(ctx, &flow, s.ro.Rebind(`
		SELECT state_hash, operator_user_id, owner_type, owner_login, public_base_url,
			manifest_revision, expires_at, consumed_at, created_at
		FROM github_app_registration_flows WHERE state_hash = ?`), stateHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &flow, err
}

// ConsumeDeploymentAppRegistrationFlow atomically claims unexpired registration state.
func (s *Store) ConsumeDeploymentAppRegistrationFlow(
	ctx context.Context,
	stateHash string,
	now time.Time,
) (*DeploymentAppRegistrationFlow, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_registration_flows SET consumed_at = ?
		WHERE state_hash = ? AND consumed_at IS NULL AND expires_at > ?
			AND EXISTS (
				SELECT 1 FROM github_app_registration_flow_head
				WHERE singleton_id = ? AND state_hash = ?
			)`), now, stateHash, now, deploymentAppRegistrationFlowHeadID, stateHash)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrDeploymentAppFlowUnavailable
	}
	return s.GetDeploymentAppRegistrationFlow(ctx, stateHash)
}

// IsLatestDeploymentAppRegistrationFlow verifies that a consumed callback
// still belongs to the most recently persisted setup attempt.
func (s *Store) IsLatestDeploymentAppRegistrationFlow(
	ctx context.Context,
	stateHash string,
) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM github_app_registration_flow_head
		WHERE singleton_id = ? AND state_hash = ?`),
		deploymentAppRegistrationFlowHeadID, stateHash)
	return count == 1, err
}

// DeleteDeploymentAppRegistrationFlows removes all manifest flow state,
// including the persisted latest-attempt head.
func (s *Store) DeleteDeploymentAppRegistrationFlows(ctx context.Context) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_app_registration_flow_head`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_app_registration_flows`); err != nil {
		return err
	}
	return tx.Commit()
}
