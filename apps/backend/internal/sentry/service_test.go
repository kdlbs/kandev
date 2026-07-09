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
		return "", errors.New("secret not found: " + id)
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

// seedInstance persists an instance + secret directly via the store/secret fake,
// bypassing Service.CreateInstance (and its async probe).
func (f *svcFixture) seedInstance(t *testing.T, workspaceID, name, secret string) *SentryConfig {
	t.Helper()
	cfg := &SentryConfig{WorkspaceID: workspaceID, Name: name, AuthMethod: AuthMethodAuthToken, URL: DefaultSentryURL}
	if err := f.store.CreateInstance(context.Background(), cfg); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if secret != "" {
		if err := f.secrets.Set(context.Background(), secretKeyForInstance(cfg.ID), "tok", secret); err != nil {
			t.Fatalf("seed secret: %v", err)
		}
	}
	return cfg
}

// ensureInstance returns the ID of a workspace's sole instance, creating a
// "Primary" instance when none exists. Reused across tests that need a valid
// SentryInstanceID for a watch without minding which instance it is.
func (f *svcFixture) ensureInstance(t *testing.T, workspaceID string) string {
	t.Helper()
	instances, err := f.store.ListInstances(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(instances) > 0 {
		return instances[0].ID
	}
	return f.seedInstance(t, workspaceID, "Primary", "").ID
}

func waitForAuthProbe(t *testing.T, f *svcFixture) {
	t.Helper()
	select {
	case <-f.probed:
	case <-time.After(2 * time.Second):
		t.Fatalf("async probe hook did not fire within 2s")
	}
}

func TestService_CreateInstance_PersistsAndStoresSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	cfg, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{
		Name: "SaaS", AuthMethod: AuthMethodAuthToken, Secret: "sntrys_xyz",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cfg.ID == "" || cfg.WorkspaceID != "ws-1" || cfg.Name != "SaaS" {
		t.Errorf("instance not stored: %+v", cfg)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
	if got, _ := f.secrets.Reveal(ctx, secretKeyForInstance(cfg.ID)); got != "sntrys_xyz" {
		t.Errorf("secret stored under instance key = %q", got)
	}
	waitForAuthProbe(t, f)
	reloaded, _ := f.store.GetInstance(ctx, cfg.ID)
	if !reloaded.LastOk {
		t.Errorf("expected LastOk=true after async probe, got %+v", reloaded)
	}
}

func TestService_CreateInstance_Validation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	cases := map[string]*CreateConfigRequest{
		"missing name": {Name: "", Secret: "t"},
		"bad auth":     {Name: "X", AuthMethod: "bogus"},
		"bad scheme":   {Name: "X", URL: "ftp://nope"},
		"non-root url": {Name: "X", URL: "https://sentry.example.com/path"},
	}
	for name, req := range cases {
		if _, err := f.svc.CreateInstance(ctx, "ws-1", req); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: expected ErrInvalidConfig, got %v", name, err)
		}
	}
}

func TestService_CreateInstance_DefaultsAndNormalizesURL(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	blank, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "SaaS", Secret: "t"})
	if err != nil {
		t.Fatalf("create blank url: %v", err)
	}
	if blank.URL != DefaultSentryURL {
		t.Errorf("expected URL defaulted to %q, got %q", DefaultSentryURL, blank.URL)
	}
	host, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "Self", URL: "sentry.example.com/", Secret: "t"})
	if err != nil {
		t.Fatalf("create host url: %v", err)
	}
	if host.URL != "https://sentry.example.com" {
		t.Errorf("expected normalized URL, got %q", host.URL)
	}
}

// TestService_UniqueName_PerWorkspace pins acceptance (h) at the service layer.
func TestService_UniqueName_PerWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "Prod", Secret: "t"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "Prod", Secret: "t"}); !errors.Is(err, ErrDuplicateInstanceName) {
		t.Errorf("expected ErrDuplicateInstanceName, got %v", err)
	}
	if _, err := f.svc.CreateInstance(ctx, "ws-2", &CreateConfigRequest{Name: "Prod", Secret: "t"}); err != nil {
		t.Errorf("same name in another workspace should succeed, got %v", err)
	}
}

// TestService_InstanceCRUD_ScopedPerWorkspace pins acceptance (d): an instance
// is only reachable through its owning workspace.
func TestService_InstanceCRUD_ScopedPerWorkspace(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	a := f.seedInstance(t, "ws-1", "A", "ta")
	f.seedInstance(t, "ws-2", "B", "tb")

	if _, err := f.svc.GetInstance(ctx, "ws-1", a.ID); err != nil {
		t.Fatalf("get own instance: %v", err)
	}
	// Cross-workspace GetInstance is a 404, not a leak.
	if _, err := f.svc.GetInstance(ctx, "ws-2", a.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound cross-workspace, got %v", err)
	}
	// List is scoped.
	list, err := f.svc.ListInstances(ctx, "ws-1")
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("ListInstances(ws-1) = %+v err=%v", list, err)
	}
	if !list[0].HasSecret {
		t.Error("expected HasSecret=true for seeded instance")
	}
	// Update / delete cross-workspace are 404.
	if _, err := f.svc.UpdateInstance(ctx, "ws-2", a.ID, &UpdateConfigRequest{Name: "hijack"}); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound on cross-workspace update, got %v", err)
	}
	if err := f.svc.DeleteInstance(ctx, "ws-2", a.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound on cross-workspace delete, got %v", err)
	}
}

func TestService_UpdateInstance_EmptySecretKeepsExisting(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "A", Secret: "first"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	waitForAuthProbe(t, f)
	if _, err := f.svc.UpdateInstance(ctx, "ws-1", created.ID, &UpdateConfigRequest{Name: "A2"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	waitForAuthProbe(t, f)
	if got, _ := f.secrets.Reveal(ctx, secretKeyForInstance(created.ID)); got != "first" {
		t.Errorf("secret should be preserved on empty-secret update, got %q", got)
	}
	reloaded, _ := f.svc.GetInstance(ctx, "ws-1", created.ID)
	if reloaded.Name != "A2" {
		t.Errorf("name not updated: %q", reloaded.Name)
	}
}

func TestService_DeleteInstance_RemovesSecretAndCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateInstance(ctx, "ws-1", &CreateConfigRequest{Name: "A", Secret: "t"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	waitForAuthProbe(t, f)
	if err := f.svc.DeleteInstance(ctx, "ws-1", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if exists, _ := f.secrets.Exists(ctx, secretKeyForInstance(created.ID)); exists {
		t.Error("expected secret removed")
	}
	if _, err := f.svc.GetInstance(ctx, "ws-1", created.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected instance gone, got %v", err)
	}
}

// TestService_DeleteInstance_InUse pins acceptance (f): an instance referenced
// by a watch cannot be deleted; the error carries the watch count.
func TestService_DeleteInstance_InUse(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	inst := f.seedInstance(t, "ws-1", "A", "t")
	w := newTestIssueWatch("ws-1")
	w.SentryInstanceID = inst.ID
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	// Add a disabled watch too — the count must include it.
	w2 := newTestIssueWatch("ws-1")
	w2.SentryInstanceID = inst.ID
	w2.Enabled = false
	if err := f.store.CreateIssueWatch(ctx, w2); err != nil {
		t.Fatalf("create disabled watch: %v", err)
	}
	err := f.svc.DeleteInstance(ctx, "ws-1", inst.ID)
	var inUse ErrInstanceInUse
	if !errors.As(err, &inUse) {
		t.Fatalf("expected ErrInstanceInUse, got %v", err)
	}
	if inUse.WatchCount != 2 {
		t.Errorf("expected watch count 2 (enabled + disabled), got %d", inUse.WatchCount)
	}
}

func TestService_TestConnectionCandidate(t *testing.T) {
	f := newSvcFixture(t)
	var seenURL string
	f.svc.clientFn = func(cfg *SentryConfig, secret string) Client {
		seenURL = cfg.URL
		return f.client
	}
	called := false
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		called = true
		return &TestConnectionResult{OK: true}, nil
	}
	res, err := f.svc.TestConnectionCandidate(context.Background(), &CreateConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "inline", URL: "https://sentry.example.com",
	})
	if err != nil || !called || !res.OK {
		t.Fatalf("called=%v res=%+v err=%v", called, res, err)
	}
	if seenURL != "https://sentry.example.com" {
		t.Errorf("expected configured URL passed to client, got %q", seenURL)
	}
	// No secret → failure result, no client call.
	res, err = f.svc.TestConnectionCandidate(context.Background(), &CreateConfigRequest{AuthMethod: AuthMethodAuthToken})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Error("expected OK=false when no secret provided")
	}
}

func TestService_TestInstance_UsesStoredSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	inst := f.seedInstance(t, "ws-1", "A", "stored")
	var sawSecret string
	f.svc.clientFn = func(_ *SentryConfig, secret string) Client {
		sawSecret = secret
		return f.client
	}
	if _, err := f.svc.TestInstance(ctx, "ws-1", inst.ID); err != nil {
		t.Fatalf("test: %v", err)
	}
	if sawSecret != "stored" {
		t.Errorf("expected stored secret used, got %q", sawSecret)
	}
	// Cross-workspace test is a 404.
	if _, err := f.svc.TestInstance(ctx, "ws-2", inst.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound cross-workspace, got %v", err)
	}
}

func TestService_Browse_RequireInstanceAndForwardsFilter(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	inst := f.seedInstance(t, "ws-1", "A", "t")

	// Missing instance ID → ErrInstanceRequired.
	if _, err := f.svc.SearchIssues(ctx, "ws-1", "", SearchFilter{OrgSlug: "acme"}, ""); !errors.Is(err, ErrInstanceRequired) {
		t.Errorf("expected ErrInstanceRequired for blank instanceID, got %v", err)
	}
	// Cross-workspace instance → ErrInstanceNotFound.
	if _, err := f.svc.SearchIssues(ctx, "ws-2", inst.ID, SearchFilter{OrgSlug: "acme"}, ""); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound cross-workspace, got %v", err)
	}
	var seenOrg string
	f.client.searchIssuesFn = func(filter SearchFilter, _ string) (*SearchResult, error) {
		seenOrg = filter.OrgSlug
		return &SearchResult{IsLast: true}, nil
	}
	if _, err := f.svc.SearchIssues(ctx, "ws-1", inst.ID, SearchFilter{OrgSlug: "acme"}, ""); err != nil {
		t.Fatalf("search: %v", err)
	}
	if seenOrg != "acme" {
		t.Errorf("expected org passed through, got %q", seenOrg)
	}
}

func TestService_GetIssue_NotConfiguredWhenNoSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	// Instance exists but has no stored secret → ErrNotConfigured (503).
	inst := f.seedInstance(t, "ws-1", "A", "")
	if _, err := f.svc.GetIssue(ctx, "ws-1", inst.ID, "PROJ-1"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured for secretless instance, got %v", err)
	}
	// Unknown instance → ErrInstanceNotFound (404).
	if _, err := f.svc.GetIssue(ctx, "ws-1", "ghost", "PROJ-1"); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound for unknown instance, got %v", err)
	}
}

// TestService_ClientCache_PerInstanceIsolation pins acceptance (j): each
// instance caches its own client; invalidating one does not rebuild the other.
func TestService_ClientCache_PerInstanceIsolation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	a := f.seedInstance(t, "ws-1", "A", "ta")
	b := f.seedInstance(t, "ws-1", "B", "tb")

	if _, err := f.svc.clientForInstance(ctx, a.ID); err != nil {
		t.Fatalf("client a: %v", err)
	}
	if _, err := f.svc.clientForInstance(ctx, b.ID); err != nil {
		t.Fatalf("client b: %v", err)
	}
	if got := f.factoryHit.Load(); got != 2 {
		t.Fatalf("expected 2 factory hits (one per instance), got %d", got)
	}
	// Cached: no new build.
	_, _ = f.svc.clientForInstance(ctx, a.ID)
	_, _ = f.svc.clientForInstance(ctx, b.ID)
	if got := f.factoryHit.Load(); got != 2 {
		t.Fatalf("expected cache hits (still 2 builds), got %d", got)
	}
	// Invalidate A only → A rebuilds, B stays cached.
	f.svc.invalidateClient(a.ID)
	_, _ = f.svc.clientForInstance(ctx, a.ID)
	_, _ = f.svc.clientForInstance(ctx, b.ID)
	if got := f.factoryHit.Load(); got != 3 {
		t.Fatalf("expected only A to rebuild (3 total), got %d", got)
	}
}

// TestService_ClientFor_InvalidateDuringBuild verifies the TOCTOU fix: when
// invalidateClient runs while clientForInstance is blocked on I/O, the freshly
// built (now-stale) client must not be cached. The next call must rebuild.
func TestService_ClientFor_InvalidateDuringBuild(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := &SentryConfig{WorkspaceID: "ws-1", Name: "A", AuthMethod: AuthMethodAuthToken, URL: DefaultSentryURL}
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	fakes := newFakeSecretStore()
	_ = fakes.Set(ctx, secretKeyForInstance(cfg.ID), "tok", "sntrys_xyz")

	var factoryHit atomic.Int32
	client := &fakeClient{}
	slow := &slowSecretStore{fakeSecretStore: fakes}
	svc := NewService(store, slow, func(_ *SentryConfig, _ string) Client {
		factoryHit.Add(1)
		return client
	}, logger.Default())

	invalidateCh := make(chan struct{})
	doneCh := make(chan struct{})
	slow.revealHook = func() {
		close(invalidateCh)
		time.Sleep(10 * time.Millisecond)
		close(doneCh)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.clientForInstance(ctx, cfg.ID)
		errCh <- err
	}()
	<-invalidateCh
	svc.invalidateClient(cfg.ID)
	if err := <-errCh; err != nil {
		t.Fatalf("first clientForInstance: %v", err)
	}
	slow.revealHook = nil
	if _, err := svc.clientForInstance(ctx, cfg.ID); err != nil {
		t.Fatalf("second clientForInstance: %v", err)
	}
	if got := factoryHit.Load(); got < 2 {
		t.Errorf("expected factory called at least twice after invalidation, got %d", got)
	}
}
