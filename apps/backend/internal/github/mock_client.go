package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const mockDefaultUser = "mock-user"

// prKey is a composite key for PR lookups by owner/repo/number.
type prKey struct {
	Owner  string
	Repo   string
	Number int
}

type issueKey struct {
	Owner  string
	Repo   string
	Number int
}

// branchKey is a composite key for PR lookups by owner/repo/branch.
type branchKey struct {
	Owner  string
	Repo   string
	Branch string
}

// checkKey is a composite key for check-run lookups by owner/repo/ref.
type checkKey struct {
	Owner string
	Repo  string
	Ref   string
}

// submittedReview records a SubmitReview call for test assertions.
type submittedReview struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Event  string `json:"event"`
	Body   string `json:"body"`
}

// mergedPR records a MergePR call for test assertions.
type mergedPR struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Number      int    `json:"number"`
	MergeMethod string `json:"merge_method"`
}

// repoKey is a composite key for per-repo lookups by owner/repo.
type repoKey struct {
	Owner string
	Repo  string
}

// repoFileEntry is one seeded file for MockClient.ListRepoDirectory /
// GetRepoFileContent. Ref "" is a wildcard that matches any requested ref;
// a non-empty Ref only matches that exact ref.
type repoFileEntry struct {
	Ref     string
	Path    string
	Content []byte
}

// MockClient implements Client with in-memory configurable data for E2E testing.
// All data is protected by a sync.RWMutex for thread safety.
type MockClient struct {
	mu            sync.RWMutex
	user          string
	authenticated bool
	authError     string
	// reposUnavailable, when true, makes the org/user listing methods that
	// back ListAccessibleRepos return ErrNoClient — driving the
	// `/api/v1/github/repos` handler to respond with 503
	// `github_not_configured`. Used by e2e tests that need to verify the
	// "Connect GitHub" banner in the Remote-tab chip popover without ripping
	// the whole mock client out of the wiring.
	reposUnavailable bool
	prs              map[prKey]*PR
	issues           map[issueKey]*Issue
	prsByBranch      map[branchKey]*PR
	orgs             []GitHubOrg
	repos            map[string][]GitHubRepo
	branches         map[repoKey][]RepoBranch
	reviews          map[prKey][]PRReview
	comments         map[prKey][]PRComment
	checks           map[checkKey][]CheckRun
	files            map[prKey][]PRFile
	commits          map[prKey][]PRCommitInfo
	submittedReviews []submittedReview
	mergedPRs        []mergedPR
	mergeMethods     map[repoKey]RepoMergeMethods
	gists            map[string]mockGist
	deletedGists     []string
	nextGistID       int
	repoFiles        map[repoKey][]repoFileEntry

	// findPRByBranchCalls counts FindPRByBranch invocations so tests can
	// assert that branch-detection probes are throttled. Atomic because
	// FindPRByBranch otherwise only takes a read lock.
	findPRByBranchCalls atomic.Int64

	// probeEntered/probeRelease let a test gate FindPRByBranch: when set, each
	// invocation signals on probeEntered and then blocks until probeRelease is
	// closed. Used to force concurrent probes to overlap and assert
	// singleflight coalescing.
	probeEntered chan string
	probeRelease chan struct{}
}

// mockGist captures a gist that was created via the mock client so tests
// can inspect what would have been uploaded.
type mockGist struct {
	ID          string
	Description string
	Public      bool
	Files       map[string]GistFile
	HTMLURL     string
}

// NewMockClient creates a new MockClient with default values.
func NewMockClient() *MockClient {
	return &MockClient{
		user:          mockDefaultUser,
		authenticated: true,
		prs:           make(map[prKey]*PR),
		issues:        make(map[issueKey]*Issue),
		prsByBranch:   make(map[branchKey]*PR),
		repos:         make(map[string][]GitHubRepo),
		branches:      make(map[repoKey][]RepoBranch),
		reviews:       make(map[prKey][]PRReview),
		comments:      make(map[prKey][]PRComment),
		checks:        make(map[checkKey][]CheckRun),
		files:         make(map[prKey][]PRFile),
		commits:       make(map[prKey][]PRCommitInfo),
		mergeMethods:  make(map[repoKey]RepoMergeMethods),
		gists:         make(map[string]mockGist),
		repoFiles:     make(map[repoKey][]repoFileEntry),
	}
}

// --- Client interface implementation ---

func (m *MockClient) IsAuthenticated(context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.authenticated, nil
}

func (m *MockClient) GetAuthenticatedUser(context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.user, nil
}

func (m *MockClient) GetPR(_ context.Context, owner, repo string, number int) (*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pr, ok := m.prs[prKey{owner, repo, number}]
	if !ok {
		return nil, fmt.Errorf("mock: PR %s/%s#%d not found", owner, repo, number)
	}
	return pr, nil
}

func (m *MockClient) GetIssue(_ context.Context, owner, repo string, number int) (*Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	issue, ok := m.issues[issueKey{owner, repo, number}]
	if !ok {
		return nil, fmt.Errorf("mock: issue %s/%s#%d not found", owner, repo, number)
	}
	return issue, nil
}

func (m *MockClient) FindPRByBranch(_ context.Context, owner, repo, branch string) (*PR, error) {
	m.findPRByBranchCalls.Add(1)
	m.mu.RLock()
	pr := m.prsByBranch[branchKey{owner, repo, branch}]
	entered, release := m.probeEntered, m.probeRelease
	m.mu.RUnlock()
	// Gate (if a test installed one) outside the lock so a blocked probe
	// doesn't wedge other mock operations.
	if entered != nil {
		entered <- branch
	}
	if release != nil {
		<-release
	}
	return pr, nil
}

// FindPRByBranchCallCount returns how many times FindPRByBranch has been
// called. Used by tests asserting detection-probe throttling.
func (m *MockClient) FindPRByBranchCallCount() int {
	return int(m.findPRByBranchCalls.Load())
}

// GateFindPRByBranch installs a gate around FindPRByBranch: each invocation
// signals on entered, then blocks until release is closed. Pass nil/nil to
// remove the gate. Used to force concurrent probes to overlap.
func (m *MockClient) GateFindPRByBranch(entered chan string, release chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeEntered = entered
	m.probeRelease = release
}

func (m *MockClient) ListAuthoredPRs(_ context.Context, owner, repo string) ([]*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*PR
	for k, pr := range m.prs {
		if k.Owner == owner && k.Repo == repo && pr.AuthorLogin == m.user {
			result = append(result, pr)
		}
	}
	return result, nil
}

func (m *MockClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*PR
	for _, pr := range m.prs {
		if len(pr.RequestedReviewers) > 0 {
			result = append(result, pr)
		}
	}
	return result, nil
}

func (m *MockClient) ListIssues(_ context.Context, filter, customQuery string) ([]*Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Issue, 0, len(m.issues))
	for _, issue := range m.issues {
		if !matchesIssueRepositoryQuery(issue, filter+" "+customQuery) {
			continue
		}
		result = append(result, issue)
	}
	return result, nil
}

func (m *MockClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	issues, err := m.ListIssues(ctx, filter, customQuery)
	if err != nil {
		return nil, err
	}
	return &IssueSearchPage{Issues: issues, TotalCount: len(issues), Page: page, PerPage: perPage}, nil
}

func matchesIssueRepositoryQuery(issue *Issue, query string) bool {
	for _, term := range strings.Fields(query) {
		if !strings.HasPrefix(term, "repo:") {
			continue
		}
		owner, repo, found := strings.Cut(strings.TrimPrefix(term, "repo:"), "/")
		if !found || !strings.EqualFold(issue.RepoOwner, owner) || !strings.EqualFold(issue.RepoName, repo) {
			return false
		}
	}
	return true
}

func (m *MockClient) SearchPRs(context.Context, string, string) ([]*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*PR, 0, len(m.prs))
	for _, pr := range m.prs {
		result = append(result, pr)
	}
	return result, nil
}

func (m *MockClient) SearchPRsPaged(ctx context.Context, _, _ string, page, perPage int) (*PRSearchPage, error) {
	prs, err := m.SearchPRs(ctx, "", "")
	if err != nil {
		return nil, err
	}
	return &PRSearchPage{PRs: prs, TotalCount: len(prs), Page: page, PerPage: perPage}, nil
}

func (m *MockClient) GetIssueState(context.Context, string, string, int) (string, error) {
	return defaultPRState, nil
}

func (m *MockClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	if m.orgs == nil {
		return []GitHubOrg{}, nil
	}
	return m.orgs, nil
}

func (m *MockClient) SearchOrgRepos(_ context.Context, org, query string, _ int) ([]GitHubRepo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	repos := m.repos[org]
	if query == "" {
		return repos, nil
	}
	var filtered []GitHubRepo
	for _, r := range repos {
		if strings.Contains(strings.ToLower(r.FullName), strings.ToLower(query)) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// ListUserRepos returns repos seeded for the authenticated user via AddRepos,
// keyed by the current user login. The query parameter is matched
// case-insensitively against the repo full_name; an empty query returns
// every repo for the user.
func (m *MockClient) ListUserRepos(_ context.Context, query string, _ int) ([]GitHubRepo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	repos := m.repos[m.user]
	if query == "" {
		return repos, nil
	}
	var filtered []GitHubRepo
	for _, r := range repos {
		if strings.Contains(strings.ToLower(r.FullName), strings.ToLower(query)) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// ListAccessibleRepos returns the union of every seeded repo (the user's own
// repos plus every org's repos), deduped by full_name, applying the same
// case-insensitive full_name substring filter the real clients use. Honours the
// reposUnavailable toggle by returning ErrNoClient so the 503/banner e2e path
// still works. Mirrors the single GET /user/repos call the real clients make.
func (m *MockClient) ListAccessibleRepos(_ context.Context, query string, _ int) ([]GitHubRepo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	seen := make(map[string]struct{})
	var all []GitHubRepo
	for _, repos := range m.repos {
		for _, r := range repos {
			if _, ok := seen[r.FullName]; ok {
				continue
			}
			seen[r.FullName] = struct{}{}
			all = append(all, r)
		}
	}
	return filterReposByQuery(all, query), nil
}

func (m *MockClient) ListPRReviews(_ context.Context, owner, repo string, number int) ([]PRReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reviews[prKey{owner, repo, number}], nil
}

func (m *MockClient) ListPRComments(_ context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	all := m.comments[prKey{owner, repo, number}]
	if since == nil {
		return all, nil
	}
	var filtered []PRComment
	for _, c := range all {
		if c.UpdatedAt.After(*since) {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

func (m *MockClient) ListCheckRuns(_ context.Context, owner, repo, ref string) ([]CheckRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checks[checkKey{owner, repo, ref}], nil
}

func (m *MockClient) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	return getPRFeedback(ctx, m, owner, repo, number)
}

func (m *MockClient) GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error) {
	return getPRStatus(ctx, m, owner, repo, number)
}

func (m *MockClient) ListPRFiles(_ context.Context, owner, repo string, number int) ([]PRFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.files[prKey{owner, repo, number}], nil
}

func (m *MockClient) ListPRCommits(_ context.Context, owner, repo string, number int) ([]PRCommitInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.commits[prKey{owner, repo, number}], nil
}

func (m *MockClient) ListRepoBranches(_ context.Context, owner, repo string) ([]RepoBranch, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	branches, ok := m.branches[repoKey{owner, repo}]
	if !ok {
		return nil, &GitHubAPIError{
			StatusCode: 404,
			Endpoint:   fmt.Sprintf("/repos/%s/%s/branches", owner, repo),
			Body:       fmt.Sprintf("repository %s/%s not found", owner, repo),
		}
	}
	out := make([]RepoBranch, len(branches))
	copy(out, branches)
	return out, nil
}

// ListRepoDirectory returns the immediate children of dir, derived from the
// paths seeded via SeedRepoFile for owner/repo. Entries seeded with a
// specific ref are only visible when ref matches; entries seeded with ref ""
// are visible for any requested ref. A directory with no matching seeded
// descendants (including an entirely unseeded repo) returns a 404, mirroring
// "missing directory" on the real API.
func (m *MockClient) ListRepoDirectory(_ context.Context, owner, repo, dir, ref string) ([]RepoContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cleanDir := repoContentsPath(dir)
	children := make(map[string]RepoContentEntry)
	for _, e := range m.repoFiles[repoKey{owner, repo}] {
		if e.Ref != "" && e.Ref != ref {
			continue
		}
		name, isDir, ok := repoDirChild(cleanDir, e.Path)
		if !ok {
			continue
		}
		if _, exists := children[name]; exists {
			continue
		}
		entryType, childPath := RepoContentTypeFile, name
		if isDir {
			entryType = RepoContentTypeDir
		}
		if cleanDir != "" {
			childPath = cleanDir + "/" + name
		}
		children[name] = RepoContentEntry{Name: name, Path: childPath, Type: entryType}
	}
	if len(children) == 0 {
		return nil, &GitHubAPIError{
			StatusCode: 404,
			Endpoint:   fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, cleanDir),
			Body:       fmt.Sprintf("directory %q not found", dir),
		}
	}
	out := make([]RepoContentEntry, 0, len(children))
	for _, c := range children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// repoDirChild reports the immediate child name of path relative to dir
// (""  meaning root), and whether that child is itself a directory (path has
// further segments beyond the child name). ok is false when path does not
// live under dir.
func repoDirChild(dir, path string) (name string, isDir bool, ok bool) {
	rest := path
	if dir != "" {
		prefix := dir + "/"
		if !strings.HasPrefix(path, prefix) {
			return "", false, false
		}
		rest = strings.TrimPrefix(path, prefix)
	}
	if rest == "" {
		return "", false, false
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx], true, true
	}
	return rest, false, true
}

// GetRepoFileContent returns the seeded content for owner/repo/path,
// preferring an exact-ref seed over a wildcard (ref "") seed. Returns a 404
// *GitHubAPIError when no seed matches.
func (m *MockClient) GetRepoFileContent(_ context.Context, owner, repo, path, ref string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cleanPath := repoContentsPath(path)
	var wildcard *repoFileEntry
	entries := m.repoFiles[repoKey{owner, repo}]
	for i := range entries {
		e := &entries[i]
		if e.Path != cleanPath {
			continue
		}
		if ref != "" && e.Ref == ref {
			return cloneBytes(e.Content), nil
		}
		if e.Ref == "" {
			wildcard = e
		}
	}
	if wildcard != nil {
		return cloneBytes(wildcard.Content), nil
	}
	return nil, &GitHubAPIError{
		StatusCode: 404,
		Endpoint:   fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, cleanPath),
		Body:       fmt.Sprintf("file %q not found", path),
	}
}

func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (m *MockClient) SubmitReview(_ context.Context, owner, repo string, number int, event, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submittedReviews = append(m.submittedReviews, submittedReview{
		Owner: owner, Repo: repo, Number: number, Event: event, Body: body,
	})
	return nil
}

func (m *MockClient) GetRepoMergeMethods(_ context.Context, owner, repo string) (RepoMergeMethods, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if methods, ok := m.mergeMethods[repoKey{owner, repo}]; ok {
		return methods, nil
	}
	// Default to all three allowed so existing e2e fixtures don't have to
	// seed merge settings just to exercise the merge button.
	return RepoMergeMethods{Merge: true, Squash: true, Rebase: true}, nil
}

// SetRepoMergeMethods overrides the allowed merge methods for a repo.
// Used by e2e fixtures to exercise the squash-only / rebase-only paths.
func (m *MockClient) SetRepoMergeMethods(owner, repo string, methods RepoMergeMethods) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeMethods[repoKey{owner, repo}] = methods
}

func (m *MockClient) MergePR(_ context.Context, owner, repo string, number int, mergeMethod string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergedPRs = append(m.mergedPRs, mergedPR{
		Owner: owner, Repo: repo, Number: number, MergeMethod: mergeMethod,
	})
	now := time.Now().UTC()
	if pr, ok := m.prs[prKey{owner, repo, number}]; ok {
		pr.State = "merged"
		pr.MergedAt = &now
		pr.Mergeable = false
	}
	return nil
}

func (m *MockClient) CreateGist(_ context.Context, in CreateGistInput) (*GistResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextGistID++
	id := fmt.Sprintf("mock-gist-%d", m.nextGistID)
	htmlURL := "https://gist.github.com/" + m.user + "/" + id
	files := make(map[string]GistFile, len(in.Files))
	for k, v := range in.Files {
		files[k] = v
	}
	m.gists[id] = mockGist{
		ID:          id,
		Description: in.Description,
		Public:      in.Public,
		Files:       files,
		HTMLURL:     htmlURL,
	}
	return &GistResponse{
		ID:        id,
		HTMLURL:   htmlURL,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (m *MockClient) DeleteGist(_ context.Context, gistID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.gists[gistID]; !ok {
		return &GitHubAPIError{
			StatusCode: 404,
			Endpoint:   "/gists/" + gistID,
			Body:       "gist not found",
		}
	}
	delete(m.gists, gistID)
	m.deletedGists = append(m.deletedGists, gistID)
	return nil
}

// Gists returns all currently-stored gists for test inspection.
func (m *MockClient) Gists() map[string]mockGist {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]mockGist, len(m.gists))
	for k, v := range m.gists {
		out[k] = v
	}
	return out
}

// DeletedGists returns the IDs of gists that were deleted, in call order.
func (m *MockClient) DeletedGists() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.deletedGists))
	copy(out, m.deletedGists)
	return out
}

// --- Setter methods for HTTP control endpoints ---

// SetUser sets the authenticated username.
func (m *MockClient) SetUser(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = username
}

// SetAuthHealth toggles the authenticated state for the e2e auth-lost test.
// When authenticated=false the popover renders the "Reconnect GitHub" branch.
// authError is an opaque message preserved for diagnostics.
func (m *MockClient) SetAuthHealth(authenticated bool, authError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authenticated = authenticated
	m.authError = authError
}

// SetReposUnavailable toggles whether the org/user repo-listing methods that
// back ListAccessibleRepos return ErrNoClient. Used by e2e tests to drive
// the `/api/v1/github/repos` handler into its 503 `github_not_configured`
// branch without rewiring the mock client out of the factory.
func (m *MockClient) SetReposUnavailable(unavailable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reposUnavailable = unavailable
}

// AddPR adds a PR to the mock data store, indexed by owner/repo/number and branch.
func (m *MockClient) AddPR(pr *PR) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prs[prKey{pr.RepoOwner, pr.RepoName, pr.Number}] = pr
	if pr.HeadBranch != "" {
		m.prsByBranch[branchKey{pr.RepoOwner, pr.RepoName, pr.HeadBranch}] = pr
	}
}

func (m *MockClient) AddIssue(issue *Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issues[issueKey{issue.RepoOwner, issue.RepoName, issue.Number}] = issue
}

// AddOrgs appends organizations to the mock data store.
func (m *MockClient) AddOrgs(orgs []GitHubOrg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs = append(m.orgs, orgs...)
}

// AddBranches sets branches for a repository.
func (m *MockClient) AddBranches(owner, repo string, branches []RepoBranch) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]RepoBranch, len(branches))
	copy(cp, branches)
	m.branches[repoKey{owner, repo}] = cp
}

// SeedRepoFile stores file content for ListRepoDirectory/GetRepoFileContent.
// ref "" matches any requested ref. Re-seeding the same owner/repo/ref/path
// replaces the previous content.
func (m *MockClient) SeedRepoFile(owner, repo, ref, path string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cleanPath := repoContentsPath(path)
	k := repoKey{owner, repo}
	cp := cloneBytes(content)
	for i, e := range m.repoFiles[k] {
		if e.Ref == ref && e.Path == cleanPath {
			m.repoFiles[k][i].Content = cp
			return
		}
	}
	m.repoFiles[k] = append(m.repoFiles[k], repoFileEntry{Ref: ref, Path: cleanPath, Content: cp})
}

// AddRepos adds repos under a key (an org login OR the authenticated user's
// login). The mock's ListUserRepos reads m.repos[m.user], so the same store
// backs both SearchOrgRepos and ListUserRepos in tests.
func (m *MockClient) AddRepos(org string, repos []GitHubRepo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repos[org] = append(m.repos[org], repos...)
}

// AddReviews appends reviews for a PR.
func (m *MockClient) AddReviews(owner, repo string, number int, reviews []PRReview) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	m.reviews[k] = append(m.reviews[k], reviews...)
}

// AddComments appends comments for a PR.
func (m *MockClient) AddComments(owner, repo string, number int, comments []PRComment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	m.comments[k] = append(m.comments[k], comments...)
}

// AddCheckRuns appends check runs for a ref.
func (m *MockClient) AddCheckRuns(owner, repo, ref string, checks []CheckRun) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := checkKey{owner, repo, ref}
	m.checks[k] = append(m.checks[k], checks...)
}

// ReplaceCheckRuns overwrites the check runs for a ref. Used by the e2e mock
// controller so a follow-up seed call yields deterministic state.
func (m *MockClient) ReplaceCheckRuns(owner, repo, ref string, checks []CheckRun) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]CheckRun, len(checks))
	copy(cp, checks)
	m.checks[checkKey{owner, repo, ref}] = cp
}

// ReplaceReviews overwrites the reviews for a PR.
func (m *MockClient) ReplaceReviews(owner, repo string, number int, reviews []PRReview) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]PRReview, len(reviews))
	copy(cp, reviews)
	m.reviews[prKey{owner, repo, number}] = cp
}

// ReplaceComments overwrites the comments for a PR.
func (m *MockClient) ReplaceComments(owner, repo string, number int, comments []PRComment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]PRComment, len(comments))
	copy(cp, comments)
	m.comments[prKey{owner, repo, number}] = cp
}

// AddPRFiles appends files for a PR.
func (m *MockClient) AddPRFiles(owner, repo string, number int, files []PRFile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	m.files[k] = append(m.files[k], files...)
}

// AddPRCommits appends commits for a PR.
func (m *MockClient) AddPRCommits(owner, repo string, number int, commits []PRCommitInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	m.commits[k] = append(m.commits[k], commits...)
}

// Reset clears all mock data and resets the user to the default.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = mockDefaultUser
	m.authenticated = true
	m.authError = ""
	m.reposUnavailable = false
	m.prs = make(map[prKey]*PR)
	m.issues = make(map[issueKey]*Issue)
	m.prsByBranch = make(map[branchKey]*PR)
	m.orgs = nil
	m.repos = make(map[string][]GitHubRepo)
	m.branches = make(map[repoKey][]RepoBranch)
	m.reviews = make(map[prKey][]PRReview)
	m.comments = make(map[prKey][]PRComment)
	m.checks = make(map[checkKey][]CheckRun)
	m.files = make(map[prKey][]PRFile)
	m.commits = make(map[prKey][]PRCommitInfo)
	m.submittedReviews = nil
	m.mergedPRs = nil
	m.mergeMethods = make(map[repoKey]RepoMergeMethods)
	m.gists = make(map[string]mockGist)
	m.deletedGists = nil
	m.nextGistID = 0
	m.repoFiles = make(map[repoKey][]repoFileEntry)
	m.findPRByBranchCalls.Store(0)
	m.probeEntered = nil
	m.probeRelease = nil
}

// SubmittedReviews returns all recorded SubmitReview calls.
func (m *MockClient) SubmittedReviews() []submittedReview {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]submittedReview, len(m.submittedReviews))
	copy(result, m.submittedReviews)
	return result
}

// MergedPRs returns all recorded MergePR calls.
func (m *MockClient) MergedPRs() []mergedPR {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]mergedPR, len(m.mergedPRs))
	copy(result, m.mergedPRs)
	return result
}
