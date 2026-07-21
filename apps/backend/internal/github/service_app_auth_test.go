package github

import (
	"context"
	"errors"
	"testing"
)

func TestAppAuthRegistryHotAddsAndInvalidatesRegistrationsIndependently(t *testing.T) {
	store := newTestStore(t)
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(newFakeConnectionSecrets())

	first := testResolvedAppRegistration(t, "registration-a", 101, 1)
	second := testResolvedAppRegistration(t, "registration-b", 202, 3)
	if err := service.ApplyAppRegistrationRuntime(first); err != nil {
		t.Fatalf("ApplyAppRegistrationRuntime(first): %v", err)
	}
	if err := service.ApplyAppRegistrationRuntime(second); err != nil {
		t.Fatalf("ApplyAppRegistrationRuntime(second): %v", err)
	}

	if got := service.AppRegistrationRuntimeSnapshot("registration-a"); !got.Ready || got.AppID != 101 || got.Generation != 1 {
		t.Fatalf("first snapshot = %+v", got)
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-b"); !got.Ready || got.AppID != 202 || got.Generation != 3 {
		t.Fatalf("second snapshot = %+v", got)
	}

	service.InvalidateAppRegistrationRuntime("registration-a", 1)
	if got := service.AppRegistrationRuntimeSnapshot("registration-a"); got.Ready {
		t.Fatalf("invalidated snapshot = %+v", got)
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-b"); !got.Ready {
		t.Fatalf("unrelated snapshot = %+v", got)
	}
}

func TestAppAuthRegistryRejectsOneRegistrationWithoutReplacingAnother(t *testing.T) {
	store := newTestStore(t)
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(newFakeConnectionSecrets())
	valid := testResolvedAppRegistration(t, "registration-a", 101, 1)
	if err := service.ApplyAppRegistrationRuntime(valid); err != nil {
		t.Fatal(err)
	}
	invalid := testResolvedAppRegistration(t, "registration-b", 202, 1)
	invalid.Config.PrivateKey = "not a private key"
	if err := service.ApplyAppRegistrationRuntime(invalid); err == nil {
		t.Fatal("invalid registration unexpectedly loaded")
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-a"); !got.Ready {
		t.Fatalf("valid registration was removed: %+v", got)
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-b"); got.Ready {
		t.Fatalf("invalid registration became ready: %+v", got)
	}
}

func TestAppAuthRegistryStaleInvalidationDoesNotRemoveNewGeneration(t *testing.T) {
	store := newTestStore(t)
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(newFakeConnectionSecrets())
	if err := service.ApplyAppRegistrationRuntime(testResolvedAppRegistration(t, "registration-a", 101, 2)); err != nil {
		t.Fatal(err)
	}
	service.InvalidateAppRegistrationRuntime("registration-a", 1)
	if got := service.AppRegistrationRuntimeSnapshot("registration-a"); !got.Ready || got.Generation != 2 {
		t.Fatalf("new generation was removed by stale invalidation: %+v", got)
	}
}

func testResolvedAppRegistration(
	t *testing.T,
	registrationID string,
	appID, generation int64,
) ResolvedDeploymentAppConfig {
	t.Helper()
	_, privateKey := testAppPrivateKey(t)
	return ResolvedDeploymentAppConfig{
		Source: DeploymentAppSourceManaged,
		Registration: &AppRegistration{
			ID: registrationID, Source: AppRegistrationSourceManaged, DisplayName: registrationID,
			GitHubHost: "github.com", AppID: appID, ClientID: "Iv1.client", Slug: "kandev-app",
			OwnerLogin: "acme", OwnerType: AppRegistrationOwnerOrganization,
			Visibility:    AppRegistrationVisibilityPrivate,
			PublicBaseURL: "https://kandev.example", CredentialGeneration: generation,
			CredentialSecretID: "github:app-registration:" + registrationID + ":test",
			Status:             AppRegistrationStatusActive, WebhookStatus: DeploymentAppWebhookUnverified,
		},
		Config: AppRegistrationRuntimeConfig{
			AppID: appID, ClientID: "Iv1.client", ClientSecret: "client-secret",
			PrivateKey: string(privateKey), WebhookSecret: "webhook-secret",
			Slug: "kandev-app", PublicBaseURL: "https://kandev.example",
		},
	}
}

func TestAppAuthRegistryLoadsValidRegistrationsWhenAnotherFails(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	valid := testResolvedAppRegistration(t, "registration-a", 101, 1)
	if err := repository.SaveRegistration(context.Background(), valid.Registration, DeploymentAppCredentials{
		PrivateKey: valid.Config.PrivateKey, ClientSecret: valid.Config.ClientSecret,
		WebhookSecret: valid.Config.WebhookSecret,
	}); err != nil {
		t.Fatal(err)
	}
	invalid := *valid.Registration
	invalid.ID = "registration-b"
	invalid.AppID = 202
	invalid.CredentialGeneration = 1
	if err := repository.SaveRegistration(context.Background(), &invalid, DeploymentAppCredentials{
		PrivateKey: "invalid", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}

	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	err := service.InitializeAppRegistrationRuntimes(context.Background())
	if err == nil {
		t.Fatal("startup did not report invalid registration")
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-a"); !got.Ready {
		t.Fatalf("valid runtime not loaded: %+v", got)
	}
	if got := service.AppRegistrationRuntimeSnapshot("registration-b"); got.Ready {
		t.Fatalf("invalid runtime loaded: %+v", got)
	}
}

func TestAppAuthRegistrySkipsPersistedInvalidRegistration(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	active := testResolvedAppRegistration(t, "registration-active", 101, 1)
	invalid := testResolvedAppRegistration(t, "registration-invalid", 202, 1)
	invalid.Registration.Status = AppRegistrationStatusInvalid
	for _, resolved := range []ResolvedDeploymentAppConfig{active, invalid} {
		if err := repository.SaveRegistration(context.Background(), resolved.Registration, DeploymentAppCredentials{
			PrivateKey: resolved.Config.PrivateKey, ClientSecret: resolved.Config.ClientSecret,
			WebhookSecret: resolved.Config.WebhookSecret,
		}); err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	if err := service.InitializeAppRegistrationRuntimes(context.Background()); err == nil {
		t.Fatal("startup did not report invalid registration")
	}
	if got := service.AppRegistrationRuntimeSnapshot(active.Registration.ID); !got.Ready {
		t.Fatalf("active registration was not loaded: %+v", got)
	}
	if got := service.AppRegistrationRuntimeSnapshot(invalid.Registration.ID); got.Ready {
		t.Fatalf("invalid registration was loaded: %+v", got)
	}
	if err := service.ApplyAppRegistrationRuntime(invalid); err == nil {
		t.Fatalf("ApplyAppRegistrationRuntime(invalid) error = %v", err)
	}
}

func TestWorkspaceAuthStatusUsesSelectedAppRegistration(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "ws-a", "ws-b", "ws-invalid")
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(newFakeConnectionSecrets())
	activeA := testResolvedAppRegistration(t, "registration-a", 101, 1)
	activeB := testResolvedAppRegistration(t, "registration-b", 202, 1)
	invalid := testResolvedAppRegistration(t, "registration-invalid", 303, 1)
	invalid.Registration.Status = AppRegistrationStatusInvalid
	for _, resolved := range []ResolvedDeploymentAppConfig{activeA, activeB} {
		if err := service.ApplyAppRegistrationRuntime(resolved); err != nil {
			t.Fatal(err)
		}
	}
	for _, registration := range []*AppRegistration{
		activeA.Registration, activeB.Registration, invalid.Registration,
	} {
		if err := store.UpsertDeploymentAppRegistration(context.Background(), registration); err != nil {
			t.Fatal(err)
		}
	}
	installationID := int64(42)
	for workspaceID, registrationID := range map[string]string{
		"ws-a": "registration-a", "ws-b": "registration-b", "ws-invalid": "registration-invalid",
	} {
		if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
			WorkspaceID: workspaceID, Source: ConnectionSourceGitHubAppInstallation,
			GitHubHost: defaultGitHubHost, InstallationID: &installationID,
			InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
			AppRegistrationID: registrationID, Status: ConnectionStatusActive,
			CredentialGeneration: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for workspaceID, registrationID := range map[string]string{"ws-a": "registration-a", "ws-b": "registration-b"} {
		status, err := service.GetWorkspaceAuthStatus(context.Background(), workspaceID, DefaultUserID)
		if err != nil {
			t.Fatal(err)
		}
		if status.AppRegistration == nil || status.AppRegistration.ID != registrationID ||
			!status.GitHubAppAvailable || !status.AppAvailable {
			t.Fatalf("status for %s = %+v", workspaceID, status)
		}
	}
	status, err := service.GetWorkspaceAuthStatus(context.Background(), "ws-invalid", DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if status.AppRegistration == nil || status.AppRegistration.ID != "registration-invalid" ||
		status.GitHubAppAvailable || status.AppAvailable {
		t.Fatalf("invalid registration status = %+v", status)
	}
}

func TestServicePreparesImportAndRejectsWrongBinding(t *testing.T) {
	_, service, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1", "workspace-2")
	prepared, err := service.PrepareAppRegistrationImport(
		context.Background(), DefaultUserID, AppRegistrationImportPrepareRequest{
			WorkspaceID: "workspace-1", PublicBaseURL: "https://kandev.example",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RegistrationID == "" || prepared.PublicBaseURL != "https://kandev.example" ||
		prepared.SetupURL != prepared.InstallCallbackURL {
		t.Fatalf("prepared import = %+v", prepared)
	}
	_, err = service.ImportAppRegistration(context.Background(), DefaultUserID, AppRegistrationImportRequest{
		RegistrationID: prepared.RegistrationID, WorkspaceID: "workspace-2",
		PublicBaseURL: prepared.PublicBaseURL,
	})
	if !errors.Is(err, ErrAppRegistrationImportPreparationUnavailable) {
		t.Fatalf("wrong workspace import error = %v", err)
	}
	stored, err := store.GetAppRegistrationImportPreparation(context.Background(), prepared.RegistrationID)
	if err != nil || stored == nil || stored.ConsumedAt != nil {
		t.Fatalf("wrong binding consumed preparation = %+v, err %v", stored, err)
	}
}
