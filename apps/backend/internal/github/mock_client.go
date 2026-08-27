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

// requestedReviewers records a RequestReviewers call for test assertions.
type requestedReviewers struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Number    int      `json:"number"`
	Reviewers []string `json:"reviewers"`
}

// mergedPR records a MergePR call for test assertions.
type mergedPR struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Number      int    `json:"number"`
	MergeMethod string `json:"merge_method"`
}

// mockPRMergeQueueState is the provider-side queue snapshot used by the E2E
// controller. The in-memory client exposes it through GetPRStatus so the
// normal TaskPR sync and automation event flow remains under test.
type mockPRMergeQueueState struct {
	HeadSHA                     string
	State                       string
	Position                    *int
	EntryID                     string
	EntryHeadSHA                string
	EstimatedTimeToMergeSeconds *int
	LastRemovalID               string
	LastRemovedAt               *time.Time
	LastRemovalReason           string
	LastRemovalBeforeSHA        string
	QueueObserved               bool
	RecoveryObserved            bool
}

// repoKey is a composite key for per-repo lookups by owner/repo.
type repoKey struct {
	Owner string
	Repo  string
}

type commitDetailKey struct {
	Owner string
	Repo  string
	SHA   string
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
	reposUnavailable  bool
	prs               map[prKey]*PR
	issues            map[issueKey]*Issue
	prsByBranch       map[branchKey]*PR
	orgs              []GitHubOrg
	repos             map[string][]GitHubRepo
	branches          map[repoKey][]RepoBranch
	reviews           map[prKey][]PRReview
	comments          map[prKey][]PRComment
	checks            map[checkKey][]CheckRun
	files             map[prKey][]PRFile
	commits           map[prKey][]PRCommitInfo
	prCommitsFailures map[prKey]int
	commitDetails     map[commitDetailKey]PRCommitDetail
	submittedReviews  []submittedReview
	requestedReviews  []requestedReviewers
	mergedPRs         []mergedPR
	mergeOutcomes     map[prKey]MergeOutcome
	mergeMethods      map[repoKey]RepoMergeMethods
	repositoryDetails map[repoKey]*GitHubRepository
	gists             map[string]mockGist
	deletedGists      []string
	nextGistID        int
	repoFiles         map[repoKey][]repoFileEntry

	// findPRByBranchCalls counts FindPRByBranch invocations so tests can
	// assert that branch-detection probes are throttled. Atomic because
	// FindPRByBranch otherwise only takes a read lock.
	findPRByBranchCalls atomic.Int64

	// getRepositoryCalls counts GetRepository invocations so tests can assert
	// that fork-parent resolution is cached rather than re-fetched per watch.
	getRepositoryCalls atomic.Int64

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
		user:              mockDefaultUser,
		authenticated:     true,
		prs:               make(map[prKey]*PR),
		issues:            make(map[issueKey]*Issue),
		prsByBranch:       make(map[branchKey]*PR),
		repos:             make(map[string][]GitHubRepo),
		branches:          make(map[repoKey][]RepoBranch),
		reviews:           make(map[prKey][]PRReview),
		comments:          make(map[prKey][]PRComment),
		checks:            make(map[checkKey][]CheckRun),
		files:             make(map[prKey][]PRFile),
		commits:           make(map[prKey][]PRCommitInfo),
		prCommitsFailures: make(map[prKey]int),
		commitDetails:     make(map[commitDetailKey]PRCommitDetail),
		mergeMethods:      make(map[repoKey]RepoMergeMethods),
		mergeOutcomes:     make(map[prKey]MergeOutcome),
		repositoryDetails: make(map[repoKey]*GitHubRepository),
		gists:             make(map[string]mockGist),
		repoFiles:         make(map[repoKey][]repoFileEntry),
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

func (m *MockClient) FindPRByHead(ctx context.Context, owner, repo, headOwner, headRepo, branch string) (*PR, error) {
	pr, err := m.FindPRByBranch(ctx, owner, repo, branch)
	if err != nil || pr == nil {
		return pr, err
	}
	if !sameRepositoryIdentity(pr.HeadRepoOwner, pr.HeadRepoName, headOwner, headRepo) {
		return nil, nil
	}
	return pr, nil
}

// FindPRByBranchCallCount returns how many times FindPRByBranch has been
// called. Used by tests asserting detection-probe throttling.
//
// This also counts FindPRByHead: the mock implements it by delegating to
// FindPRByBranch (it reuses the same branch index), so a fork-parent probe
// increments this counter too. Tests asserting on fork-parent lookups are
// reading it through that delegation — do not split the counters without
// re-reading every assertion that uses it.
func (m *MockClient) FindPRByBranchCallCount() int {
	return int(m.findPRByBranchCalls.Load())
}

// GetRepositoryCallCount returns how many times GetRepository has been called.
// Used by tests asserting that fork-parent resolution is cached.
func (m *MockClient) GetRepositoryCallCount() int {
	return int(m.getRepositoryCalls.Load())
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

func (m *MockClient) HasRepositoryAccess(_ context.Context, owner, repo string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return false, ErrNoClient
	}
	want := owner + "/" + repo
	for _, repos := range m.repos {
		for _, candidate := range repos {
			if strings.EqualFold(candidate.FullName, want) ||
				(strings.EqualFold(candidate.Owner, owner) && strings.EqualFold(candidate.Name, repo)) {
				return true, nil
			}
		}
	}
	return false, nil
}

// GetRepository returns an explicitly seeded repository identity. The
// lightweight repo-search fixture remains separate so existing autocomplete
// tests do not accidentally grant write access.
func (m *MockClient) GetRepository(_ context.Context, owner, repo string) (*GitHubRepository, error) {
	m.getRepositoryCalls.Add(1)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	repository, ok := m.repositoryDetails[repoKey{owner, repo}]
	if !ok {
		return nil, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + owner + "/" + repo}
	}
	copy := *repository
	return &copy, nil
}

func (m *MockClient) ListRepositoryForks(_ context.Context, owner, repo string) ([]*GitHubRepository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	parent, ok := m.repositoryDetails[repoKey{owner, repo}]
	if !ok {
		return nil, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + owner + "/" + repo}
	}
	forks := make([]*GitHubRepository, 0)
	for _, repository := range m.repositoryDetails {
		if repository == nil || !repository.Fork || repository.ParentID != parent.ID {
			continue
		}
		forks = append(forks, copyGitHubRepository(repository))
	}
	return forks, nil
}

// CreateFork creates a deterministic in-memory fork for mock Improve Kandev
// flows. Production clients still use the provider API and bounded polling.
func (m *MockClient) CreateFork(_ context.Context, owner, repo string) (*GitHubRepository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reposUnavailable {
		return nil, ErrNoClient
	}
	parent, ok := m.repositoryDetails[repoKey{owner, repo}]
	if !ok {
		return nil, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + owner + "/" + repo}
	}
	login := m.user
	fullName := login + "/" + repo
	fork := &GitHubRepository{
		ID:             parent.ID + 1,
		FullName:       fullName,
		Owner:          login,
		Name:           repo,
		CloneURL:       "https://github.com/" + fullName + ".git",
		Fork:           true,
		ParentID:       parent.ID,
		ParentFullName: parent.FullName,
		PushAccess:     true,
		AdminAccess:    true,
	}
	m.repositoryDetails[repoKey{login, repo}] = fork
	return copyGitHubRepository(fork), nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	if remaining := m.prCommitsFailures[k]; remaining > 0 {
		m.prCommitsFailures[k] = remaining - 1
		return nil, fmt.Errorf("mock: PR commits unavailable for %s/%s#%d", owner, repo, number)
	}
	return m.commits[k], nil
}

func (m *MockClient) GetPRCommitDetail(_ context.Context, owner, repo, sha string) (PRCommitDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	detail, ok := m.commitDetails[commitDetailKey{Owner: owner, Repo: repo, SHA: sha}]
	if !ok {
		return PRCommitDetail{}, fmt.Errorf("mock: commit %s/%s@%s not found", owner, repo, sha)
	}
	detail.Files = append([]PRFile(nil), detail.Files...)
	return detail, nil
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

func (m *MockClient) RequestReviewers(_ context.Context, owner, repo string, number int, reviewers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestedReviews = append(m.requestedReviews, requestedReviewers{
		Owner: owner, Repo: repo, Number: number, Reviewers: append([]string(nil), reviewers...),
	})
	pr := m.prs[prKey{Owner: owner, Repo: repo, Number: number}]
	if pr == nil {
		return nil
	}
	for _, reviewer := range reviewers {
		if !mockPRHasRequestedReviewer(pr, reviewer) {
			pr.RequestedReviewers = append(pr.RequestedReviewers, RequestedReviewer{Login: reviewer, Type: reviewerTypeUser})
		}
	}
	return nil
}

func mockPRHasRequestedReviewer(pr *PR, login string) bool {
	for _, reviewer := range pr.RequestedReviewers {
		if strings.EqualFold(reviewer.Login, login) {
			return true
		}
	}
	return false
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

func (m *MockClient) MergePR(_ context.Context, owner, repo string, number int, mergeMethod string) (MergeOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergedPRs = append(m.mergedPRs, mergedPR{
		Owner: owner, Repo: repo, Number: number, MergeMethod: mergeMethod,
	})
	outcome := m.mergeOutcomes[prKey{owner, repo, number}]
	if outcome == "" {
		outcome = MergeOutcomeMerged
	}
	if outcome == MergeOutcomeQueued {
		return outcome, nil
	}
	now := time.Now().UTC()
	if pr, ok := m.prs[prKey{owner, repo, number}]; ok {
		pr.State = "merged"
		pr.MergedAt = &now
		pr.Mergeable = false
	}
	return outcome, nil
}

func (m *MockClient) SetMergeOutcome(owner, repo string, number int, outcome MergeOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeOutcomes[prKey{owner, repo, number}] = outcome
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
	if pr.MergeQueueState != "" || pr.MergeQueuePosition != nil ||
		pr.MergeQueueEntryID != "" || pr.MergeQueueEntryHeadSHA != "" {
		pr.mergeQueuePopulated = true
	}
	if pr.MergeQueueLastRemovalID != "" || pr.MergeQueueLastRemovedAt != nil ||
		pr.MergeQueueLastRemovalReason != "" || pr.MergeQueueLastRemovalBeforeSHA != "" {
		pr.mergeQueueRecoveryPopulated = true
	}
	m.prs[prKey{pr.RepoOwner, pr.RepoName, pr.Number}] = pr
	if pr.HeadBranch != "" {
		m.prsByBranch[branchKey{pr.RepoOwner, pr.RepoName, pr.HeadBranch}] = pr
	}
}

// SetPRMergeQueue replaces the provider-side queue snapshot for a PR and can
// advance its head. It is intentionally separate from AddPR so E2E tests can
// drive removal and requeue transitions without replacing the whole fixture.
func (m *MockClient) SetPRMergeQueue(owner, repo string, number int, state mockPRMergeQueueState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr, ok := m.prs[prKey{owner, repo, number}]
	if !ok {
		return fmt.Errorf("mock: PR %s/%s#%d not found", owner, repo, number)
	}
	if state.HeadSHA != "" {
		pr.HeadSHA = state.HeadSHA
	}
	pr.MergeQueueState = state.State
	pr.MergeQueuePosition = state.Position
	pr.MergeQueueEntryID = state.EntryID
	pr.MergeQueueEntryHeadSHA = state.EntryHeadSHA
	pr.MergeQueueEstimatedTimeToMergeSeconds = state.EstimatedTimeToMergeSeconds
	pr.MergeQueueLastRemovalID = state.LastRemovalID
	pr.MergeQueueLastRemovedAt = state.LastRemovedAt
	pr.MergeQueueLastRemovalReason = state.LastRemovalReason
	pr.MergeQueueLastRemovalBeforeSHA = state.LastRemovalBeforeSHA
	pr.mergeQueuePopulated = state.QueueObserved
	pr.mergeQueueRecoveryPopulated = state.RecoveryObserved
	pr.UpdatedAt = time.Now().UTC()
	return nil
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

// SetRepositoryDetails seeds the provider-authoritative repository response
// used by managed fork preparation tests.
func (m *MockClient) SetRepositoryDetails(repository GitHubRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := repositoryKeyFromFullName(repository.FullName)
	m.repositoryDetails[key] = copyGitHubRepository(&repository)
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

// SetPRCommitsFailures queues a number of failed ListPRCommits responses for a PR.
func (m *MockClient) SetPRCommitsFailures(owner, repo string, number, failures int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prKey{owner, repo, number}
	if failures <= 0 {
		delete(m.prCommitsFailures, k)
		return
	}
	m.prCommitsFailures[k] = failures
}

// AddPRCommitDetail seeds an individual GitHub commit response.
func (m *MockClient) AddPRCommitDetail(owner, repo, sha string, detail PRCommitDetail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if detail.SHA == "" {
		detail.SHA = sha
	}
	detail.Files = append([]PRFile(nil), detail.Files...)
	m.commitDetails[commitDetailKey{Owner: owner, Repo: repo, SHA: sha}] = detail
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
	m.prCommitsFailures = make(map[prKey]int)
	m.commitDetails = make(map[commitDetailKey]PRCommitDetail)
	m.submittedReviews = nil
	m.requestedReviews = nil
	m.mergedPRs = nil
	m.mergeOutcomes = make(map[prKey]MergeOutcome)
	m.mergeMethods = make(map[repoKey]RepoMergeMethods)
	m.gists = make(map[string]mockGist)
	m.deletedGists = nil
	m.nextGistID = 0
	m.repoFiles = make(map[repoKey][]repoFileEntry)
	m.findPRByBranchCalls.Store(0)
	m.getRepositoryCalls.Store(0)
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

// RequestedReviews returns all recorded RequestReviewers calls.
func (m *MockClient) RequestedReviews() []requestedReviewers {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]requestedReviewers, len(m.requestedReviews))
	copy(result, m.requestedReviews)
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
