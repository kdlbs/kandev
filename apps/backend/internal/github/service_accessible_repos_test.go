package github

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// accessibleReposStubClient lets each test wire its own ListUserOrgs /
// ListUserRepos / SearchOrgRepos behaviour without touching the broader
// stubClient (which other tests depend on with its current shape).
type accessibleReposStubClient struct {
	stubClient
	orgs            []GitHubOrg
	orgsErr         error
	userRepos       []GitHubRepo
	userReposErr    error
	repoByOrg       map[string][]GitHubRepo
	repoByOrgErr    error
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
	if s.repoByOrgErr != nil {
		return nil, s.repoByOrgErr
	}
	return s.repoByOrg[org], nil
}

func newAccessibleReposTestService(client Client) *Service {
	return &Service{
		client:               client,
		authMethod:           AuthMethodPAT,
		userOrgsCache:        newAccessibleReposCache(),
		accessibleReposCache: newAccessibleReposCache(),
	}
}

func TestListAccessibleRepos_MergesOrgsAndUserRepos(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sc := &accessibleReposStubClient{
		orgs: []GitHubOrg{{Login: "acme"}, {Login: "globex"}},
		userRepos: []GitHubRepo{
			{FullName: "alice/personal", Owner: "alice", Name: "personal", PushedAt: t0.Add(3 * time.Hour)},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme":   {{FullName: "acme/widget", Owner: "acme", Name: "widget", PushedAt: t0.Add(2 * time.Hour)}},
			"globex": {{FullName: "globex/foo", Owner: "globex", Name: "foo", PushedAt: t0.Add(time.Hour)}},
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
	wantNames := map[string]bool{"alice/personal": false, "acme/widget": false, "globex/foo": false}
	for _, r := range got {
		if _, ok := wantNames[r.FullName]; !ok {
			t.Errorf("unexpected repo %q in result", r.FullName)
			continue
		}
		wantNames[r.FullName] = true
	}
	for name, seen := range wantNames {
		if !seen {
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
			{FullName: "acme/shared", Owner: "acme", Name: "shared", PushedAt: time.Unix(100, 0)},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {{FullName: "acme/shared", Owner: "acme", Name: "shared", PushedAt: time.Unix(50, 0)}},
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
			{FullName: "u/middle", Owner: "u", Name: "middle", PushedAt: t0.Add(2 * time.Hour)},
		},
		repoByOrg: map[string][]GitHubRepo{
			"acme": {
				{FullName: "acme/newest", Owner: "acme", Name: "newest", PushedAt: t0.Add(5 * time.Hour)},
				{FullName: "acme/oldest", Owner: "acme", Name: "oldest", PushedAt: t0},
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
			FullName: "acme/repo" + itoaSimple(i),
			Owner:    "acme",
			Name:     "repo" + itoaSimple(i),
			PushedAt: base.Add(time.Duration(i) * time.Minute),
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
			FullName: "acme/r" + itoaSimple(i),
			Owner:    "acme",
			Name:     "r" + itoaSimple(i),
			PushedAt: base.Add(time.Duration(i) * time.Minute),
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

// itoaSimple is a tiny non-allocating int-to-decimal helper for test data;
// using strconv.Itoa would force a separate import only the synthetic-repo
// tests need.
func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
