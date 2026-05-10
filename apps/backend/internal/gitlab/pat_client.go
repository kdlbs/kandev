package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiPathPrefix    = "/api/v4"
	defaultPageSize  = 50
	maxPageSize      = 100
	requestTimeout   = 30 * time.Second
	defaultErrBytes  = 4096
	defaultPageStart = 1
)

// APIError represents an error response from the GitLab API.
type APIError struct {
	StatusCode int
	Endpoint   string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitLab API %s returned %d: %s", e.Endpoint, e.StatusCode, e.Body)
}

// PATClient implements Client against the GitLab REST v4 API using a
// personal access token (or any other token type sent via PRIVATE-TOKEN).
type PATClient struct {
	host       string
	token      string
	httpClient *http.Client
	username   string
}

// NewPATClient creates a new PAT-based GitLab client.
// host is the GitLab base URL (e.g. "https://gitlab.com" or
// "https://gitlab.acme.corp"); a trailing slash is trimmed. An empty host
// falls back to the public DefaultHost.
func NewPATClient(host, token string) *PATClient {
	if host == "" {
		host = DefaultHost
	}
	host = strings.TrimRight(host, "/")
	return &PATClient{
		host:       host,
		token:      token,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

func (c *PATClient) Host() string { return c.host }

func (c *PATClient) setHeaders(req *http.Request) {
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")
}

func (c *PATClient) IsAuthenticated(ctx context.Context) (bool, error) {
	_, err := c.GetAuthenticatedUser(ctx)
	return err == nil, nil
}

func (c *PATClient) GetAuthenticatedUser(ctx context.Context) (string, error) {
	if c.username != "" {
		return c.username, nil
	}
	var user struct {
		Username string `json:"username"`
	}
	if err := c.get(ctx, "/user", &user); err != nil {
		return "", err
	}
	c.username = user.Username
	return c.username, nil
}

// projectRef returns the URL-encoded project path used as :id in API URLs.
// GitLab requires the slash between namespace and path to be %2F-encoded —
// url.PathEscape leaves "/" alone, so do the substitution manually before
// URL-escaping the rest.
func projectRef(projectPath string) string {
	return strings.ReplaceAll(url.PathEscape(projectPath), "/", "%2F")
}

func (c *PATClient) GetMR(ctx context.Context, projectPath string, iid int) (*MR, error) {
	var raw rawMR
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d", projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("get MR !%d: %w", iid, err)
	}
	return convertRawMR(&raw), nil
}

func (c *PATClient) FindMRByBranch(ctx context.Context, projectPath, branch string) (*MR, error) {
	var raw []rawMR
	endpoint := fmt.Sprintf(
		"/projects/%s/merge_requests?source_branch=%s&state=opened&per_page=1",
		projectRef(projectPath), url.QueryEscape(branch),
	)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("find MR by branch: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return convertRawMR(&raw[0]), nil
}

func (c *PATClient) ListAuthoredMRs(ctx context.Context, projectPath string) ([]*MR, error) {
	user, err := c.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	var raw []rawMR
	endpoint := fmt.Sprintf(
		"/projects/%s/merge_requests?state=opened&author_username=%s&per_page=100",
		projectRef(projectPath), url.QueryEscape(user),
	)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list MRs: %w", err)
	}
	return convertRawMRSlice(raw), nil
}

func (c *PATClient) ListReviewRequestedMRs(ctx context.Context, filter, customQuery string) ([]*MR, error) {
	query := buildReviewMRQuery(filter, customQuery)
	endpoint := "/merge_requests?" + query
	var raw []rawMR
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list review-requested MRs: %w", err)
	}
	return convertRawMRSlice(raw), nil
}

func (c *PATClient) SearchMRs(ctx context.Context, filter, customQuery string) ([]*MR, error) {
	page, err := c.SearchMRsPaged(ctx, filter, customQuery, defaultPageStart, defaultPageSize)
	if err != nil {
		return nil, err
	}
	return page.MRs, nil
}

func (c *PATClient) SearchMRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildMRSearchQuery(filter, customQuery)
	endpoint := fmt.Sprintf("/merge_requests?%s&page=%d&per_page=%d", query, page, perPage)

	var raw []rawMR
	total, err := c.getWithTotal(ctx, endpoint, &raw)
	if err != nil {
		return nil, fmt.Errorf("search MRs: %w", err)
	}
	return &MRSearchPage{
		MRs:        convertRawMRSlice(raw),
		TotalCount: total,
		Page:       page,
		PerPage:    perPage,
	}, nil
}

func (c *PATClient) ListIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error) {
	page, err := c.ListIssuesPaged(ctx, filter, customQuery, defaultPageStart, defaultPageSize)
	if err != nil {
		return nil, err
	}
	return page.Issues, nil
}

func (c *PATClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildIssueSearchQuery(filter, customQuery)
	endpoint := fmt.Sprintf("/issues?%s&page=%d&per_page=%d", query, page, perPage)
	var raw []rawIssue
	total, err := c.getWithTotal(ctx, endpoint, &raw)
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}
	issues := make([]*Issue, len(raw))
	for i := range raw {
		issues[i] = convertRawIssue(&raw[i])
	}
	return &IssueSearchPage{
		Issues:     issues,
		TotalCount: total,
		Page:       page,
		PerPage:    perPage,
	}, nil
}

func (c *PATClient) GetIssueState(ctx context.Context, projectPath string, iid int) (string, error) {
	var result struct {
		State string `json:"state"`
	}
	endpoint := fmt.Sprintf("/projects/%s/issues/%d", projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return "", fmt.Errorf("get issue state: %w", err)
	}
	return result.State, nil
}

func (c *PATClient) ListUserGroups(ctx context.Context) ([]Group, error) {
	var raw []struct {
		ID        int64  `json:"id"`
		Path      string `json:"path"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := c.get(ctx, "/groups?per_page=100&min_access_level=10", &raw); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	groups := make([]Group, len(raw))
	for i, g := range raw {
		groups[i] = Group{ID: g.ID, Path: g.Path, Name: g.Name, AvatarURL: g.AvatarURL}
	}
	return groups, nil
}

func (c *PATClient) SearchGroupProjects(ctx context.Context, group, query string, limit int) ([]Project, error) {
	if limit <= 0 {
		limit = 20
	}
	endpoint := fmt.Sprintf("/groups/%s/projects?per_page=%d&simple=true&include_subgroups=true",
		url.PathEscape(group), limit)
	if query != "" {
		endpoint += "&search=" + url.QueryEscape(query)
	}
	var raw []rawProject
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list group projects: %w", err)
	}
	projects := make([]Project, len(raw))
	for i := range raw {
		projects[i] = convertRawProject(&raw[i])
	}
	return projects, nil
}

func (c *PATClient) ListMRApprovals(ctx context.Context, projectPath string, iid int) ([]MRApproval, error) {
	var raw struct {
		ApprovedBy []struct {
			User struct {
				Username  string `json:"username"`
				AvatarURL string `json:"avatar_url"`
			} `json:"user"`
		} `json:"approved_by"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approvals", projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	approvals := make([]MRApproval, 0, len(raw.ApprovedBy))
	for _, a := range raw.ApprovedBy {
		approvals = append(approvals, MRApproval{
			Username:  a.User.Username,
			Avatar:    a.User.AvatarURL,
			CreatedAt: raw.UpdatedAt,
		})
	}
	return approvals, nil
}

func (c *PATClient) ListMRDiscussions(ctx context.Context, projectPath string, iid int, since *time.Time) ([]MRDiscussion, error) {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions?per_page=100",
		projectRef(projectPath), iid)
	var raw []rawDiscussion
	for endpoint != "" {
		var page []rawDiscussion
		nextLink, err := c.getPaginated(ctx, endpoint, &page)
		if err != nil {
			return nil, fmt.Errorf("list discussions: %w", err)
		}
		raw = append(raw, page...)
		endpoint = nextLink
	}
	discussions := make([]MRDiscussion, 0, len(raw))
	for i := range raw {
		d := convertRawDiscussion(&raw[i])
		if since != nil && d.UpdatedAt.Before(*since) {
			continue
		}
		discussions = append(discussions, d)
	}
	return discussions, nil
}

func (c *PATClient) CreateMRDiscussionNote(ctx context.Context, projectPath string, iid int, discussionID, body string) (*MRNote, error) {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s/notes",
		projectRef(projectPath), iid, url.PathEscape(discussionID))
	payload := map[string]string{"body": body}
	var raw rawNote
	if err := c.postJSON(ctx, endpoint, payload, &raw); err != nil {
		return nil, fmt.Errorf("create discussion note: %w", err)
	}
	note := convertRawNote(&raw)
	return &note, nil
}

func (c *PATClient) ResolveMRDiscussion(ctx context.Context, projectPath string, iid int, discussionID string) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s?resolved=true",
		projectRef(projectPath), iid, url.PathEscape(discussionID))
	return c.put(ctx, endpoint, nil)
}

func (c *PATClient) ListPipelines(ctx context.Context, projectPath, ref string) ([]Pipeline, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines?per_page=20", projectRef(projectPath))
	if ref != "" {
		endpoint += "&ref=" + url.QueryEscape(ref)
	}
	var raw []rawPipeline
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}
	pipelines := make([]Pipeline, len(raw))
	for i := range raw {
		pipelines[i] = convertRawPipeline(&raw[i])
	}
	return pipelines, nil
}

func (c *PATClient) GetMRFeedback(ctx context.Context, projectPath string, iid int) (*MRFeedback, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	approvals, err := c.ListMRApprovals(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	discussions, err := c.ListMRDiscussions(ctx, projectPath, iid, nil)
	if err != nil {
		return nil, err
	}
	var pipelines []Pipeline
	if mr.HeadSHA != "" {
		pipelines, err = c.ListPipelines(ctx, projectPath, mr.HeadBranch)
		if err != nil {
			return nil, err
		}
	}
	return &MRFeedback{
		MR:          mr,
		Approvals:   approvals,
		Discussions: discussions,
		Pipelines:   pipelines,
		HasIssues:   hasOpenDiscussions(discussions) || pipelineFailing(pipelines),
	}, nil
}

func (c *PATClient) GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error) {
	mr, err := c.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	approvals, err := c.ListMRApprovals(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	required, err := c.getRequiredApprovals(ctx, projectPath, iid)
	if err != nil {
		return nil, err
	}
	pipelines, err := c.ListPipelines(ctx, projectPath, mr.HeadBranch)
	if err != nil {
		return nil, err
	}
	pipelineState, jobsTotal, jobsPassing := summarizePipelines(pipelines)
	approvalState := summarizeApprovals(len(approvals), required)
	return &MRStatus{
		MR:                  mr,
		ApprovalState:       approvalState,
		PipelineState:       pipelineState,
		MergeStatus:         mr.MergeStatus,
		ApprovalCount:       len(approvals),
		RequiredApprovals:   required,
		PipelineJobsTotal:   jobsTotal,
		PipelineJobsPassing: jobsPassing,
	}, nil
}

func (c *PATClient) ListMRFiles(ctx context.Context, projectPath string, iid int) ([]MRFile, error) {
	var raw struct {
		Changes []struct {
			OldPath     string `json:"old_path"`
			NewPath     string `json:"new_path"`
			NewFile     bool   `json:"new_file"`
			DeletedFile bool   `json:"deleted_file"`
			RenamedFile bool   `json:"renamed_file"`
			Diff        string `json:"diff"`
		} `json:"changes"`
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/changes",
		projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list MR changes: %w", err)
	}
	files := make([]MRFile, len(raw.Changes))
	for i, ch := range raw.Changes {
		additions, deletions := countDiffLines(ch.Diff)
		files[i] = MRFile{
			Filename:  ch.NewPath,
			Status:    diffStatus(ch.NewFile, ch.DeletedFile, ch.RenamedFile),
			Additions: additions,
			Deletions: deletions,
			Patch:     ch.Diff,
			OldPath:   ch.OldPath,
		}
	}
	return files, nil
}

func (c *PATClient) ListMRCommits(ctx context.Context, projectPath string, iid int) ([]MRCommitInfo, error) {
	var raw []struct {
		ID           string `json:"id"`
		Message      string `json:"message"`
		AuthorName   string `json:"author_name"`
		AuthoredDate string `json:"authored_date"`
		Stats        struct {
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
			Total     int `json:"total"`
		} `json:"stats"`
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/commits?per_page=100",
		projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list MR commits: %w", err)
	}
	commits := make([]MRCommitInfo, len(raw))
	for i, r := range raw {
		commits[i] = MRCommitInfo{
			SHA:            r.ID,
			Message:        r.Message,
			AuthorUsername: r.AuthorName,
			AuthorDate:     r.AuthoredDate,
			Additions:      r.Stats.Additions,
			Deletions:      r.Stats.Deletions,
		}
	}
	return commits, nil
}

func (c *PATClient) SubmitMRApproval(ctx context.Context, projectPath string, iid int) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approve",
		projectRef(projectPath), iid)
	return c.post(ctx, endpoint, nil)
}

func (c *PATClient) SubmitMRUnapproval(ctx context.Context, projectPath string, iid int) error {
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/unapprove",
		projectRef(projectPath), iid)
	return c.post(ctx, endpoint, nil)
}

func (c *PATClient) CreateMR(ctx context.Context, projectPath, sourceBranch, targetBranch, title, description string, draft bool) (*MR, error) {
	finalTitle := title
	if draft && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(finalTitle)), "draft:") {
		finalTitle = "Draft: " + finalTitle
	}
	payload := map[string]any{
		"source_branch":        sourceBranch,
		"target_branch":        targetBranch,
		"title":                finalTitle,
		"description":          description,
		"remove_source_branch": true,
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests", projectRef(projectPath))
	var raw rawMR
	if err := c.postJSON(ctx, endpoint, payload, &raw); err != nil {
		return nil, fmt.Errorf("create MR: %w", err)
	}
	return convertRawMR(&raw), nil
}

func (c *PATClient) ListProjectBranches(ctx context.Context, projectPath string) ([]RepoBranch, error) {
	endpoint := fmt.Sprintf("/projects/%s/repository/branches?per_page=100", projectRef(projectPath))
	var branches []RepoBranch
	for endpoint != "" {
		var page []struct {
			Name string `json:"name"`
		}
		nextLink, err := c.getPaginated(ctx, endpoint, &page)
		if err != nil {
			return nil, fmt.Errorf("list branches: %w", err)
		}
		for _, b := range page {
			branches = append(branches, RepoBranch{Name: b.Name})
		}
		endpoint = nextLink
	}
	return branches, nil
}

func (c *PATClient) getRequiredApprovals(ctx context.Context, projectPath string, iid int) (int, error) {
	var raw struct {
		ApprovalsRequired int `json:"approvals_required"`
	}
	endpoint := fmt.Sprintf("/projects/%s/merge_requests/%d/approvals", projectRef(projectPath), iid)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return 0, err
	}
	return raw.ApprovalsRequired, nil
}

// --- HTTP plumbing ---

func (c *PATClient) get(ctx context.Context, endpoint string, result any) error {
	_, err := c.getWithTotal(ctx, endpoint, result)
	return err
}

func (c *PATClient) getWithTotal(ctx context.Context, endpoint string, result any) (int, error) {
	u := c.host + apiPathPrefix + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrBytes))
		return 0, &APIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	total := 0
	if t := resp.Header.Get("X-Total"); t != "" {
		if n, convErr := strconv.Atoi(t); convErr == nil {
			total = n
		}
	}
	if result == nil {
		return total, nil
	}
	return total, json.NewDecoder(resp.Body).Decode(result)
}

func (c *PATClient) getPaginated(ctx context.Context, endpoint string, result any) (string, error) {
	u := c.host + apiPathPrefix + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrBytes))
		return "", &APIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return "", err
	}
	return parseNextLink(resp.Header.Get("Link"), c.host+apiPathPrefix), nil
}

func (c *PATClient) post(ctx context.Context, endpoint string, body []byte) error {
	return c.doWrite(ctx, http.MethodPost, endpoint, body, nil)
}

func (c *PATClient) put(ctx context.Context, endpoint string, body []byte) error {
	return c.doWrite(ctx, http.MethodPut, endpoint, body, nil)
}

func (c *PATClient) postJSON(ctx context.Context, endpoint string, payload, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return c.doWrite(ctx, http.MethodPost, endpoint, body, result)
}

func (c *PATClient) doWrite(ctx context.Context, method, endpoint string, body []byte, result any) error {
	u := c.host + apiPathPrefix + endpoint
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, defaultErrBytes))
		return &APIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(respBody)}
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// parseNextLink extracts the "next" URL path+query from a GitLab Link header.
// Strips the API base prefix so the caller can pass the result back to get().
func parseNextLink(header, apiBase string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		link := part[start+1 : end]
		if strings.HasPrefix(link, apiBase) {
			return link[len(apiBase):]
		}
		return link
	}
	return ""
}

func clampSearchPage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = defaultPageSize
	}
	if perPage > maxPageSize {
		perPage = maxPageSize
	}
	return page, perPage
}
