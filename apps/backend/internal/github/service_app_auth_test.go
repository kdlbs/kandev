package github

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type rateLimitSeedClient struct {
	*MockClient
	tracker *RateTracker
	calls   int
}

func (c *rateLimitSeedClient) FetchRateLimit(context.Context) error {
	c.calls++
	now := time.Now().UTC()
	for _, snapshot := range []RateSnapshot{
		{Resource: ResourceCore, Remaining: 4321, Limit: 5000},
		{Resource: ResourceGraphQL, Remaining: 4322, Limit: 5000},
		{Resource: ResourceSearch, Remaining: 29, Limit: 30},
	} {
		snapshot.ResetAt = now.Add(time.Hour)
		snapshot.UpdatedAt = now
		c.tracker.Record(snapshot)
	}
	return nil
}

type rateLimitAutomationProvider struct {
	client  Client
	tracker *RateTracker
}

type concurrentRateLimitSeedClient struct {
	*MockClient
	tracker       *RateTracker
	mu            sync.Mutex
	calls         int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	release       chan struct{}
}

func (c *concurrentRateLimitSeedClient) FetchRateLimit(ctx context.Context) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	switch call {
	case 1:
		close(c.firstStarted)
	case 2:
		close(c.secondStarted)
	}
	select {
	case <-c.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	now := time.Now().UTC()
	for _, snapshot := range []RateSnapshot{
		{Resource: ResourceCore, Remaining: 4000 - call, Limit: 5000},
		{Resource: ResourceGraphQL, Remaining: 4001 - call, Limit: 5000},
		{Resource: ResourceSearch, Remaining: 28 - call, Limit: 30},
	} {
		snapshot.ResetAt = now.Add(time.Hour)
		snapshot.UpdatedAt = now
		c.tracker.Record(snapshot)
	}
	return nil
}

func (c *concurrentRateLimitSeedClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (p rateLimitAutomationProvider) ResolveAutomation(
	context.Context,
	*WorkspaceConnection,
	ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client:       p.client,
		Capabilities: allTokenCapabilities(),
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourceGHCLI,
			Login:  "octocat",
		},
		RateTracker: p.tracker,
	}, nil
}

func TestWorkspaceAuthStatusSeedsRateLimitOncePerCachedCredential(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-rate-limit")
	connection := &WorkspaceConnection{
		WorkspaceID:          "workspace-rate-limit",
		Source:               ConnectionSourceGHCLI,
		GitHubHost:           defaultGitHubHost,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}
	if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
		t.Fatalf("seed workspace connection: %v", err)
	}

	tracker := NewRateTracker(nil, nil)
	client := &rateLimitSeedClient{MockClient: NewMockClient(), tracker: tracker}
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	service.resolver = NewCredentialResolver(testConnectionReader{
		workspaces: map[string]*WorkspaceConnection{connection.WorkspaceID: connection},
	}, nil)
	service.resolver.SetAutomationProvider(rateLimitAutomationProvider{client: client, tracker: tracker})

	for i := 0; i < 2; i++ {
		status, err := service.GetWorkspaceAuthStatus(context.Background(), connection.WorkspaceID, DefaultUserID)
		if err != nil {
			t.Fatalf("GetWorkspaceAuthStatus call %d: %v", i+1, err)
		}
		if status.RateLimit == nil || status.RateLimit.Core == nil {
			t.Fatalf("call %d rate limit = %+v, want seeded core snapshot", i+1, status.RateLimit)
		}
		if status.RateLimit.Core.Remaining != 4321 {
			t.Errorf("call %d remaining = %d, want 4321", i+1, status.RateLimit.Core.Remaining)
		}
	}
	if client.calls != 1 {
		t.Fatalf("FetchRateLimit calls = %d, want 1 for cached credential", client.calls)
	}

	status, err := service.RefreshWorkspaceRateLimit(
		context.Background(), connection.WorkspaceID, DefaultUserID,
	)
	if err != nil {
		t.Fatalf("RefreshWorkspaceRateLimit: %v", err)
	}
	if status.RateLimit == nil || status.RateLimit.Core == nil {
		t.Fatalf("forced refresh rate limit = %+v, want seeded core snapshot", status.RateLimit)
	}
	if client.calls != 2 {
		t.Fatalf("FetchRateLimit calls after forced refresh = %d, want 2", client.calls)
	}
}

func TestWorkspaceAuthStatusCompletesPartialRateLimitSeed(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-partial-rate-limit")
	connection := &WorkspaceConnection{
		WorkspaceID:          "workspace-partial-rate-limit",
		Source:               ConnectionSourceGHCLI,
		GitHubHost:           defaultGitHubHost,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}
	if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
		t.Fatalf("seed workspace connection: %v", err)
	}

	tracker := NewRateTracker(nil, nil)
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 4999, Limit: 5000,
		ResetAt: time.Now().Add(time.Hour).UTC(), UpdatedAt: time.Now().UTC(),
	})
	client := &rateLimitSeedClient{MockClient: NewMockClient(), tracker: tracker}
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	service.resolver = NewCredentialResolver(testConnectionReader{
		workspaces: map[string]*WorkspaceConnection{connection.WorkspaceID: connection},
	}, nil)
	service.resolver.SetAutomationProvider(rateLimitAutomationProvider{client: client, tracker: tracker})

	status, err := service.GetWorkspaceAuthStatus(
		context.Background(), connection.WorkspaceID, DefaultUserID,
	)
	if err != nil {
		t.Fatalf("GetWorkspaceAuthStatus: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("FetchRateLimit calls = %d, want 1 for a partial tracker", client.calls)
	}
	if status.RateLimit == nil || status.RateLimit.GraphQL == nil || status.RateLimit.Search == nil {
		t.Fatalf("rate limit = %+v, want all seeded buckets", status.RateLimit)
	}
}

func TestWorkspaceRateLimitSeedSerializesConcurrentRefreshes(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-concurrent-rate-limit")
	connection := &WorkspaceConnection{
		WorkspaceID:          "workspace-concurrent-rate-limit",
		Source:               ConnectionSourceGHCLI,
		GitHubHost:           defaultGitHubHost,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}
	if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
		t.Fatalf("seed workspace connection: %v", err)
	}

	tracker := NewRateTracker(nil, nil)
	client := &concurrentRateLimitSeedClient{
		MockClient:    NewMockClient(),
		tracker:       tracker,
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	service.resolver = NewCredentialResolver(testConnectionReader{
		workspaces: map[string]*WorkspaceConnection{connection.WorkspaceID: connection},
	}, nil)
	service.resolver.SetAutomationProvider(rateLimitAutomationProvider{client: client, tracker: tracker})

	regularDone := make(chan error, 1)
	go func() {
		_, err := service.GetWorkspaceAuthStatus(context.Background(), connection.WorkspaceID, DefaultUserID)
		regularDone <- err
	}()
	<-client.firstStarted

	forcedDone := make(chan error, 1)
	go func() {
		_, err := service.RefreshWorkspaceRateLimit(context.Background(), connection.WorkspaceID, DefaultUserID)
		forcedDone <- err
	}()

	secondStarted := false
	select {
	case <-client.secondStarted:
		secondStarted = true
	case <-time.After(100 * time.Millisecond):
	}
	close(client.release)
	if err := <-regularDone; err != nil {
		t.Fatalf("regular status: %v", err)
	}
	if err := <-forcedDone; err != nil {
		t.Fatalf("forced status: %v", err)
	}
	if secondStarted {
		t.Fatal("concurrent forced refresh started a duplicate rate-limit fetch")
	}
	if got := client.callCount(); got != 1 {
		t.Fatalf("FetchRateLimit calls = %d, want 1", got)
	}
}

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
		prepared.SetupURL != "" {
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
