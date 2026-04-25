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

// CloudClient is an Atlassian Cloud REST v3 client. It supports two auth modes:
//
//   - api_token: Basic auth with {email}:{token} (the standard, recommended path).
//   - session_cookie: the secret is the JWT value of `cloud.session.token`
//     (password accounts) or `tenant.session.token` (SSO), copied from DevTools
//     → Application → Cookies. The client wraps it under both cookie names so
//     a single paste works for both account types.
//
// The client holds no state beyond credentials so it can be recreated cheaply
// per workspace if config changes.
type CloudClient struct {
	http        *http.Client
	siteURL     string
	email       string
	secret      string
	authMethod  string
	maxBodySize int64
}

// NewCloudClient builds a client from a JiraConfig + secret. siteURL is
// normalized: trailing slash stripped, https:// prepended when the user saved
// only a hostname (legacy rows; new rows are normalized on save).
func NewCloudClient(cfg *JiraConfig, secret string) *CloudClient {
	site := strings.TrimRight(cfg.SiteURL, "/")
	if site != "" && !strings.Contains(site, "://") {
		site = "https://" + site
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
		siteURL:     site,
		email:       cfg.Email,
		secret:      secret,
		authMethod:  cfg.AuthMethod,
		maxBodySize: 4 << 20, // 4 MB — Jira payloads are small by design.
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
		return &APIError{StatusCode: resp.StatusCode, Message: summarizeBody(resp, raw)}
	}
	// Guardrail: some misconfigured Atlassian flows return a 200 HTML login
	// page instead of JSON. If we accidentally get HTML, surface an auth error
	// rather than letting json.Unmarshal fail with "invalid character '<'".
	if isHTMLResponse(resp, raw) {
		return &APIError{StatusCode: resp.StatusCode, Message: "Atlassian returned an HTML page — the session likely requires step-up auth or has expired. Refresh your Jira tab and copy the cookie again."}
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

// summarizeBody returns a short, useful error message from a non-2xx response.
// For HTML bodies it skips the page content in favor of a step-up hint; for
// plain text or JSON it returns the body verbatim (capped, so we don't spam
// logs with multi-KB pages).
func summarizeBody(resp *http.Response, raw []byte) string {
	if isHTMLResponse(resp, raw) {
		return "Atlassian returned an HTML page (status " + strconv.Itoa(resp.StatusCode) +
			"). This usually means step-up authentication is required; sign in again in your Jira tab and re-copy the cookie."
	}
	const maxMsg = 500
	if len(raw) > maxMsg {
		return string(raw[:maxMsg]) + "…"
	}
	return string(raw)
}

// TestAuth hits /rest/api/3/myself which is the cheapest authenticated
// endpoint; a 200 proves credentials work and identifies the user.
func (c *CloudClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	var body struct {
		AccountID    string `json:"accountId"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/myself", nil, &body); err != nil {
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
	path := "/rest/api/3/issue/" + url.PathEscape(ticketKey) + "?expand=renderedFields"
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
	path := "/rest/api/3/issue/" + url.PathEscape(ticketKey) + "/transitions"
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
	path := "/rest/api/3/issue/" + url.PathEscape(ticketKey) + "/transitions"
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
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/project/search?maxResults=200", nil, &body); err != nil {
		return nil, err
	}
	out := make([]JiraProject, 0, len(body.Values))
	for _, p := range body.Values {
		out = append(out, JiraProject{ID: p.ID, Key: p.Key, Name: p.Name})
	}
	return out, nil
}

// searchResponse mirrors the subset of /rest/api/3/search/jql we consume. The
// new endpoint is token-paginated: there's no total count and pagination uses
// nextPageToken rather than startAt. Transitions are intentionally omitted from
// search results (fetched lazily when the user opens a ticket).
type searchResponse struct {
	Issues        []issueResponse `json:"issues"`
	NextPageToken string          `json:"nextPageToken"`
	IsLast        bool            `json:"isLast"`
}

// SearchTickets runs a JQL search and returns a page of tickets. Uses the
// /rest/api/3/search/jql endpoint (the legacy /search was removed by Atlassian
// in 2025). pageToken is the cursor returned in the previous page's
// NextPageToken; pass "" for the first page. maxResults is capped at 100.
func (c *CloudClient) SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 25
	}
	if maxResults > 100 {
		maxResults = 100
	}
	body := map[string]interface{}{
		"jql":        jql,
		"maxResults": maxResults,
		"fields": []string{
			"summary", "status", "project", "issuetype",
			"priority", "assignee", "reporter", "updated",
		},
	}
	if pageToken != "" {
		body["nextPageToken"] = pageToken
	}
	var resp searchResponse
	if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &resp); err != nil {
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
