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

// accessibleReposStubClient lets each test wire its own ListUserOrgs /
// ListUserRepos / SearchOrgRepos behaviour without touching the broader
// stubClient (which other tests depend on with its current shape).
type accessibleReposStubClient struct {
	stubClient
	orgs         []GitHubOrg
	orgsErr      error
	userRepos    []GitHubRepo
	userReposErr error
	repoByOrg    map[string][]GitHubRepo
	repoByOrgErr error
	// errByOrg lets a test fail one specific org while others succeed —
	// needed by the partial-failure / best-effort merge test.
	errByOrg        map[string]error
	listOrgCalls    atomic.Int32
	listUserCalls   atomic.Int32
	searchOrgsCalls atomic.Int32
}

func (s *accessibleReposStubClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) {
	s.listOrgCalls.Add(1)
	if s.orgsErr != nil {
		return nil, s.orgsErr
	}
	return s.orgs, nil
}

func (s *accessibleReposStubClient) ListUserRepos(_ context.Context, _ string, _ int) ([]GitHubRepo, error) {
	s.listUserCalls.Add(1)
	if s.userReposErr != nil {
		return nil, s.userReposErr
	}
	return s.userRepos, nil
}

func (s *accessibleReposStubClient) SearchOrgRepos(_ context.Context, org, _ string, _ int) ([]GitHubRepo, error) {
	s.searchOrgsCalls.Add(1)
	if err, ok := s.errByOrg[org]; ok {
		return nil, err
	}
	if s.repoByOrgErr != nil {
		return nil, s.repoByOrgErr
	}
	return s.repoByOrg[org], nil
}

func newAccessibleReposTestService(client Client) *Service {
	return &Service{
		client:               client,
		authMethod:           AuthMethodPAT,
		logger:               logger.Default(),
		userOrgsCache:        newAccessibleReposCache(),
		accessibleReposCache: newAccessibleReposCache(),
	}
}

// ptrTime is a tiny helper to take the address of a time.Time literal for the
// pointer-typed GitHubRepo.PushedAt field used in these tests.
func ptrTime(t time.Time) *time.Time { return &t }

func TestListAccessibleRepos_MergesOrgsAndUserRepos(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}, {Login: "globex"}},
		userRepos: []GitHubRepo{
			{FullName: "alice/personal", Owner: "alice", Name: "personal", DefaultBranch: "main", Description: "Alice's personal repo", PushedAt: ptrTime(t0.Add(3 * time.Hour))},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme":   {{FullName: "acme/widget", Owner: "acme", Name: "widget", DefaultBranch: "trunk", Description: "Widget service", PushedAt: ptrTime(t0.Add(2 * time.Hour))}},
			"globex": {{FullName: "globex/foo", Owner: "globex", Name: "foo", DefaultBranch: "main", PushedAt: ptrTime(t0.Add(time.Hour))}},
		},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (1 user + 2 orgs)", len(got))
	}
	wantByName := map[string]struct {
		defaultBranch string
		description   string
	}{
		"alice/personal": {"main", "Alice's personal repo"},
		"acme/widget":    {"trunk", "Widget service"},
		"globex/foo":     {"main", ""},
	}
	seen := map[string]bool{}
	for _, r := range got {
		want, ok := wantByName[r.FullName]
		if !ok {
			t.Errorf("unexpected repo %q in result", r.FullName)
			continue
		}
		seen[r.FullName] = true
		if r.DefaultBranch != want.defaultBranch {
			t.Errorf("repo %q DefaultBranch = %q, want %q", r.FullName, r.DefaultBranch, want.defaultBranch)
		}
		if r.Description != want.description {
			t.Errorf("repo %q Description = %q, want %q", r.FullName, r.Description, want.description)
		}
	}
	for name := range wantByName {
		if !seen[name] {
			t.Errorf("expected repo %q in result", name)
		}
	}
}

func TestListAccessibleRepos_DedupesByFullName(t *testing.T) {
	// Same full_name appears in both the user's own repos and one of the
	// orgs they belong to (e.g. user is also an admin of "acme" and the
	// acme/shared repo surfaces in both lists). The merge must collapse it
	// to one entry.
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}},
		userRepos: []GitHubRepo{
			{FullName: "acme/shared", Owner: "acme", Name: "shared", PushedAt: ptrTime(time.Unix(100, 0))},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {{FullName: "acme/shared", Owner: "acme", Name: "shared", PushedAt: ptrTime(time.Unix(50, 0))}},
		},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (deduped)", len(got))
	}
	if got[0].FullName != "acme/shared" {
		t.Errorf("got %q, want acme/shared", got[0].FullName)
	}
}

func TestListAccessibleRepos_SortsByPushedAt(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}},
		userRepos: []GitHubRepo{
			{FullName: "u/middle", Owner: "u", Name: "middle", PushedAt: ptrTime(t0.Add(2 * time.Hour))},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {
				{FullName: "acme/newest", Owner: "acme", Name: "newest", PushedAt: ptrTime(t0.Add(5 * time.Hour))},
				{FullName: "acme/oldest", Owner: "acme", Name: "oldest", PushedAt: ptrTime(t0)},
			},
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

func TestListAccessibleRepos_LimitClamp(t *testing.T) {
	// Build 150 repos in a single org; with limit=10000 (above cap), service
	// must clamp to 100 — even though the dedupe/sort step happens first,
	// the truncation must keep at most 100.
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
	sc := &accessibleReposStubClient{
		orgs:      []GitHubOrg{{Login: "acme"}},
		repoByOrg: map[string][]GitHubRepo{"acme": repos},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 10000)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != maxAccessibleReposLimit {
		t.Errorf("len = %d, want %d (clamped to cap)", len(got), maxAccessibleReposLimit)
	}
}

func TestListAccessibleRepos_NotAuthenticated(t *testing.T) {
	// nil client => ErrNoClient straight from the guard (matches the
	// behaviour required by the 503-mapping handler test below).
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
	sc := &accessibleReposStubClient{
		orgs:      []GitHubOrg{{Login: "acme"}},
		repoByOrg: map[string][]GitHubRepo{"acme": repos},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if len(got) != defaultAccessibleReposLimit {
		t.Errorf("len = %d, want %d (default limit)", len(got), defaultAccessibleReposLimit)
	}
}

func TestListAccessibleRepos_UsesCache(t *testing.T) {
	// Subsequent calls within the cache window must coalesce — verifies
	// the 60s cache on both the org list and the merged result. Without
	// caching, two calls would issue four upstream requests
	// (2× ListUserOrgs + 2× ListUserRepos + 2× SearchOrgRepos per org).
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}},
		userRepos: []GitHubRepo{
			{FullName: "u/own", Owner: "u", Name: "own"},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {{FullName: "acme/widget", Owner: "acme", Name: "widget"}},
		},
	}
	svc := newAccessibleReposTestService(sc)
	for i := 0; i < 3; i++ {
		if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := sc.listOrgCalls.Load(); got != 1 {
		t.Errorf("ListUserOrgs called %d times, want 1 (cached)", got)
	}
	if got := sc.listUserCalls.Load(); got != 1 {
		t.Errorf("ListUserRepos called %d times, want 1 (cached)", got)
	}
	if got := sc.searchOrgsCalls.Load(); got != 1 {
		t.Errorf("SearchOrgRepos called %d times, want 1 (cached)", got)
	}
}

// TestListAccessibleRepos_PartialFailure_BestEffort verifies the fan-out is
// best-effort: when one org's SearchOrgRepos errors, the other sources still
// contribute and the call returns the partial union with no error. The
// failed source's error is logged (we don't assert on the log contents — the
// observable contract is "no error returned, healthy sources retained").
func TestListAccessibleRepos_PartialFailure_BestEffort(t *testing.T) {
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}, {Login: "broken"}},
		userRepos: []GitHubRepo{
			{FullName: "u/own", Owner: "u", Name: "own"},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {{FullName: "acme/widget", Owner: "acme", Name: "widget"}},
		},
		errByOrg: map[string]error{
			"broken": errors.New("simulated 500 from broken org"),
		},
	}
	svc := newAccessibleReposTestService(sc)
	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos returned error on partial failure: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (user-repos + acme; broken skipped)", len(got))
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.FullName] = true
	}
	if !names["u/own"] || !names["acme/widget"] {
		t.Errorf("missing expected repos in %v", names)
	}
}

// TestListAccessibleRepos_PartialFailure_NotCached verifies the partial-result
// caching footgun is closed: if ANY source errored (rate limit, transient
// 5xx, etc.), the merged result is returned to the caller but NOT written to
// the cache. The next call must re-fan out so a recovering source can
// contribute, instead of being shadowed by a 60s stale-cache entry that
// mistakenly excludes its repos.
//
// Real-world incident: user had a repo "kdlbs/kandev" under an org. The org
// search rate-limited, the user-repos endpoint legitimately returned 0
// matches (it can't see org repos), the partial result (empty) was cached for
// 60s, and the picker rendered "No repositories found" for the next minute
// even after the rate limit cleared.
func TestListAccessibleRepos_PartialFailure_NotCached(t *testing.T) {
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "broken"}},
		userRepos: []GitHubRepo{
			{FullName: "u/own", Owner: "u", Name: "own"},
		},
		errByOrg: map[string]error{
			"broken": errors.New("simulated rate limit"),
		},
	}
	svc := newAccessibleReposTestService(sc)

	// First call: partial success — user-repos returns, org errors.
	got1, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(got1) != 1 || got1[0].FullName != "u/own" {
		t.Fatalf("first call got %v, want [u/own]", got1)
	}
	if got := sc.searchOrgsCalls.Load(); got != 1 {
		t.Fatalf("first call SearchOrgRepos = %d, want 1", got)
	}
	if got := sc.listUserCalls.Load(); got != 1 {
		t.Fatalf("first call ListUserRepos = %d, want 1", got)
	}

	// Second call MUST re-fan out (cache miss) because the previous result
	// was a partial failure and should not have been cached.
	got2, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(got2) != 1 || got2[0].FullName != "u/own" {
		t.Fatalf("second call got %v, want [u/own]", got2)
	}
	if got := sc.searchOrgsCalls.Load(); got != 2 {
		t.Errorf("second call SearchOrgRepos = %d, want 2 (cache miss expected)", got)
	}
	if got := sc.listUserCalls.Load(); got != 2 {
		t.Errorf("second call ListUserRepos = %d, want 2 (cache miss expected)", got)
	}
}

// TestListAccessibleRepos_FullSuccess_IsCached verifies the cache write path
// is intact when every source succeeds — a guard against accidentally turning
// off caching entirely while fixing the partial-failure case.
func TestListAccessibleRepos_FullSuccess_IsCached(t *testing.T) {
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}},
		userRepos: []GitHubRepo{
			{FullName: "u/own", Owner: "u", Name: "own"},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {{FullName: "acme/widget", Owner: "acme", Name: "widget"}},
		},
	}
	svc := newAccessibleReposTestService(sc)

	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := sc.searchOrgsCalls.Load(); got != 1 {
		t.Errorf("SearchOrgRepos = %d, want 1 (second call should hit cache)", got)
	}
	if got := sc.listUserCalls.Load(); got != 1 {
		t.Errorf("ListUserRepos = %d, want 1 (second call should hit cache)", got)
	}
}

// TestListAccessibleRepos_AllSourcesFail verifies the call DOES error when
// every fan-out source returns an error — partial failure is best-effort, but
// "nothing worked" must surface so the picker can show an error state.
func TestListAccessibleRepos_AllSourcesFail(t *testing.T) {
	sc := &accessibleReposStubClient{
		orgs:         []GitHubOrg{{Login: "broken"}},
		userReposErr: errors.New("user-repos boom"),
		errByOrg: map[string]error{
			"broken": errors.New("org boom"),
		},
	}
	svc := newAccessibleReposTestService(sc)
	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err == nil {
		t.Fatalf("expected error when every source fails, got nil")
	}
}

// TestListAccessibleRepos_CacheClearedOnTokenChange verifies the
// user-orgs + merged-repos caches are invalidated when authentication
// changes. The Service lives across token swaps (ConfigureToken /
// ClearToken mutate s.client in place rather than rebuilding the Service),
// so without an explicit invalidation, a token swap would surface the
// previous user's repos for up to the 60s TTL.
//
// We exercise the invalidation path directly via ClearAccessibleReposCaches
// — both ConfigureToken and ClearToken call it after every auth change.
func TestListAccessibleRepos_CacheClearedOnTokenChange(t *testing.T) {
	scA := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "user-a-org"}},
		userRepos: []GitHubRepo{
			{FullName: "user-a/repo", Owner: "user-a", Name: "repo"},
		},
		repoByOrg: map[string][]GitHubRepo{
			"user-a-org": {{FullName: "user-a-org/widget", Owner: "user-a-org", Name: "widget"}},
		},
	}
	svc := newAccessibleReposTestService(scA)

	// Populate the caches by issuing one call under "user A".
	if _, err := svc.ListAccessibleRepos(context.Background(), "", 50); err != nil {
		t.Fatalf("initial ListAccessibleRepos: %v", err)
	}
	if got := scA.listOrgCalls.Load(); got != 1 {
		t.Fatalf("user-a ListUserOrgs called %d times, want 1", got)
	}

	// Swap the client to a "user B" stub and clear caches — this mirrors
	// what ConfigureToken / ClearToken do when auth changes.
	scB := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "user-b-org"}},
		userRepos: []GitHubRepo{
			{FullName: "user-b/repo", Owner: "user-b", Name: "repo"},
		},
		repoByOrg: map[string][]GitHubRepo{
			"user-b-org": {{FullName: "user-b-org/foo", Owner: "user-b-org", Name: "foo"}},
		},
	}
	svc.client = scB
	svc.ClearAccessibleReposCaches()

	got, err := svc.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("post-swap ListAccessibleRepos: %v", err)
	}
	// user-b's fan-out must have actually run (cache miss after the clear).
	if calls := scB.listOrgCalls.Load(); calls != 1 {
		t.Errorf("user-b ListUserOrgs called %d times, want 1 (cache miss expected)", calls)
	}
	// And we should see ONLY user-b's repos — no leakage from user-a's cache.
	names := map[string]bool{}
	for _, r := range got {
		names[r.FullName] = true
	}
	if !names["user-b/repo"] || !names["user-b-org/foo"] {
		t.Errorf("missing user-b repos in result: %v", names)
	}
	if names["user-a/repo"] || names["user-a-org/widget"] {
		t.Errorf("user-a repos leaked across token swap: %v", names)
	}
}
