package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestDeploymentAppStoreSchemaReplay(t *testing.T) {
	store := newTestStore(t)

	for _, table := range []string{
		"github_app_registration",
		"github_app_registration_flows",
		"github_app_registration_flow_head",
	} {
		var count int
		if err := store.ro.GetContext(context.Background(), &count,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("read table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s count = %d, want 1", table, count)
		}
	}

	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("replay deployment App schema: %v", err)
	}
}

func TestDeploymentAppStorePostgresSchemaReplay(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store := &Store{db: database, ro: database}
	if err := store.initDeploymentAppSchema(); err != nil {
		t.Fatalf("initialize deployment App Postgres schema: %v", err)
	}
	if err := store.initDeploymentAppSchema(); err != nil {
		t.Fatalf("replay deployment App Postgres schema: %v", err)
	}
	for _, table := range []string{
		"github_app_registration",
		"github_app_registration_flows",
		"github_app_registration_flow_head",
	} {
		var count int
		if err := database.Get(&count, database.Rebind(`
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = ?`), table); err != nil {
			t.Fatalf("read Postgres table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Postgres table %s count = %d, want 1", table, count)
		}
	}
}

func TestDeploymentAppStoreMigratesCredentialPointer(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.db.Exec(`
		DROP TABLE github_app_registration;
		CREATE TABLE github_app_registration (
			singleton_id INTEGER PRIMARY KEY, github_host TEXT NOT NULL, app_id BIGINT NOT NULL,
			client_id TEXT NOT NULL, slug TEXT NOT NULL, owner_login TEXT NOT NULL,
			owner_type TEXT NOT NULL, public_base_url TEXT NOT NULL,
			credential_generation BIGINT NOT NULL, webhook_status TEXT NOT NULL,
			last_webhook_at TIMESTAMP, last_error TEXT, created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		INSERT INTO github_app_registration (
			singleton_id, github_host, app_id, client_id, slug, owner_login, owner_type,
			public_base_url, credential_generation, webhook_status, created_at, updated_at
		) VALUES (1, 'github.com', 123, 'Iv1.client', 'kandev', 'acme', 'Organization',
			'https://kandev.example.com', 1, 'unverified', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatal(err)
	}
	replayed, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("migrate deployment App credential pointer: %v", err)
	}
	registration, err := replayed.GetDeploymentAppRegistration(context.Background())
	if err != nil || registration == nil || registration.CredentialSecretID != DeploymentAppCredentialsSecretID {
		t.Fatalf("migrated registration = %+v, err %v", registration, err)
	}
}

func TestDeploymentAppStoreFlowIsSingleUse(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	flow := &DeploymentAppRegistrationFlow{
		StateHash: "state-hash", OperatorUserID: "default-user",
		OwnerType: DeploymentAppOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example.com", ManifestRevision: 1,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := store.CreateDeploymentAppRegistrationFlow(ctx, flow); err != nil {
		t.Fatalf("create deployment App flow: %v", err)
	}
	got, err := store.ConsumeDeploymentAppRegistrationFlow(ctx, flow.StateHash, now)
	if err != nil || got == nil || got.ConsumedAt == nil || got.OperatorUserID != "default-user" {
		t.Fatalf("consume deployment App flow = %+v, err %v", got, err)
	}
	if _, err := store.ConsumeDeploymentAppRegistrationFlow(ctx, flow.StateHash, now); !errors.Is(err, ErrDeploymentAppFlowUnavailable) {
		t.Fatalf("reconsume deployment App flow error = %v, want %v", err, ErrDeploymentAppFlowUnavailable)
	}

	expired := *flow
	expired.StateHash = "expired-state"
	expired.ConsumedAt = nil
	expired.ExpiresAt = now
	if err := store.CreateDeploymentAppRegistrationFlow(ctx, &expired); err != nil {
		t.Fatalf("create expired deployment App flow: %v", err)
	}
	if _, err := store.ConsumeDeploymentAppRegistrationFlow(ctx, expired.StateHash, now); !errors.Is(err, ErrDeploymentAppFlowUnavailable) {
		t.Fatalf("consume expired deployment App flow error = %v, want %v", err, ErrDeploymentAppFlowUnavailable)
	}
}

func TestDeploymentAppStoreRegistrationRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	registration := &DeploymentAppRegistration{
		GitHubHost: "github.com", AppID: 123, ClientID: "Iv1.client", Slug: "kandev-test",
		OwnerLogin: "acme", OwnerType: DeploymentAppOwnerOrganization,
		PublicBaseURL: "https://kandev.example.com", CredentialGeneration: 2,
		CredentialSecretID: "github:deployment-app:credentials:g2:test",
		WebhookStatus:      DeploymentAppWebhookUnverified, CreatedAt: now, UpdatedAt: now,
	}

	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatalf("upsert deployment App registration: %v", err)
	}
	got, err := store.GetDeploymentAppRegistration(ctx)
	if err != nil {
		t.Fatalf("get deployment App registration: %v", err)
	}
	if got == nil || got.AppID != registration.AppID || got.OwnerType != registration.OwnerType ||
		got.CredentialGeneration != registration.CredentialGeneration {
		t.Fatalf("registration = %+v, want %+v", got, registration)
	}

	replacement := *registration
	replacement.AppID = 456
	replacement.CredentialGeneration++
	if err := store.UpsertDeploymentAppRegistration(ctx, &replacement); err != nil {
		t.Fatalf("replace deployment App registration: %v", err)
	}
	got, err = store.GetDeploymentAppRegistration(ctx)
	if err != nil || got == nil || got.AppID != replacement.AppID || got.CredentialGeneration != 3 {
		t.Fatalf("replaced registration = %+v, err %v", got, err)
	}
}

func TestDeploymentAppStorePersistsOneVersionedCredentialBundle(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	registration := testDeploymentAppRegistration(1)
	credentials := DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}

	if err := repository.SaveManagedRegistration(context.Background(), registration, credentials); err != nil {
		t.Fatalf("save managed registration: %v", err)
	}
	if len(secrets.values) != 1 {
		t.Fatalf("stored secret count = %d, want one bundle", len(secrets.values))
	}
	storedRegistration, err := store.GetDeploymentAppRegistration(context.Background())
	if err != nil || storedRegistration == nil {
		t.Fatalf("load stored registration: %+v, %v", storedRegistration, err)
	}
	if storedRegistration.CredentialSecretID == "" ||
		!strings.HasPrefix(storedRegistration.CredentialSecretID, DeploymentAppCredentialsSecretPrefix+"g1:") {
		t.Fatalf("credential secret ID = %q", storedRegistration.CredentialSecretID)
	}
	storedJSON := secrets.values[storedRegistration.CredentialSecretID]
	var stored DeploymentAppCredentialBundle
	if err := json.Unmarshal([]byte(storedJSON), &stored); err != nil {
		t.Fatalf("decode stored credential bundle: %v", err)
	}
	if stored.Version != DeploymentAppCredentialBundleVersion || stored.Generation != 1 ||
		stored.Credentials != credentials {
		t.Fatalf("stored credential bundle = %+v", stored)
	}

	gotRegistration, gotCredentials, err := repository.LoadManagedRegistration(context.Background())
	if err != nil || gotRegistration == nil || gotRegistration.AppID != registration.AppID ||
		gotCredentials != credentials {
		t.Fatalf("loaded registration = %+v, credentials = %+v, err %v", gotRegistration, gotCredentials, err)
	}
}

func TestDeploymentAppStoreMetadataFailureLeavesPreviousGenerationActive(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	previous := testDeploymentAppRegistration(1)
	previousCredentials := DeploymentAppCredentials{
		PrivateKey: "old-key", ClientSecret: "old-client", WebhookSecret: "old-webhook",
	}
	if err := repository.SaveManagedRegistration(context.Background(), previous, previousCredentials); err != nil {
		t.Fatal(err)
	}

	invalid := testDeploymentAppRegistration(2)
	invalid.OwnerType = "Enterprise"
	newCredentials := DeploymentAppCredentials{
		PrivateKey: "new-key", ClientSecret: "new-client", WebhookSecret: "new-webhook",
	}
	if err := repository.SaveManagedRegistration(context.Background(), invalid, newCredentials); err == nil {
		t.Fatal("expected invalid metadata write to fail")
	}
	gotRegistration, gotCredentials, err := repository.LoadManagedRegistration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotRegistration == nil || gotRegistration.CredentialGeneration != 1 || gotCredentials != previousCredentials {
		t.Fatalf("registration after failed replacement = %+v, credentials = %+v", gotRegistration, gotCredentials)
	}
	if len(secrets.values) != 2 {
		t.Fatalf("secret count after metadata failure = %d, want active plus orphan", len(secrets.values))
	}
	if err := repository.CleanupOrphanedCredentialBundles(context.Background()); err != nil {
		t.Fatalf("cleanup orphaned credential bundles: %v", err)
	}
	if len(secrets.values) != 1 {
		t.Fatalf("secret count after orphan cleanup = %d, want active only", len(secrets.values))
	}
}

func TestDeploymentAppStoreInitialSaveRejectsExistingAppBinding(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation, GitHubHost: "github.com",
		InstallationID: &installationID, InstallationAccountLogin: "acme",
		InstallationAccountType: "Organization", Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	err := repository.SaveManagedRegistration(context.Background(), testDeploymentAppRegistration(1),
		DeploymentAppCredentials{PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook"})
	if !errors.Is(err, ErrDeploymentAppInUse) {
		t.Fatalf("initial save error = %v, want %v", err, ErrDeploymentAppInUse)
	}
	registration, getErr := store.GetDeploymentAppRegistration(context.Background())
	if getErr != nil || registration != nil || len(secrets.values) != 0 {
		t.Fatalf("rejected initial save left registration = %+v, secrets = %v, err %v",
			registration, secrets.values, getErr)
	}
}

func TestDeploymentAppStoreCanceledMetadataWriteKeepsPreviousGeneration(t *testing.T) {
	store := newTestStore(t)
	baseSecrets := newFakeConnectionSecrets()
	secrets := &cancelAfterSetSecrets{fakeConnectionSecrets: baseSecrets}
	repository := NewDeploymentAppRepository(store, secrets)
	previousCredentials := DeploymentAppCredentials{
		PrivateKey: "old-key", ClientSecret: "old-client", WebhookSecret: "old-webhook",
	}
	if err := repository.SaveManagedRegistration(
		context.Background(), testDeploymentAppRegistration(1), previousCredentials,
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	secrets.cancel = cancel
	err := repository.SaveManagedRegistration(ctx, testDeploymentAppRegistration(2), DeploymentAppCredentials{
		PrivateKey: "new-key", ClientSecret: "new-client", WebhookSecret: "new-webhook",
	})
	if err == nil {
		t.Fatal("expected canceled metadata write to fail")
	}
	gotRegistration, gotCredentials, loadErr := repository.LoadManagedRegistration(context.Background())
	if loadErr != nil || gotRegistration == nil || gotRegistration.CredentialGeneration != 1 ||
		gotCredentials != previousCredentials {
		t.Fatalf("registration after cancellation = %+v, credentials = %+v, err %v",
			gotRegistration, gotCredentials, loadErr)
	}
	if len(baseSecrets.values) != 2 {
		t.Fatalf("secret count after cancellation = %d, want active plus safe orphan", len(baseSecrets.values))
	}
}

func TestDeploymentAppStoreDeletesPreviousBundleAfterMetadataSwitch(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	if err := repository.SaveManagedRegistration(context.Background(), testDeploymentAppRegistration(1),
		DeploymentAppCredentials{PrivateKey: "old", ClientSecret: "old", WebhookSecret: "old"}); err != nil {
		t.Fatal(err)
	}
	first, err := store.GetDeploymentAppRegistration(context.Background())
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if err := repository.SaveManagedRegistration(context.Background(), testDeploymentAppRegistration(2),
		DeploymentAppCredentials{PrivateKey: "new", ClientSecret: "new", WebhookSecret: "new"}); err != nil {
		t.Fatal(err)
	}
	second, err := store.GetDeploymentAppRegistration(context.Background())
	if err != nil || second == nil {
		t.Fatal(err)
	}
	if first.CredentialSecretID == second.CredentialSecretID {
		t.Fatal("replacement reused mutable credential bundle ID")
	}
	if _, exists := secrets.values[first.CredentialSecretID]; exists {
		t.Fatal("previous bundle remains after metadata switch")
	}
	if _, exists := secrets.values[second.CredentialSecretID]; !exists {
		t.Fatal("active bundle is missing after metadata switch")
	}
}

func TestDeploymentAppStoreGenerationMismatchFailsClosed(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	credentials := DeploymentAppCredentials{PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook"}
	if err := repository.SaveManagedRegistration(
		context.Background(), testDeploymentAppRegistration(1), credentials,
	); err != nil {
		t.Fatal(err)
	}
	registration, err := store.GetDeploymentAppRegistration(context.Background())
	if err != nil || registration == nil {
		t.Fatal(err)
	}
	bundle := DeploymentAppCredentialBundle{
		Version: DeploymentAppCredentialBundleVersion, Generation: 2, Credentials: credentials,
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	secrets.values[registration.CredentialSecretID] = string(encoded)
	if _, _, err := repository.LoadManagedRegistration(context.Background()); err == nil {
		t.Fatal("generation mismatch loaded instead of failing closed")
	}
}

func TestDeploymentAppStoreBundleFailurePreservesPreviousGeneration(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	previous := testDeploymentAppRegistration(1)
	previousCredentials := DeploymentAppCredentials{
		PrivateKey: "old-key", ClientSecret: "old-client", WebhookSecret: "old-webhook",
	}
	if err := repository.SaveManagedRegistration(context.Background(), previous, previousCredentials); err != nil {
		t.Fatal(err)
	}
	secrets.setErr = errors.New("encrypted store unavailable")
	if err := repository.SaveManagedRegistration(
		context.Background(), testDeploymentAppRegistration(2), DeploymentAppCredentials{
			PrivateKey: "new-key", ClientSecret: "new-client", WebhookSecret: "new-webhook",
		},
	); err == nil {
		t.Fatal("expected credential bundle write to fail")
	}
	secrets.setErr = nil
	gotRegistration, gotCredentials, err := repository.LoadManagedRegistration(context.Background())
	if err != nil || gotRegistration == nil || gotRegistration.CredentialGeneration != 1 ||
		gotCredentials != previousCredentials {
		t.Fatalf("registration after failed bundle = %+v, credentials = %+v, err %v",
			gotRegistration, gotCredentials, err)
	}
}

func TestDeploymentAppDeleteRejectsWorkspaceAppBindings(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-1")
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	registration := testDeploymentAppRegistration(1)
	credentials := DeploymentAppCredentials{
		PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook",
	}
	if err := repository.SaveManagedRegistration(context.Background(), registration, credentials); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourceGitHubAppInstallation, GitHubHost: "github.com",
		InstallationID: &installationID, InstallationAccountLogin: "acme",
		InstallationAccountType: "Organization", Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatal(err)
	}

	if err := repository.DeleteManagedRegistration(context.Background()); !errors.Is(err, ErrDeploymentAppInUse) {
		t.Fatalf("delete bound registration error = %v, want %v", err, ErrDeploymentAppInUse)
	}
	gotRegistration, gotCredentials, err := repository.LoadManagedRegistration(context.Background())
	if err != nil || gotRegistration == nil || gotRegistration.CredentialGeneration != 1 ||
		gotCredentials != credentials {
		t.Fatalf("registration after rejected delete = %+v, credentials = %+v, err %v",
			gotRegistration, gotCredentials, err)
	}

	if err := store.DeleteWorkspaceConnection(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteManagedRegistration(context.Background()); err != nil {
		t.Fatalf("delete unbound registration: %v", err)
	}
	got, _, err := repository.LoadManagedRegistration(context.Background())
	if err != nil || got != nil {
		t.Fatalf("registration after delete = %+v, err %v", got, err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("credential bundles remain after registration deletion: %v", secrets.values)
	}
}

func TestDeploymentAppDeleteCommitsMetadataBeforeBestEffortBundleCleanup(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	if err := repository.SaveManagedRegistration(context.Background(), testDeploymentAppRegistration(1),
		DeploymentAppCredentials{PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook"}); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("cleanup unavailable")
	if err := repository.DeleteManagedRegistration(context.Background()); err != nil {
		t.Fatalf("metadata-first delete returned cleanup error: %v", err)
	}
	registration, err := store.GetDeploymentAppRegistration(context.Background())
	if err != nil || registration != nil || len(secrets.values) != 1 {
		t.Fatalf("delete result registration = %+v, secrets = %v, err %v", registration, secrets.values, err)
	}
	secrets.deleteErr = nil
	if err := repository.CleanupOrphanedCredentialBundles(context.Background()); err != nil {
		t.Fatalf("reconcile deleted registration bundle: %v", err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("orphan remains after reconciliation: %v", secrets.values)
	}
}

func testDeploymentAppRegistration(generation int64) *DeploymentAppRegistration {
	return &DeploymentAppRegistration{
		GitHubHost: "github.com", AppID: 123, ClientID: "Iv1.client", Slug: "kandev-test",
		OwnerLogin: "acme", OwnerType: DeploymentAppOwnerOrganization,
		PublicBaseURL: "https://kandev.example.com", CredentialGeneration: generation,
		WebhookStatus: DeploymentAppWebhookUnverified,
	}
}

type cancelAfterSetSecrets struct {
	*fakeConnectionSecrets
	cancel context.CancelFunc
}

func (s *cancelAfterSetSecrets) Set(ctx context.Context, id, name, value string) error {
	if err := s.fakeConnectionSecrets.Set(ctx, id, name, value); err != nil {
		return err
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return nil
}
