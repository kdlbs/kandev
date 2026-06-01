package sentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// SentryAPIEndpoint is the SaaS API base. Phase 1 targets sentry.io only;
// self-hosted instances will need a per-config base-URL field later.
const SentryAPIEndpoint = "https://sentry.io/api/0"

const userAgent = "kandev/1.0 (+https://github.com/kdlbs/kandev)"

// RESTClient talks to Sentry's REST API using a Bearer auth token.
type RESTClient struct {
	http        *http.Client
	endpoint    string
	token       string
	maxBodySize int64
}

// NewRESTClient builds a client from a SentryConfig + secret. The config is
// accepted for symmetry with other integrations; only the token is read today.
func NewRESTClient(_ *SentryConfig, secret string) *RESTClient {
	return &RESTClient{
		http:        &http.Client{Timeout: 30 * time.Second},
		endpoint:    SentryAPIEndpoint,
		token:       secret,
		maxBodySize: 8 << 20, // 8 MB — issue payloads can include event samples.
	}
}

// do executes a GET against path (relative to endpoint), parses query params,
// and decodes the JSON body into out. Returns the response so callers can read
// headers (e.g. Link for pagination).
func (c *RESTClient) do(ctx context.Context, path string, query url.Values, out interface{}) (*http.Response, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodySize))
	if err != nil {
		return resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, &APIError{StatusCode: resp.StatusCode, Message: summarizeBody(raw)}
	}
	if out == nil {
		return resp, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp, &APIError{StatusCode: resp.StatusCode, Message: "invalid JSON response: " + err.Error()}
	}
	return resp, nil
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

// --- whoami / TestAuth ---

type whoamiResponse struct {
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"user"`
}

// TestAuth hits the root API endpoint, which returns the authenticated user
// when the token is valid. Mirrors Sentry's official "verify your token"
// guidance.
func (c *RESTClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	var data whoamiResponse
	if _, err := c.do(ctx, "/", nil, &data); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return &TestConnectionResult{OK: false, Error: apiErr.Error()}, nil
		}
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	name := data.User.Name
	if name == "" {
		name = data.User.Username
	}
	return &TestConnectionResult{
		OK:          true,
		UserID:      data.User.ID,
		DisplayName: name,
		Email:       data.User.Email,
	}, nil
}

// --- organizations ---

// listOrganizationsMaxPages bounds the pagination loop so a misbehaving API or
// an enormous account can't spin forever. 100 pages × 100/page = 10k orgs.
const listOrganizationsMaxPages = 100

// ListOrganizations returns all organizations the token can access, following
// Sentry's Link-header cursor pagination (the endpoint returns ~100 per page).
// The settings dropdown uses these to populate the default-org selector.
// SentryOrganization's json tags match the API payload, so the response decodes
// straight into it.
func (c *RESTClient) ListOrganizations(ctx context.Context) ([]SentryOrganization, error) {
	out := make([]SentryOrganization, 0, 32)
	cursor := ""
	for page := 0; page < listOrganizationsMaxPages; page++ {
		q := url.Values{}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var nodes []SentryOrganization
		resp, err := c.do(ctx, "/organizations/", q, &nodes)
		if err != nil {
			return nil, err
		}
		out = append(out, nodes...)
		next, hasNext := parseNextCursor(resp.Header.Get("Link"))
		if !hasNext {
			break
		}
		cursor = next
	}
	return out, nil
}

// --- projects ---

type projectNode struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Organization struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"organization"`
}

// listProjectsMaxPages bounds the pagination loop so a misbehaving API or an
// enormous account can't spin forever. 100 pages × 100/page = 10k projects.
const listProjectsMaxPages = 100

// ListProjects returns all projects the token can access, across every
// organization. It enumerates the token's organizations first, then lists each
// org's projects via the org-scoped /organizations/{org}/projects/ endpoint.
//
// The user-scoped /projects/ endpoint only returns projects whose teams the
// token's user belongs to, so an org owner who is not a member of any team sees
// an empty list there. The org-scoped endpoint returns every project in the
// org, which is what the settings dropdown needs. The dropdown then filters
// client-side by DefaultOrgSlug.
func (c *RESTClient) ListProjects(ctx context.Context) ([]SentryProject, error) {
	orgs, err := c.ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SentryProject, 0, 32)
	for i := range orgs {
		orgSlug := orgs[i].Slug
		if orgSlug == "" {
			continue
		}
		projects, err := c.listOrgProjects(ctx, orgSlug)
		if err != nil {
			return nil, err
		}
		out = append(out, projects...)
	}
	return out, nil
}

// listOrgProjects pages through one organization's projects via the org-scoped
// endpoint, following Sentry's Link-header cursor pagination (~100 per page).
func (c *RESTClient) listOrgProjects(ctx context.Context, orgSlug string) ([]SentryProject, error) {
	out := make([]SentryProject, 0, 32)
	path := "/organizations/" + url.PathEscape(orgSlug) + "/projects/"
	cursor := ""
	for page := 0; page < listProjectsMaxPages; page++ {
		q := url.Values{}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var nodes []projectNode
		resp, err := c.do(ctx, path, q, &nodes)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			// The org-scoped endpoint may omit the nested organization object;
			// fall back to the org slug we queried so OrgSlug is never empty.
			projOrg := n.Organization.Slug
			if projOrg == "" {
				projOrg = orgSlug
			}
			out = append(out, SentryProject{
				ID:      n.ID,
				Slug:    n.Slug,
				Name:    n.Name,
				OrgSlug: projOrg,
			})
		}
		next, hasNext := parseNextCursor(resp.Header.Get("Link"))
		if !hasNext {
			break
		}
		cursor = next
	}
	return out, nil
}

// --- issues ---

type issueNode struct {
	ID        string `json:"id"`
	ShortID   string `json:"shortId"`
	Title     string `json:"title"`
	Culprit   string `json:"culprit"`
	Permalink string `json:"permalink"`
	Level     string `json:"level"`
	Status    string `json:"status"`
	Count     string `json:"count"`
	UserCount int    `json:"userCount"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	Project   struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"project"`
	AssignedTo *struct {
		Name string `json:"name"`
	} `json:"assignedTo"`
}

func issueNodeToIssue(n *issueNode) SentryIssue {
	out := SentryIssue{
		ID:          n.ID,
		ShortID:     n.ShortID,
		Title:       n.Title,
		Culprit:     n.Culprit,
		Permalink:   n.Permalink,
		ProjectSlug: n.Project.Slug,
		ProjectName: n.Project.Name,
		Level:       n.Level,
		Status:      n.Status,
		Count:       n.Count,
		UserCount:   n.UserCount,
		FirstSeen:   n.FirstSeen,
		LastSeen:    n.LastSeen,
	}
	if n.AssignedTo != nil {
		out.AssigneeName = n.AssignedTo.Name
	}
	return out
}

// SearchIssues runs a filtered issue search scoped to filter.OrgSlug. cursor
// is the opaque token returned in the previous page's NextPageToken; pass ""
// for the first page.
//
// Results are always sorted by first-seen date descending (sort=new) so that
// newly created issues appear on page one. The issue watcher relies on this to
// detect new issues in a single bounded page read per poll tick.
func (c *RESTClient) SearchIssues(ctx context.Context, filter SearchFilter, cursor string) (*SearchResult, error) {
	if filter.OrgSlug == "" {
		return nil, &APIError{StatusCode: http.StatusBadRequest, Message: "orgSlug required"}
	}
	q := url.Values{}
	// Sort by first-seen descending so the newest issues land on page one.
	// The issue watcher reads only a single page per tick; this ensures newly
	// created issues are not buried behind older frequently-occurring ones.
	q.Set("sort", "new")
	if filter.Environment != "" {
		q.Set("environment", filter.Environment)
	}
	if filter.StatsPeriod != "" {
		q.Set("statsPeriod", filter.StatsPeriod)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if built := buildIssueQueryString(filter); built != "" {
		q.Set("query", built)
	}
	var nodes []issueNode
	resp, err := c.do(ctx, issuesSearchPath(filter.OrgSlug, filter.ProjectSlug), q, &nodes)
	if err != nil {
		return nil, err
	}
	next, hasNext := parseNextCursor(resp.Header.Get("Link"))
	out := &SearchResult{
		Issues:        make([]SentryIssue, 0, len(nodes)),
		NextPageToken: next,
		IsLast:        !hasNext,
	}
	for i := range nodes {
		out.Issues = append(out.Issues, issueNodeToIssue(&nodes[i]))
	}
	return out, nil
}

// issuesSearchPath picks the issue-search endpoint. The org-scoped
// /organizations/{org}/issues/ endpoint requires a NUMERIC project id in
// ?project=, so when a project slug is set we use the project-scoped
// endpoint, which takes the slug directly in the path. Empty project slug
// (browse "all projects" in an org) falls back to the org endpoint.
func issuesSearchPath(orgSlug, projectSlug string) string {
	if projectSlug == "" {
		return "/organizations/" + url.PathEscape(orgSlug) + "/issues/"
	}
	return "/projects/" + url.PathEscape(orgSlug) + "/" + url.PathEscape(projectSlug) + "/issues/"
}

// buildIssueQueryString assembles Sentry's search-bar syntax from a
// SearchFilter. Levels and statuses are encoded as repeated `level:foo` /
// `is:bar` tokens — Sentry treats repeated tokens of the same key as an OR.
func buildIssueQueryString(f SearchFilter) string {
	parts := make([]string, 0, 4)
	for _, lvl := range f.Levels {
		lvl = strings.TrimSpace(lvl)
		if lvl == "" {
			continue
		}
		parts = append(parts, "level:"+lvl)
	}
	for _, st := range f.Statuses {
		st = strings.TrimSpace(st)
		if st == "" {
			continue
		}
		parts = append(parts, "is:"+st)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		parts = append(parts, q)
	}
	return strings.Join(parts, " ")
}

// nextCursorRe extracts the cursor from a Sentry Link header entry of the form
// `<url>; rel="next"; results="true"; cursor="0:100:0"`. The header may carry
// both `previous` and `next` entries separated by commas.
var nextCursorRe = regexp.MustCompile(`rel="next"[^,]*?cursor="([^"]+)"[^,]*?results="([^"]+)"|rel="next"[^,]*?results="([^"]+)"[^,]*?cursor="([^"]+)"`)

// parseNextCursor returns (cursor, hasNext). hasNext is true only when the
// `next` entry's `results` flag is "true"; Sentry always sends a `next` link
// but flips `results` to "false" on the final page.
func parseNextCursor(link string) (string, bool) {
	if link == "" {
		return "", false
	}
	m := nextCursorRe.FindStringSubmatch(link)
	if m == nil {
		return "", false
	}
	var cursor, results string
	switch {
	case m[1] != "":
		cursor, results = m[1], m[2]
	default:
		cursor, results = m[4], m[3]
	}
	if results != "true" {
		return "", false
	}
	return cursor, true
}

// GetIssue loads a single issue by numeric ID or short ID (e.g. "PROJ-123").
func (c *RESTClient) GetIssue(ctx context.Context, idOrShortID string) (*SentryIssue, error) {
	if idOrShortID == "" {
		return nil, fmt.Errorf("idOrShortID required")
	}
	var n issueNode
	if _, err := c.do(ctx, "/issues/"+url.PathEscape(idOrShortID)+"/", nil, &n); err != nil {
		return nil, err
	}
	issue := issueNodeToIssue(&n)
	return &issue, nil
}
