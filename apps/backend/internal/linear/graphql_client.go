package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// LinearAPIEndpoint is the single GraphQL endpoint exposed by Linear. There is
// no per-organization host: auth scopes the request to the right workspace.
const LinearAPIEndpoint = "https://api.linear.app/graphql"

// GraphQLClient talks to Linear's GraphQL API using a Personal API Key. The
// client holds no state beyond the credential so it can be recreated cheaply
// per workspace if config changes.
type GraphQLClient struct {
	http        *http.Client
	endpoint    string
	apiKey      string
	maxBodySize int64
}

// NewGraphQLClient builds a client from a LinearConfig + secret.
func NewGraphQLClient(_ *LinearConfig, secret string) *GraphQLClient {
	return &GraphQLClient{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		endpoint:    LinearAPIEndpoint,
		apiKey:      secret,
		maxBodySize: 4 << 20, // 4 MB — Linear payloads are small by design.
	}
}

const userAgent = "kandev/1.0 (+https://github.com/kdlbs/kandev)"

// graphqlRequest is the standard {query, variables} envelope.
type graphqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// graphqlError captures one entry from the GraphQL `errors` array.
type graphqlError struct {
	Message    string                 `json:"message"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// graphqlResponse is the raw envelope; `Data` is decoded into the caller's
// struct via a second pass to avoid generics.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors,omitempty"`
}

// do executes a GraphQL operation and decodes `data` into out (may be nil).
// Non-2xx responses and GraphQL-level errors both surface as *APIError so
// callers can switch on status. Linear returns 4xx for auth failures and 200
// with errors[] for everything else.
func (c *GraphQLClient) do(ctx context.Context, query string, variables map[string]interface{}, out interface{}) error {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	// Linear accepts the API key directly as the Authorization header value.
	req.Header.Set("Authorization", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodySize))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: summarizeBody(raw)}
	}
	var env graphqlResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: "invalid GraphQL response: " + err.Error()}
	}
	if len(env.Errors) > 0 {
		return &APIError{StatusCode: graphqlErrorStatus(env.Errors), Message: joinGraphQLErrors(env.Errors)}
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// graphqlErrorStatus maps a GraphQL error envelope to an HTTP-like status. Auth
// failures get 401 so the UI can surface them as "credentials invalid";
// everything else falls back to 400 (request-level failure rather than server).
func graphqlErrorStatus(errs []graphqlError) int {
	for _, e := range errs {
		if e.Extensions == nil {
			continue
		}
		if t, ok := e.Extensions["type"].(string); ok {
			switch strings.ToLower(t) {
			case "authentication error", "authentication", "unauthenticated":
				return http.StatusUnauthorized
			case "feature not access", "permission", "forbidden":
				return http.StatusForbidden
			case "invalid input":
				return http.StatusBadRequest
			}
		}
	}
	return http.StatusBadRequest
}

func joinGraphQLErrors(errs []graphqlError) string {
	if len(errs) == 1 {
		return errs[0].Message
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, "; ")
}

// summarizeBody trims an error body to a useful length for log/error messages.
func summarizeBody(raw []byte) string {
	const maxMsg = 500
	s := strings.TrimSpace(string(raw))
	if len(s) > maxMsg {
		return s[:maxMsg] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}

// --- viewer / TestAuth ---

const viewerQuery = `
query Viewer {
	viewer {
		id
		name
		displayName
		email
	}
	organization {
		urlKey
		name
	}
}`

type viewerData struct {
	Viewer struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
	} `json:"viewer"`
	Organization struct {
		URLKey string `json:"urlKey"`
		Name   string `json:"name"`
	} `json:"organization"`
}

// TestAuth hits the viewer query — the cheapest authenticated query Linear
// exposes — and returns a structured result so the UI can render a meaningful
// failure inline.
func (c *GraphQLClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	var data viewerData
	if err := c.do(ctx, viewerQuery, nil, &data); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return &TestConnectionResult{OK: false, Error: apiErr.Error()}, nil
		}
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	name := data.Viewer.DisplayName
	if name == "" {
		name = data.Viewer.Name
	}
	return &TestConnectionResult{
		OK:          true,
		UserID:      data.Viewer.ID,
		DisplayName: name,
		Email:       data.Viewer.Email,
		OrgSlug:     data.Organization.URLKey,
		OrgName:     data.Organization.Name,
	}, nil
}

// --- teams ---

const teamsQuery = `
query Teams {
	teams(first: 100, orderBy: updatedAt) {
		nodes { id key name }
	}
}`

type teamsData struct {
	Teams struct {
		Nodes []struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"teams"`
}

// ListTeams returns up to 100 teams (Linear's max page size). Fine for the
// settings dropdown; pagination can be added later if needed.
func (c *GraphQLClient) ListTeams(ctx context.Context) ([]LinearTeam, error) {
	var data teamsData
	if err := c.do(ctx, teamsQuery, nil, &data); err != nil {
		return nil, err
	}
	out := make([]LinearTeam, 0, len(data.Teams.Nodes))
	for _, t := range data.Teams.Nodes {
		out = append(out, LinearTeam{ID: t.ID, Key: t.Key, Name: t.Name})
	}
	return out, nil
}

// --- workflow states ---

const teamStatesQuery = `
query TeamStates($filter: WorkflowStateFilter!) {
	workflowStates(first: 100, filter: $filter) {
		nodes { id name type color position }
	}
}`

type statesData struct {
	WorkflowStates struct {
		Nodes []struct {
			ID       string  `json:"id"`
			Name     string  `json:"name"`
			Type     string  `json:"type"`
			Color    string  `json:"color"`
			Position float64 `json:"position"`
		} `json:"nodes"`
	} `json:"workflowStates"`
}

// ListStates returns the workflow states for a team identified by its key.
// Equivalent of Jira's "ListTransitions" but unconditional — the same set of
// states is available regardless of the issue's current state.
func (c *GraphQLClient) ListStates(ctx context.Context, teamKey string) ([]LinearWorkflowState, error) {
	if teamKey == "" {
		return nil, fmt.Errorf("teamKey required")
	}
	vars := map[string]interface{}{
		"filter": map[string]interface{}{
			"team": map[string]interface{}{"key": map[string]interface{}{"eq": teamKey}},
		},
	}
	var data statesData
	if err := c.do(ctx, teamStatesQuery, vars, &data); err != nil {
		return nil, err
	}
	out := make([]LinearWorkflowState, 0, len(data.WorkflowStates.Nodes))
	for _, s := range data.WorkflowStates.Nodes {
		out = append(out, LinearWorkflowState{
			ID:   s.ID,
			Name: s.Name,
			Type: s.Type,
			// Linear uses fractional indexing for state ordering — round
			// rather than truncate so adjacent fractional positions don't
			// collapse onto the same int.
			Color:    s.Color,
			Position: int(math.Round(s.Position)),
		})
	}
	return out, nil
}

// --- issues ---

// issueFragment is the projection used by both single-issue fetches and search
// results. Centralised so the wire schema stays consistent.
const issueFragment = `
	id
	identifier
	title
	description
	url
	updatedAt
	priority
	priorityLabel
	state { id name type color }
	team { id key }
	assignee { name email avatarUrl }
	creator { name avatarUrl }
`

type issueNode struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	UpdatedAt   string `json:"updatedAt"`
	Priority    int    `json:"priority"`
	PriorityLab string `json:"priorityLabel"`
	State       struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Color string `json:"color"`
	} `json:"state"`
	Team struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	} `json:"team"`
	Assignee *struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"assignee"`
	Creator *struct {
		Name      string `json:"name"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"creator"`
}

func issueNodeToIssue(n *issueNode) LinearIssue {
	out := LinearIssue{
		ID:            n.ID,
		Identifier:    n.Identifier,
		Title:         n.Title,
		Description:   n.Description,
		StateID:       n.State.ID,
		StateName:     n.State.Name,
		StateType:     n.State.Type,
		StateCategory: stateCategory(n.State.Type),
		TeamID:        n.Team.ID,
		TeamKey:       n.Team.Key,
		Priority:      n.Priority,
		PriorityLabel: n.PriorityLab,
		Updated:       n.UpdatedAt,
		URL:           n.URL,
	}
	if n.Assignee != nil {
		out.AssigneeName = n.Assignee.Name
		out.AssigneeEmail = n.Assignee.Email
		out.AssigneeIcon = n.Assignee.AvatarURL
	}
	if n.Creator != nil {
		out.CreatorName = n.Creator.Name
		out.CreatorIcon = n.Creator.AvatarURL
	}
	return out
}

// stateCategory maps Linear's state type onto Jira's three-bucket category so
// the frontend can style status pills uniformly across integrations.
func stateCategory(stateType string) string {
	switch strings.ToLower(stateType) {
	case "backlog", "unstarted", "triage":
		return "new"
	case "started":
		return "indeterminate"
	case "completed", "canceled":
		return "done"
	default:
		return "new"
	}
}

const issueByIDQuery = `
query IssueByID($id: String!) {
	issue(id: $id) {` + issueFragment + `}
}`

// GetIssue loads a Linear issue by identifier (e.g. "ENG-123") and attaches
// the team's available workflow states. Linear's `issue(id)` field accepts
// either the UUID or the human identifier.
func (c *GraphQLClient) GetIssue(ctx context.Context, identifier string) (*LinearIssue, error) {
	var data struct {
		Issue *issueNode `json:"issue"`
	}
	if err := c.do(ctx, issueByIDQuery, map[string]interface{}{"id": identifier}, &data); err != nil {
		return nil, err
	}
	if data.Issue == nil {
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: "issue not found: " + identifier}
	}
	issue := issueNodeToIssue(data.Issue)
	states, err := c.ListStates(ctx, issue.TeamKey)
	if err != nil {
		// Caller cancellation / deadline must propagate — returning a
		// partial-success response after the request was abandoned would
		// break downstream correctness.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return nil, err
		}
		// Network blip or 5xx: keep the issue and let the UI render without
		// the state selector. The user can still read the issue and refresh.
	} else {
		issue.States = states
	}
	return &issue, nil
}

// --- mutations ---

const setStateMutation = `
mutation SetIssueState($id: String!, $stateId: String!) {
	issueUpdate(id: $id, input: { stateId: $stateId }) {
		success
	}
}`

type setStateData struct {
	IssueUpdate struct {
		Success bool `json:"success"`
	} `json:"issueUpdate"`
}

// SetIssueState moves an issue to a new workflow state. issueID may be either
// the UUID or the human identifier — Linear accepts both. Mirrors Jira's
// DoTransition.
func (c *GraphQLClient) SetIssueState(ctx context.Context, issueID, stateID string) error {
	var data setStateData
	vars := map[string]interface{}{"id": issueID, "stateId": stateID}
	if err := c.do(ctx, setStateMutation, vars, &data); err != nil {
		return err
	}
	if !data.IssueUpdate.Success {
		return &APIError{StatusCode: http.StatusInternalServerError, Message: "issueUpdate returned success=false"}
	}
	return nil
}

// --- search ---

const searchIssuesQuery = `
query SearchIssues($filter: IssueFilter, $first: Int!, $after: String) {
	issues(filter: $filter, first: $first, after: $after, orderBy: updatedAt) {
		nodes {` + issueFragment + `}
		pageInfo { hasNextPage endCursor }
	}
}`

type searchIssuesData struct {
	Issues struct {
		Nodes    []issueNode `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"issues"`
}

// SearchIssues runs a filtered search and returns a page of issues. pageToken
// is the `endCursor` returned in the previous page; pass "" for the first
// page. maxResults is capped at 100 (Linear's max).
func (c *GraphQLClient) SearchIssues(ctx context.Context, filter SearchFilter, pageToken string, maxResults int) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 25
	}
	if maxResults > 100 {
		maxResults = 100
	}
	gqlFilter := buildIssueFilter(filter)
	vars := map[string]interface{}{"filter": gqlFilter, "first": maxResults}
	if pageToken != "" {
		vars["after"] = pageToken
	}
	var data searchIssuesData
	if err := c.do(ctx, searchIssuesQuery, vars, &data); err != nil {
		return nil, err
	}
	out := &SearchResult{
		MaxResults:    maxResults,
		IsLast:        !data.Issues.PageInfo.HasNextPage,
		NextPageToken: data.Issues.PageInfo.EndCursor,
		Issues:        make([]LinearIssue, 0, len(data.Issues.Nodes)),
	}
	for i := range data.Issues.Nodes {
		out.Issues = append(out.Issues, issueNodeToIssue(&data.Issues.Nodes[i]))
	}
	return out, nil
}

// buildIssueFilter translates our SearchFilter into Linear's IssueFilter input.
// Empty fields are dropped so we don't send `{ "team": null }` and similar
// (which Linear treats as a typed null and rejects).
func buildIssueFilter(f SearchFilter) map[string]interface{} {
	out := map[string]interface{}{}
	if q := strings.TrimSpace(f.Query); q != "" {
		// Linear has no top-level free-text field, but `searchableContent`
		// matches across title and description.
		out["searchableContent"] = map[string]interface{}{"contains": q}
	}
	if f.TeamKey != "" {
		out["team"] = map[string]interface{}{"key": map[string]interface{}{"eq": f.TeamKey}}
	}
	if len(f.StateIDs) > 0 {
		out["state"] = map[string]interface{}{"id": map[string]interface{}{"in": f.StateIDs}}
	}
	switch f.Assigned {
	case "me":
		// `null: false` + assignee filter would require the viewer ID; instead
		// we use the `viewer` shortcut Linear exposes via `assignee.isMe`.
		out["assignee"] = map[string]interface{}{"isMe": map[string]interface{}{"eq": true}}
	case "unassigned":
		out["assignee"] = map[string]interface{}{"null": true}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
