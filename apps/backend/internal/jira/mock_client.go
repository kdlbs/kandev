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
	statuses    map[string][]JiraStatus // projectKey → statuses
	searchHits  []JiraTicket            // returned by SearchTickets regardless of JQL
	doneCalls   []doneTransitionCall
	getError    *APIError
}

type doneTransitionCall struct {
	TicketKey    string
	TransitionID string
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
		statuses:    make(map[string][]JiraStatus),
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

func (m *MockClient) ListProjectStatuses(_ context.Context, projectKey string) ([]JiraStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.statuses[projectKey]
	out := make([]JiraStatus, len(src))
	copy(out, src)
	return out, nil
}

// SearchTickets returns the tickets seeded via SetSearchHits. When jql is
// empty (or no seeded ticket key appears literally in the query) the full
// seeded set is returned — tests are expected to seed exactly the result
// they want to assert on. To narrow the response, mention a seeded key in
// the JQL (e.g. `key = PROJ-12`) and only matching tickets are returned.
func (m *MockClient) SearchTickets(_ context.Context, jql, _ string, maxResults int) (*SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hits := filterByJQL(m.searchHits, jql)
	if maxResults > 0 && len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	out := make([]JiraTicket, len(hits))
	copy(out, hits)
	return &SearchResult{Tickets: out, MaxResults: maxResults, IsLast: true}, nil
}

// filterByJQL is a stand-in for real JQL parsing. An empty query passes every
// hit through; a `project in (...)` clause narrows to hits in those projects;
// a `status in (...)` clause narrows to hits whose StatusName is listed; a
// mentioned seeded ticket key restricts to that key. All three compose, so a
// query combining project/status with a key still narrows to the key. A
// non-empty query that carries none of these clauses returns every hit (so
// tests don't have to construct a fully parseable JQL string just to fetch what
// they seeded). The clause handling lets E2E assert the filters narrow the list
// rather than being a no-op.
func filterByJQL(hits []JiraTicket, jql string) []JiraTicket {
	if jql == "" {
		return hits
	}
	if keys := projectKeysFromJQL(jql); len(keys) > 0 {
		hits = filterByProjectKeys(hits, keys)
	}
	if names := statusNamesFromJQL(jql); len(names) > 0 {
		hits = filterByStatusNames(hits, names)
	}
	// Ticket-key narrowing composes with project/status filters: apply it
	// whenever the (already narrowed) hits mention a seeded key, so a query that
	// combines project/status with a key still restricts to that key. When no
	// key is mentioned, return whatever the project/status clauses left (which
	// is every hit when the query carried none of these clauses).
	if matched := filterByTicketKey(hits, jql); matched != nil {
		return matched
	}
	return hits
}

// filterByTicketKey narrows hits to those whose key appears in the JQL. Returns
// nil (not an empty slice) when no seeded key is mentioned, so callers can tell
// "no key clause" apart from "key clause matched nothing".
func filterByTicketKey(hits []JiraTicket, jql string) []JiraTicket {
	matched := make([]JiraTicket, 0, len(hits))
	for _, t := range hits {
		if t.Key != "" && strings.Contains(jql, t.Key) {
			matched = append(matched, t)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	return matched
}

// projectKeysFromJQL extracts the quoted project keys from a `project in (...)`
// clause. Returns nil when the JQL carries no such clause.
func projectKeysFromJQL(jql string) map[string]struct{} {
	lower := strings.ToLower(jql)
	idx := strings.Index(lower, "project in (")
	if idx == -1 {
		return nil
	}
	rest := jql[idx+len("project in ("):]
	end := strings.Index(rest, ")")
	if end == -1 {
		return nil
	}
	keys := make(map[string]struct{})
	for _, part := range strings.Split(rest[:end], ",") {
		key := strings.Trim(strings.TrimSpace(part), `"`)
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func filterByProjectKeys(hits []JiraTicket, keys map[string]struct{}) []JiraTicket {
	matched := make([]JiraTicket, 0, len(hits))
	for _, t := range hits {
		if _, ok := keys[t.ProjectKey]; ok {
			matched = append(matched, t)
		}
	}
	return matched
}

// statusNamesFromJQL extracts the quoted status names from a `status in (...)`
// clause. Returns nil when the JQL carries no such clause.
//
// NOTE: the clause boundary is the first ")" in the remaining string, so a
// status name that itself contains ")" (e.g. "Ready (for review)") would be
// misextracted. The frontend never emits such names in these mock-driven tests,
// so a full quote-aware scan isn't warranted for this test stand-in. The same
// caveat applies to projectKeysFromJQL above.
func statusNamesFromJQL(jql string) map[string]struct{} {
	lower := strings.ToLower(jql)
	idx := strings.Index(lower, "status in (")
	if idx == -1 {
		return nil
	}
	rest := jql[idx+len("status in ("):]
	end := strings.Index(rest, ")")
	if end == -1 {
		return nil
	}
	names := make(map[string]struct{})
	for _, part := range strings.Split(rest[:end], ",") {
		name := strings.Trim(strings.TrimSpace(part), `"`)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func filterByStatusNames(hits []JiraTicket, names map[string]struct{}) []JiraTicket {
	matched := make([]JiraTicket, 0, len(hits))
	for _, t := range hits {
		if _, ok := names[t.StatusName]; ok {
			matched = append(matched, t)
		}
	}
	return matched
}

// --- Setters used by MockController ---

// SetAuthResult overrides the result returned by TestAuth (and consequently
// the auth-health probe). Pass nil to simulate an unconfigured auth state
// (returns OK=false with "mock: no auth result configured"); call Reset to
// restore the default success.
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

// SetProjectStatuses replaces the statuses returned by ListProjectStatuses for
// a project key.
func (m *MockClient) SetProjectStatuses(projectKey string, statuses []JiraStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]JiraStatus, len(statuses))
	copy(cp, statuses)
	m.statuses[projectKey] = cp
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
	m.statuses = make(map[string][]JiraStatus)
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
