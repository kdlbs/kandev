package github

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

type manifestConverterFunc func(context.Context, string) (ManifestConversionResult, error)

func (f manifestConverterFunc) Convert(ctx context.Context, code string) (ManifestConversionResult, error) {
	return f(ctx, code)
}

func TestDeploymentAppRegistrationBootAndCallbackUseAtomicRuntime(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	repository := NewDeploymentAppRepository(store, secrets)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	conversion := testManifestConversion(t, "acme", "Organization")
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Environment: config.GitHubAppConfig{},
		Repository:  repository,
		Store:       store,
		Runtime:     service,
		Converter: manifestConverterFunc(func(_ context.Context, code string) (ManifestConversionResult, error) {
			if code != "one-time-code" {
				t.Fatalf("conversion code = %q", code)
			}
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Now:    func() time.Time { return now },
		Random: strings.NewReader(strings.Repeat("s", oauthRandomBytes)),
	})

	if err := registration.Boot(ctx); err != nil {
		t.Fatalf("Boot() error = %v", err)
	}
	if got := service.DeploymentAppRuntimeSnapshot(); got.Ready || got.Source != DeploymentAppSourceNone {
		t.Fatalf("initial runtime = %+v", got)
	}

	started, err := registration.Start(ctx, DefaultUserID, DeploymentAppRegistrationStartRequest{
		OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.State == "" || !strings.Contains(started.Manifest.RedirectURL, "state=") {
		t.Fatalf("started flow = %+v", started)
	}

	result, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: started.State, Code: " one-time-code ",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Source != DeploymentAppSourceManaged || result.AppID != conversion.AppID {
		t.Fatalf("callback result = %+v", result)
	}
	snapshot := service.DeploymentAppRuntimeSnapshot()
	if !snapshot.Ready || snapshot.Source != DeploymentAppSourceManaged ||
		snapshot.AppID != conversion.AppID || snapshot.Generation != 1 {
		t.Fatalf("activated runtime = %+v", snapshot)
	}
	status, err := registration.Status(ctx, DefaultUserID)
	if err != nil || !status.Ready || status.Registration == nil ||
		status.Registration.AppID != conversion.AppID {
		t.Fatalf("Status() = %+v, error = %v", status, err)
	}
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: started.State, Code: "one-time-code",
	}); !errors.Is(err, ErrDeploymentAppManifestStateUnavailable) {
		t.Fatalf("replayed callback error = %v", err)
	}
}

func TestDeploymentAppRegistrationLatestAttemptWinsSequentially(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	conversion := testManifestConversion(t, "acme", "Organization")
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(
			strings.Repeat("a", oauthRandomBytes) + strings.Repeat("b", oauthRandomBytes),
		),
	})

	first := startTestDeploymentAppRegistration(t, registration)
	second := startTestDeploymentAppRegistration(t, registration)
	firstFlow, err := store.GetDeploymentAppRegistrationFlow(
		ctx, deploymentAppStateHash(first.State),
	)
	if err != nil || firstFlow == nil || firstFlow.ConsumedAt == nil {
		t.Fatalf("superseded first flow = %+v, error = %v", firstFlow, err)
	}
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: first.State, Code: "first-code",
	}); !errors.Is(err, ErrDeploymentAppManifestStateUnavailable) {
		t.Fatalf("superseded callback error = %v", err)
	}
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: second.State, Code: "second-code",
	}); err != nil {
		t.Fatalf("latest callback error = %v", err)
	}
}

func TestDeploymentAppRegistrationConcurrentStaleCallbackCannotOverwriteLatest(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	firstConversion := testManifestConversion(t, "acme", "Organization")
	firstConversion.AppID = 111
	firstConversion.Slug = "kandev-first"
	secondConversion := testManifestConversion(t, "acme", "Organization")
	secondConversion.AppID = 222
	secondConversion.Slug = "kandev-second"
	firstConverting := make(chan struct{})
	releaseFirst := make(chan struct{})
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		Converter: manifestConverterFunc(func(_ context.Context, code string) (ManifestConversionResult, error) {
			if code == "first-code" {
				close(firstConverting)
				<-releaseFirst
				return firstConversion, nil
			}
			return secondConversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(
			strings.Repeat("c", oauthRandomBytes) + strings.Repeat("d", oauthRandomBytes),
		),
	})

	first := startTestDeploymentAppRegistration(t, registration)
	firstResult := make(chan error, 1)
	go func() {
		_, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
			State: first.State, Code: "first-code",
		})
		firstResult <- err
	}()
	<-firstConverting
	second := startTestDeploymentAppRegistration(t, registration)
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: second.State, Code: "second-code",
	}); err != nil {
		t.Fatalf("latest callback error = %v", err)
	}
	close(releaseFirst)
	if err := <-firstResult; !errors.Is(err, ErrDeploymentAppManifestStateUnavailable) {
		t.Fatalf("concurrent stale callback error = %v", err)
	}
	if snapshot := service.DeploymentAppRuntimeSnapshot(); snapshot.AppID != secondConversion.AppID {
		t.Fatalf("runtime after stale callback = %+v", snapshot)
	}
}

func TestDeploymentAppRegistrationBootReconcilesOrphansForEverySource(t *testing.T) {
	for _, source := range []DeploymentAppSource{
		DeploymentAppSourceNone,
		DeploymentAppSourceEnvironment,
	} {
		t.Run(string(source), func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			secrets := newFakeConnectionSecrets()
			repository := NewDeploymentAppRepository(store, secrets)
			var environment config.GitHubAppConfig
			var activeSecretID string
			if source == DeploymentAppSourceEnvironment {
				managed := testDeploymentAppRegistration(1)
				if err := repository.SaveManagedRegistration(ctx, managed, DeploymentAppCredentials{
					PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook",
				}); err != nil {
					t.Fatal(err)
				}
				activeSecretID = managed.CredentialSecretID
				environment = testEnvironmentDeploymentAppConfig(t)
			}
			const orphanID = DeploymentAppCredentialsSecretPrefix + "g99:boot-orphan"
			if err := secrets.Set(ctx, orphanID, "orphan", `{"version":1}`); err != nil {
				t.Fatal(err)
			}
			service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
			service.SetConnectionSecretStore(secrets)
			registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
				Environment: environment, Repository: repository, Store: store, Runtime: service,
			})
			if err := registration.Boot(ctx); err != nil {
				t.Fatalf("Boot() error = %v", err)
			}
			if exists, err := secrets.Exists(ctx, orphanID); err != nil || exists {
				t.Fatalf("orphan exists = %v, error = %v", exists, err)
			}
			if activeSecretID != "" {
				if exists, err := secrets.Exists(ctx, activeSecretID); err != nil || !exists {
					t.Fatalf("dormant managed bundle exists = %v, error = %v", exists, err)
				}
			}
		})
	}
}

func TestDeploymentAppRegistrationRequiresExactManifestPolicy(t *testing.T) {
	base := testManifestConversion(t, "acme", "Organization")
	if !matchesDeploymentAppPolicy(base) {
		t.Fatal("exact generated policy was rejected")
	}
	extraPermission := base
	extraPermission.Permissions = make(map[string]string, len(base.Permissions)+1)
	for name, level := range base.Permissions {
		extraPermission.Permissions[name] = level
	}
	extraPermission.Permissions["deployments"] = "write"
	if matchesDeploymentAppPolicy(extraPermission) {
		t.Fatal("policy with an extra permission was accepted")
	}
	extraEvent := base
	extraEvent.Events = append(slices.Clone(base.Events), "push")
	if matchesDeploymentAppPolicy(extraEvent) {
		t.Fatal("policy with an extra event was accepted")
	}
	reordered := base
	reordered.Events = []string{"github_app_authorization", "installation", "installation_repositories"}
	if !matchesDeploymentAppPolicy(reordered) {
		t.Fatal("order-insensitive exact event set was rejected")
	}
}

func TestDeploymentAppRegistrationWebhookHealthUsesOnlySignedDeliveries(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	registration := testDeploymentAppRegistration(1)
	registration.CredentialSecretID = "github:deployment-app:credentials:g1:test"
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatal(err)
	}
	webhooks := NewGitHubWebhookService(
		"webhook-secret", &githubWebhookMemoryStore{}, nil, nil,
	)
	service := &Service{
		store: store,
		deploymentAppRuntime: &githubAppRuntime{
			source: DeploymentAppSourceManaged, generation: 1, webhookAuth: webhooks,
		},
	}
	valid := signedWebhookRequest("webhook-secret", "delivery-1", "ping", []byte(`{}`))
	if _, err := service.HandleAppWebhook(ctx, valid); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetDeploymentAppRegistration(ctx)
	if err != nil || got == nil || got.WebhookStatus != DeploymentAppWebhookVerified ||
		got.LastWebhookAt == nil {
		t.Fatalf("health after valid delivery = %+v, error = %v", got, err)
	}
	invalid := valid
	invalid.DeliveryID = "delivery-invalid"
	invalid.Signature = "sha256=invalid"
	if _, err := service.HandleAppWebhook(ctx, invalid); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("invalid signature error = %v", err)
	}
	got, err = store.GetDeploymentAppRegistration(ctx)
	if err != nil || got == nil || got.WebhookStatus != DeploymentAppWebhookVerified {
		t.Fatalf("invalid signature changed health = %+v, error = %v", got, err)
	}
	failing := signedWebhookRequest(
		"webhook-secret", "delivery-2", "installation", []byte(`{"action":`),
	)
	if _, err := service.HandleAppWebhook(ctx, failing); err == nil {
		t.Fatal("malformed signed delivery unexpectedly succeeded")
	}
	got, err = store.GetDeploymentAppRegistration(ctx)
	if err != nil || got == nil || got.WebhookStatus != DeploymentAppWebhookFailing ||
		got.LastError != "signed webhook processing failed" {
		t.Fatalf("health after signed failure = %+v, error = %v", got, err)
	}
}

func TestDeploymentAppWebhookHealthPersistenceFailureDoesNotRetryProcessedDelivery(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	webhooks := NewGitHubWebhookService(
		"webhook-secret", &githubWebhookMemoryStore{}, nil, nil,
	)
	service := &Service{
		store:  store,
		logger: testLogger(t),
		deploymentAppRuntime: &githubAppRuntime{
			source: DeploymentAppSourceManaged, generation: 1, webhookAuth: webhooks,
		},
	}
	if _, err := store.db.Exec(`DROP TABLE github_app_registration`); err != nil {
		t.Fatal(err)
	}

	valid := signedWebhookRequest("webhook-secret", "delivery-1", "ping", []byte(`{}`))
	if _, err := service.HandleAppWebhook(ctx, valid); err != nil {
		t.Fatalf("processed webhook returned health persistence error: %v", err)
	}
	health := service.currentDeploymentAppWebhookHealth()
	if health.status != DeploymentAppWebhookVerified || health.lastWebhookAt == nil {
		t.Fatalf("in-memory health after processed webhook = %+v", health)
	}
}

func TestDeploymentAppRegistrationDelayedOldGenerationWebhookCannotChangeCurrentHealth(t *testing.T) {
	for _, transition := range []string{"replacement", "deletion"} {
		t.Run(transition, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			oldRegistration := testDeploymentAppRegistration(1)
			oldRegistration.CredentialSecretID = "github:deployment-app:credentials:g1:old"
			if err := store.UpsertDeploymentAppRegistration(ctx, oldRegistration); err != nil {
				t.Fatal(err)
			}
			completing := make(chan struct{})
			release := make(chan struct{})
			webhookStore := &delayedWebhookStore{
				githubWebhookMemoryStore: &githubWebhookMemoryStore{},
				completing:               completing,
				release:                  release,
			}
			webhooks := NewGitHubWebhookService("old-secret", webhookStore, nil, nil)
			oldRuntime := &githubAppRuntime{
				source: DeploymentAppSourceManaged, generation: 1, webhookAuth: webhooks,
				initialWebhookHealth: deploymentAppWebhookHealth{status: DeploymentAppWebhookUnverified},
			}
			service := &Service{
				store: store, deploymentAppRuntime: oldRuntime,
				deploymentAppWebhookHealth: oldRuntime.initialWebhookHealth,
			}
			result := make(chan error, 1)
			go func() {
				_, err := service.HandleAppWebhook(ctx, signedWebhookRequest(
					"old-secret", "old-delivery", "ping", []byte(`{}`),
				))
				result <- err
			}()
			<-completing

			if transition == "replacement" {
				next := testDeploymentAppRegistration(2)
				next.CredentialSecretID = "github:deployment-app:credentials:g2:new"
				if err := store.UpsertDeploymentAppRegistration(ctx, next); err != nil {
					t.Fatal(err)
				}
				service.swapDeploymentAppRuntime(&githubAppRuntime{
					source: DeploymentAppSourceManaged, generation: 2,
					initialWebhookHealth: deploymentAppWebhookHealth{
						status: DeploymentAppWebhookUnverified,
					},
				})
			} else {
				if err := store.DeleteDeploymentAppRegistration(ctx); err != nil {
					t.Fatal(err)
				}
				service.swapDeploymentAppRuntime(nil)
			}
			close(release)
			if err := <-result; err != nil {
				t.Fatalf("delayed webhook error = %v", err)
			}

			stored, err := store.GetDeploymentAppRegistration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if transition == "replacement" {
				if stored == nil || stored.CredentialGeneration != 2 ||
					stored.WebhookStatus != DeploymentAppWebhookUnverified || stored.LastWebhookAt != nil {
					t.Fatalf("replacement health after old webhook = %+v", stored)
				}
				health := service.currentDeploymentAppWebhookHealth()
				if health.status != DeploymentAppWebhookUnverified || health.lastWebhookAt != nil {
					t.Fatalf("in-memory replacement health = %+v", health)
				}
			} else if stored != nil || service.DeploymentAppRuntimeSnapshot().Ready {
				t.Fatalf("deleted registration revived: stored=%+v runtime=%+v",
					stored, service.DeploymentAppRuntimeSnapshot())
			}
		})
	}
}

func TestDeploymentAppRegistrationRejectsWrongOperatorAndReturnedOwner(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	conversion := testManifestConversion(t, "other-owner", "Organization")
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Repository: NewDeploymentAppRepository(store, secrets), Store: store, Runtime: service,
		Converter: manifestConverterFunc(func(context.Context, string) (ManifestConversionResult, error) {
			return conversion, nil
		}),
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(strings.Repeat("o", oauthRandomBytes)),
	})
	if _, err := registration.Start(ctx, "another-user", DeploymentAppRegistrationStartRequest{
		OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example",
	}); !errors.Is(err, ErrDeploymentAppOperatorRequired) {
		t.Fatalf("wrong operator error = %v", err)
	}
	started, err := registration.Start(ctx, DefaultUserID, DeploymentAppRegistrationStartRequest{
		OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
		PublicBaseURL: "https://kandev.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: started.State, Code: "code",
	}); !errors.Is(err, ErrDeploymentAppIdentityMismatch) {
		t.Fatalf("owner mismatch error = %v", err)
	}
	status, statusErr := registration.Status(ctx, DefaultUserID)
	if statusErr != nil || status.Ready || status.Source != DeploymentAppSourceNone {
		t.Fatalf("status after mismatch = %+v, error = %v", status, statusErr)
	}
}

func TestDeploymentAppRegistrationEnvironmentSourceIsReadOnly(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	environment := testEnvironmentDeploymentAppConfig(t)
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Environment: environment, Repository: NewDeploymentAppRepository(store, secrets),
		Store: store, Runtime: service,
	})
	if err := registration.Boot(ctx); err != nil {
		t.Fatalf("Boot() error = %v", err)
	}
	status, err := registration.Status(ctx, DefaultUserID)
	if err != nil || status.Source != DeploymentAppSourceEnvironment || !status.Ready || !status.ReadOnly {
		t.Fatalf("environment status = %+v, error = %v", status, err)
	}
	if status.Registration == nil || status.Registration.WebhookStatus != DeploymentAppWebhookUnverified {
		t.Fatalf("initial environment webhook health = %+v", status.Registration)
	}
	request := signedWebhookRequest("environment-webhook", "environment-delivery", "ping", []byte(`{}`))
	if _, err := service.HandleAppWebhook(ctx, request); err != nil {
		t.Fatalf("environment webhook error = %v", err)
	}
	status, err = registration.Status(ctx, DefaultUserID)
	if err != nil || status.Registration == nil ||
		status.Registration.WebhookStatus != DeploymentAppWebhookVerified ||
		status.Registration.LastWebhookAt == nil {
		t.Fatalf("verified environment webhook health = %+v, error = %v", status.Registration, err)
	}
	if _, err := registration.Start(ctx, DefaultUserID, DeploymentAppRegistrationStartRequest{}); !errors.Is(
		err, ErrDeploymentAppEnvironmentReadOnly,
	) {
		t.Fatalf("environment start error = %v", err)
	}
	if err := registration.Delete(ctx, DefaultUserID); !errors.Is(err, ErrDeploymentAppEnvironmentReadOnly) {
		t.Fatalf("environment delete error = %v", err)
	}
}

func TestDeploymentAppRegistrationDeleteWinsBeforeWaitingInstallationCallback(t *testing.T) {
	service := &Service{}
	service.deploymentAppRuntime = &githubAppRuntime{source: DeploymentAppSourceManaged, appID: 123}
	service.deploymentAppMutationMu.Lock()
	started := make(chan struct{})
	completed := make(chan error, 1)
	go func() {
		close(started)
		_, err := service.CompleteAppInstallation(context.Background(), AppInstallationCallback{})
		completed <- err
	}()
	<-started
	for range 100 {
		runtime.Gosched()
	}
	select {
	case err := <-completed:
		service.deploymentAppMutationMu.Unlock()
		t.Fatalf("installation callback bypassed deployment mutation lock: %v", err)
	default:
	}
	service.swapDeploymentAppRuntime(nil)
	service.deploymentAppMutationMu.Unlock()
	if err := <-completed; !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("callback after deletion error = %v, want %v", err, ErrGitHubNotConfigured)
	}
}

func TestDeploymentAppRegistrationDeleteInvalidatesFlowsBeforeRegistration(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakeConnectionSecrets()
	repository := NewDeploymentAppRepository(store, secrets)
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.SetConnectionSecretStore(secrets)
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Repository: repository,
		Store:      store,
		Runtime:    service,
		Resolver: manifestResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
		}),
		Random: strings.NewReader(strings.Repeat("z", oauthRandomBytes)),
	})
	managed := testDeploymentAppRegistration(1)
	if err := repository.SaveManagedRegistration(ctx, managed, DeploymentAppCredentials{
		PrivateKey: "key", ClientSecret: "client", WebhookSecret: "webhook",
	}); err != nil {
		t.Fatal(err)
	}
	service.deploymentAppRuntime = &githubAppRuntime{
		source: DeploymentAppSourceManaged, appID: managed.AppID, generation: 1,
	}
	started := startTestDeploymentAppRegistration(t, registration)
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER fail_deployment_app_delete
		BEFORE DELETE ON github_app_registration
		BEGIN SELECT RAISE(ABORT, 'injected delete failure'); END`); err != nil {
		t.Fatal(err)
	}

	if err := registration.Delete(ctx, DefaultUserID); err == nil {
		t.Fatal("Delete() unexpectedly succeeded with an injected metadata failure")
	}
	stored, _, err := repository.LoadManagedRegistration(ctx)
	if err != nil || stored == nil || !service.DeploymentAppRuntimeSnapshot().Ready {
		t.Fatalf("registration after failed delete = %+v, runtime = %+v, error = %v",
			stored, service.DeploymentAppRuntimeSnapshot(), err)
	}
	if _, err := registration.Complete(ctx, DeploymentAppRegistrationCallback{
		State: started.State, Code: "late-code",
	}); !errors.Is(err, ErrDeploymentAppManifestStateUnavailable) {
		t.Fatalf("callback after failed delete error = %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER fail_deployment_app_delete`); err != nil {
		t.Fatal(err)
	}
	if err := registration.Delete(ctx, DefaultUserID); err != nil {
		t.Fatalf("Delete() after removing failure = %v", err)
	}
	stored, _, err = repository.LoadManagedRegistration(ctx)
	if err != nil || stored != nil || service.DeploymentAppRuntimeSnapshot().Ready {
		t.Fatalf("registration after delete = %+v, runtime = %+v, error = %v",
			stored, service.DeploymentAppRuntimeSnapshot(), err)
	}
}

func testManifestConversion(t *testing.T, ownerLogin, ownerType string) ManifestConversionResult {
	t.Helper()
	return ManifestConversionResult{
		AppID: 123, Slug: "kandev-acme", Name: "Kandev Acme",
		Owner:    ManifestConversionOwner{ID: 42, Login: ownerLogin, Type: ownerType},
		ClientID: "Iv1.client", ClientSecret: "generated-client",
		WebhookSecret: "generated-webhook", PrivateKeyPEM: testDeploymentAppPrivateKey(t),
		Permissions: map[string]string{
			"actions": "read", "administration": "read", "checks": "read",
			"contents": "write", "issues": "write", "members": "read",
			"metadata": "read", "pull_requests": "write", "statuses": "read",
			"workflows": "write",
		},
		Events: []string{"installation", "installation_repositories", "github_app_authorization"},
	}
}

func startTestDeploymentAppRegistration(
	t *testing.T,
	registration *DeploymentAppRegistrationService,
) DeploymentAppRegistrationStart {
	t.Helper()
	started, err := registration.Start(
		context.Background(),
		DefaultUserID,
		DeploymentAppRegistrationStartRequest{
			OwnerType: ManifestOwnerOrganization, OwnerLogin: "acme",
			PublicBaseURL: "https://kandev.example",
		},
	)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return started
}

func deploymentAppStateHash(state string) string {
	digest := sha256.Sum256([]byte(state))
	return stateDigestString(digest)
}

type delayedWebhookStore struct {
	*githubWebhookMemoryStore
	completing chan struct{}
	release    chan struct{}
}

func (s *delayedWebhookStore) CompleteWebhookDelivery(
	ctx context.Context,
	deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	close(s.completing)
	<-s.release
	return s.githubWebhookMemoryStore.CompleteWebhookDelivery(
		ctx, deliveryID, status, result, processedAt,
	)
}
