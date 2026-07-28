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
	AppRegistrationCredentialsSecretPrefix = "github:app-registration:"
	DeploymentAppCredentialsSecretPrefix   = AppRegistrationCredentialsSecretPrefix
	DeploymentAppCredentialBundleVersion   = 1
	appRegistrationCredentialsSecretName   = "GitHub App registration credentials"
)

var ErrDeploymentAppFlowUnavailable = errors.New("GitHub App registration flow is expired, consumed, or missing")
var ErrDeploymentAppInUse = errors.New("GitHub App registration is in use")
var ErrAppRegistrationImportPreparationUnavailable = errors.New("GitHub App import preparation is expired, consumed, or invalid")
var ErrAppRegistrationCredentialCleanup = errors.New("GitHub App credentials could not be deleted; retry the operation")
var ErrAppRegistrationDeletionFailed = errors.New("GitHub App registration could not be deleted; retry the operation")

type DeploymentAppCredentials struct {
	PrivateKey    string `json:"private_key"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
}

type DeploymentAppCredentialBundle struct {
	Version     int                      `json:"version"`
	Generation  int64                    `json:"generation"`
	Credentials DeploymentAppCredentials `json:"credentials"`
}

type DeploymentAppRepository struct {
	store   *Store
	secrets ConnectionSecretStore
}

type AppRegistrationRepository = DeploymentAppRepository

func NewDeploymentAppRepository(store *Store, secrets ConnectionSecretStore) *DeploymentAppRepository {
	return &DeploymentAppRepository{store: store, secrets: secrets}
}

func NewAppRegistrationRepository(store *Store, secrets ConnectionSecretStore) *AppRegistrationRepository {
	return NewDeploymentAppRepository(store, secrets)
}

const appRegistrationSelect = `
	SELECT id, source, display_name, github_host, app_id, client_id, slug, owner_login, owner_type,
		visibility, public_base_url, COALESCE(created_for_workspace_id, '') AS created_for_workspace_id,
		credential_generation, credential_secret_id, status, webhook_status, last_webhook_at,
		COALESCE(last_error, '') AS last_error, created_at, updated_at
	FROM github_app_registrations`

const appRegistrationInsertSQL = `
	INSERT INTO github_app_registrations (
		id, source, display_name, github_host, app_id, client_id, slug, owner_login, owner_type,
		visibility, public_base_url, created_for_workspace_id, credential_generation,
		credential_secret_id, status, webhook_status, last_webhook_at, last_error, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const appRegistrationUpsertSQL = appRegistrationInsertSQL + `
	ON CONFLICT(id) DO UPDATE SET
		source = excluded.source,
		display_name = excluded.display_name,
		github_host = excluded.github_host,
		app_id = excluded.app_id,
		client_id = excluded.client_id,
		slug = excluded.slug,
		owner_login = excluded.owner_login,
		owner_type = excluded.owner_type,
		visibility = excluded.visibility,
		public_base_url = excluded.public_base_url,
		created_for_workspace_id = excluded.created_for_workspace_id,
		credential_generation = excluded.credential_generation,
		credential_secret_id = excluded.credential_secret_id,
		status = excluded.status,
		webhook_status = excluded.webhook_status,
		last_webhook_at = excluded.last_webhook_at,
		last_error = excluded.last_error,
		updated_at = excluded.updated_at`

func (s *Store) GetAppRegistration(ctx context.Context, registrationID string) (*AppRegistration, error) {
	var registration AppRegistration
	err := s.ro.GetContext(ctx, &registration,
		s.ro.Rebind(appRegistrationSelect+` WHERE id = ?`), registrationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &registration, err
}

func (s *Store) GetAppRegistrationByGitHubApp(
	ctx context.Context,
	githubHost string,
	appID int64,
) (*AppRegistration, error) {
	var registration AppRegistration
	err := s.ro.GetContext(ctx, &registration,
		s.ro.Rebind(appRegistrationSelect+` WHERE github_host = ? AND app_id = ?`), githubHost, appID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &registration, err
}

func (s *Store) ListAppRegistrations(ctx context.Context) ([]*AppRegistration, error) {
	var registrations []*AppRegistration
	err := s.ro.SelectContext(ctx, &registrations, appRegistrationSelect+` ORDER BY display_name, id`)
	return registrations, err
}

func (s *Store) UpsertDeploymentAppRegistration(
	ctx context.Context,
	registration *DeploymentAppRegistration,
) error {
	arguments, err := appRegistrationWriteArguments(registration)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(appRegistrationUpsertSQL), arguments...)
	return err
}

func (s *Store) InsertAppRegistration(ctx context.Context, registration *AppRegistration) error {
	arguments, err := appRegistrationWriteArguments(registration)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(appRegistrationInsertSQL), arguments...)
	return err
}

func appRegistrationWriteArguments(registration *AppRegistration) ([]any, error) {
	if registration == nil || strings.TrimSpace(registration.ID) == "" {
		return nil, errors.New("GitHub App registration ID is required")
	}
	now := time.Now().UTC()
	if registration.CreatedAt.IsZero() {
		registration.CreatedAt = now
	}
	registration.UpdatedAt = now
	return []any{
		registration.ID, registration.Source, registration.DisplayName, registration.GitHubHost,
		registration.AppID, registration.ClientID, registration.Slug, registration.OwnerLogin,
		registration.OwnerType, registration.Visibility, registration.PublicBaseURL,
		nullString(registration.CreatedForWorkspaceID), registration.CredentialGeneration,
		registration.CredentialSecretID, registration.Status, registration.WebhookStatus,
		registration.LastWebhookAt, nullString(registration.LastError), registration.CreatedAt,
		registration.UpdatedAt,
	}, nil
}

func (s *Store) DeleteAppRegistration(ctx context.Context, registrationID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM github_app_registrations WHERE id = ?`), registrationID)
	return err
}

func (s *Store) CountAppRegistrationReferences(ctx context.Context, registrationID string) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT
			(SELECT COUNT(*) FROM github_workspace_connections WHERE app_registration_id = ?) +
			(SELECT COUNT(*) FROM github_user_connections WHERE app_registration_id = ?) +
			(SELECT COUNT(*) FROM github_auth_flows WHERE app_registration_id = ?)`),
		registrationID, registrationID, registrationID)
	return count, err
}

func (s *Store) CountAppRegistrationBindings(ctx context.Context, registrationID string) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT
			(SELECT COUNT(*) FROM github_workspace_connections WHERE app_registration_id = ?) +
			(SELECT COUNT(*) FROM github_user_connections WHERE app_registration_id = ?)`),
		registrationID, registrationID)
	return count, err
}

func (s *Store) CountAppRegistrationWorkspaceBindings(
	ctx context.Context,
	registrationID string,
) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM github_workspace_connections WHERE app_registration_id = ?`),
		registrationID)
	return count, err
}

func (s *Store) RenameAppRegistration(
	ctx context.Context,
	registrationID, displayName string,
) (*AppRegistration, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_registrations SET display_name = ?, updated_at = ? WHERE id = ?`),
		displayName, time.Now().UTC(), registrationID)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return nil, err
	}
	return s.GetAppRegistration(ctx, registrationID)
}

func (s *Store) DeleteAppRegistrationData(ctx context.Context, registrationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM github_auth_flows WHERE app_registration_id = ?`,
		`DELETE FROM github_webhook_deliveries WHERE app_registration_id = ?`,
		`DELETE FROM github_app_import_preparations WHERE registration_id = ?`,
		`DELETE FROM github_app_registration_flows WHERE registration_id = ?`,
		`DELETE FROM github_app_registrations WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(query), registrationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkAppRegistrationInvalid(
	ctx context.Context,
	registrationID, reason string,
) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_registrations
		SET status = ?, last_error = ?, updated_at = ?
		WHERE id = ?`), AppRegistrationStatusInvalid, reason, time.Now().UTC(), registrationID)
	return err
}

func (s *Store) CreateAppRegistrationImportPreparation(
	ctx context.Context,
	preparation *AppRegistrationImportPreparation,
) error {
	if preparation == nil || strings.TrimSpace(preparation.RegistrationID) == "" ||
		strings.TrimSpace(preparation.WorkspaceID) == "" || strings.TrimSpace(preparation.UserID) == "" ||
		strings.TrimSpace(preparation.PublicBaseURL) == "" {
		return errors.New("GitHub App import preparation is invalid")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_app_import_preparations SET consumed_at = ?
		WHERE workspace_id = ? AND user_id = ? AND consumed_at IS NULL`),
		preparation.CreatedAt, preparation.WorkspaceID, preparation.UserID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_app_import_preparations (
			registration_id, workspace_id, user_id, public_base_url, expires_at, consumed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		preparation.RegistrationID, preparation.WorkspaceID, preparation.UserID,
		preparation.PublicBaseURL, preparation.ExpiresAt, preparation.ConsumedAt, preparation.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetAppRegistrationImportPreparation(
	ctx context.Context,
	registrationID string,
) (*AppRegistrationImportPreparation, error) {
	var preparation AppRegistrationImportPreparation
	err := s.ro.GetContext(ctx, &preparation, s.ro.Rebind(`
		SELECT registration_id, workspace_id, user_id, public_base_url,
			expires_at, consumed_at, created_at
		FROM github_app_import_preparations WHERE registration_id = ?`), registrationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &preparation, err
}

func (s *Store) ConsumeAppRegistrationImportPreparation(
	ctx context.Context,
	registrationID, workspaceID, userID, publicBaseURL string,
	now time.Time,
) (*AppRegistrationImportPreparation, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_import_preparations SET consumed_at = ?
		WHERE registration_id = ? AND workspace_id = ? AND user_id = ? AND public_base_url = ?
			AND consumed_at IS NULL AND expires_at > ?
			AND NOT EXISTS (
				SELECT 1 FROM github_app_registrations registration
				WHERE registration.id = github_app_import_preparations.registration_id
			)`), now, registrationID, workspaceID, userID, publicBaseURL, now)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrAppRegistrationImportPreparationUnavailable
	}
	return s.GetAppRegistrationImportPreparation(ctx, registrationID)
}

func (s *Store) DeleteAppRegistrationImportPreparationsByWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	if strings.TrimSpace(workspaceID) == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM github_app_import_preparations`)
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM github_app_import_preparations WHERE workspace_id = ?`), workspaceID)
	return err
}

func (r *DeploymentAppRepository) SaveRegistration(
	ctx context.Context,
	registration *AppRegistration,
	credentials DeploymentAppCredentials,
) error {
	if r == nil || r.store == nil || r.secrets == nil || registration == nil {
		return errors.New("GitHub App registration repository is not configured")
	}
	if strings.TrimSpace(registration.ID) == "" {
		return errors.New("GitHub App registration ID is required")
	}
	if err := validateDeploymentAppCredentials(credentials); err != nil {
		return err
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()

	previous, err := r.store.GetAppRegistration(ctx, registration.ID)
	if err != nil {
		return err
	}
	if previous != nil && registration.CredentialGeneration != previous.CredentialGeneration+1 {
		return errors.New("GitHub App credential generation is not the next generation")
	}
	if previous == nil && registration.CredentialGeneration != 1 {
		return errors.New("initial GitHub App credential generation must be 1")
	}
	return r.saveRegistrationLocked(ctx, previous, registration, credentials)
}

// CreateRegistration persists a new registration and never updates an
// existing catalog ID, including when another writer wins after preflight.
func (r *DeploymentAppRepository) CreateRegistration(
	ctx context.Context,
	registration *AppRegistration,
	credentials DeploymentAppCredentials,
) error {
	if r == nil || r.store == nil || r.secrets == nil || registration == nil {
		return errors.New("GitHub App registration repository is not configured")
	}
	if strings.TrimSpace(registration.ID) == "" || registration.CredentialGeneration != 1 {
		return errors.New("new GitHub App registration is invalid")
	}
	if err := validateDeploymentAppCredentials(credentials); err != nil {
		return err
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()
	existing, err := r.store.GetAppRegistration(ctx, registration.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return errors.New("GitHub App registration already exists")
	}
	secretID := appRegistrationCredentialSecretID(registration.ID, registration.CredentialGeneration)
	bundle := DeploymentAppCredentialBundle{
		Version: DeploymentAppCredentialBundleVersion, Generation: registration.CredentialGeneration,
		Credentials: credentials,
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode GitHub App credential bundle: %w", err)
	}
	if err := r.secrets.Set(ctx, secretID, appRegistrationCredentialsSecretName, string(encoded)); err != nil {
		return fmt.Errorf("store GitHub App credential bundle: %w", err)
	}
	next := *registration
	next.CredentialSecretID = secretID
	if err := r.store.InsertAppRegistration(ctx, &next); err != nil {
		_ = r.secrets.Delete(context.WithoutCancel(ctx), secretID)
		return err
	}
	*registration = next
	return nil
}

func (r *DeploymentAppRepository) saveRegistrationLocked(
	ctx context.Context,
	previous *AppRegistration,
	registration *AppRegistration,
	credentials DeploymentAppCredentials,
) error {
	secretID := appRegistrationCredentialSecretID(registration.ID, registration.CredentialGeneration)
	bundle := DeploymentAppCredentialBundle{
		Version: DeploymentAppCredentialBundleVersion, Generation: registration.CredentialGeneration,
		Credentials: credentials,
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode GitHub App credential bundle: %w", err)
	}
	if err := r.secrets.Set(ctx, secretID, appRegistrationCredentialsSecretName, string(encoded)); err != nil {
		return fmt.Errorf("store GitHub App credential bundle: %w", err)
	}
	next := *registration
	next.CredentialSecretID = secretID
	if err := r.store.UpsertDeploymentAppRegistration(ctx, &next); err != nil {
		_ = r.secrets.Delete(context.WithoutCancel(ctx), secretID)
		return err
	}
	*registration = next
	if previous != nil && previous.CredentialSecretID != "" && previous.CredentialSecretID != secretID {
		_ = r.secrets.Delete(context.WithoutCancel(ctx), previous.CredentialSecretID)
	}
	return nil
}

func (r *DeploymentAppRepository) LoadRegistration(
	ctx context.Context,
	registrationID string,
) (*AppRegistration, DeploymentAppCredentials, error) {
	if r == nil || r.store == nil || r.secrets == nil {
		return nil, DeploymentAppCredentials{}, errors.New("GitHub App registration repository is not configured")
	}
	registration, err := r.store.GetAppRegistration(ctx, registrationID)
	if err != nil || registration == nil {
		return registration, DeploymentAppCredentials{}, err
	}
	if registration.CredentialSecretID == "" {
		return registration, DeploymentAppCredentials{}, errors.New("GitHub App credential pointer is missing")
	}
	encoded, err := r.secrets.Reveal(ctx, registration.CredentialSecretID)
	if err != nil {
		return registration, DeploymentAppCredentials{}, fmt.Errorf("load GitHub App credential bundle: %w", err)
	}
	bundle, err := decodeDeploymentAppCredentialBundle(encoded)
	if err != nil {
		return registration, DeploymentAppCredentials{}, err
	}
	if bundle.Generation != registration.CredentialGeneration {
		return registration, DeploymentAppCredentials{}, errors.New("GitHub App credential generation mismatch")
	}
	return registration, bundle.Credentials, nil
}

func (r *DeploymentAppRepository) DeleteRegistration(ctx context.Context, registrationID string) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return errors.New("GitHub App registration repository is not configured")
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()
	references, err := r.store.CountAppRegistrationBindings(ctx, registrationID)
	if err != nil {
		return err
	}
	if references > 0 {
		return ErrDeploymentAppInUse
	}
	registration, err := r.store.GetAppRegistration(ctx, registrationID)
	if err != nil {
		return err
	}
	if registration == nil {
		return r.store.DeleteAppRegistrationData(ctx, registrationID)
	}
	cleanupCtx := context.WithoutCancel(ctx)
	encoded, err := r.secrets.Reveal(cleanupCtx, registration.CredentialSecretID)
	if err != nil {
		return errors.Join(
			ErrAppRegistrationCredentialCleanup,
			r.invalidateRegistrationAfterCredentialFailure(cleanupCtx, registration.ID),
		)
	}
	if err := r.secrets.Delete(cleanupCtx, registration.CredentialSecretID); err != nil {
		if restoreErr := r.ensureRegistrationCredential(
			cleanupCtx, registration.CredentialSecretID, encoded,
		); restoreErr != nil {
			return errors.Join(
				ErrAppRegistrationCredentialCleanup,
				r.invalidateRegistrationAfterCredentialFailure(cleanupCtx, registration.ID),
			)
		}
		return ErrAppRegistrationCredentialCleanup
	}
	if err := r.store.DeleteAppRegistrationData(ctx, registrationID); err != nil {
		if restoreErr := r.secrets.Set(
			cleanupCtx, registration.CredentialSecretID,
			appRegistrationCredentialsSecretName, encoded,
		); restoreErr != nil {
			return errors.Join(
				ErrAppRegistrationDeletionFailed,
				ErrAppRegistrationCredentialCleanup,
				r.invalidateRegistrationAfterCredentialFailure(cleanupCtx, registration.ID),
			)
		}
		return ErrAppRegistrationDeletionFailed
	}
	return nil
}

func (r *DeploymentAppRepository) ensureRegistrationCredential(
	ctx context.Context,
	secretID, encoded string,
) error {
	exists, err := r.secrets.Exists(ctx, secretID)
	if err == nil && exists {
		return nil
	}
	return r.secrets.Set(ctx, secretID, appRegistrationCredentialsSecretName, encoded)
}

func (r *DeploymentAppRepository) invalidateRegistrationAfterCredentialFailure(
	ctx context.Context,
	registrationID string,
) error {
	if err := r.store.MarkAppRegistrationInvalid(
		ctx, registrationID, "GitHub App credentials are unavailable after a failed cleanup",
	); err != nil {
		return ErrAppRegistrationDeletionFailed
	}
	return nil
}

func (r *DeploymentAppRepository) CleanupOrphanedCredentialBundles(ctx context.Context) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return errors.New("GitHub App registration repository is not configured")
	}
	r.store.deploymentAppPersistenceMu.Lock()
	defer r.store.deploymentAppPersistenceMu.Unlock()
	registrations, err := r.store.ListAppRegistrations(ctx)
	if err != nil {
		return err
	}
	activeIDs := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		activeIDs[registration.CredentialSecretID] = struct{}{}
	}
	ids, err := r.secrets.ListIDs(ctx)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, id := range ids {
		if !isDeploymentAppCredentialSecretID(id) {
			continue
		}
		if _, active := activeIDs[id]; active {
			continue
		}
		if err := r.secrets.Delete(ctx, id); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func appRegistrationCredentialSecretID(registrationID string, generation int64) string {
	return fmt.Sprintf("%s%s:g%d:%s", AppRegistrationCredentialsSecretPrefix,
		registrationID, generation, uuid.NewString())
}

func isDeploymentAppCredentialSecretID(id string) bool {
	return strings.HasPrefix(id, AppRegistrationCredentialsSecretPrefix)
}

func decodeDeploymentAppCredentialBundle(encoded string) (DeploymentAppCredentialBundle, error) {
	var bundle DeploymentAppCredentialBundle
	if err := json.Unmarshal([]byte(encoded), &bundle); err != nil {
		return bundle, errors.New("GitHub App credential bundle is invalid")
	}
	if bundle.Version != DeploymentAppCredentialBundleVersion || bundle.Generation < 1 {
		return bundle, errors.New("GitHub App credential bundle version is unsupported")
	}
	if err := validateDeploymentAppCredentials(bundle.Credentials); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func validateDeploymentAppCredentials(credentials DeploymentAppCredentials) error {
	if strings.TrimSpace(credentials.PrivateKey) == "" || credentials.ClientSecret == "" ||
		credentials.WebhookSecret == "" {
		return errors.New("GitHub App credentials are incomplete")
	}
	return nil
}

func (s *Store) CreateDeploymentAppRegistrationFlow(
	ctx context.Context,
	flow *DeploymentAppRegistrationFlow,
) error {
	if flow == nil || flow.WorkspaceID == "" || flow.RegistrationID == "" {
		return fmt.Errorf("workspace-bound GitHub App registration flow is required")
	}
	if flow.UserID == "" {
		flow.UserID = flow.OperatorUserID
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
		WHERE workspace_id = ? AND consumed_at IS NULL`), flow.CreatedAt, flow.WorkspaceID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_app_registration_flows (
			state_hash, registration_id, workspace_id, user_id, owner_type, owner_login,
			display_name, visibility, public_base_url, manifest_revision, expires_at, consumed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		flow.StateHash, flow.RegistrationID, flow.WorkspaceID, flow.UserID, flow.OwnerType,
		flow.OwnerLogin, flow.DisplayName, flow.Visibility, flow.PublicBaseURL,
		flow.ManifestRevision, flow.ExpiresAt, flow.ConsumedAt, flow.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetDeploymentAppRegistrationFlow(
	ctx context.Context,
	stateHash string,
) (*DeploymentAppRegistrationFlow, error) {
	var flow DeploymentAppRegistrationFlow
	err := s.ro.GetContext(ctx, &flow, s.ro.Rebind(`
		SELECT state_hash, registration_id, workspace_id, user_id, owner_type, owner_login,
			display_name, visibility, public_base_url, manifest_revision, expires_at, consumed_at, created_at
		FROM github_app_registration_flows WHERE state_hash = ?`), stateHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	flow.OperatorUserID = flow.UserID
	return &flow, err
}

func (s *Store) ConsumeDeploymentAppRegistrationFlow(
	ctx context.Context,
	stateHash, registrationID string,
	now time.Time,
) (*DeploymentAppRegistrationFlow, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_registration_flows SET consumed_at = ?
		WHERE state_hash = ? AND registration_id = ? AND consumed_at IS NULL AND expires_at > ?`),
		now, stateHash, registrationID, now)
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

func (s *Store) IsLatestDeploymentAppRegistrationFlow(ctx context.Context, stateHash string) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM github_app_registration_flows candidate
		WHERE candidate.state_hash = ? AND NOT EXISTS (
			SELECT 1 FROM github_app_registration_flows newer
			WHERE newer.workspace_id = candidate.workspace_id AND newer.created_at > candidate.created_at
		)`), stateHash)
	return count == 1, err
}

func (s *Store) DeleteDeploymentAppRegistrationFlows(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_app_registration_flows`)
	return err
}

func (s *Store) DeleteAppRegistrationFlowsByWorkspace(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM github_app_registration_flows WHERE workspace_id = ?`), workspaceID)
	return err
}
