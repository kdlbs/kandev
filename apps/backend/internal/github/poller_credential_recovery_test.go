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

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestPRMonitoringSharesCapacityAcrossWorkspaces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		counts := map[string]int{}
		clients := watchPrincipalProvider{}
		for _, ws := range []string{"ws-1", "ws1"} {
			client := NewPATClient(ws)
			client.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"fork":false}`
				if req.URL.Path == "/graphql" {
					counts[strings.TrimPrefix(req.Header.Get("Authorization"), "token ")]++
					body = `{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			clients[ws] = client
			seedTask(t, store, ws, false)
			w := &PRWatch{WorkspaceID: ws, TaskID: ws, SessionID: ws, Owner: "o", Repo: "r", Branch: "b"}
			if err := store.CreatePRWatch(context.Background(), w); err != nil {
				t.Fatal(err)
			}
		}
		configureTestWorkspaceAuth(t, svc, nil, "ws-1", "ws1")
		svc.resolver.SetAutomationProvider(clients)
		for i := 0; i < 10; i++ {
			poller.checkPRWatches(pollerContext(context.Background()))
			time.Sleep(time.Minute)
		}
		t.Logf("GraphQL provider requests by workspace: %v", counts)
		if counts["ws1"] == 0 || counts["ws-1"] == 0 {
			t.Fatal("later workspace never receives a provider request")
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestWorkspaceFingerprintTracksLegacyTokenReplacement(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"alice"}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "work")
	ctx := context.Background()
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{WorkspaceID: "work", GitHubHost: defaultGitHubHost, Source: ConnectionSourceLegacyShared, Status: ConnectionStatusActive, CredentialGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	mgr := newMockSecretManager()
	svc := NewService(nil, AuthMethodNone, &mockSecretProvider{mgr: mgr}, store, nil, testLogger(t))
	svc.secretManager = mgr
	t.Cleanup(svc.Stop)
	if err := svc.ConfigureToken(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	before, err := svc.WorkspaceConnectionFingerprint(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfigureToken(ctx, "replacement"); err != nil {
		t.Fatal(err)
	}
	after, err := svc.WorkspaceConnectionFingerprint(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fingerprint before=%s after=%s", before, after)
	if before == after {
		t.Fatal("legacy replacement is invisible to Workflow Sync retry and continuation validation")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestPRMonitoringDoesNotGateHealthyWorkspaceOnLegacyQuota(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		calls := 0
		client := NewPATClient("healthy-workspace")
		client.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"fork":false}`
			if req.URL.Path == "/graphql" {
				calls++
				body = `{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		configureTestWorkspaceAuth(t, svc, client, "ws1")
		seedTask(t, store, "healthy", false)
		if err := store.CreatePRWatch(context.Background(), &PRWatch{WorkspaceID: "ws1", TaskID: "healthy", SessionID: "healthy", Owner: "o", Repo: "r", Branch: "b"}); err != nil {
			t.Fatal(err)
		}
		svc.RateTracker().ObservePrimary(ResourceGraphQL, time.Now().Add(time.Hour), RetrySourcePrimaryReset)
		poller.Start(context.Background())
		t.Cleanup(poller.Stop)
		time.Sleep(time.Minute)
		poller.Stop()
		if calls == 0 {
			t.Fatal("exhausted legacy quota blocked the healthy workspace's PR monitor")
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestWorkspaceFingerprintTracksAppKeyRotation(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "work")
	ctx := context.Background()
	if err := store.UpsertDeploymentAppRegistration(ctx, newAppRegistration("registration-a", 101, "Work", time.Now())); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	connection := &WorkspaceConnection{WorkspaceID: "work", Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: defaultGitHubHost, AppRegistrationID: "registration-a", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		Status: ConnectionStatusActive, CredentialGeneration: 1}
	if err := store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.SetConnectionSecretStore(newFakeConnectionSecrets())
	t.Cleanup(svc.Stop)
	readFingerprint := func() string {
		t.Helper()
		value, err := svc.WorkspaceConnectionFingerprint(ctx, "work")
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := svc.ApplyAppRegistrationRuntime(testResolvedAppRegistration(t, "registration-a", 101, 1)); err != nil {
		t.Fatal(err)
	}
	before := readFingerprint()
	if err := svc.ApplyAppRegistrationRuntime(testResolvedAppRegistration(t, "registration-a", 101, 2)); err != nil {
		t.Fatal(err)
	}
	after := readFingerprint()
	if before == after {
		t.Fatal("App key rotation did not invalidate the workspace fingerprint")
	}
	restarted := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	restarted.SetConnectionSecretStore(newFakeConnectionSecrets())
	t.Cleanup(restarted.Stop)
	if err := restarted.ApplyAppRegistrationRuntime(testResolvedAppRegistration(t, "registration-a", 101, 2)); err != nil {
		t.Fatal(err)
	}
	restored, err := restarted.WorkspaceConnectionFingerprint(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	if restored != after {
		t.Fatal("same persisted App generation changed fingerprint after restart")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.6
func TestReviewCleanupKeepsPrincipalQuotasIsolated(t *testing.T) {
	poller, svc, _, _ := setupPollerTest(t)
	t.Cleanup(svc.Stop)
	alice := NewPATClient("alice-token")
	alice.username = "alice"
	svc.bindLegacyClient(alice, "alice")
	svc.client = alice
	configureTestWorkspaceAuth(t, svc, NewPATClient("bob-token"), "ws1")
	svc.resolver.connections.(testConnectionReader).workspaces["ws1"].Login = "bob"
	svc.resolver.SetRateCoordinator(svc.rateCoordinator)
	bob, err := svc.resolveAutomationClient(context.Background(), "ws1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	alice.rateTracker.ObserveSecondary(ResourceCore, time.Now().Add(time.Hour), RetrySourceRetryAfter, "")
	cycle := newReviewCleanupCycle(pollerContext(context.Background()), poller)
	cycle.registerWatch(&ReviewWatch{ID: "bob-watch", WorkspaceID: "ws1"})
	allowed, stop := cycle.admit(&ReviewPRTask{ID: "bob-record", ReviewWatchID: "bob-watch"}, bob.RateTracker)
	t.Logf("Bob wait=%s, cleanup allowed=%v stopWorkspace=%v", bob.RateTracker.WaitDuration(ResourceCore), allowed, stop)
	if !allowed {
		t.Fatal("Alice's throttle blocked Bob's review cleanup")
	}
	bob.RateTracker.Record(RateSnapshot{Resource: ResourceCore, Limit: 5000, Remaining: 500, ResetAt: time.Now().Add(time.Hour)})
	allowed, _ = cycle.admit(&ReviewPRTask{ID: "bob-record", ReviewWatchID: "bob-watch"}, bob.RateTracker)
	if allowed {
		t.Fatal("Bob's cleanup consumed the quota reserved for interactive work")
	}
}
