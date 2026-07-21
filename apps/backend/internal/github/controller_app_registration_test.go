package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type blockingAppRegistrationRuntime struct {
	applyStarted chan struct{}
	releaseApply chan struct{}
	mu           sync.Mutex
	active       bool
}

type failingAppRegistrationRuntime struct {
	mu          sync.Mutex
	active      bool
	applyErr    error
	invalidated bool
}

func (r *failingAppRegistrationRuntime) ValidateAppRegistrationRuntime(ResolvedDeploymentAppConfig) error {
	return nil
}

func (r *failingAppRegistrationRuntime) ApplyAppRegistrationRuntime(ResolvedDeploymentAppConfig) error {
	return r.applyErr
}

func (r *failingAppRegistrationRuntime) InvalidateAppRegistrationRuntime(string, int64) {
	r.mu.Lock()
	r.active = false
	r.invalidated = true
	r.mu.Unlock()
}

func (r *failingAppRegistrationRuntime) state() (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active, r.invalidated
}

func (r *blockingAppRegistrationRuntime) ValidateAppRegistrationRuntime(ResolvedDeploymentAppConfig) error {
	return nil
}

func (r *blockingAppRegistrationRuntime) ApplyAppRegistrationRuntime(ResolvedDeploymentAppConfig) error {
	close(r.applyStarted)
	<-r.releaseApply
	r.mu.Lock()
	r.active = true
	r.mu.Unlock()
	return nil
}

func (r *blockingAppRegistrationRuntime) InvalidateAppRegistrationRuntime(string, int64) {
	r.mu.Lock()
	r.active = false
	r.mu.Unlock()
}

func (r *blockingAppRegistrationRuntime) isActive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func setupAppRegistrationController(t *testing.T) (*gin.Engine, *Service, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	repository := NewAppRegistrationRepository(store, secrets)
	conversion := testManifestConversion(t, "acme", "Organization")
	service.appRegistrationLifecycle = NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: service,
		Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Importer: NewAppRegistrationImporter(repository, manifestResolverFunc(func(
			context.Context, string, string,
		) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}), func(int64, []byte) (AppRegistrationVerifier, error) {
			return appRegistrationVerifierFunc(func(context.Context) (AuthenticatedApp, error) {
				return AuthenticatedApp{
					ID: 101, ClientID: "Iv1.client", Slug: "kandev-app",
					OwnerLogin: "acme", OwnerType: "Organization",
					ExternalURL: "https://kandev.example",
					Permissions: conversion.Permissions, Events: conversion.Events,
				}, nil
			}), nil
		}),
		Random: strings.NewReader(strings.Repeat("h", oauthRandomBytes*3)),
	})
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)
	return router, service, store
}

func TestHTTPAppRegistrationManifestIsRouteBoundAndReturnsToWorkspace(t *testing.T) {
	router, _, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	start := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/manifest/start", `{
			"workspace_id":"workspace-1","display_name":"Work automation",
			"owner_type":"organization","owner_login":"acme","visibility":"private",
			"public_base_url":"https://kandev.example"
		}`)
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	var started AppRegistrationManifestStart
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil || started.RegistrationID == "" {
		t.Fatalf("start response = %s, error = %v", start.Body.String(), err)
	}

	wrong := httptest.NewRecorder()
	router.ServeHTTP(wrong, httptest.NewRequest(http.MethodGet,
		"/api/v1/github/app/registrations/"+uuid.NewString()+
			"/manifest/callback?state="+started.State+"&code=one-time-code", nil))
	assertPrivateGitHubRedirect(t, wrong)
	flowHash := stateDigest(started.State)
	flow, err := store.GetDeploymentAppRegistrationFlow(context.Background(), flowHash)
	if err != nil || flow == nil || flow.ConsumedAt != nil {
		t.Fatalf("wrong route consumed flow: flow=%+v err=%v", flow, err)
	}

	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(http.MethodGet,
		"/api/v1/github/app/registrations/"+started.RegistrationID+
			"/manifest/callback?state="+started.State+"&code=one-time-code", nil))
	assertPrivateGitHubRedirect(t, callback)
	location := callback.Header().Get("Location")
	if !strings.Contains(location, "workspace_id=workspace-1") ||
		!strings.Contains(location, "github_result=app_registered") {
		t.Fatalf("callback location = %q", location)
	}

	catalog := httptest.NewRecorder()
	router.ServeHTTP(catalog, httptest.NewRequest(http.MethodGet,
		"/api/v1/github/app/registrations?workspace_id=workspace-1", nil))
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), `"display_name":"Work automation"`) {
		t.Fatalf("catalog = %d %s", catalog.Code, catalog.Body.String())
	}
	for _, secret := range []string{"generated-client", "generated-webhook", "PRIVATE KEY"} {
		if strings.Contains(catalog.Body.String()+location, secret) {
			t.Fatalf("registration response leaked %q", secret)
		}
	}
}

func TestAppRegistrationDeleteWaitsForRuntimePublication(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	runtime := &blockingAppRegistrationRuntime{
		applyStarted: make(chan struct{}), releaseApply: make(chan struct{}),
	}
	conversion := testManifestConversion(t, "acme", "Organization")
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
		Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(strings.Repeat("r", oauthRandomBytes*3)),
	})
	ctx := context.Background()
	started, err := lifecycle.StartManifest(ctx, DefaultUserID, AppRegistrationManifestStartRequest{
		WorkspaceID: "workspace-1", DisplayName: "Concurrent App",
		OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
		Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	completeResult := make(chan error, 1)
	go func() {
		_, completeErr := lifecycle.CompleteManifest(ctx, started.RegistrationID, AppRegistrationManifestCallback{
			State: started.State, Code: "one-time-code",
		})
		completeResult <- completeErr
	}()
	<-runtime.applyStarted
	if registration, getErr := store.GetAppRegistration(ctx, started.RegistrationID); getErr != nil || registration == nil {
		t.Fatalf("registration before runtime publication = %+v, err %v", registration, getErr)
	}

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- lifecycle.Delete(ctx, DefaultUserID, started.RegistrationID)
	}()
	select {
	case deleteErr := <-deleteResult:
		t.Fatalf("Delete() completed before runtime publication: %v", deleteErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(runtime.releaseApply)
	if err := <-completeResult; err != nil {
		t.Fatalf("CompleteManifest(): %v", err)
	}
	if err := <-deleteResult; err != nil {
		t.Fatalf("Delete(): %v", err)
	}
	if runtime.isActive() {
		t.Fatal("deleted registration runtime remains active")
	}
	registration, err := store.GetAppRegistration(ctx, started.RegistrationID)
	if err != nil || registration != nil {
		t.Fatalf("registration after delete = %+v, err %v", registration, err)
	}
}

func TestAppRegistrationActivationSurfacesSanitizedCleanupFailure(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	secrets.deleteErr = errors.New("cleanup detail containing private-key")
	repository := NewAppRegistrationRepository(store, secrets)
	runtime := &failingAppRegistrationRuntime{applyErr: errors.New("activation detail containing client-secret")}
	conversion := testManifestConversion(t, "acme", "Organization")
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
		Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(strings.Repeat("s", oauthRandomBytes*3)),
	})
	ctx := context.Background()
	started, err := lifecycle.StartManifest(ctx, DefaultUserID, AppRegistrationManifestStartRequest{
		WorkspaceID: "workspace-1", DisplayName: "Cleanup failure App",
		OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
		Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lifecycle.CompleteManifest(ctx, started.RegistrationID, AppRegistrationManifestCallback{
		State: started.State, Code: "one-time-code",
	})
	if err == nil || !strings.Contains(err.Error(), "could not be activated") ||
		!strings.Contains(err.Error(), "could not be cleaned up") {
		t.Fatalf("CompleteManifest() error = %v", err)
	}
	for _, sensitive := range []string{"private-key", "client-secret"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("CompleteManifest() error leaked %q: %v", sensitive, err)
		}
	}
}

func TestAppRegistrationDeleteFailureInvalidatesRuntime(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	registration := newAppRegistration("registration-delete-runtime", 101, "Work", time.Now().UTC())
	if err := repository.SaveRegistration(context.Background(), registration, DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("delete unavailable")
	secrets.deleteErrKey = registration.CredentialSecretID
	secrets.deleteThenError = true
	secrets.setErr = errors.New("restore unavailable")
	secrets.setErrKey = registration.CredentialSecretID
	runtime := &failingAppRegistrationRuntime{active: true}
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
	})
	err := lifecycle.Delete(context.Background(), DefaultUserID, registration.ID)
	if !errors.Is(err, ErrAppRegistrationCredentialCleanup) {
		t.Fatalf("Delete() error = %v, want credential cleanup error", err)
	}
	active, invalidated := runtime.state()
	if active || !invalidated {
		t.Fatalf("runtime after failed delete: active=%v invalidated=%v", active, invalidated)
	}
	stored, getErr := store.GetAppRegistration(context.Background(), registration.ID)
	if getErr != nil || stored == nil || stored.Status != AppRegistrationStatusInvalid {
		t.Fatalf("registration after failed credential restoration = %+v, err %v", stored, getErr)
	}
}

func TestAppRegistrationDeleteFailurePreservesRuntimeAfterCredentialRollback(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	registration := newAppRegistration("registration-delete-runtime-rollback", 101, "Work", time.Now().UTC())
	if err := repository.SaveRegistration(context.Background(), registration, DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("delete unavailable")
	secrets.deleteErrKey = registration.CredentialSecretID
	runtime := &failingAppRegistrationRuntime{active: true}
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
	})
	err := lifecycle.Delete(context.Background(), DefaultUserID, registration.ID)
	if !errors.Is(err, ErrAppRegistrationCredentialCleanup) {
		t.Fatalf("Delete() error = %v, want credential cleanup error", err)
	}
	active, invalidated := runtime.state()
	if !active || invalidated {
		t.Fatalf("runtime after compensated delete: active=%v invalidated=%v", active, invalidated)
	}
	stored, getErr := store.GetAppRegistration(context.Background(), registration.ID)
	if getErr != nil || stored == nil || stored.Status != AppRegistrationStatusActive {
		t.Fatalf("registration after compensated delete = %+v, err %v", stored, getErr)
	}
}

func TestAppRegistrationDeleteFailureInvalidatesRuntimeWhenCredentialAndStatusRestoreFail(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	registration := newAppRegistration("registration-delete-double-failure", 101, "Work", time.Now().UTC())
	if err := repository.SaveRegistration(context.Background(), registration, DeploymentAppCredentials{
		PrivateKey: "private-key", ClientSecret: "client-secret", WebhookSecret: "webhook-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER fail_app_registration_invalidation
		BEFORE UPDATE OF status ON github_app_registrations
		WHEN NEW.status = 'invalid'
		BEGIN SELECT RAISE(FAIL, 'invalidation unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("delete unavailable")
	secrets.deleteErrKey = registration.CredentialSecretID
	secrets.deleteThenError = true
	secrets.setErr = errors.New("restore unavailable")
	secrets.setErrKey = registration.CredentialSecretID
	runtime := &failingAppRegistrationRuntime{active: true}
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
	})
	err := lifecycle.Delete(context.Background(), DefaultUserID, registration.ID)
	if !errors.Is(err, ErrAppRegistrationCredentialCleanup) ||
		!errors.Is(err, ErrAppRegistrationDeletionFailed) {
		t.Fatalf("Delete() error = %v, want cleanup and deletion errors", err)
	}
	active, invalidated := runtime.state()
	if active || !invalidated {
		t.Fatalf("runtime after double failure: active=%v invalidated=%v", active, invalidated)
	}
	stored, getErr := store.GetAppRegistration(context.Background(), registration.ID)
	if getErr != nil || stored == nil || stored.Status != AppRegistrationStatusActive {
		t.Fatalf("metadata after failed invalidation = %+v, err %v", stored, getErr)
	}
	if _, _, loadErr := repository.LoadRegistration(context.Background(), registration.ID); loadErr == nil {
		t.Fatal("registration credentials unexpectedly remained readable")
	}
}

func TestAppRegistrationDurableCheckInvalidatesSupersededRuntimeGeneration(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repository := NewAppRegistrationRepository(store, secrets)
	registration := newAppRegistration("registration-generation-change", 101, "Work", time.Now().UTC())
	if err := repository.SaveRegistration(context.Background(), registration, DeploymentAppCredentials{
		PrivateKey: "private-key-1", ClientSecret: "client-secret-1", WebhookSecret: "webhook-secret-1",
	}); err != nil {
		t.Fatal(err)
	}
	previous := *registration
	registration.CredentialGeneration++
	if err := repository.SaveRegistration(context.Background(), registration, DeploymentAppCredentials{
		PrivateKey: "private-key-2", ClientSecret: "client-secret-2", WebhookSecret: "webhook-secret-2",
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &failingAppRegistrationRuntime{active: true}
	lifecycle := NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: store, Runtime: runtime,
	})
	lifecycle.invalidateRuntimeForUnusableRegistration(context.Background(), &previous)
	active, invalidated := runtime.state()
	if active || !invalidated {
		t.Fatalf("superseded runtime state: active=%v invalidated=%v", active, invalidated)
	}
}

func TestHTTPAppRegistrationDuplicateImportReturnsExistingID(t *testing.T) {
	router, _, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	existing := newAppRegistration("registration-existing", 101, "Existing", time.Now().UTC())
	if err := store.UpsertDeploymentAppRegistration(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	prepare := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import/prepare",
		`{"workspace_id":"workspace-1","public_base_url":"https://kandev.example"}`)
	if prepare.Code != http.StatusCreated {
		t.Fatalf("prepare = %d %s", prepare.Code, prepare.Body.String())
	}
	var preparation AppRegistrationImportPreparationResult
	if err := json.Unmarshal(prepare.Body.Bytes(), &preparation); err != nil {
		t.Fatal(err)
	}
	response := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import", `{
			"registration_id":"`+preparation.RegistrationID+`","workspace_id":"workspace-1","display_name":"Duplicate","github_host":"github.com",
			"app_id":101,"client_id":"Iv1.client","client_secret":"client-secret",
			"private_key":"private-key","webhook_secret":"webhook-secret","slug":"kandev-app",
			"owner_login":"acme","owner_type":"Organization","visibility":"private",
			"public_base_url":"https://kandev.example"
		}`)
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), `"code":"github_app_already_registered"`) ||
		!strings.Contains(response.Body.String(), `"existing_registration_id":"registration-existing"`) {
		t.Fatalf("duplicate import = %d %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{"client-secret", "private-key", "webhook-secret"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("duplicate response leaked %q", secret)
		}
	}
}

func TestHTTPAppRegistrationImportPreparationIsSingleUse(t *testing.T) {
	router, _, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	prepare := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import/prepare",
		`{"workspace_id":"workspace-1","public_base_url":"https://KANDEV.example:443/"}`)
	if prepare.Code != http.StatusCreated || prepare.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("prepare = %d headers=%v body=%s", prepare.Code, prepare.Header(), prepare.Body.String())
	}
	var preparation AppRegistrationImportPreparationResult
	if err := json.Unmarshal(prepare.Body.Bytes(), &preparation); err != nil {
		t.Fatal(err)
	}
	base := "https://kandev.example/api/v1/github/app/registrations/" + preparation.RegistrationID
	if preparation.PublicBaseURL != "https://kandev.example" ||
		preparation.ManifestCallbackURL != base+"/manifest/callback" ||
		preparation.InstallCallbackURL != base+"/install/callback" ||
		preparation.PersonalCallbackURL != base+"/personal/callback" ||
		preparation.WebhookURL != base+"/webhook" || preparation.SetupURL != preparation.InstallCallbackURL ||
		preparation.Permissions["contents"] != "write" || !containsString(preparation.Events, "installation") {
		t.Fatalf("preparation = %+v", preparation)
	}
	_, privateKey := testAppPrivateKey(t)
	request := AppRegistrationImportRequest{
		RegistrationID: preparation.RegistrationID, WorkspaceID: "workspace-1",
		DisplayName: "Existing App", GitHubHost: defaultGitHubAppHost, AppID: 101,
		ClientID: "Iv1.client", ClientSecret: "client-secret", PrivateKey: string(privateKey),
		WebhookSecret: "webhook-secret", Slug: "kandev-app", OwnerLogin: "acme",
		OwnerType: AppRegistrationOwnerOrganization, Visibility: AppRegistrationVisibilityPrivate,
		PublicBaseURL: preparation.PublicBaseURL,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	imported := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import", string(body))
	if imported.Code != http.StatusCreated || !strings.Contains(imported.Body.String(), preparation.RegistrationID) {
		t.Fatalf("import = %d %s", imported.Code, imported.Body.String())
	}
	replayed := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import", string(body))
	if replayed.Code != http.StatusBadRequest ||
		!strings.Contains(replayed.Body.String(), `"code":"github_app_import_preparation_invalid"`) {
		t.Fatalf("replay = %d %s", replayed.Code, replayed.Body.String())
	}
	for _, secret := range []string{request.ClientSecret, request.PrivateKey, request.WebhookSecret} {
		if strings.Contains(replayed.Body.String(), secret) {
			t.Fatalf("replay response leaked credential %q", secret)
		}
	}
}

func TestHTTPAppRegistrationImportWithoutPreparationFailsWithoutSecretLeak(t *testing.T) {
	router, _, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	_, privateKey := testAppPrivateKey(t)
	request := AppRegistrationImportRequest{
		RegistrationID: uuid.NewString(), WorkspaceID: "workspace-1",
		DisplayName: "Unprepared App", GitHubHost: defaultGitHubAppHost, AppID: 101,
		ClientID: "Iv1.client", ClientSecret: "unprepared-client-secret",
		PrivateKey: string(privateKey), WebhookSecret: "unprepared-webhook-secret",
		Slug: "kandev-app", OwnerLogin: "acme", OwnerType: AppRegistrationOwnerOrganization,
		Visibility: AppRegistrationVisibilityPrivate, PublicBaseURL: "https://kandev.example",
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := serveRegistrationJSON(t, router, http.MethodPost,
		"/api/v1/github/app/registrations/import", string(body))
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), `"code":"github_app_import_preparation_invalid"`) {
		t.Fatalf("unprepared import = %d %s", response.Code, response.Body.String())
	}
	for _, secret := range []string{request.ClientSecret, request.PrivateKey, request.WebhookSecret} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("unprepared response leaked credential %q", secret)
		}
	}
}

func TestAppRegistrationCatalogSharedCountsWorkspaceBindingsOnly(t *testing.T) {
	_, service, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1", "workspace-2")
	ctx := context.Background()
	now := time.Now().UTC()
	registration := newAppRegistration("registration-shared", 101, "Shared App", now)
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	connection := func(workspaceID string) *WorkspaceConnection {
		return &WorkspaceConnection{
			WorkspaceID: workspaceID, Source: ConnectionSourceGitHubAppInstallation,
			GitHubHost: defaultGitHubHost, InstallationID: &installationID,
			InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
			AppRegistrationID: registration.ID, Status: ConnectionStatusActive,
			CredentialGeneration: 1,
		}
	}
	if err := store.UpsertWorkspaceConnection(ctx, connection("workspace-1")); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserConnection(ctx, &UserConnection{
		WorkspaceID: "workspace-1", UserID: DefaultUserID, AppRegistrationID: registration.ID,
		GitHubUserID: 7, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}
	catalog, err := service.ListAppRegistrationCatalog(ctx, DefaultUserID, "workspace-1")
	if err != nil || len(catalog.Registrations) != 1 {
		t.Fatalf("single-workspace catalog = %+v, err %v", catalog, err)
	}
	item := catalog.Registrations[0]
	if item.BindingCount != 2 || item.WorkspaceBindingCount != 1 || item.Shared {
		t.Fatalf("single-workspace item = %+v", item)
	}
	if err := store.UpsertWorkspaceConnection(ctx, connection("workspace-2")); err != nil {
		t.Fatal(err)
	}
	catalog, err = service.ListAppRegistrationCatalog(ctx, DefaultUserID, "workspace-1")
	if err != nil || len(catalog.Registrations) != 1 {
		t.Fatalf("shared catalog = %+v, err %v", catalog, err)
	}
	item = catalog.Registrations[0]
	if item.BindingCount != 3 || item.WorkspaceBindingCount != 2 || !item.Shared {
		t.Fatalf("shared item = %+v", item)
	}
}

func TestLegacySingletonGitHubAppRoutesAreRemoved(t *testing.T) {
	router, _, _ := setupAppRegistrationController(t)
	for _, route := range []string{
		"/api/v1/github/app/registration",
		"/api/v1/github/app/registration/callback",
		"/api/v1/github/app/callback",
		"/api/v1/github/personal/callback",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", route, response.Code)
		}
	}
}

func TestGitHubAuthCallbackErrorsReturnToInitiatingWorkspace(t *testing.T) {
	router, _, store := setupAppRegistrationController(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	registration := newAppRegistration("registration-work", 101, "Work", time.Now().UTC())
	if err := store.UpsertDeploymentAppRegistration(context.Background(), registration); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		kind AuthFlowKind
		path string
	}{
		{name: "installation", kind: AuthFlowKindAppInstallation,
			path: "/api/v1/github/app/registrations/wrong-registration/install/callback?installation_id=42"},
		{name: "personal", kind: AuthFlowKindPersonal,
			path: "/api/v1/github/app/registrations/wrong-registration/personal/callback?code=oauth-code"},
	} {
		t.Run(test.name, func(t *testing.T) {
			flows := NewOAuthFlowManager(store)
			flows.random = strings.NewReader(strings.Repeat(test.name, oauthRandomBytes*4))
			started, err := flows.Start(context.Background(), OAuthFlowRequest{
				WorkspaceID: "workspace-1", UserID: DefaultUserID,
				AppRegistrationID: registration.ID, Kind: test.kind,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
				test.path+"&state="+started.State, nil))
			assertPrivateGitHubRedirect(t, response)
			if location := response.Header().Get("Location"); !strings.Contains(location, "workspace_id=workspace-1") ||
				!strings.Contains(location, "github_result=github_not_configured") {
				t.Fatalf("callback location = %q", location)
			}
			flow, err := store.GetAuthFlow(context.Background(), stateDigest(started.State))
			if err != nil || flow == nil || flow.ConsumedAt != nil {
				t.Fatalf("wrong route consumed flow: flow=%+v err=%v", flow, err)
			}
		})
	}
}

func serveRegistrationJSON(
	t *testing.T,
	router http.Handler,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertPrivateGitHubRedirect(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusSeeOther || response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if location := response.Header().Get("Location"); strings.Contains(location, "state=") || strings.Contains(location, "code=") {
		t.Fatalf("callback leaked provider parameters: %q", location)
	}
}
