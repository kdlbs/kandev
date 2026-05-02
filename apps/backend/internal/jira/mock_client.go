package jira

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// MockClient implements Client with in-memory data for E2E tests. The mock is
// shared across all workspaces in the process — that mirrors how the GitHub
// mock works and keeps the e2e seeding API simple. Per-workspace isolation is
// not needed for the scenarios these tests cover (single workspace per worker).
type MockClient struct {
	mu          sync.RWMutex
	authResult  *TestConnectionResult
	tickets     map[string]*JiraTicket      // key → ticket
	transitions map[string][]JiraTransition // ticketKey → transitions
	projects    []JiraProject
	searchHits  []JiraTicket // returned by SearchTickets regardless of JQL
	doneCalls   []doneTransitionCall
	getError    *APIError
}

type doneTransitionCall struct {
	TicketKey    string `json:"ticketKey"`
	TransitionID string `json:"transitionId"`
}

// NewMockClient returns a MockClient with TestAuth set to a successful result
// so a freshly-created config flips to "Authenticated" without seeding.
func NewMockClient() *MockClient {
	return &MockClient{
		authResult: &TestConnectionResult{
			OK:          true,
			AccountID:   "mock-account",
			DisplayName: "Mock User",
			Email:       "mock@example.com",
		},
		tickets:     make(map[string]*JiraTicket),
		transitions: make(map[string][]JiraTransition),
	}
}

// --- Client interface ---

func (m *MockClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.authResult == nil {
		return &TestConnectionResult{OK: false, Error: "mock: no auth result configured"}, nil
	}
	// Return a copy so callers can't mutate the canned result.
	res := *m.authResult
	return &res, nil
}

func (m *MockClient) GetTicket(_ context.Context, ticketKey string) (*JiraTicket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getError != nil {
		err := *m.getError
		return nil, &err
	}
	t, ok := m.tickets[ticketKey]
	if !ok {
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: "ticket not found: " + ticketKey}
	}
	cp := *t
	return &cp, nil
}

func (m *MockClient) ListTransitions(_ context.Context, ticketKey string) ([]JiraTransition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]JiraTransition, len(m.transitions[ticketKey]))
	copy(out, m.transitions[ticketKey])
	return out, nil
}

func (m *MockClient) DoTransition(_ context.Context, ticketKey, transitionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doneCalls = append(m.doneCalls, doneTransitionCall{TicketKey: ticketKey, TransitionID: transitionID})
	return nil
}

func (m *MockClient) ListProjects(context.Context) ([]JiraProject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]JiraProject, len(m.projects))
	copy(out, m.projects)
	return out, nil
}

func (m *MockClient) SearchTickets(_ context.Context, jql, _ string, maxResults int) (*SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hits := m.searchHits
	if jql != "" {
		hits = filterByJQL(hits, jql)
	}
	if maxResults > 0 && len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	out := make([]JiraTicket, len(hits))
	copy(out, hits)
	return &SearchResult{Tickets: out, MaxResults: maxResults, IsLast: true}, nil
}

// filterByJQL is the cheapest possible JQL imitation: if the query mentions a
// key like "PROJ-12", restrict to tickets with that key. Otherwise return
// everything. Real JQL parsing is out of scope — tests should seed exactly
// what they expect to see and rely on this naive filter only as a smoke check.
func filterByJQL(hits []JiraTicket, jql string) []JiraTicket {
	out := make([]JiraTicket, 0, len(hits))
	for _, t := range hits {
		if t.Key != "" && strings.Contains(jql, t.Key) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return hits
	}
	return out
}

// --- Setters used by MockController ---

// SetAuthResult overrides the result returned by TestAuth (and consequently
// the auth-health probe). Pass nil to revert to the default success.
func (m *MockClient) SetAuthResult(r *TestConnectionResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authResult = r
}

// AddTicket inserts (or replaces) a ticket keyed by its Key field.
func (m *MockClient) AddTicket(t *JiraTicket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *t
	m.tickets[t.Key] = &cp
}

// AddTransitions appends transitions available for a ticket.
func (m *MockClient) AddTransitions(ticketKey string, ts []JiraTransition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitions[ticketKey] = append(m.transitions[ticketKey], ts...)
}

// SetProjects replaces the projects list returned by ListProjects.
func (m *MockClient) SetProjects(projects []JiraProject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]JiraProject, len(projects))
	copy(cp, projects)
	m.projects = cp
}

// SetSearchHits replaces the tickets returned by SearchTickets.
func (m *MockClient) SetSearchHits(hits []JiraTicket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]JiraTicket, len(hits))
	copy(cp, hits)
	m.searchHits = cp
}

// SetGetTicketError forces GetTicket to return the given APIError until cleared
// with nil. Lets tests assert on the popover's error-state UI.
func (m *MockClient) SetGetTicketError(err *APIError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getError = err
}

// TransitionCalls returns the recorded DoTransition calls.
func (m *MockClient) TransitionCalls() []doneTransitionCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]doneTransitionCall, len(m.doneCalls))
	copy(out, m.doneCalls)
	return out
}

// Reset clears every seeded value back to defaults. Called between tests.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authResult = &TestConnectionResult{
		OK:          true,
		AccountID:   "mock-account",
		DisplayName: "Mock User",
		Email:       "mock@example.com",
	}
	m.tickets = make(map[string]*JiraTicket)
	m.transitions = make(map[string][]JiraTransition)
	m.projects = nil
	m.searchHits = nil
	m.doneCalls = nil
	m.getError = nil
}

// MockClientFactory always returns the same shared MockClient regardless of
// per-workspace credentials. Use this from Provide when KANDEV_MOCK_JIRA=true.
func MockClientFactory(shared *MockClient) ClientFactory {
	return func(*JiraConfig, string) Client {
		return shared
	}
}
