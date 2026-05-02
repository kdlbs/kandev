package linear

import (
	"context"
	"net/http"
	"sync"
)

// MockClient implements Client with in-memory data for E2E tests. Mirrors the
// Jira mock: a single shared instance per process, driven by HTTP control
// routes mounted in mock mode. Per-workspace isolation isn't needed for the
// scenarios these tests cover.
type MockClient struct {
	mu           sync.RWMutex
	authResult   *TestConnectionResult
	teams        []LinearTeam
	statesByTeam map[string][]LinearWorkflowState
	issues       map[string]*LinearIssue // identifier (e.g. ENG-12) → issue
	// issueOrder preserves insertion order so SearchIssues returns a stable
	// sequence — Go map range is randomised, which would make any future test
	// that asserts on result position intermittently flaky.
	issueOrder    []string
	getError      *APIError
	setStateCalls []setStateCall
}

type setStateCall struct {
	IssueID string
	StateID string
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
			OrgSlug:     "mock-org",
			OrgName:     "Mock Org",
		},
		statesByTeam: make(map[string][]LinearWorkflowState),
		issues:       make(map[string]*LinearIssue),
	}
}

// --- Client interface ---

func (m *MockClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.authResult == nil {
		return &TestConnectionResult{OK: false, Error: "mock: no auth result configured"}, nil
	}
	res := *m.authResult
	return &res, nil
}

func (m *MockClient) GetIssue(_ context.Context, identifier string) (*LinearIssue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getError != nil {
		err := *m.getError
		return nil, &err
	}
	i, ok := m.issues[identifier]
	if !ok {
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: "issue not found: " + identifier}
	}
	cp := *i
	return &cp, nil
}

func (m *MockClient) ListStates(_ context.Context, teamKey string) ([]LinearWorkflowState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.statesByTeam[teamKey]
	out := make([]LinearWorkflowState, len(src))
	copy(out, src)
	return out, nil
}

func (m *MockClient) SetIssueState(_ context.Context, issueID, stateID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setStateCalls = append(m.setStateCalls, setStateCall{IssueID: issueID, StateID: stateID})
	return nil
}

func (m *MockClient) ListTeams(context.Context) ([]LinearTeam, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]LinearTeam, len(m.teams))
	copy(out, m.teams)
	return out, nil
}

func (m *MockClient) SearchIssues(_ context.Context, _ SearchFilter, _ string, maxResults int) (*SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]LinearIssue, 0, len(m.issueOrder))
	for _, id := range m.issueOrder {
		issue, ok := m.issues[id]
		if !ok {
			continue
		}
		out = append(out, *issue)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return &SearchResult{Issues: out, MaxResults: maxResults, IsLast: true}, nil
}

// --- Setters used by MockController ---

// SetAuthResult overrides the result returned by TestAuth (and the auth-health
// probe). Pass nil to simulate an unconfigured auth state (returns OK=false);
// call Reset to restore the default success.
func (m *MockClient) SetAuthResult(r *TestConnectionResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authResult = r
}

// SetTeams replaces the teams returned by ListTeams.
func (m *MockClient) SetTeams(teams []LinearTeam) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]LinearTeam, len(teams))
	copy(cp, teams)
	m.teams = cp
}

// SetStates replaces the workflow states returned by ListStates for a team.
func (m *MockClient) SetStates(teamKey string, states []LinearWorkflowState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]LinearWorkflowState, len(states))
	copy(cp, states)
	m.statesByTeam[teamKey] = cp
}

// AddIssue inserts (or replaces) an issue keyed by its Identifier.
func (m *MockClient) AddIssue(issue *LinearIssue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *issue
	if _, exists := m.issues[issue.Identifier]; !exists {
		m.issueOrder = append(m.issueOrder, issue.Identifier)
	}
	m.issues[issue.Identifier] = &cp
}

// SetGetIssueError forces GetIssue to return the given APIError. Pass nil to clear.
func (m *MockClient) SetGetIssueError(err *APIError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getError = err
}

// SetStateCalls returns recorded SetIssueState calls.
func (m *MockClient) SetStateCalls() []setStateCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]setStateCall, len(m.setStateCalls))
	copy(out, m.setStateCalls)
	return out
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
		OrgSlug:     "mock-org",
		OrgName:     "Mock Org",
	}
	m.teams = nil
	m.statesByTeam = make(map[string][]LinearWorkflowState)
	m.issues = make(map[string]*LinearIssue)
	m.issueOrder = nil
	m.getError = nil
	m.setStateCalls = nil
}

// MockClientFactory returns a ClientFactory that always hands back the shared
// MockClient regardless of credentials.
func MockClientFactory(shared *MockClient) ClientFactory {
	return func(*LinearConfig, string) Client {
		return shared
	}
}
