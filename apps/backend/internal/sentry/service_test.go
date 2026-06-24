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

// create persists an instance via the service and returns it (fails the test on
// error).
func (f *svcFixture) create(t *testing.T, req *SetConfigRequest) *SentryConfig {
	t.Helper()
	cfg, err := f.svc.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	return cfg
}

// waitProbe blocks until the async auth probe fires, then returns the refreshed
// instance config.
func (f *svcFixture) waitProbe(t *testing.T, id string) *SentryConfig {
	t.Helper()
	select {
	case <-f.probed:
		cfg, err := f.svc.GetInstance(context.Background(), id)
		if err != nil {
			t.Fatalf("get instance after probe: %v", err)
		}
		return cfg
	case <-time.After(2 * time.Second):
		t.Fatalf("async probe hook did not fire within 2s")
		return nil
	}
}

func TestService_CreateInstance_PersistsAndStoresSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	cfg := f.create(t, &SetConfigRequest{Name: "Prod", AuthMethod: AuthMethodAuthToken, Secret: "sntrys_xyz"})
	if cfg.ID == "" || cfg.Name != "Prod" || cfg.AuthMethod != AuthMethodAuthToken {
		t.Errorf("config not stored: %+v", cfg)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
	if got, _ := f.secrets.Reveal(ctx, secretKeyFor(cfg.ID)); got != "sntrys_xyz" {
		t.Errorf("secret stored = %q", got)
	}
}

func TestService_GetInstance_HidesSecretValue(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "secret-tok"})
	cfg, err := f.svc.GetInstance(ctx, created.ID)
	if err != nil || cfg == nil {
		t.Fatalf("get: %v / %v", err, cfg)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
}

func TestService_UpdateInstance_EmptySecret_KeepsExisting(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "first"})
	if _, err := f.svc.UpdateInstance(ctx, created.ID, &SetConfigRequest{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := f.secrets.Reveal(ctx, secretKeyFor(created.ID)); got != "first" {
		t.Errorf("secret should be preserved, got %q", got)
	}
}

func TestService_UpdateInstance_UnknownID(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.UpdateInstance(context.Background(), "ghost", &SetConfigRequest{AuthMethod: AuthMethodAuthToken})
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound, got %v", err)
	}
}

func TestService_CreateInstance_ProbesAsync(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	cfg := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "tok"})
	got := f.waitProbe(t, cfg.ID)
	if !got.LastOk {
		t.Errorf("expected LastOk=true after async probe, got %+v", got)
	}
}

func TestService_Validation_RejectsBadAuth(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CreateInstance(context.Background(), &SetConfigRequest{AuthMethod: "bogus"}); err == nil {
		t.Error("expected validation error for unknown auth method")
	}
}

func TestService_Validation_DefaultsAuthMethod(t *testing.T) {
	f := newSvcFixture(t)
	if _, err := f.svc.CreateInstance(context.Background(), &SetConfigRequest{Secret: "tok"}); err != nil {
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
	res, err := f.svc.TestConnection(context.Background(), "", &SetConfigRequest{
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
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "stored"})
	f.waitProbe(t, created.ID)
	var sawSecret string
	f.svc.clientFn = func(_ *SentryConfig, secret string) Client {
		sawSecret = secret
		return f.client
	}
	if _, err := f.svc.TestConnection(ctx, created.ID, &SetConfigRequest{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawSecret != "stored" {
		t.Errorf("expected stored secret used, got %q", sawSecret)
	}
}

func TestService_TestConnection_NoStoredSecret_ReturnsFailure(t *testing.T) {
	f := newSvcFixture(t)
	res, err := f.svc.TestConnection(context.Background(), "", &SetConfigRequest{AuthMethod: AuthMethodAuthToken})
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
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "t"})
	f.waitProbe(t, created.ID)
	var seenOrg string
	f.client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seenOrg = filter.OrgSlug
		return &SearchResult{IsLast: true}, nil
	}
	if _, err := f.svc.SearchIssues(ctx, created.ID, SearchFilter{OrgSlug: "acme"}, ""); err != nil {
		t.Fatalf("search: %v", err)
	}
	if seenOrg != "acme" {
		t.Errorf("expected org passed through, got %q", seenOrg)
	}
}

func TestService_GetIssue_NotConfigured(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.GetIssue(context.Background(), "ghost-instance", "PROJ-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestService_GetIssue_EmptyInstance(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.GetIssue(context.Background(), "", "PROJ-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured for empty instance id, got %v", err)
	}
}

func TestService_DeleteInstance_RemovesSecretAndCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "t"})
	f.waitProbe(t, created.ID)
	if err := f.svc.DeleteInstance(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if exists, _ := f.secrets.Exists(ctx, secretKeyFor(created.ID)); exists {
		t.Error("expected secret removed")
	}
	cfg, _ := f.svc.GetInstance(ctx, created.ID)
	if cfg != nil {
		t.Errorf("expected config gone, got %+v", cfg)
	}
}

// TestService_DeleteInstance_InUse_Blocked asserts a delete is refused with
// ErrInstanceInUse (carrying the watch count) while watches still reference it.
func TestService_DeleteInstance_InUse_Blocked(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "t"})
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		InstanceID:     created.ID,
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Filter:         SearchFilter{OrgSlug: "org", ProjectSlug: "proj"},
	}); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	err := f.svc.DeleteInstance(ctx, created.ID)
	var inUse *ErrInstanceInUse
	if !errors.As(err, &inUse) {
		t.Fatalf("expected ErrInstanceInUse, got %v", err)
	}
	if inUse.WatchCount != 1 {
		t.Errorf("expected WatchCount=1, got %d", inUse.WatchCount)
	}
	// The instance must still exist after a blocked delete.
	if cfg, _ := f.svc.GetInstance(ctx, created.ID); cfg == nil {
		t.Error("instance should remain after blocked delete")
	}
}

// TestService_TwoInstances_DistinctClients asserts each instance builds a client
// from its own config (distinct URLs), proving the per-instance cache routes
// browse calls to the right Sentry host.
func TestService_TwoInstances_DistinctClients(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	ctx := context.Background()

	var mu sync.Mutex
	builtURLs := map[string]int{}
	svc := NewService(store, secrets, func(cfg *SentryConfig, _ string) Client {
		mu.Lock()
		builtURLs[cfg.URL]++
		mu.Unlock()
		return &fakeClient{}
	}, logger.Default())

	a := mustCreate(t, svc, &SetConfigRequest{Name: "A", AuthMethod: AuthMethodAuthToken, Secret: "ta", URL: "https://sentry.io"})
	b := mustCreate(t, svc, &SetConfigRequest{Name: "B", AuthMethod: AuthMethodAuthToken, Secret: "tb", URL: "https://sentry.acme.com"})

	if _, err := svc.ListProjects(ctx, a.ID); err != nil {
		t.Fatalf("list projects A: %v", err)
	}
	if _, err := svc.ListProjects(ctx, b.ID); err != nil {
		t.Fatalf("list projects B: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if builtURLs["https://sentry.io"] == 0 || builtURLs["https://sentry.acme.com"] == 0 {
		t.Errorf("expected a client built per instance URL, got %v", builtURLs)
	}
}

// TestService_InvalidateInstance_KeepsOthersCached asserts invalidating one
// instance does not drop a sibling's cached client.
func TestService_InvalidateInstance_KeepsOthersCached(t *testing.T) {
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	ctx := context.Background()

	var hits atomic.Int32
	svc := NewService(store, secrets, func(_ *SentryConfig, _ string) Client {
		hits.Add(1)
		return &fakeClient{}
	}, logger.Default())

	a := mustCreate(t, svc, &SetConfigRequest{Name: "A", AuthMethod: AuthMethodAuthToken, Secret: "ta"})
	b := mustCreate(t, svc, &SetConfigRequest{Name: "B", AuthMethod: AuthMethodAuthToken, Secret: "tb"})
	// Warm both caches.
	_, _ = svc.ListProjects(ctx, a.ID)
	_, _ = svc.ListProjects(ctx, b.ID)
	hitsAfterWarm := hits.Load()

	svc.invalidateInstance(a.ID)
	// B stays cached: no new factory hit.
	if _, err := svc.ListProjects(ctx, b.ID); err != nil {
		t.Fatalf("list B: %v", err)
	}
	if hits.Load() != hitsAfterWarm {
		t.Errorf("invalidating A should not rebuild B; hits %d -> %d", hitsAfterWarm, hits.Load())
	}
	// A rebuilds.
	if _, err := svc.ListProjects(ctx, a.ID); err != nil {
		t.Fatalf("list A: %v", err)
	}
	if hits.Load() <= hitsAfterWarm {
		t.Errorf("expected A to rebuild after invalidation, hits=%d", hits.Load())
	}
}

// TestService_RecordAuthHealth_StampsAllInstances asserts the poller-facing
// RecordAuthHealth probes and stamps every configured instance.
func TestService_RecordAuthHealth_StampsAllInstances(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	a := f.create(t, &SetConfigRequest{Name: "A", AuthMethod: AuthMethodAuthToken, Secret: "ta"})
	b := f.create(t, &SetConfigRequest{Name: "B", AuthMethod: AuthMethodAuthToken, Secret: "tb"})
	// Drain probes from the two async creates.
	f.waitProbe(t, a.ID)
	f.waitProbe(t, b.ID)

	f.svc.RecordAuthHealth(ctx)
	for _, id := range []string{a.ID, b.ID} {
		cfg, _ := f.svc.GetInstance(ctx, id)
		if cfg == nil || cfg.LastCheckedAt == nil {
			t.Errorf("expected instance %s probed, got %+v", id, cfg)
		}
	}
}

func mustCreate(t *testing.T, svc *Service, req *SetConfigRequest) *SentryConfig {
	t.Helper()
	cfg, err := svc.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	return cfg
}

// TestService_ClientForInstance_InvalidateDuringBuild verifies the per-instance
// TOCTOU fix: when invalidateInstance runs while clientForInstance is blocked on
// I/O, the freshly built (now-stale) client must not be cached. The subsequent
// call must rebuild, hitting the factory a second time.
func TestService_ClientForInstance_InvalidateDuringBuild(t *testing.T) {
	fakes := newFakeSecretStore()
	store := newTestStore(t)
	ctx := context.Background()
	id := createTestConfig(t, store, "A", "https://sentry.io")
	_ = fakes.Set(ctx, secretKeyFor(id), "tok", "sntrys_xyz")

	var factoryHit atomic.Int32
	client := &fakeClient{}

	invalidateCh := make(chan struct{})
	doneCh := make(chan struct{})

	slow := &slowSecretStore{fakeSecretStore: fakes}
	svc := NewService(store, slow, func(_ *SentryConfig, _ string) Client {
		factoryHit.Add(1)
		return client
	}, logger.Default())

	slow.revealHook = func() {
		close(invalidateCh)
		time.Sleep(10 * time.Millisecond)
		close(doneCh)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.clientForInstance(ctx, id)
		errCh <- err
	}()

	<-invalidateCh
	svc.invalidateInstance(id)

	if err := <-errCh; err != nil {
		t.Fatalf("first clientForInstance: %v", err)
	}

	slow.revealHook = nil
	if _, err := svc.clientForInstance(ctx, id); err != nil {
		t.Fatalf("second clientForInstance: %v", err)
	}

	if got := factoryHit.Load(); got < 2 {
		t.Errorf("expected factory called at least twice after invalidation, got %d", got)
	}
}

// TestService_CreateInstance_DefaultsURLToSentryIO asserts a blank instance URL
// is stored as the sentry.io SaaS default.
func TestService_CreateInstance_DefaultsURLToSentryIO(t *testing.T) {
	f := newSvcFixture(t)
	cfg := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "tok"})
	if cfg.URL != DefaultSentryURL {
		t.Errorf("expected URL defaulted to %q, got %q", DefaultSentryURL, cfg.URL)
	}
}

// TestService_CreateInstance_NormalizesAndPersistsURL asserts a host-only URL is
// normalized (scheme added, trailing slash trimmed) before being stored.
func TestService_CreateInstance_NormalizesAndPersistsURL(t *testing.T) {
	f := newSvcFixture(t)
	cfg := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "sentry.example.com/"})
	if cfg.URL != "https://sentry.example.com" {
		t.Errorf("expected normalized URL, got %q", cfg.URL)
	}
}

// TestService_CreateInstance_RejectsMalformedURL asserts a non-http(s) URL is
// rejected at save time.
func TestService_CreateInstance_RejectsMalformedURL(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.CreateInstance(context.Background(), &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "ftp://nope",
	})
	if err == nil {
		t.Error("expected validation error for non-http(s) URL")
	}
}

// TestService_TestConnection_PassesConfiguredURL ensures a pre-save test uses
// the URL the user typed in the form, so a self-hosted instance can be validated
// before the config is persisted.
func TestService_TestConnection_PassesConfiguredURL(t *testing.T) {
	f := newSvcFixture(t)
	var sawURL string
	f.svc.clientFn = func(cfg *SentryConfig, _ string) Client {
		sawURL = cfg.URL
		return f.client
	}
	if _, err := f.svc.TestConnection(context.Background(), "", &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "inline", URL: "https://sentry.example.com",
	}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawURL != "https://sentry.example.com" {
		t.Errorf("expected configured URL passed to client, got %q", sawURL)
	}
}

// TestService_TestConnection_FallsBackToStoredURL covers the post-save "Test
// connection" path (blank secret, blank URL in the request): the stored instance
// URL must be used rather than silently reverting to sentry.io.
func TestService_TestConnection_FallsBackToStoredURL(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "stored", URL: "https://sentry.example.com"})
	f.waitProbe(t, created.ID)
	var sawURL string
	f.svc.clientFn = func(cfg *SentryConfig, _ string) Client {
		sawURL = cfg.URL
		return f.client
	}
	if _, err := f.svc.TestConnection(ctx, created.ID, &SetConfigRequest{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawURL != "https://sentry.example.com" {
		t.Errorf("expected stored URL used, got %q", sawURL)
	}
}

// TestService_CreateInstance_RejectsNonRootURL asserts URLs carrying a path,
// query, or fragment are rejected, so /api/0 cannot be appended to a malformed
// base.
func TestService_CreateInstance_RejectsNonRootURL(t *testing.T) {
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
			_, err := f.svc.CreateInstance(context.Background(), &SetConfigRequest{
				AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: tc.url,
			})
			if err == nil {
				t.Errorf("expected rejection of non-root URL %q", tc.url)
			}
		})
	}
}

// TestService_CreateInstance_AllowsHostRootWithTrailingSlash guards the boundary:
// a bare host root (with or without a trailing slash) must still be accepted and
// normalized, so the non-root rejection above doesn't over-reach.
func TestService_CreateInstance_AllowsHostRootWithTrailingSlash(t *testing.T) {
	f := newSvcFixture(t)
	cfg := f.create(t, &SetConfigRequest{AuthMethod: AuthMethodAuthToken, Secret: "tok", URL: "https://sentry.example.com/"})
	if cfg.URL != "https://sentry.example.com" {
		t.Errorf("expected trailing slash trimmed, got %q", cfg.URL)
	}
}
