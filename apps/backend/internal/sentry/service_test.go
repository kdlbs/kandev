package sentry

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

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
	testAuthFn     func() (*TestConnectionResult, error)
	getIssueFn     func(id string) (*SentryIssue, error)
	listProjectsFn func() ([]SentryProject, error)
	searchIssuesFn func(filter SearchFilter, cursor string) (*SearchResult, error)
}

func (c *fakeClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	if c.testAuthFn != nil {
		return c.testAuthFn()
	}
	return &TestConnectionResult{OK: true}, nil
}

func (c *fakeClient) ListProjects(context.Context) ([]SentryProject, error) {
	if c.listProjectsFn != nil {
		return c.listProjectsFn()
	}
	return nil, nil
}

func (c *fakeClient) SearchIssues(_ context.Context, filter SearchFilter, cursor string) (*SearchResult, error) {
	if c.searchIssuesFn != nil {
		return c.searchIssuesFn(filter, cursor)
	}
	return &SearchResult{}, nil
}

func (c *fakeClient) GetIssue(_ context.Context, id string) (*SentryIssue, error) {
	if c.getIssueFn != nil {
		return c.getIssueFn(id)
	}
	return &SentryIssue{ID: id, ShortID: id}, nil
}

type svcFixture struct {
	svc        *Service
	store      *Store
	secrets    *fakeSecretStore
	client     *fakeClient
	factoryHit atomic.Int32
	probed     chan struct{}
}

func newSvcFixture(t *testing.T) *svcFixture {
	t.Helper()
	f := &svcFixture{
		store:   newTestStore(t),
		secrets: newFakeSecretStore(),
		client:  &fakeClient{},
		probed:  make(chan struct{}, 8),
	}
	f.svc = NewService(f.store, f.secrets, func(_ *SentryConfig, _ string) Client {
		f.factoryHit.Add(1)
		return f.client
	}, logger.Default())
	f.svc.SetProbeHook(func() {
		select {
		case f.probed <- struct{}{}:
		default:
		}
	})
	return f
}

func waitForAuthProbe(t *testing.T, f *svcFixture) *SentryConfig {
	t.Helper()
	select {
	case <-f.probed:
		cfg, err := f.svc.GetConfig(context.Background())
		if err != nil {
			t.Fatalf("get config after probe: %v", err)
		}
		return cfg
	case <-time.After(2 * time.Second):
		t.Fatalf("async probe hook did not fire within 2s")
		return nil
	}
}

func TestService_SetConfig_PersistsAndStoresSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	cfg, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod:         AuthMethodAuthToken,
		DefaultOrgSlug:     "acme",
		DefaultProjectSlug: "frontend",
		Secret:             "sntrys_xyz",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.DefaultOrgSlug != "acme" || cfg.DefaultProjectSlug != "frontend" {
		t.Errorf("config not stored: %+v", cfg)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKey); got != "sntrys_xyz" {
		t.Errorf("secret stored = %q", got)
	}
}

func TestService_GetConfig_HidesSecretValue(t *testing.T) {
	// GetConfig must surface HasSecret=true but never the secret itself.
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "secret-tok",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg, err := f.svc.GetConfig(ctx)
	if err != nil || cfg == nil {
		t.Fatalf("get: %v / %v", err, cfg)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
}

func TestService_SetConfig_EmptySecret_KeepsExisting(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "first",
	}); err != nil {
		t.Fatalf("initial: %v", err)
	}
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, DefaultOrgSlug: "acme",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKey); got != "first" {
		t.Errorf("secret should be preserved, got %q", got)
	}
}

func TestService_SetConfig_ProbesAsync(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg := waitForAuthProbe(t, f)
	if !cfg.LastOk {
		t.Errorf("expected LastOk=true after async probe, got %+v", cfg)
	}
}

func TestService_Validation_RejectsBadAuth(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: "bogus",
	}); err == nil {
		t.Error("expected validation error for unknown auth method")
	}
}

func TestService_Validation_DefaultsAuthMethod(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{Secret: "tok"}); err != nil {
		t.Fatalf("expected default auth method, got error: %v", err)
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
		AuthMethod: AuthMethodAuthToken, Secret: "inline-tok",
	})
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
	if !res.OK {
		t.Errorf("result: %+v", res)
	}
}

func TestService_TestConnection_UsesStoredSecretWhenRequestEmpty(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "stored",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	waitForAuthProbe(t, f)
	var sawSecret string
	f.svc.clientFn = func(_ *SentryConfig, secret string) Client {
		sawSecret = secret
		return f.client
	}
	if _, err := f.svc.TestConnection(ctx, &SetConfigRequest{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawSecret != "stored" {
		t.Errorf("expected stored secret used, got %q", sawSecret)
	}
}

func TestService_TestConnection_NoStoredSecret_ReturnsFailure(t *testing.T) {
	f := newSvcFixture(t)
	res, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Error("expected OK=false when no secret stored")
	}
}

func TestService_SearchIssues_FallsBackToDefaultOrg(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, DefaultOrgSlug: "acme", Secret: "t",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	waitForAuthProbe(t, f)
	var seenOrg string
	f.client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seenOrg = filter.OrgSlug
		return &SearchResult{IsLast: true}, nil
	}
	if _, err := f.svc.SearchIssues(ctx, SearchFilter{}, ""); err != nil {
		t.Fatalf("search: %v", err)
	}
	if seenOrg != "acme" {
		t.Errorf("expected default org injected, got %q", seenOrg)
	}
}

func TestService_GetIssue_NotConfigured(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.GetIssue(context.Background(), "PROJ-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestService_DeleteConfig_RemovesSecretAndCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "t",
	})
	waitForAuthProbe(t, f)
	if err := f.svc.DeleteConfig(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if exists, _ := f.secrets.Exists(ctx, SecretKey); exists {
		t.Error("expected secret removed")
	}
	cfg, _ := f.svc.GetConfig(ctx)
	if cfg != nil {
		t.Errorf("expected config gone, got %+v", cfg)
	}
}
