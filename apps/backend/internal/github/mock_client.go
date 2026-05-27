package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const mockDefaultUser = "mock-user"

// prKey is a composite key for PR lookups by owner/repo/number.
type prKey struct {
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

// MockClient implements Client with in-memory configurable data for E2E testing.
// All data is protected by a sync.RWMutex for thread safety.
type MockClient struct {
	mu               sync.RWMutex
	user             string
	authenticated    bool
	authError        string
	prs              map[prKey]*PR
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

func (m *MockClient) FindPRByBranch(_ context.Context, owner, repo, branch string) (*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pr := m.prsByBranch[branchKey{owner, repo, branch}]
	return pr, nil
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

func (m *MockClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return nil, nil
}

func (m *MockClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return &IssueSearchPage{Issues: []*Issue{}, TotalCount: 0, Page: 1, PerPage: 50}, nil
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
	if m.orgs == nil {
		return []GitHubOrg{}, nil
	}
	return m.orgs, nil
}

func (m *MockClient) SearchOrgRepos(_ context.Context, org, query string, _ int) ([]GitHubRepo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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

// AddPR adds a PR to the mock data store, indexed by owner/repo/number and branch.
func (m *MockClient) AddPR(pr *PR) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prs[prKey{pr.RepoOwner, pr.RepoName, pr.Number}] = pr
	if pr.HeadBranch != "" {
		m.prsByBranch[branchKey{pr.RepoOwner, pr.RepoName, pr.HeadBranch}] = pr
	}
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
	m.prs = make(map[prKey]*PR)
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
