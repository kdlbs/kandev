package jira

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// fakeSecretStore is an in-memory SecretStore for tests.
type fakeSecretStore struct {
	mu      sync.Mutex
	secrets map[string]string
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{secrets: map[string]string{}}
}

func (f *fakeSecretStore) Reveal(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.secrets[id]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeSecretStore) Set(_ context.Context, id, _, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[id] = value
	return nil
}

func (f *fakeSecretStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.secrets, id)
	return nil
}

func (f *fakeSecretStore) Exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.secrets[id]
	return ok, nil
}

// fakeClient is an in-memory Client for verifying service plumbing.
type fakeClient struct {
	testAuthFn    func() (*TestConnectionResult, error)
	getTicketFn   func(key string) (*JiraTicket, error)
	transitionFn  func(key, id string) error
	listProjects  func() ([]JiraProject, error)
	transitionLog []string // "key:id"
}

func (c *fakeClient) TestAuth(_ context.Context) (*TestConnectionResult, error) {
	if c.testAuthFn != nil {
		return c.testAuthFn()
	}
	return &TestConnectionResult{OK: true}, nil
}
func (c *fakeClient) GetTicket(_ context.Context, k string) (*JiraTicket, error) {
	if c.getTicketFn != nil {
		return c.getTicketFn(k)
	}
	return &JiraTicket{Key: k}, nil
}
func (c *fakeClient) ListTransitions(_ context.Context, _ string) ([]JiraTransition, error) {
	return nil, nil
}
func (c *fakeClient) DoTransition(_ context.Context, key, id string) error {
	c.transitionLog = append(c.transitionLog, key+":"+id)
	if c.transitionFn != nil {
		return c.transitionFn(key, id)
	}
	return nil
}
func (c *fakeClient) ListProjects(_ context.Context) ([]JiraProject, error) {
	if c.listProjects != nil {
		return c.listProjects()
	}
	return nil, nil
}
func (c *fakeClient) SearchTickets(_ context.Context, _, _ string, _ int) (*SearchResult, error) {
	return &SearchResult{}, nil
}

type svcFixture struct {
	svc        *Service
	store      *Store
	secrets    *fakeSecretStore
	client     *fakeClient
	factoryHit int
	probed     chan string
}

func newSvcFixture(t *testing.T) *svcFixture {
	t.Helper()
	f := &svcFixture{
		store:   newTestStore(t),
		secrets: newFakeSecretStore(),
		client:  &fakeClient{},
		probed:  make(chan string, 8),
	}
	f.svc = NewService(f.store, f.secrets, func(_ *JiraConfig, _ string) Client {
		f.factoryHit++
		return f.client
	}, logger.Default())
	f.svc.SetProbeHook(func(workspaceID string) {
		select {
		case f.probed <- workspaceID:
		default:
		}
	})
	return f
}

func TestService_SetConfig_UpsertsAndStoresSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	cfg, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1",
		SiteURL:     "https://acme.atlassian.net/",
		Email:       "u@example.com",
		AuthMethod:  AuthMethodAPIToken,
		Secret:      "tok1",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.SiteURL != "https://acme.atlassian.net" {
		t.Errorf("site url not trimmed: %q", cfg.SiteURL)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKeyForWorkspace("ws-1")); got != "tok1" {
		t.Errorf("secret stored = %q", got)
	}
}

func TestService_SetConfig_ProbesAuthImmediately(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "tok",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg := waitForAuthProbe(t, f, "ws-1")
	if !cfg.LastOk {
		t.Errorf("expected LastOk=true after async probe, got %+v", cfg)
	}
	if cfg.LastCheckedAt == nil {
		t.Error("expected LastCheckedAt to be set after async probe")
	}
}

func TestService_SetConfig_PersistsProbeFailure(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "bad",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg := waitForAuthProbe(t, f, "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected probe error preserved, got %q", cfg.LastError)
	}
}

// waitForAuthProbe blocks until the async probe spawned by SetConfig has
// completed (signaled via the fixture's probeHook), then returns the persisted
// config. A 2s ceiling guards against bugs that prevent the hook from firing.
func waitForAuthProbe(t *testing.T, f *svcFixture, workspaceID string) *JiraConfig {
	t.Helper()
	for {
		select {
		case got := <-f.probed:
			if got != workspaceID {
				continue
			}
			cfg, err := f.svc.GetConfig(context.Background(), workspaceID)
			if err != nil {
				t.Fatalf("get config after probe: %v", err)
			}
			return cfg
		case <-time.After(2 * time.Second):
			t.Fatalf("async probe hook did not fire for %q within 2s", workspaceID)
			return nil
		}
	}
}

func TestService_SetConfig_EmptySecret_KeepsExisting(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "first",
	})
	if err != nil {
		t.Fatalf("initial: %v", err)
	}
	_, err = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "new@x",
		AuthMethod: AuthMethodAPIToken, Secret: "",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKeyForWorkspace("ws-1")); got != "first" {
		t.Errorf("secret should be preserved, got %q", got)
	}
}

func TestService_SetConfig_InvalidatesClientCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "t",
	})
	// First ticket call builds client.
	if _, err := f.svc.GetTicket(ctx, "ws-1", "A-1"); err != nil {
		t.Fatalf("get1: %v", err)
	}
	hits := f.factoryHit
	// Second call reuses cached client.
	if _, err := f.svc.GetTicket(ctx, "ws-1", "A-2"); err != nil {
		t.Fatalf("get2: %v", err)
	}
	if f.factoryHit != hits {
		t.Errorf("factory should be cached, hits %d→%d", hits, f.factoryHit)
	}
	// Updating config invalidates cache.
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "t2",
	})
	if _, err := f.svc.GetTicket(ctx, "ws-1", "A-3"); err != nil {
		t.Fatalf("get3: %v", err)
	}
	if f.factoryHit <= hits {
		t.Errorf("factory should rebuild after config change, hits=%d", f.factoryHit)
	}
}

func TestService_Validation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		req  SetConfigRequest
	}{
		{"missing ws", SetConfigRequest{SiteURL: "x", AuthMethod: AuthMethodAPIToken, Email: "e"}},
		{"missing site", SetConfigRequest{WorkspaceID: "w", AuthMethod: AuthMethodAPIToken, Email: "e"}},
		{"missing email api_token", SetConfigRequest{WorkspaceID: "w", SiteURL: "x", AuthMethod: AuthMethodAPIToken}},
		{"bad auth", SetConfigRequest{WorkspaceID: "w", SiteURL: "x", AuthMethod: "bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.SetConfig(ctx, &tc.req); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestService_TestConnection_InlineSecret(t *testing.T) {
	f := newSvcFixture(t)
	called := false
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		called = true
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	res, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "inline",
	})
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
	if !res.OK || res.DisplayName != "Alice" {
		t.Errorf("result: %+v", res)
	}
}

func TestService_TestConnection_NoStoredSecret_ReturnsFailure(t *testing.T) {
	f := newSvcFixture(t)
	res, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-nope", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
}

func TestService_DeleteConfig_RemovesSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "t",
	})
	if err := f.svc.DeleteConfig(ctx, "ws-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if exists, _ := f.secrets.Exists(ctx, SecretKeyForWorkspace("ws-1")); exists {
		t.Error("secret should be removed")
	}
	cfg, _ := f.svc.GetConfig(ctx, "ws-1")
	if cfg != nil {
		t.Errorf("expected config gone, got %+v", cfg)
	}
}

func TestService_GetTicket_UnconfiguredWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.GetTicket(context.Background(), "ws-nope", "X-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestService_DoTransition_PassThrough(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "t",
	})
	if err := f.svc.DoTransition(ctx, "ws-1", "PROJ-9", "31"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if len(f.client.transitionLog) != 1 || f.client.transitionLog[0] != "PROJ-9:31" {
		t.Errorf("log: %v", f.client.transitionLog)
	}
}
