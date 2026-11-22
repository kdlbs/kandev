package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestLegacyAccountReplacementIsolatesRateState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := http.DefaultTransport
		http.DefaultTransport = legacyIdentityRoundTripper(func(req *http.Request) (*http.Response, error) {
			login := strings.TrimPrefix(req.Header.Get("Authorization"), "token ")
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"` + login + `"}`))}, nil
		})
		t.Cleanup(func() { http.DefaultTransport = original })
		mgr := newMockSecretManager()
		svc := NewService(nil, AuthMethodNone, &mockSecretProvider{mgr: mgr}, nil, nil, testLogger(t))
		svc.secretManager = mgr
		t.Cleanup(svc.Stop)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := svc.ConfigureToken(ctx, "alice"); err != nil {
			t.Fatal(err)
		}
		alice := svc.client.(*PATClient)
		deadline := time.Now().Add(time.Hour)
		alice.rateTracker.ObserveSecondary(ResourceCore, deadline, RetrySourceRetryAfter, "")
		if err := svc.ConfigureToken(ctx, "bob"); err != nil {
			t.Fatalf("Bob setup blocked by Alice: %v", err)
		}
		bob := svc.client.(*PATClient)
		alice.rateTracker.ObservePrimary(ResourceSearch, deadline, RetrySourcePrimaryReset)
		for _, resource := range []Resource{ResourceCore, ResourceSearch} {
			release, err := bob.rateAdmission.acquire(pollerContext(context.Background()), resource)
			if err != nil {
				t.Fatalf("Bob inherited Alice's %s restriction: %v", resource, err)
			}
			release()
		}
		if svc.RateTracker() != bob.rateTracker {
			t.Fatal("service status does not follow the active account")
		}
		if err := svc.ConfigureToken(ctx, "ALICE"); err != nil {
			t.Fatalf("same-account rotation: %v", err)
		}
		rotated := svc.client.(*PATClient)
		if !rotated.rateTracker.Secondary(ResourceCore).RetryAt.Equal(deadline) {
			t.Fatal("rotation erased Alice's restriction")
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestLegacyResolverIsolatesAccountReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := NewCredentialResolver(nil, nil)
		resolver.SetRateCoordinator(NewRateCoordinator(nil, nil))
		login := "alice"
		resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
			c := NewPATClient("fixture")
			c.httpClient.Transport = legacyIdentityRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"` + login + `"}`))}, nil
			})
			return c, AuthMethodPAT, nil
		})
		connection := &WorkspaceConnection{WorkspaceID: "work", GitHubHost: "github.com", Login: "alice"}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		alice, err := resolver.resolveLegacy(ctx, connection, CredentialPurposeAutomation)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Hour)
		alice.RateTracker.ObserveSecondary(ResourceCore, deadline, RetrySourceRetryAfter, "")
		login = "bob"
		bob, err := resolver.resolveLegacy(ctx, connection, CredentialPurposeAutomation)
		if err != nil {
			t.Fatalf("new legacy account blocked before identity resolution: %v", err)
		}
		if bob.Principal.Login != "bob" || bob.RateTracker.Secondary(ResourceCore).Active {
			t.Fatalf("new account inherited stale identity/rate state: %+v", bob.Principal)
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestCoordinatorBindsOnlyPrivateProvisionalObservations(t *testing.T) {
	c := NewRateCoordinator(nil, nil)
	alice := AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"}
	a, _ := c.coordinate("github.com", alice, nil)
	deadline := time.Now().Add(time.Hour)
	a.ObserveSecondary(ResourceCore, deadline, RetrySourceRetryAfter, "")
	b, _ := c.coordinate("github.com", AuthPrincipal{Kind: AuthPrincipalHuman, Login: "bob"}, a)
	if b.Secondary(ResourceCore).Active {
		t.Fatal("coordinator aliased an already-owned tracker to another human")
	}
	provisional, _ := c.coordinate("github.com", AuthPrincipal{Kind: AuthPrincipalHuman, WorkspaceID: "work"}, nil)
	provisional.ObserveSecondary(ResourceSearch, deadline, RetrySourceRetryAfter, "")
	bound, _ := c.coordinate("github.com", alice, provisional)
	if !bound.Secondary(ResourceSearch).Active || !bound.Secondary(ResourceCore).Active {
		t.Fatal("canonical binding lost observations from the same client's identity probe")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestConfigureTokenRefreshesLegacyResolverCache(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = legacyIdentityRoundTripper(func(req *http.Request) (*http.Response, error) {
		login := strings.TrimPrefix(req.Header.Get("Authorization"), "token ")
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"` + login + `"}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "work")
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{WorkspaceID: "work", GitHubHost: defaultGitHubHost, Source: ConnectionSourceLegacyShared, Status: ConnectionStatusActive, CredentialGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}
	svc := NewService(nil, AuthMethodNone, provider, store, nil, testLogger(t))
	svc.secretManager = mgr
	t.Cleanup(svc.Stop)
	svc.resolver.SetLegacyFactory(func(ctx context.Context) (Client, string, error) {
		id, exists := svc.findGitHubTokenSecret(ctx)
		if !exists {
			return nil, AuthMethodNone, nil
		}
		token, err := provider.Reveal(ctx, id)
		return NewPATClient(token), AuthMethodPAT, err
	})
	request := ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation}
	for _, login := range []string{"alice", "bob"} {
		if err := svc.ConfigureToken(context.Background(), login); err != nil {
			t.Fatal(err)
		}
		resolved, err := svc.resolver.Resolve(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Principal.Login != login {
			t.Fatalf("configured %s, workspace still uses %s", login, resolved.Principal.Login)
		}
	}
	if err := svc.ClearToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.resolver.Resolve(context.Background(), request); err == nil {
		t.Fatal("cleared credential remains cached for workspace")
	}
}
