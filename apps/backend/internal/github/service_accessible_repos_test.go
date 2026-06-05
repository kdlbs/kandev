package github

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// accessibleReposStubClient lets each test wire its own ListAccessibleRepos
// behaviour without touching the broader stubClient (which other tests depend
// on with its current shape). It also records the context each call observed so
// the detached-context regression test can assert the fetch is not cancelled
// when the caller's request is.
type accessibleReposStubClient struct {
	stubClient
	repos    []GitHubRepo
	reposErr error
	calls    atomic.Int32
	// lastCtxErr records ctx.Err() observed at call time. The detached-context
	// test cancels the caller context before calling and asserts this stays nil
	// — proving the service ran the fetch under a context that survives the
	// request abort.
	lastCtxErr atomic.Value // error
}

func (s *accessibleReposStubClient) ListAccessibleRepos(ctx context.Context, query string, _ int) ([]GitHubRepo, error) {
	s.calls.Add(1)
	s.lastCtxErr.Store(errBox{ctx.Err()})
	if s.reposErr != nil {
		return nil, s.reposErr
	}
	return filterReposByQuery(s.repos, query), nil
}

// errBox wraps an error so it can be stored in an atomic.Value even when nil.
type errBox struct{ err error }

func (s *accessibleReposStubClient) observedCtxErr() error {
	v, ok := s.lastCtxErr.Load().(errBox)
	if !ok {
		return nil
	}
	return v.err
}

func newAccessibleReposTestService(client Client) *Service {
	return &Service{
		client:               client,
		authMethod:           AuthMethodPAT,
		logger:               logger.Default(),
		accessibleReposCache: newAccessibleReposCache(),
	}
}

// ptrTime is a tiny helper to take the address of a time.Time literal for the
// pointer-typed GitHubRepo.PushedAt field used in these tests.
func ptrTime(t time.Time) *time.Time { return &t }

func TestListAccessibleRepos_ReturnsRepos(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{
			{FullName: "alice/personal", Owner: "alice", Name: "personal", DefaultBranch: "main", Description: "Alice's personal repo", PushedAt: ptrTime(t0.Add(3 * time.Hour))},
			{FullName: "acme/widget", Owner: "acme", Name: "widget", DefaultBranch: "trunk", Description: "Widget service", PushedAt: ptrTime(t0.Add(2 * time.Hour))},
			{FullName: "globex/foo", Owner: "globex", Name: "foo", DefaultBranch: "main", PushedAt: ptrTime(t0.Add(time.Hour))},
		},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantByName := map[string]struct {
		defaultBranch string
		description   string
	}{
		"alice/personal": {"main", "Alice's personal repo"},
		"acme/widget":    {"trunk", "Widget service"},
		"globex/foo":     {"main", ""},
	}
	for _, r := range got {
		want, ok := wantByName[r.FullName]
		if !ok {
			t.Errorf("unexpected repo %q in result", r.FullName)
			continue
		}
		if r.DefaultBranch != want.defaultBranch {
			t.Errorf("repo %q DefaultBranch = %q, want %q", r.FullName, r.DefaultBranch, want.defaultBranch)
		}
		if r.Description != want.description {
			t.Errorf("repo %q Description = %q, want %q", r.FullName, r.Description, want.description)
		}
	}
}

func TestListAccessibleRepos_SortsByPushedAt(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{
			{FullName: "u/middle", Owner: "u", Name: "middle", PushedAt: ptrTime(t0.Add(2 * time.Hour))},
			{FullName: "acme/oldest", Owner: "acme", Name: "oldest", PushedAt: ptrTime(t0)},
			{FullName: "acme/newest", Owner: "acme", Name: "newest", PushedAt: ptrTime(t0.Add(5 * time.Hour))},
		},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	wantOrder := []string{"acme/newest", "u/middle", "acme/oldest"}
	if len(got) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got[i].FullName != name {
			t.Errorf("pos %d = %q, want %q", i, got[i].FullName, name)
		}
	}
}

func TestListAccessibleRepos_QueryFilter(t *testing.T) {
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{
			{FullName: "acme/widget", Owner: "acme", Name: "widget"},
			{FullName: "acme/gadget", Owner: "acme", Name: "gadget"},
			{FullName: "u/other", Owner: "u", Name: "other"},
		},
	}
	svc := newAccessibleReposTestService(sc)
	// Case-insensitive substring match on full_name.
	got, err := svc.ListAccessibleRepos(context.Background(), "ACME/W", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != 1 || got[0].FullName != "acme/widget" {
		t.Fatalf("got %v, want [acme/widget]", got)
	}
}

func TestListAccessibleRepos_LimitClamp(t *testing.T) {
	repos := make([]GitHubRepo, 150)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range repos {
		repos[i] = GitHubRepo{
			FullName: "acme/repo" + strconv.Itoa(i),
			Owner:    "acme",
			Name:     "repo" + strconv.Itoa(i),
			PushedAt: ptrTime(base.Add(time.Duration(i) * time.Minute)),
		}
	}
	sc := &accessibleReposStubClient{repos: repos}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 10000)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != maxAccessibleReposLimit {
		t.Errorf("len = %d, want %d (clamped to cap)", len(got), maxAccessibleReposLimit)
	}
}

func TestListAccessibleRepos_DefaultLimit(t *testing.T) {
	// limit=0 must resolve to the default (50); we verify by feeding 60
	// distinct repos and observing the truncation to exactly 50.
	repos := make([]GitHubRepo, 60)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range repos {
		repos[i] = GitHubRepo{
			FullName: "acme/r" + strconv.Itoa(i),
			Owner:    "acme",
			Name:     "r" + strconv.Itoa(i),
			PushedAt: ptrTime(base.Add(time.Duration(i) * time.Minute)),
		}
	}
	sc := &accessibleReposStubClient{repos: repos}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != defaultAccessibleReposLimit {
		t.Errorf("len = %d, want %d (default limit)", len(got), defaultAccessibleReposLimit)
	}
}

func TestListAccessibleRepos_NotAuthenticated(t *testing.T) {
	// nil client => ErrNoClient straight from the guard (matches the
	// behaviour required by the 503-mapping handler test).
	svc := newAccessibleReposTestService(nil)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if !errors.Is(err, ErrNoClient) {
		t.Fatalf("err = %v, want ErrNoClient", err)
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}

	// Same sentinel must bubble up when the client itself is the noop
	// (e.g. GH CLI absent and no PAT configured).
	svcNoop := newAccessibleReposTestService(&NoopClient{})
	gotNoop, errNoop := svcNoop.ListAccessibleRepos(context.Background(), "", 50)
	if !errors.Is(errNoop, ErrNoClient) {
		t.Fatalf("noop err = %v, want ErrNoClient", errNoop)
	}
	if gotNoop != nil {
		t.Errorf("noop got = %v, want nil", gotNoop)
	}
}

func TestListAccessibleRepos_UsesCache(t *testing.T) {
	// Subsequent calls within the cache window must coalesce — the fetch runs
	// once and later calls hit the 60s cache.
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{
			{FullName: "u/own", Owner: "u", Name: "own"},
			{FullName: "acme/widget", Owner: "acme", Name: "widget"},
		},
	}
	svc := newAccessibleReposTestService(sc)
	for i := 0; i < 3; i++ {
		if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := sc.calls.Load(); got != 1 {
		t.Errorf("ListAccessibleRepos called %d times, want 1 (cached)", got)
	}
}

// TestListAccessibleRepos_DetachedContext is the key regression test for the
// `signal: killed` 500: the picker frontend aborts its request via an
// AbortController, which previously cancelled the shared fetch's context and
// SIGKILLed the `gh` subprocess. The service must run the fetch under a context
// detached from the caller's request, so an already-cancelled caller context
// does NOT propagate into the client call.
func TestListAccessibleRepos_DetachedContext(t *testing.T) {
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{{FullName: "u/own", Owner: "u", Name: "own"}},
	}
	svc := newAccessibleReposTestService(sc)

	// Cancel the caller context BEFORE calling the service.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("expected caller context to be cancelled")
	}

	got, err := svc.ListAccessibleRepos(ctx, "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos with cancelled caller ctx: %v", err)
	}
	if len(got) != 1 || got[0].FullName != "u/own" {
		t.Fatalf("got %v, want [u/own]", got)
	}
	// The fetch must have observed a NON-cancelled context.
	if ctxErr := sc.observedCtxErr(); ctxErr != nil {
		t.Errorf("fetch observed ctx.Err() = %v, want nil (context must be detached)", ctxErr)
	}
	if got := sc.calls.Load(); got != 1 {
		t.Errorf("fetch called %d times, want 1", got)
	}
}

// TestListAccessibleRepos_ErrorNotCached verifies a failed fetch is NOT cached:
// the next call must retry rather than serve a poisoned cache entry.
func TestListAccessibleRepos_ErrorNotCached(t *testing.T) {
	sc := &accessibleReposStubClient{
		reposErr: errors.New("simulated rate limit"),
	}
	svc := newAccessibleReposTestService(sc)

	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err == nil {
		t.Fatal("first call: expected error, got nil")
	}
	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err == nil {
		t.Fatal("second call: expected error, got nil")
	}
	if got := sc.calls.Load(); got != 2 {
		t.Errorf("fetch called %d times, want 2 (error must not be cached)", got)
	}
}

// TestListAccessibleRepos_SuccessIsCached guards against accidentally turning
// off caching while reworking the endpoint.
func TestListAccessibleRepos_SuccessIsCached(t *testing.T) {
	sc := &accessibleReposStubClient{
		repos: []GitHubRepo{{FullName: "u/own", Owner: "u", Name: "own"}},
	}
	svc := newAccessibleReposTestService(sc)

	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := sc.calls.Load(); got != 1 {
		t.Errorf("fetch called %d times, want 1 (second call should hit cache)", got)
	}
}

// TestListAccessibleRepos_CacheClearedOnTokenChange verifies the
// accessible-repos cache is invalidated when authentication changes. The
// Service lives across token swaps (ConfigureToken / ClearToken mutate s.client
// in place rather than rebuilding the Service), so without an explicit
// invalidation a token swap would surface the previous user's repos for up to
// the 60s TTL. We exercise the invalidation path directly via
// ClearAccessibleReposCaches — both ConfigureToken and ClearToken call it.
func TestListAccessibleRepos_CacheClearedOnTokenChange(t *testing.T) {
	scA := &accessibleReposStubClient{
		repos: []GitHubRepo{{FullName: "user-a/repo", Owner: "user-a", Name: "repo"}},
	}
	svc := newAccessibleReposTestService(scA)

	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("initial ListAccessibleRepos: %v", err)
	}
	if got := scA.calls.Load(); got != 1 {
		t.Fatalf("user-a fetch called %d times, want 1", got)
	}

	// Swap the client to a "user B" stub and clear caches — this mirrors what
	// ConfigureToken / ClearToken do when auth changes.
	scB := &accessibleReposStubClient{
		repos: []GitHubRepo{{FullName: "user-b/repo", Owner: "user-b", Name: "repo"}},
	}
	svc.client = scB
	svc.ClearAccessibleReposCaches()

	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("post-swap ListAccessibleRepos: %v", err)
	}
	if calls := scB.calls.Load(); calls != 1 {
		t.Errorf("user-b fetch called %d times, want 1 (cache miss expected)", calls)
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.FullName] = true
	}
	if !names["user-b/repo"] {
		t.Errorf("missing user-b repo in result: %v", names)
	}
	if names["user-a/repo"] {
		t.Errorf("user-a repo leaked across token swap: %v", names)
	}
}

// blockingAccessibleReposClient blocks inside ListAccessibleRepos until the test
// releases it, letting the test race a ClearAccessibleReposCaches() with an
// in-flight fetch.
type blockingAccessibleReposClient struct {
	stubClient
	repos   []GitHubRepo
	started chan struct{}
	release chan struct{}
}

func (c *blockingAccessibleReposClient) ListAccessibleRepos(_ context.Context, query string, _ int) ([]GitHubRepo, error) {
	close(c.started)
	<-c.release
	return filterReposByQuery(c.repos, query), nil
}

// TestListAccessibleRepos_ClearDuringFetchDropsStaleWrite is the service-level
// mirror of the ttl_cache TestTTLCache_ClearInvalidatesInFlightFetch regression:
// a fetch that started BEFORE a clear() (token swap) must not write its stale
// result back into the just-cleared cache. The fix snapshots the cache
// generation before the fetch and writes via setIfCurrentGeneration so the bump
// from clear() drops the late write.
func TestListAccessibleRepos_ClearDuringFetchDropsStaleWrite(t *testing.T) {
	sc := &blockingAccessibleReposClient{
		repos:   []GitHubRepo{{FullName: "user-a/repo", Owner: "user-a", Name: "repo"}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := newAccessibleReposTestService(sc)

	done := make(chan struct{})
	go func() {
		_, _ = svc.ListAccessibleRepos(context.Background(), "", 50)
		close(done)
	}()

	// Wait until the fetch is in flight, then clear (simulating a token swap),
	// then let the stale fetch complete.
	<-sc.started
	svc.ClearAccessibleReposCaches()
	close(sc.release)
	<-done

	// The stale write must have been dropped: the cache key must miss so the
	// next call refetches under the new generation rather than serving the
	// pre-clear (prior-user) repos.
	key := accessibleReposCacheKey("", 50)
	if v, ok := svc.accessibleReposCache.get(key); ok {
		t.Fatalf("expected cache to miss after clear() during in-flight fetch; got %v", v)
	}
}
