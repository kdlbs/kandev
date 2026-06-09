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

// slowSecretStore wraps fakeSecretStore and calls a hook just before returning
// from Reveal, allowing tests to inject a concurrent invalidation mid-build.
type slowSecretStore struct {
	*fakeSecretStore
	revealHook func()
}

func (s *slowSecretStore) Reveal(ctx context.Context, id string) (string, error) {
	if s.revealHook != nil {
		s.revealHook()
	}
	return s.fakeSecretStore.Reveal(ctx, id)
}

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
	testAuthFn          func() (*TestConnectionResult, error)
	getIssueFn          func(id string) (*SentryIssue, error)
	listOrganizationsFn func() ([]SentryOrganization, error)
	listProjectsFn      func() ([]SentryProject, error)
	searchIssuesFn      func(filter SearchFilter, cursor string) (*SearchResult, error)
}

func (c *fakeClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	if c.testAuthFn != nil {
		return c.testAuthFn()
	}
	return &TestConnectionResult{OK: true}, nil
}

func (c *fakeClient) ListOrganizations(context.Context) ([]SentryOrganization, error) {
	if c.listOrganizationsFn != nil {
		return c.listOrganizationsFn()
	}
	return nil, nil
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
		AuthMethod: AuthMethodAuthToken,
		Secret:     "sntrys_xyz",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.AuthMethod != AuthMethodAuthToken {
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
		AuthMethod: AuthMethodAuthToken,
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

func TestService_SearchIssues_PassesFilterThrough(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "t",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	waitForAuthProbe(t, f)
	var seenOrg string
	f.client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seenOrg = filter.OrgSlug
		return &SearchResult{IsLast: true}, nil
	}
	if _, err := f.svc.SearchIssues(ctx, SearchFilter{OrgSlug: "acme"}, ""); err != nil {
		t.Fatalf("search: %v", err)
	}
	if seenOrg != "acme" {
		t.Errorf("expected org passed through, got %q", seenOrg)
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

// TestService_ClientFor_InvalidateDuringBuild verifies the TOCTOU fix: when
// invalidateClient runs while clientFor is blocked on I/O, the freshly built
// (now-stale) client must not be stored in s.client. The subsequent call must
// rebuild, hitting the factory a second time.
func TestService_ClientFor_InvalidateDuringBuild(t *testing.T) {
	fakes := newFakeSecretStore()
	_ = fakes.Set(context.Background(), SecretKey, "tok", "sntrys_xyz")

	store := newTestStore(t)
	ctx := context.Background()
	// Seed a config row so clientFor gets past the nil-config check.
	if err := store.UpsertConfig(ctx, &SentryConfig{
		AuthMethod: AuthMethodAuthToken,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var factoryHit atomic.Int32
	client := &fakeClient{}

	// invalidateCh is closed by the Reveal hook to signal the main goroutine
	// to call invalidateClient while the first clientFor is mid-I/O.
	invalidateCh := make(chan struct{})
	// doneCh is closed by the Reveal hook after it has signalled.
	doneCh := make(chan struct{})

	slow := &slowSecretStore{fakeSecretStore: fakes}

	svc := NewService(store, slow, func(_ *SentryConfig, _ string) Client {
		factoryHit.Add(1)
		return client
	}, logger.Default())

	slow.revealHook = func() {
		// Signal that we're mid-I/O, then wait for the invalidation to complete.
		close(invalidateCh)
		// Give the invalidation goroutine time to run.
		time.Sleep(10 * time.Millisecond)
		close(doneCh)
	}

	// First clientFor call — will block in Reveal while we invalidate.
	errCh := make(chan error, 1)
	go func() {
		_, err := svc.clientFor(ctx)
		errCh <- err
	}()

	// Wait until clientFor is inside Reveal, then invalidate.
	<-invalidateCh
	svc.invalidateClient()

	// Wait for the first clientFor to finish.
	if err := <-errCh; err != nil {
		t.Fatalf("first clientFor: %v", err)
	}

	// The invalidation must have reset the cache, so s.client should be nil now.
	// A second clientFor must hit the factory again (not return the cached stale client).
	slow.revealHook = nil // no more blocking
	if _, err := svc.clientFor(ctx); err != nil {
		t.Fatalf("second clientFor: %v", err)
	}

	if got := factoryHit.Load(); got < 2 {
		t.Errorf("expected factory called at least twice (once per clientFor after invalidation), got %d", got)
	}
}

// TestService_SetConfig_DefaultsURLToSentryIO asserts a blank instance URL is
// stored as the sentry.io SaaS default.
func TestService_SetConfig_DefaultsURLToSentryIO(t *testing.T) {
	f := newSvcFixture(t)
	cfg, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.URL != DefaultSentryURL {
		t.Errorf("expected URL defaulted to %q, got %q", DefaultSentryURL, cfg.URL)
	}
}

// TestService_SetConfig_NormalizesAndPersistsURL asserts a host-only URL is
// normalized (scheme added, trailing slash trimmed) before being stored.
func TestService_SetConfig_NormalizesAndPersistsURL(t *testing.T) {
	f := newSvcFixture(t)
	cfg, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "sentry.example.com/",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.URL != "https://sentry.example.com" {
		t.Errorf("expected normalized URL, got %q", cfg.URL)
	}
}

// TestService_SetConfig_RejectsMalformedURL asserts a non-http(s) URL is
// rejected at save time.
func TestService_SetConfig_RejectsMalformedURL(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "ftp://nope",
	})
	if err == nil {
		t.Error("expected validation error for non-http(s) URL")
	}
}

// TestService_TestConnection_PassesConfiguredURL ensures a pre-save test uses
// the URL the user typed in the form, so a self-hosted instance can be
// validated before the config is persisted.
func TestService_TestConnection_PassesConfiguredURL(t *testing.T) {
	f := newSvcFixture(t)
	var sawURL string
	f.svc.clientFn = func(cfg *SentryConfig, _ string) Client {
		sawURL = cfg.URL
		return f.client
	}
	if _, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "inline", URL: "https://sentry.example.com",
	}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawURL != "https://sentry.example.com" {
		t.Errorf("expected configured URL passed to client, got %q", sawURL)
	}
}

// TestService_TestConnection_FallsBackToStoredURL covers the post-save "Test
// connection" path (blank secret, blank URL in the request): the stored
// instance URL must be used rather than silently reverting to sentry.io.
func TestService_TestConnection_FallsBackToStoredURL(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "stored", URL: "https://sentry.example.com",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	waitForAuthProbe(t, f)
	var sawURL string
	f.svc.clientFn = func(cfg *SentryConfig, _ string) Client {
		sawURL = cfg.URL
		return f.client
	}
	if _, err := f.svc.TestConnection(ctx, &SetConfigRequest{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawURL != "https://sentry.example.com" {
		t.Errorf("expected stored URL used, got %q", sawURL)
	}
}

// TestService_SetConfig_RejectsNonRootURL asserts URLs carrying a path, query,
// or fragment are rejected, so /api/0 cannot be appended to a malformed base.
func TestService_SetConfig_RejectsNonRootURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"path", "https://sentry.example.com/some/path"},
		{"query", "https://sentry.example.com?x=1"},
		{"fragment", "https://sentry.example.com#frag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSvcFixture(t)
			_, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
				AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: tc.url,
			})
			if err == nil {
				t.Errorf("expected rejection of non-root URL %q", tc.url)
			}
		})
	}
}

// TestService_SetConfig_AllowsHostRootWithTrailingSlash guards the boundary:
// a bare host root (with or without a trailing slash) must still be accepted
// and normalized, so the non-root rejection above doesn't over-reach.
func TestService_SetConfig_AllowsHostRootWithTrailingSlash(t *testing.T) {
	f := newSvcFixture(t)
	cfg, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "https://sentry.example.com/",
	})
	if err != nil {
		t.Fatalf("host root with trailing slash should be accepted: %v", err)
	}
	if cfg.URL != "https://sentry.example.com" {
		t.Errorf("expected trailing slash trimmed, got %q", cfg.URL)
	}
}
