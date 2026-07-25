package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAppRegistrationSchemaUsesCatalogInsteadOfSingleton(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, table := range []string{
		"github_app_registrations",
		"github_app_registration_flows",
	} {
		var count int
		if err := store.ro.GetContext(ctx, &count,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("read table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s count = %d, want 1", table, count)
		}
	}

	for _, table := range []string{
		"github_app_registration",
		"github_app_registration_flow_head",
	} {
		var count int
		if err := store.ro.GetContext(ctx, &count,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("read table %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("unpublished singleton table %s still exists", table)
		}
	}
}

func TestAppRegistrationStorePersistsMultipleRegistrations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	registrations := []*AppRegistration{
		newAppRegistration("registration-work", 101, "Work", now),
		newAppRegistration("registration-personal", 202, "Personal", now),
	}
	for _, registration := range registrations {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatalf("save registration %s: %v", registration.ID, err)
		}
	}

	for _, want := range registrations {
		var got AppRegistration
		err := store.ro.GetContext(ctx, &got, `
			SELECT id, source, display_name, github_host, app_id, client_id, slug, owner_login,
				owner_type, visibility, public_base_url, COALESCE(created_for_workspace_id, '') AS created_for_workspace_id,
				credential_generation, credential_secret_id, status, webhook_status, last_webhook_at,
				COALESCE(last_error, '') AS last_error, created_at, updated_at
			FROM github_app_registrations WHERE id = ?`, want.ID)
		if err != nil {
			t.Fatalf("get registration %s: %v", want.ID, err)
		}
		if got.ID != want.ID || got.AppID != want.AppID || got.DisplayName != want.DisplayName {
			t.Fatalf("registration %s = %+v, want %+v", want.ID, got, want)
		}
	}

	var got []*AppRegistration
	if err := store.ro.SelectContext(ctx, &got, `
		SELECT id, source, display_name, github_host, app_id, client_id, slug, owner_login,
			owner_type, visibility, public_base_url, COALESCE(created_for_workspace_id, '') AS created_for_workspace_id,
			credential_generation, credential_secret_id, status, webhook_status, last_webhook_at,
			COALESCE(last_error, '') AS last_error, created_at, updated_at
		FROM github_app_registrations ORDER BY id`); err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(got) != 2 || got[0].ID != "registration-personal" || got[1].ID != "registration-work" {
		t.Fatalf("registrations = %+v", got)
	}
}

func newAppRegistration(id string, appID int64, displayName string, now time.Time) *AppRegistration {
	return &AppRegistration{
		ID: id, Source: AppRegistrationSourceManaged, DisplayName: displayName,
		GitHubHost: "github.com", AppID: appID, ClientID: "Iv1.client", Slug: "kandev-" + id,
		OwnerLogin: "acme", OwnerType: AppRegistrationOwnerOrganization,
		Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example.com",
		CredentialGeneration: 1, CredentialSecretID: "github:app-registration:" + id + ":g1:test",
		Status: AppRegistrationStatusActive, WebhookStatus: DeploymentAppWebhookUnverified,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestAppRegistrationReferencesRoundTrip(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	registration := newAppRegistration("registration-1", 101, "Work", now)
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: "github.com", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: registration.ID, Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatalf("save workspace registration reference: %v", err)
	}
	workspace, err := store.GetWorkspaceConnection(ctx, "ws-1")
	if err != nil || workspace == nil || workspace.AppRegistrationID != registration.ID {
		t.Fatalf("workspace registration reference = %+v, err %v", workspace, err)
	}

	if err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", AppRegistrationID: registration.ID,
		GitHubUserID: 7, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}); err != nil {
		t.Fatalf("save personal registration reference: %v", err)
	}
	personal, err := store.GetUserConnection(ctx, "ws-1", "user-1")
	if err != nil || personal == nil || personal.AppRegistrationID != registration.ID {
		t.Fatalf("personal registration reference = %+v, err %v", personal, err)
	}

	flow := &AuthFlow{
		StateHash: "state", WorkspaceID: "ws-1", UserID: "user-1",
		AppRegistrationID: registration.ID, Kind: AuthFlowKindPersonal,
		ExpiresAt: now.Add(time.Hour),
	}
	if err := store.CreateAuthFlow(ctx, flow); err != nil {
		t.Fatalf("save auth flow registration reference: %v", err)
	}
	storedFlow, err := store.GetAuthFlow(ctx, flow.StateHash)
	if err != nil || storedFlow == nil || storedFlow.AppRegistrationID != registration.ID {
		t.Fatalf("auth flow registration reference = %+v, err %v", storedFlow, err)
	}
}

func TestPersonalConnectionRejectsDifferentWorkspaceRegistration(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC()
	work := newAppRegistration("registration-work", 101, "Work", now)
	personal := newAppRegistration("registration-personal", 202, "Personal", now)
	for _, registration := range []*AppRegistration{work, personal} {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: "github.com", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: work.ID, Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", AppRegistrationID: personal.ID,
		GitHubUserID: 7, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	})
	if !errors.Is(err, ErrOAuthFlowStale) {
		t.Fatalf("mismatched personal registration error = %v, want %v", err, ErrOAuthFlowStale)
	}
}

func TestAppRegistrationRepositoryKeepsCredentialBundlesIndependent(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	now := time.Now().UTC()
	work := newAppRegistration("registration-work", 101, "Work", now)
	personal := newAppRegistration("registration-personal", 202, "Personal", now)
	workCredentials := DeploymentAppCredentials{
		PrivateKey: "work-key", ClientSecret: "work-client", WebhookSecret: "work-webhook",
	}
	personalCredentials := DeploymentAppCredentials{
		PrivateKey: "personal-key", ClientSecret: "personal-client", WebhookSecret: "personal-webhook",
	}
	if err := repository.SaveRegistration(ctx, work, workCredentials); err != nil {
		t.Fatalf("save work registration: %v", err)
	}
	if err := repository.SaveRegistration(ctx, personal, personalCredentials); err != nil {
		t.Fatalf("save personal registration: %v", err)
	}
	if work.CredentialSecretID == personal.CredentialSecretID || len(secrets.values) != 2 {
		t.Fatalf("credential bundles are not independent: work=%q personal=%q values=%v",
			work.CredentialSecretID, personal.CredentialSecretID, secrets.values)
	}
	gotWork, gotWorkCredentials, err := repository.LoadRegistration(ctx, work.ID)
	if err != nil || gotWork == nil || gotWorkCredentials != workCredentials {
		t.Fatalf("loaded work registration = %+v, credentials = %+v, err %v",
			gotWork, gotWorkCredentials, err)
	}
	gotPersonal, gotPersonalCredentials, err := repository.LoadRegistration(ctx, personal.ID)
	if err != nil || gotPersonal == nil || gotPersonalCredentials != personalCredentials {
		t.Fatalf("loaded personal registration = %+v, credentials = %+v, err %v",
			gotPersonal, gotPersonalCredentials, err)
	}
}

func TestAppRegistrationDeleteIsRestrictedByWorkspaceAndPersonalReferences(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	now := time.Now().UTC()
	registration := newAppRegistration("registration-1", 101, "Work", now)
	credentials := DeploymentAppCredentials{
		PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook",
	}
	if err := repository.SaveRegistration(ctx, registration, credentials); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: "github.com", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: registration.ID, Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", AppRegistrationID: registration.ID,
		GitHubUserID: 7, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteRegistration(ctx, registration.ID); !errors.Is(err, ErrDeploymentAppInUse) {
		t.Fatalf("delete referenced registration error = %v, want %v", err, ErrDeploymentAppInUse)
	}
	if err := store.DeleteUserConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWorkspaceConnection(ctx, "ws-1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteRegistration(ctx, registration.ID); err != nil {
		t.Fatalf("delete unreferenced registration: %v", err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("registration credentials remain after delete: %v", secrets.values)
	}
}

func TestAppRegistrationDeletePreservesRegistrationWhenSecretDeleteFails(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	registration := newAppRegistration("registration-delete-failure", 101, "Work", time.Now().UTC())
	credentials := DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}
	if err := repository.SaveRegistration(ctx, registration, credentials); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("backend detail containing private-key")
	secrets.deleteErrKey = registration.CredentialSecretID

	err := repository.DeleteRegistration(ctx, registration.ID)
	if !errors.Is(err, ErrAppRegistrationCredentialCleanup) {
		t.Fatalf("DeleteRegistration() error = %v, want credential cleanup error", err)
	}
	if strings.Contains(err.Error(), "private-key") {
		t.Fatalf("DeleteRegistration() leaked secret-store detail: %v", err)
	}
	got, gotCredentials, loadErr := repository.LoadRegistration(ctx, registration.ID)
	if loadErr != nil || got == nil || got.Status != AppRegistrationStatusActive || gotCredentials != credentials {
		t.Fatalf("registration after failed delete = %+v, credentials %+v, err %v", got, gotCredentials, loadErr)
	}

	secrets.deleteErr = nil
	if err := repository.DeleteRegistration(ctx, registration.ID); err != nil {
		t.Fatalf("retry DeleteRegistration(): %v", err)
	}
}

func TestAppRegistrationDeleteRestoresAmbiguouslyDeletedSecret(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	registration := newAppRegistration("registration-ambiguous-delete", 101, "Work", time.Now().UTC())
	credentials := DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}
	if err := repository.SaveRegistration(ctx, registration, credentials); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("ambiguous delete")
	secrets.deleteErrKey = registration.CredentialSecretID
	secrets.deleteThenError = true

	err := repository.DeleteRegistration(ctx, registration.ID)
	if !errors.Is(err, ErrAppRegistrationCredentialCleanup) {
		t.Fatalf("DeleteRegistration() error = %v, want credential cleanup error", err)
	}
	got, gotCredentials, loadErr := repository.LoadRegistration(ctx, registration.ID)
	if loadErr != nil || got == nil || got.Status != AppRegistrationStatusActive || gotCredentials != credentials {
		t.Fatalf("registration after ambiguous delete = %+v, credentials %+v, err %v", got, gotCredentials, loadErr)
	}
}

func TestAppRegistrationDeleteRestoresSecretWhenMetadataDeleteFails(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	registration := newAppRegistration("registration-metadata-delete-failure", 101, "Work", time.Now().UTC())
	credentials := DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}
	if err := repository.SaveRegistration(ctx, registration, credentials); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER fail_app_registration_delete
		BEFORE DELETE ON github_app_registrations
		BEGIN SELECT RAISE(FAIL, 'database detail'); END`); err != nil {
		t.Fatal(err)
	}

	err := repository.DeleteRegistration(ctx, registration.ID)
	if !errors.Is(err, ErrAppRegistrationDeletionFailed) {
		t.Fatalf("DeleteRegistration() error = %v, want registration deletion error", err)
	}
	if strings.Contains(err.Error(), "database detail") {
		t.Fatalf("DeleteRegistration() leaked database detail: %v", err)
	}
	got, gotCredentials, loadErr := repository.LoadRegistration(ctx, registration.ID)
	if loadErr != nil || got == nil || got.Status != AppRegistrationStatusActive || gotCredentials != credentials {
		t.Fatalf("registration after metadata failure = %+v, credentials %+v, err %v", got, gotCredentials, loadErr)
	}
}

func TestAppRegistrationFlowsSupersedeOnlyWithinWorkspace(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1", "ws-2")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	newFlow := func(state, registrationID, workspaceID string, createdAt time.Time) *DeploymentAppRegistrationFlow {
		return &DeploymentAppRegistrationFlow{
			StateHash: state, RegistrationID: registrationID, WorkspaceID: workspaceID,
			UserID: "user-1", OwnerType: AppRegistrationOwnerOrganization,
			OwnerLogin: "acme", DisplayName: "Kandev", Visibility: AppRegistrationVisibilityPrivate,
			PublicBaseURL: "https://kandev.example.com", ManifestRevision: 1,
			ExpiresAt: createdAt.Add(time.Hour), CreatedAt: createdAt,
		}
	}
	first := newFlow("state-first", "registration-first", "ws-1", now)
	other := newFlow("state-other", "registration-other", "ws-2", now.Add(time.Second))
	replacement := newFlow("state-replacement", "registration-replacement", "ws-1", now.Add(2*time.Second))
	for _, flow := range []*DeploymentAppRegistrationFlow{first, other, replacement} {
		if err := store.CreateDeploymentAppRegistrationFlow(ctx, flow); err != nil {
			t.Fatalf("create flow %s: %v", flow.StateHash, err)
		}
	}
	storedFirst, err := store.GetDeploymentAppRegistrationFlow(ctx, first.StateHash)
	if err != nil || storedFirst == nil || storedFirst.ConsumedAt == nil {
		t.Fatalf("superseded first flow = %+v, err %v", storedFirst, err)
	}
	if _, err := store.ConsumeDeploymentAppRegistrationFlow(
		ctx, other.StateHash, other.RegistrationID, now.Add(3*time.Second),
	); err != nil {
		t.Fatalf("other workspace flow was superseded: %v", err)
	}
	consumed, err := store.ConsumeDeploymentAppRegistrationFlow(
		ctx, replacement.StateHash, replacement.RegistrationID, now.Add(3*time.Second),
	)
	if err != nil || consumed == nil || consumed.RegistrationID != replacement.RegistrationID {
		t.Fatalf("replacement flow = %+v, err %v", consumed, err)
	}
}

func TestAppRegistrationWebhookDeliveryIdentityIsComposite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, registration := range []*AppRegistration{
		newAppRegistration("registration-1", 101, "Work", now),
		newAppRegistration("registration-2", 202, "Personal", now),
	} {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	for _, registrationID := range []string{"registration-1", "registration-2"} {
		inserted, err := store.RecordWebhookDelivery(ctx, &WebhookDelivery{
			AppRegistrationID: registrationID, DeliveryID: "delivery-1", Event: "installation",
		})
		if err != nil || !inserted {
			t.Fatalf("record %s/delivery-1: inserted %v, err %v", registrationID, inserted, err)
		}
	}
	inserted, err := store.RecordWebhookDelivery(ctx, &WebhookDelivery{
		AppRegistrationID: "registration-1", DeliveryID: "delivery-1", Event: "installation",
	})
	if err != nil || inserted {
		t.Fatalf("duplicate registration delivery: inserted %v, err %v", inserted, err)
	}
}

func TestAppRegistrationSchemaDropsUnpublishedSingletonState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`
		CREATE TABLE github_app_registration (
			singleton_id INTEGER PRIMARY KEY,
			github_host TEXT NOT NULL,
			app_id BIGINT NOT NULL
		);
		INSERT INTO github_app_registration (singleton_id, github_host, app_id)
		VALUES (1, 'github.com', 999)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("replay store with unpublished singleton: %v", err)
	}
	var catalogCount int
	if err := store.ro.GetContext(ctx, &catalogCount, `SELECT COUNT(*) FROM github_app_registrations`); err != nil {
		t.Fatal(err)
	}
	if catalogCount != 0 {
		t.Fatalf("unpublished singleton migrated into catalog: count = %d", catalogCount)
	}
	var singletonCount int
	if err := store.ro.GetContext(ctx, &singletonCount,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'github_app_registration'`); err != nil {
		t.Fatal(err)
	}
	if singletonCount != 0 {
		t.Fatalf("unpublished singleton still exists: count = %d", singletonCount)
	}
}

func TestPersonalConnectionDatabaseRejectsDifferentWorkspaceRegistration(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC()
	for _, registration := range []*AppRegistration{
		newAppRegistration("registration-work", 101, "Work", now),
		newAppRegistration("registration-personal", 202, "Personal", now),
	} {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: defaultGitHubHost, InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: "registration-work", Status: ConnectionStatusActive,
		CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO github_user_connections (
			workspace_id, user_id, app_registration_id, github_user_id, login, status,
			access_expires_at, credential_generation, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ws-1", "user-1", "registration-personal", 7, "octocat", "active",
		now.Add(time.Hour), 1, now, now)
	if err == nil || !strings.Contains(err.Error(), "must match workspace") {
		t.Fatalf("mismatched direct insert error = %v", err)
	}
}

func TestWorkspaceConnectionDatabaseRejectsSwitchWithPersonalConnection(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	ctx := context.Background()
	now := time.Now().UTC()
	for _, registration := range []*AppRegistration{
		newAppRegistration("registration-work", 101, "Work", now),
		newAppRegistration("registration-personal", 202, "Personal", now),
	} {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: defaultGitHubHost, InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: "registration-work", Status: ConnectionStatusActive,
		CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "ws-1", UserID: "user-1", AppRegistrationID: "registration-work",
		GitHubUserID: 7, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := store.db.ExecContext(ctx, `
		UPDATE github_workspace_connections
		SET app_registration_id = ?
		WHERE workspace_id = ?`, "registration-personal", "ws-1")
	if err == nil || !strings.Contains(err.Error(), "must match personal") {
		t.Fatalf("direct workspace switch error = %v", err)
	}

	_, err = store.db.ExecContext(ctx,
		`DELETE FROM github_workspace_connections WHERE workspace_id = ?`, "ws-1")
	if err == nil || !strings.Contains(err.Error(), "still has personal") {
		t.Fatalf("direct workspace disconnect error = %v", err)
	}
}

func TestAppRegistrationImportPreparationIsExactAndSingleUse(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	preparation := &AppRegistrationImportPreparation{
		RegistrationID: "d8fcfe25-6dbc-45d2-8d11-8a0c1f022c85",
		WorkspaceID:    "workspace-1", UserID: DefaultUserID,
		PublicBaseURL: "https://kandev.example", ExpiresAt: now.Add(time.Minute), CreatedAt: now,
	}
	if err := store.CreateAppRegistrationImportPreparation(ctx, preparation); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []struct {
		workspaceID, userID, publicBaseURL string
	}{
		{"wrong-workspace", DefaultUserID, preparation.PublicBaseURL},
		{preparation.WorkspaceID, "wrong-user", preparation.PublicBaseURL},
		{preparation.WorkspaceID, DefaultUserID, "https://other.example"},
	} {
		if _, err := store.ConsumeAppRegistrationImportPreparation(
			ctx, preparation.RegistrationID, attempt.workspaceID, attempt.userID, attempt.publicBaseURL, now,
		); !errors.Is(err, ErrAppRegistrationImportPreparationUnavailable) {
			t.Fatalf("wrong binding error = %v", err)
		}
	}
	consumed, err := store.ConsumeAppRegistrationImportPreparation(
		ctx, preparation.RegistrationID, preparation.WorkspaceID, preparation.UserID,
		preparation.PublicBaseURL, now,
	)
	if err != nil || consumed == nil || consumed.ConsumedAt == nil {
		t.Fatalf("consume = %+v, err %v", consumed, err)
	}
	if _, err := store.ConsumeAppRegistrationImportPreparation(
		ctx, preparation.RegistrationID, preparation.WorkspaceID, preparation.UserID,
		preparation.PublicBaseURL, now,
	); !errors.Is(err, ErrAppRegistrationImportPreparationUnavailable) {
		t.Fatalf("replay error = %v", err)
	}
}

func TestAppRegistrationImportPreparationCannotOverwriteReservedID(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	ctx := context.Background()
	now := time.Now().UTC()
	registrationID := "e72d588e-e0cb-44d4-ae7d-61a6171d4d1a"
	preparation := &AppRegistrationImportPreparation{
		RegistrationID: registrationID, WorkspaceID: "workspace-1", UserID: DefaultUserID,
		PublicBaseURL: "https://kandev.example", ExpiresAt: now.Add(time.Minute), CreatedAt: now,
	}
	if err := store.CreateAppRegistrationImportPreparation(ctx, preparation); err != nil {
		t.Fatal(err)
	}
	existing := newAppRegistration(registrationID, 999, "Existing", now)
	if err := store.UpsertDeploymentAppRegistration(ctx, existing); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConsumeAppRegistrationImportPreparation(
		ctx, registrationID, preparation.WorkspaceID, preparation.UserID,
		preparation.PublicBaseURL, now,
	); !errors.Is(err, ErrAppRegistrationImportPreparationUnavailable) {
		t.Fatalf("existing ID consume error = %v", err)
	}
	stored, err := store.GetAppRegistration(ctx, registrationID)
	if err != nil || stored == nil || stored.AppID != 999 || stored.DisplayName != "Existing" {
		t.Fatalf("existing registration changed = %+v, err %v", stored, err)
	}
}

func TestCreateAppRegistrationNeverOverwritesExistingID(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	ctx := context.Background()
	now := time.Now().UTC()
	existing := newAppRegistration("registration-existing", 101, "Existing", now)
	existingCredentials := DeploymentAppCredentials{
		PrivateKey: "existing-key", ClientSecret: "existing-client", WebhookSecret: "existing-webhook",
	}
	if err := repository.SaveRegistration(ctx, existing, existingCredentials); err != nil {
		t.Fatal(err)
	}
	candidate := newAppRegistration(existing.ID, 202, "Replacement", now)
	err := repository.CreateRegistration(ctx, candidate, DeploymentAppCredentials{
		PrivateKey: "replacement-key", ClientSecret: "replacement-client", WebhookSecret: "replacement-webhook",
	})
	if err == nil {
		t.Fatal("create-only registration overwrote existing ID")
	}
	stored, credentials, err := repository.LoadRegistration(ctx, existing.ID)
	if err != nil || stored == nil || stored.AppID != existing.AppID || stored.DisplayName != existing.DisplayName ||
		credentials != existingCredentials {
		t.Fatalf("existing registration changed = %+v, credentials=%+v, err %v", stored, credentials, err)
	}
}
