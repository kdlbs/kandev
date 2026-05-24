package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CloudClient is a Jira REST client. Despite the name, it supports both
// Atlassian Cloud and self-hosted Server / Data Center instances:
//
//   - Cloud uses REST v3 (`/rest/api/3/...`) and the token-paginated
//     `/search/jql` endpoint.
//   - Server/DC exposes only REST v2 (`/rest/api/2/...`) and the legacy
//     `startAt`-paginated `/search`.
//
// Auth modes:
//
//   - api_token (Cloud-only): Basic auth with {email}:{token} minted at
//     id.atlassian.com.
//   - pat (Server-only): Personal Access Token sent as `Authorization: Bearer`.
//   - session_cookie (Cloud-only): the secret is the JWT value of
//     `cloud.session.token` (password accounts) or `tenant.session.token`
//     (SSO), copied from DevTools → Application → Cookies.
//     buildSessionCookieHeader emits those two Atlassian-specific cookie
//     names, and validateAuthInstance rejects the combo on Server/DC where
//     the equivalent cookie is JSESSIONID — Server/DC users authenticate
//     via PAT until a Server-aware session-cookie path is added.
//
// The client holds no state beyond credentials so it can be recreated cheaply
// when config changes.
type CloudClient struct {
	http         *http.Client
	siteURL      string
	email        string
	secret       string
	authMethod   string
	instanceType string
	apiBase      string // "/rest/api/3" for cloud, "/rest/api/2" for server.
	maxBodySize  int64
}

// NewCloudClient builds a client from a JiraConfig + secret. siteURL is
// normalized: trailing slash stripped, https:// prepended when the user saved
// only a hostname (legacy rows; new rows are normalized on save). An empty
// InstanceType is treated as cloud — that's how every config row written by
// pre-Server-support releases looks, and we can't pick differently without
// breaking those installs.
func NewCloudClient(cfg *JiraConfig, secret string) *CloudClient {
	site := strings.TrimRight(cfg.SiteURL, "/")
	if site != "" && !strings.Contains(site, "://") {
		site = "https://" + site
	}
	instance := cfg.InstanceType
	if instance == "" {
		instance = InstanceTypeCloud
	}
	apiBase := "/rest/api/3"
	if instance == InstanceTypeServer {
		apiBase = "/rest/api/2"
	}
	return &CloudClient{
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Don't follow redirects: Jira REST endpoints shouldn't redirect for
			// authenticated calls. Atlassian redirects unauthenticated or
			// step-up-required requests to a login HTML page (with a 200 status),
			// which masks the real auth failure and breaks JSON decoding. By
			// returning the last response as-is, we preserve the 3xx status and
			// the informative body ("Step-up authentication is required...").
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		siteURL:      site,
		email:        cfg.Email,
		secret:       secret,
		authMethod:   cfg.AuthMethod,
		instanceType: instance,
		apiBase:      apiBase,
		maxBodySize:  4 << 20, // 4 MB — Jira payloads are small by design.
	}
}

// authorize applies the client's auth strategy to a request.
const userAgent = "kandev/1.0 (+https://github.com/kdlbs/kandev)"

func (c *CloudClient) authorize(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	switch c.authMethod {
	case AuthMethodAPIToken:
		basic := base64.StdEncoding.EncodeToString([]byte(c.email + ":" + c.secret))
		req.Header.Set("Authorization", "Basic "+basic)
	case AuthMethodPAT:
		req.Header.Set("Authorization", "Bearer "+c.secret)
	case AuthMethodSessionCookie:
		req.Header.Set("Cookie", buildSessionCookieHeader(c.secret))
	}
}

// buildSessionCookieHeader wraps a bare session-token JWT under both
// known Atlassian cookie names. A single paste works for password accounts
// (`cloud.session.token`) and SSO tenants (`tenant.session.token`) without
// asking the user to know which one they have.
func buildSessionCookieHeader(secret string) string {
	return "cloud.session.token=" + secret + "; tenant.session.token=" + secret
}

// do executes a request and decodes a 2xx JSON body into out (may be nil).
// Non-2xx responses are returned as *APIError so callers can switch on status.
func (c *CloudClient) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.siteURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req)
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
		return &APIError{StatusCode: resp.StatusCode, Message: c.summarizeBody(resp, raw, false)}
	}
	// Guardrail: some misconfigured Atlassian flows return a 200 HTML login
	// page instead of JSON. If we accidentally get HTML, surface an auth error
	// rather than letting json.Unmarshal fail with "invalid character '<'".
	if isHTMLResponse(resp, raw) {
		return &APIError{StatusCode: resp.StatusCode, Message: c.summarizeBody(resp, raw, true)}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// isHTMLResponse checks whether a response body is HTML rather than JSON, so
// we can convert Atlassian's implicit-login-page responses into a clear
// auth error message.
func isHTMLResponse(resp *http.Response, raw []byte) bool {
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(ct), "text/html") {
		return true
	}
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("<"))
}

// summarizeBody returns a short, useful error message from a Jira response.
// For HTML bodies it skips the page content in favor of an auth-method-aware
// hint; for plain text or JSON it returns the body verbatim (capped, so we
// don't spam logs with multi-KB pages). htmlOn2xx flags the "successful status
// but HTML body" case — that means the response went through an auth-filter
// login page instead of the REST API, so the hint adds "instead of JSON" to
// distinguish it from the more common error-status HTML case.
func (c *CloudClient) summarizeBody(resp *http.Response, raw []byte, htmlOn2xx bool) string {
	if htmlOn2xx || isHTMLResponse(resp, raw) {
		suffix := ". "
		if htmlOn2xx {
			suffix = " instead of JSON. "
		}
		return "Jira returned an HTML page (status " + strconv.Itoa(resp.StatusCode) +
			")" + suffix + c.authHint()
	}
	const maxMsg = 500
	if len(raw) > maxMsg {
		return string(raw[:maxMsg]) + "…"
	}
	return string(raw)
}

// authHint picks an actionable explanation for an HTML-from-API response,
// based on auth method and instance type. The wording differs because the
// failure mode is different in each combo: a Cloud API token rejection looks
// the same on the wire as a Server PAT rejection, but the user's next step
// (check id.atlassian.com vs. check the instance's PAT page) is different.
func (c *CloudClient) authHint() string {
	switch c.authMethod {
	case AuthMethodAPIToken:
		if c.instanceType == InstanceTypeServer {
			// Mismatched config: API-token auth is Cloud-only. Server/DC will
			// reject `Basic email:token` and redirect to /login.jsp, which the
			// JSON decoder sees as HTML.
			return "API token auth is Cloud-only — this site looks like Jira Server/DC. Switch the auth method to PAT (Personal Access Token)."
		}
		return "Check the email and API token; create a new one at id.atlassian.com/manage-profile/security/api-tokens if needed."
	case AuthMethodPAT:
		return "The Personal Access Token is rejected. Recreate it from your Jira profile → Personal Access Tokens and update the saved value."
	case AuthMethodSessionCookie:
		return "The session cookie likely expired or requires step-up auth. Sign in again in your Jira tab and re-copy the cookie."
	default:
		return "Check the saved credentials."
	}
}

// TestAuth hits /rest/api/3/myself which is the cheapest authenticated
// endpoint; a 200 proves credentials work and identifies the user.
func (c *CloudClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	var body struct {
		AccountID    string `json:"accountId"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/myself", nil, &body); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return &TestConnectionResult{OK: false, Error: apiErr.Error()}, nil
		}
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return &TestConnectionResult{
		OK:          true,
		AccountID:   body.AccountID,
		DisplayName: body.DisplayName,
		Email:       body.EmailAddress,
	}, nil
}

// issueResponse mirrors the subset of the Atlassian issue payload we consume.
type issueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string      `json:"summary"`
		Description interface{} `json:"description"` // ADF or string depending on API version
		Updated     string      `json:"updated"`
		Status      struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"` // "new" | "indeterminate" | "done"
			} `json:"statusCategory"`
		} `json:"status"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		IssueType struct {
			Name    string `json:"name"`
			IconURL string `json:"iconUrl"`
		} `json:"issuetype"`
		Priority struct {
			Name    string `json:"name"`
			IconURL string `json:"iconUrl"`
		} `json:"priority"`
		Assignee *jiraUser `json:"assignee"`
		Reporter *jiraUser `json:"reporter"`
	} `json:"fields"`
}

type jiraUser struct {
	DisplayName string `json:"displayName"`
	AvatarURLs  struct {
		Size24 string `json:"24x24"`
		Size32 string `json:"32x32"`
	} `json:"avatarUrls"`
}

func (u *jiraUser) avatar() string {
	if u == nil {
		return ""
	}
	if u.AvatarURLs.Size24 != "" {
		return u.AvatarURLs.Size24
	}
	return u.AvatarURLs.Size32
}

func (u *jiraUser) name() string {
	if u == nil {
		return ""
	}
	return u.DisplayName
}

type transitionsResponse struct {
	Transitions []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		To   struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"to"`
	} `json:"transitions"`
}

// GetTicket fetches the ticket + available transitions in two calls. We ask
// Jira for the ADF-rendered description so the UI gets plain-text rather than
// an opaque document tree. A 4xx on the transitions call is bubbled up so the
// UI can surface auth/permission failures rather than silently rendering a
// ticket with an empty transitions menu.
func (c *CloudClient) GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error) {
	var issue issueResponse
	path := c.apiBase + "/issue/" + url.PathEscape(ticketKey) + "?expand=renderedFields"
	if err := c.do(ctx, http.MethodGet, path, nil, &issue); err != nil {
		return nil, err
	}
	t := issueToTicket(&issue, c.siteURL)
	transitions, terr := c.ListTransitions(ctx, ticketKey)
	if terr != nil {
		var apiErr *APIError
		if errors.As(terr, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return nil, terr
		}
		// Network blips or 5xx: keep the ticket and let the UI render without
		// transitions. The user can still read the ticket and refresh.
	} else {
		t.Transitions = transitions
	}
	return &t, nil
}

// ListTransitions returns the transitions currently available for ticketKey.
func (c *CloudClient) ListTransitions(ctx context.Context, ticketKey string) ([]JiraTransition, error) {
	var resp transitionsResponse
	path := c.apiBase + "/issue/" + url.PathEscape(ticketKey) + "/transitions"
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]JiraTransition, 0, len(resp.Transitions))
	for _, t := range resp.Transitions {
		out = append(out, JiraTransition{
			ID:           t.ID,
			Name:         t.Name,
			ToStatusID:   t.To.ID,
			ToStatusName: t.To.Name,
		})
	}
	return out, nil
}

// DoTransition asks Jira to apply a specific transition by ID. The Jira API
// returns 204 on success.
func (c *CloudClient) DoTransition(ctx context.Context, ticketKey, transitionID string) error {
	body := map[string]interface{}{
		"transition": map[string]string{"id": transitionID},
	}
	path := c.apiBase + "/issue/" + url.PathEscape(ticketKey) + "/transitions"
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// ListProjects returns up to 200 projects (the Jira max per page for this
// endpoint). Fine for the settings dropdown; pagination can be added later if
// it ever becomes a problem.
func (c *CloudClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
	var body struct {
		Values []struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"values"`
	}
	// Server/DC exposes /project/search starting with Jira 8.0 (2019). For
	// older instances the call returns 404, which surfaces as an API error the
	// user can read. The legacy GET /project endpoint isn't paginated and we
	// don't need to keep two code paths just for ancient deployments.
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/project/search?maxResults=200", nil, &body); err != nil {
		return nil, err
	}
	out := make([]JiraProject, 0, len(body.Values))
	for _, p := range body.Values {
		out = append(out, JiraProject{ID: p.ID, Key: p.Key, Name: p.Name})
	}
	return out, nil
}

// cloudSearchResponse mirrors the subset of /rest/api/3/search/jql we consume.
// The token-paginated endpoint exposes no total count; pagination is driven by
// `nextPageToken`. Transitions are intentionally omitted from search results
// (fetched lazily when the user opens a ticket).
type cloudSearchResponse struct {
	Issues        []issueResponse `json:"issues"`
	NextPageToken string          `json:"nextPageToken"`
	IsLast        bool            `json:"isLast"`
}

// serverSearchResponse mirrors the legacy `startAt`-paginated `/search`
// endpoint that Jira Server / Data Center exposes. We synthesize a string
// page token from `startAt + maxResults` so the public SearchResult shape stays
// identical regardless of upstream pagination style.
type serverSearchResponse struct {
	Issues     []issueResponse `json:"issues"`
	StartAt    int             `json:"startAt"`
	MaxResults int             `json:"maxResults"`
	Total      int             `json:"total"`
}

// searchFields is the list of issue fields we ask Jira to return. Kept short
// because tickets are listed in dense table rows; the rest is fetched lazily
// on click.
const jiraFieldUpdated = "updated"

var searchFields = []string{
	"summary", "status", "project", "issuetype",
	"priority", "assignee", "reporter", jiraFieldUpdated,
}

// SearchTickets runs a JQL search and returns a page of tickets. The transport
// branches on instance type because Cloud and Server/DC expose different
// search endpoints with incompatible pagination shapes:
//
//   - Cloud: POST /rest/api/3/search/jql — token-paginated, no total count.
//     The legacy /search was removed by Atlassian in 2025.
//   - Server/DC: POST /rest/api/2/search — startAt/total style; we encode the
//     next startAt offset into the SearchResult.NextPageToken string so the
//     caller can opaquely round-trip it.
//
// pageToken is the cursor returned in the previous page's NextPageToken;
// pass "" for the first page. maxResults is capped at 100.
func (c *CloudClient) SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 25
	}
	if maxResults > 100 {
		maxResults = 100
	}
	if c.instanceType == InstanceTypeServer {
		return c.searchTicketsServer(ctx, jql, pageToken, maxResults)
	}
	return c.searchTicketsCloud(ctx, jql, pageToken, maxResults)
}

func (c *CloudClient) searchTicketsCloud(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	body := map[string]interface{}{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     searchFields,
	}
	if pageToken != "" {
		body["nextPageToken"] = pageToken
	}
	var resp cloudSearchResponse
	if err := c.do(ctx, http.MethodPost, c.apiBase+"/search/jql", body, &resp); err != nil {
		return nil, err
	}
	out := &SearchResult{
		MaxResults:    maxResults,
		IsLast:        resp.IsLast,
		NextPageToken: resp.NextPageToken,
		Tickets:       make([]JiraTicket, 0, len(resp.Issues)),
	}
	for i := range resp.Issues {
		out.Tickets = append(out.Tickets, issueToTicket(&resp.Issues[i], c.siteURL))
	}
	return out, nil
}

func (c *CloudClient) searchTicketsServer(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	startAt := 0
	if pageToken != "" {
		// A malformed token is fixed by starting over from the first page —
		// safer than 500-ing on what is ultimately UI-driven state.
		if n, err := strconv.Atoi(pageToken); err == nil && n > 0 {
			startAt = n
		}
	}
	body := map[string]interface{}{
		"jql":        jql,
		"startAt":    startAt,
		"maxResults": maxResults,
		"fields":     searchFields,
	}
	var resp serverSearchResponse
	if err := c.do(ctx, http.MethodPost, c.apiBase+"/search", body, &resp); err != nil {
		return nil, err
	}
	nextStart := resp.StartAt + len(resp.Issues)
	isLast := nextStart >= resp.Total || len(resp.Issues) == 0
	nextToken := ""
	if !isLast {
		nextToken = strconv.Itoa(nextStart)
	}
	out := &SearchResult{
		MaxResults:    maxResults,
		IsLast:        isLast,
		NextPageToken: nextToken,
		Tickets:       make([]JiraTicket, 0, len(resp.Issues)),
	}
	for i := range resp.Issues {
		out.Tickets = append(out.Tickets, issueToTicket(&resp.Issues[i], c.siteURL))
	}
	return out, nil
}

// issueToTicket converts the API response shape to our public JiraTicket.
// Factored out so GetTicket and SearchTickets stay consistent.
func issueToTicket(issue *issueResponse, siteURL string) JiraTicket {
	return JiraTicket{
		Key:            issue.Key,
		Summary:        issue.Fields.Summary,
		StatusID:       issue.Fields.Status.ID,
		StatusName:     issue.Fields.Status.Name,
		StatusCategory: issue.Fields.Status.StatusCategory.Key,
		ProjectKey:     issue.Fields.Project.Key,
		IssueType:      issue.Fields.IssueType.Name,
		IssueTypeIcon:  issue.Fields.IssueType.IconURL,
		Priority:       issue.Fields.Priority.Name,
		PriorityIcon:   issue.Fields.Priority.IconURL,
		AssigneeName:   issue.Fields.Assignee.name(),
		AssigneeAvatar: issue.Fields.Assignee.avatar(),
		ReporterName:   issue.Fields.Reporter.name(),
		ReporterAvatar: issue.Fields.Reporter.avatar(),
		Updated:        issue.Fields.Updated,
		URL:            siteURL + "/browse/" + issue.Key,
		Description:    extractDescription(issue.Fields.Description),
	}
}

// extractDescription handles the three shapes Jira's `description` field may
// return: nil (empty), a plain string (older APIs / some integrations), or an
// Atlassian Document Format node tree (API v3). For ADF we walk the tree
// pulling out text nodes so the UI gets a readable markdown-ish version.
func extractDescription(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return ""
	}
	var b strings.Builder
	walkADF(m, &b)
	return strings.TrimSpace(b.String())
}

// walkADF is a minimal Atlassian Document Format walker. It recognizes text
// nodes, hard/soft breaks, and paragraphs; other node types (code blocks,
// tables, mentions) collapse to their text content. Good enough for task
// descriptions — we don't try to preserve rich formatting.
func walkADF(node map[string]interface{}, b *strings.Builder) {
	switch node["type"] {
	case "text":
		if s, ok := node["text"].(string); ok {
			b.WriteString(s)
		}
	case "hardBreak", "softBreak":
		b.WriteString("\n")
	}
	content, ok := node["content"].([]interface{})
	if !ok {
		return
	}
	for i, c := range content {
		if child, ok := c.(map[string]interface{}); ok {
			walkADF(child, b)
		}
		// Insert paragraph breaks between top-level siblings.
		if t, _ := node["type"].(string); t == "doc" && i < len(content)-1 {
			b.WriteString("\n\n")
		}
	}
}
