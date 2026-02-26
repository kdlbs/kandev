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

// MockClient implements Client with in-memory configurable data for E2E testing.
// All data is protected by a sync.RWMutex for thread safety.
type MockClient struct {
	mu               sync.RWMutex
	user             string
	prs              map[prKey]*PR
	prsByBranch      map[branchKey]*PR
	orgs             []GitHubOrg
	repos            map[string][]GitHubRepo
	reviews          map[prKey][]PRReview
	comments         map[prKey][]PRComment
	checks           map[checkKey][]CheckRun
	files            map[prKey][]PRFile
	commits          map[prKey][]PRCommitInfo
	submittedReviews []submittedReview
}

// NewMockClient creates a new MockClient with default values.
func NewMockClient() *MockClient {
	return &MockClient{
		user:        mockDefaultUser,
		prs:         make(map[prKey]*PR),
		prsByBranch: make(map[branchKey]*PR),
		repos:       make(map[string][]GitHubRepo),
		reviews:     make(map[prKey][]PRReview),
		comments:    make(map[prKey][]PRComment),
		checks:      make(map[checkKey][]CheckRun),
		files:       make(map[prKey][]PRFile),
		commits:     make(map[prKey][]PRCommitInfo),
	}
}

// --- Client interface implementation ---

func (m *MockClient) IsAuthenticated(context.Context) (bool, error) {
	return true, nil
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

func (m *MockClient) SubmitReview(_ context.Context, owner, repo string, number int, event, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submittedReviews = append(m.submittedReviews, submittedReview{
		Owner: owner, Repo: repo, Number: number, Event: event, Body: body,
	})
	return nil
}

// --- Setter methods for HTTP control endpoints ---

// SetUser sets the authenticated username.
func (m *MockClient) SetUser(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = username
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

// AddRepos appends repositories for an organization.
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
	m.prs = make(map[prKey]*PR)
	m.prsByBranch = make(map[branchKey]*PR)
	m.orgs = nil
	m.repos = make(map[string][]GitHubRepo)
	m.reviews = make(map[prKey][]PRReview)
	m.comments = make(map[prKey][]PRComment)
	m.checks = make(map[checkKey][]CheckRun)
	m.files = make(map[prKey][]PRFile)
	m.commits = make(map[prKey][]PRCommitInfo)
	m.submittedReviews = nil
}

// SubmittedReviews returns all recorded SubmitReview calls.
func (m *MockClient) SubmittedReviews() []submittedReview {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]submittedReview, len(m.submittedReviews))
	copy(result, m.submittedReviews)
	return result
}
