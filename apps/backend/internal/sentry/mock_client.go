package sentry

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// MockClient backs the in-memory Client used by E2E tests. It is instance-aware:
// each Sentry instance has its own dataset keyed by instance ID, so a test can
// prove instance A and instance B return different data. A single shared
// MockClient per process is driven by the HTTP control routes mounted in mock
// mode; the ClientFactory hands each instance a view bound to its ID.
type MockClient struct {
	mu       sync.RWMutex
	datasets map[string]*mockDataset // instance ID → dataset
}

// mockDataset is the per-instance seeded state.
type mockDataset struct {
	authResult    *TestConnectionResult
	organizations []SentryOrganization
	projects      []SentryProject
	issues        map[string]*SentryIssue // short ID → issue
	// issueOrder preserves insertion order so SearchIssues returns a stable
	// sequence — Go map range is randomised.
	issueOrder []string
	getError   *APIError
}

// NewMockClient returns an empty instance-aware mock. Instances default to a
// successful TestAuth (seeded lazily) so a freshly-created instance flips to
// "Authenticated" without explicit seeding.
func NewMockClient() *MockClient {
	return &MockClient{datasets: make(map[string]*mockDataset)}
}

func newMockDataset() *mockDataset {
	return &mockDataset{
		authResult: defaultMockAuthResult(),
		issues:     make(map[string]*SentryIssue),
	}
}

func defaultMockAuthResult() *TestConnectionResult {
	return &TestConnectionResult{
		OK:          true,
		UserID:      "mock-user",
		DisplayName: "Mock User",
		Email:       "mock@example.com",
	}
}

// dataset returns the dataset for instanceID, lazily creating it. The caller
// must hold the write lock.
func (m *MockClient) dataset(instanceID string) *mockDataset {
	ds, ok := m.datasets[instanceID]
	if !ok {
		ds = newMockDataset()
		m.datasets[instanceID] = ds
	}
	return ds
}

// --- instance-scoped read operations (used by the per-instance Client view) ---

func (m *MockClient) testAuth(instanceID string) (*TestConnectionResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.datasets[instanceID]
	if ds == nil || ds.authResult == nil {
		return defaultMockAuthResult(), nil
	}
	r := *ds.authResult
	return &r, nil
}

func (m *MockClient) listOrganizations(instanceID string) ([]SentryOrganization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.datasets[instanceID]
	if ds == nil {
		return nil, nil
	}
	out := make([]SentryOrganization, len(ds.organizations))
	copy(out, ds.organizations)
	return out, nil
}

func (m *MockClient) listProjects(instanceID string) ([]SentryProject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.datasets[instanceID]
	if ds == nil {
		return nil, nil
	}
	out := make([]SentryProject, len(ds.projects))
	copy(out, ds.projects)
	return out, nil
}

func (m *MockClient) searchIssues(instanceID string, filter SearchFilter) (*SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.datasets[instanceID]
	if ds == nil {
		return &SearchResult{IsLast: true}, nil
	}
	out := make([]SentryIssue, 0, len(ds.issueOrder))
	for _, id := range ds.issueOrder {
		issue, ok := ds.issues[id]
		if !ok || !mockMatchesFilter(issue, filter) {
			continue
		}
		out = append(out, *issue)
	}
	return &SearchResult{Issues: out, IsLast: true}, nil
}

func (m *MockClient) getIssue(instanceID, idOrShortID string) (*SentryIssue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds := m.datasets[instanceID]
	if ds == nil {
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: "issue not found: " + idOrShortID}
	}
	if ds.getError != nil {
		err := *ds.getError
		return nil, &err
	}
	if issue, ok := ds.issues[idOrShortID]; ok {
		cp := *issue
		return &cp, nil
	}
	// Fall back to numeric ID lookup so callers can use either form.
	for _, i := range ds.issues {
		if i.ID == idOrShortID {
			cp := *i
			return &cp, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "issue not found: " + idOrShortID}
}

// --- setters used by MockController, scoped per instance ---

// SetAuthResult sets the auth result an instance returns from TestAuth.
func (m *MockClient) SetAuthResult(instanceID string, r *TestConnectionResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataset(instanceID).authResult = r
}

// SetOrganizations replaces an instance's organization list.
func (m *MockClient) SetOrganizations(instanceID string, organizations []SentryOrganization) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]SentryOrganization, len(organizations))
	copy(cp, organizations)
	m.dataset(instanceID).organizations = cp
}

// SetProjects replaces an instance's project list.
func (m *MockClient) SetProjects(instanceID string, projects []SentryProject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]SentryProject, len(projects))
	copy(cp, projects)
	m.dataset(instanceID).projects = cp
}

// AddIssue appends an issue to an instance's dataset (idempotent by key).
func (m *MockClient) AddIssue(instanceID string, issue *SentryIssue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ds := m.dataset(instanceID)
	cp := *issue
	key := issue.ShortID
	if key == "" {
		key = issue.ID
	}
	if _, exists := ds.issues[key]; !exists {
		ds.issueOrder = append(ds.issueOrder, key)
	}
	ds.issues[key] = &cp
}

// SetGetIssueError forces GetIssue on an instance to fail (nil clears it).
func (m *MockClient) SetGetIssueError(instanceID string, err *APIError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataset(instanceID).getError = err
}

// Reset clears every instance dataset back to empty.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.datasets = make(map[string]*mockDataset)
}

// mockMatchesFilter applies the filter predicates that map to a per-issue
// field, so E2E tests can assert the backend forwarded the filter rather than
// silently returning everything. Deliberately NOT enforced (no per-issue
// analog on SentryIssue): OrgSlug, Environment, and StatsPeriod. The real REST
// client's URL building for those params is covered in rest_client_test.go.
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

// mockInstanceClient is a Client view bound to one instance ID, delegating to
// the shared MockClient's per-instance dataset.
type mockInstanceClient struct {
	shared     *MockClient
	instanceID string
}

func (c *mockInstanceClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return c.shared.testAuth(c.instanceID)
}

func (c *mockInstanceClient) ListOrganizations(context.Context) ([]SentryOrganization, error) {
	return c.shared.listOrganizations(c.instanceID)
}

func (c *mockInstanceClient) ListProjects(context.Context) ([]SentryProject, error) {
	return c.shared.listProjects(c.instanceID)
}

func (c *mockInstanceClient) SearchIssues(_ context.Context, filter SearchFilter, _ string) (*SearchResult, error) {
	return c.shared.searchIssues(c.instanceID, filter)
}

func (c *mockInstanceClient) GetIssue(_ context.Context, idOrShortID string) (*SentryIssue, error) {
	return c.shared.getIssue(c.instanceID, idOrShortID)
}

// MockClientFactory returns a ClientFactory that hands each instance a view of
// the shared MockClient bound to that instance's ID.
func MockClientFactory(shared *MockClient) ClientFactory {
	return func(cfg *SentryConfig, _ string) Client {
		instanceID := ""
		if cfg != nil {
			instanceID = cfg.ID
		}
		return &mockInstanceClient{shared: shared, instanceID: instanceID}
	}
}
