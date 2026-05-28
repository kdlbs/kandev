package sentry

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// MockClient implements Client with in-memory data for E2E tests. A single
// shared instance per process, driven by HTTP control routes mounted in mock
// mode.
type MockClient struct {
	mu         sync.RWMutex
	authResult *TestConnectionResult
	projects   []SentryProject
	issues     map[string]*SentryIssue // short ID → issue
	// issueOrder preserves insertion order so SearchIssues returns a stable
	// sequence — Go map range is randomised.
	issueOrder []string
	getError   *APIError
}

// NewMockClient seeds a default-success TestAuth so a fresh config flips to
// "Authenticated" without explicit seeding.
func NewMockClient() *MockClient {
	return &MockClient{
		authResult: &TestConnectionResult{
			OK:          true,
			UserID:      "mock-user",
			DisplayName: "Mock User",
			Email:       "mock@example.com",
		},
		issues: make(map[string]*SentryIssue),
	}
}

// --- Client interface ---

func (m *MockClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.authResult == nil {
		return &TestConnectionResult{OK: false, Error: "mock: no auth result configured"}, nil
	}
	r := *m.authResult
	return &r, nil
}

func (m *MockClient) ListProjects(context.Context) ([]SentryProject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SentryProject, len(m.projects))
	copy(out, m.projects)
	return out, nil
}

func (m *MockClient) SearchIssues(_ context.Context, filter SearchFilter, _ string) (*SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SentryIssue, 0, len(m.issueOrder))
	for _, id := range m.issueOrder {
		issue, ok := m.issues[id]
		if !ok {
			continue
		}
		if !mockMatchesFilter(issue, filter) {
			continue
		}
		out = append(out, *issue)
	}
	return &SearchResult{Issues: out, IsLast: true}, nil
}

func (m *MockClient) GetIssue(_ context.Context, idOrShortID string) (*SentryIssue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getError != nil {
		err := *m.getError
		return nil, &err
	}
	if issue, ok := m.issues[idOrShortID]; ok {
		cp := *issue
		return &cp, nil
	}
	// Fall back to numeric ID lookup so callers can use either form.
	for _, i := range m.issues {
		if i.ID == idOrShortID {
			cp := *i
			return &cp, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "issue not found: " + idOrShortID}
}

// mockMatchesFilter applies the filter predicates that map to a per-issue
// field, so E2E tests can assert the backend forwarded the filter rather than
// silently returning everything. Deliberately NOT enforced (no per-issue
// analog on SentryIssue): OrgSlug (the mock's project list is one tenant),
// Environment (issues carry no environment in our projection), and StatsPeriod
// (a relative time window, not an issue attribute). The real REST client's
// URL building for those params is covered in rest_client_test.go.
func mockMatchesFilter(issue *SentryIssue, f SearchFilter) bool {
	if f.ProjectSlug != "" && issue.ProjectSlug != f.ProjectSlug {
		return false
	}
	if len(f.Levels) > 0 && !containsFold(f.Levels, issue.Level) {
		return false
	}
	if len(f.Statuses) > 0 && !containsFold(f.Statuses, issue.Status) {
		return false
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		if !strings.Contains(strings.ToLower(issue.Title), strings.ToLower(q)) {
			return false
		}
	}
	return true
}

func containsFold(xs []string, target string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, target) {
			return true
		}
	}
	return false
}

// --- Setters used by MockController ---

func (m *MockClient) SetAuthResult(r *TestConnectionResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authResult = r
}

func (m *MockClient) SetProjects(projects []SentryProject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]SentryProject, len(projects))
	copy(cp, projects)
	m.projects = cp
}

func (m *MockClient) AddIssue(issue *SentryIssue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *issue
	key := issue.ShortID
	if key == "" {
		key = issue.ID
	}
	if _, exists := m.issues[key]; !exists {
		m.issueOrder = append(m.issueOrder, key)
	}
	m.issues[key] = &cp
}

func (m *MockClient) SetGetIssueError(err *APIError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getError = err
}

// Reset clears every seeded value back to defaults.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authResult = &TestConnectionResult{
		OK:          true,
		UserID:      "mock-user",
		DisplayName: "Mock User",
		Email:       "mock@example.com",
	}
	m.projects = nil
	m.issues = make(map[string]*SentryIssue)
	m.issueOrder = nil
	m.getError = nil
}

// MockClientFactory returns a ClientFactory that always hands back the shared
// MockClient regardless of credentials.
func MockClientFactory(shared *MockClient) ClientFactory {
	return func(*SentryConfig, string) Client {
		return shared
	}
}
