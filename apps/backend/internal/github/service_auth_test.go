package github

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

const testWorkspaceID = "workspace-test"

type testConnectionReader struct {
	workspaces map[string]*WorkspaceConnection
}

func (r testConnectionReader) GetWorkspaceConnection(
	_ context.Context,
	workspaceID string,
) (*WorkspaceConnection, error) {
	return r.workspaces[workspaceID], nil
}

func (testConnectionReader) GetUserConnection(
	context.Context,
	string,
	string,
) (*UserConnection, error) {
	return nil, nil
}

type testAutomationCredentialProvider struct {
	client Client
}

func (p testAutomationCredentialProvider) ResolveAutomation(
	_ context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client:       p.client,
		Capabilities: allTokenCapabilities(),
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourcePAT,
			Login:  connection.Login,
		},
	}, nil
}

// configureTestWorkspaceAuth gives legacy service fixtures an explicit,
// workspace-owned PAT principal while preserving their injected mock client.
func configureTestWorkspaceAuth(
	t *testing.T,
	service *Service,
	client Client,
	workspaceIDs ...string,
) {
	t.Helper()
	connections := make(map[string]*WorkspaceConnection, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		connections[workspaceID] = &WorkspaceConnection{
			WorkspaceID:          workspaceID,
			Source:               ConnectionSourcePAT,
			GitHubHost:           defaultGitHubHost,
			Login:                "test-user",
			Status:               ConnectionStatusActive,
			CredentialGeneration: 1,
		}
	}
	service.resolver = NewCredentialResolver(testConnectionReader{workspaces: connections}, nil)
	service.resolver.SetAutomationProvider(testAutomationCredentialProvider{client: client})
}

func newWorkspaceAuthenticatedTestService(
	t *testing.T,
	client Client,
	store *Store,
	workspaceIDs ...string,
) *Service {
	t.Helper()
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	configureTestWorkspaceAuth(t, service, client, workspaceIDs...)
	return service
}

func TestNewServiceCoordinatesLegacyClientByResolvedLogin(t *testing.T) {
	client := &PATClient{username: "test-user"}
	service := NewService(client, AuthMethodPAT, nil, nil, nil, testLogger(t))
	t.Cleanup(service.Stop)
	resolvedTracker, _ := service.rateCoordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Source: ConnectionSourceLegacyShared, Login: "test-user",
	}, nil)
	if service.rateTracker != resolvedTracker {
		t.Fatal("startup legacy client was not registered under its resolved login")
	}
}

func TestCoordinateLegacyClientGatesIdentityProbe(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	reset := time.Now().Add(time.Hour)
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, RemainingObserved: true,
		Limit: 5000, ResetAt: reset, UpdatedAt: time.Now().UTC(),
	})
	var requests atomic.Int32
	client := &PATClient{
		token: "token",
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		})},
	}
	service := &Service{
		rateTracker:     tracker,
		rateCoordinator: NewRateCoordinator(nil, testLogger(t)),
		logger:          testLogger(t),
	}

	done := make(chan struct{})
	go func() {
		service.coordinateLegacyClient(client, "")
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("identity probe completed while the primary bucket was exhausted")
	case <-time.After(25 * time.Millisecond):
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("identity probe requests = %d, want zero while admission is blocked", got)
	}

	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 100, RemainingObserved: true,
		Limit: 5000, ResetAt: reset, UpdatedAt: time.Now().UTC(),
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("identity probe did not complete after admission recovered")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("identity probe requests = %d, want one", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withTestWorkspace(watch *PRWatch) *PRWatch {
	if watch.WorkspaceID == "" {
		watch.WorkspaceID = testWorkspaceID
	}
	return watch
}

func testAutomationScope(t *testing.T, service *Service, workspaceID string) string {
	t.Helper()
	resolved, err := service.resolveAutomationClient(context.Background(), workspaceID, "", "")
	if err != nil {
		t.Fatalf("resolve test automation client: %v", err)
	}
	return resolved.CacheScope
}
