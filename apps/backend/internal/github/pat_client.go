package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubAPIBase    = "https://api.github.com"
	githubAccept     = "application/vnd.github+json"
	githubAPIVersion = "2022-11-28"
)

// GitHubAPIError represents an error response from the GitHub API with a status code.
type GitHubAPIError struct {
	StatusCode int
	Endpoint   string
	Body       string
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub API %s returned %d: %s", e.Endpoint, e.StatusCode, e.Body)
}

// PATClient implements Client using a GitHub Personal Access Token.
type PATClient struct {
	token       string
	httpClient  *http.Client
	username    string // cached after first GetAuthenticatedUser call
	rateTracker *RateTracker
}

// NewPATClient creates a new PAT-based GitHub client.
func NewPATClient(token string) *PATClient {
	return &PATClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithRateTracker attaches a rate tracker so response headers are recorded.
// Returns the client for chaining; safe to call before any requests are made.
func (c *PATClient) WithRateTracker(t *RateTracker) *PATClient {
	c.rateTracker = t
	return c
}

// recordRateHeaders feeds rate-limit data from a response into the tracker.
// endpoint is used to pick the default resource bucket when the response
// omits the X-RateLimit-Resource header.
func (c *PATClient) recordRateHeaders(resp *http.Response, endpoint string) {
	if c.rateTracker == nil || resp == nil {
		return
	}
	defaultResource := ResourceCore
	if strings.HasPrefix(endpoint, "/search/") {
		defaultResource = ResourceSearch
	} else if strings.HasPrefix(endpoint, "/graphql") {
		defaultResource = ResourceGraphQL
	}
	snap, headersOK := parseRateHeaders(resp, defaultResource)
	if headersOK {
		c.rateTracker.Record(snap)
	}
	if !isRateLimitStatus(resp.StatusCode) {
		return
	}
	// On a 429, prefer the real X-RateLimit-Reset over the synthetic 1h
	// fallback. parseRateHeaders may return ok=true with Remaining>0 if the
	// secondary-limit response carried stale headers — only skip the
	// fallback when the headers themselves report exhaustion (Remaining<=0
	// + future ResetAt). Otherwise fall through to the conservative pause.
	if headersOK && snap.Exhausted() {
		return
	}
	c.rateTracker.markRateExhausted(defaultResource, time.Time{})
}

// isRateLimitStatus returns true for status codes GitHub uses to signal
// primary or secondary rate-limit exhaustion. 403 is documented for both
// abuse-detection and primary limits when the body indicates so; 429 is
// secondary limits.
func isRateLimitStatus(status int) bool {
	return status == http.StatusTooManyRequests
}

// maybeMarkRateExhaustedFromBody flags a rate-limit hit when GitHub returned
// a 403/429 whose body contains the rate-limit prose. The headers may be
// missing on these responses (esp. secondary limits), so the body is the
// authoritative signal.
func (c *PATClient) maybeMarkRateExhaustedFromBody(endpoint string, status int, body []byte) {
	if c.rateTracker == nil {
		return
	}
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "rate limit") && !strings.Contains(lower, "abuse detection") {
		return
	}
	resource := ResourceCore
	if strings.HasPrefix(endpoint, "/search/") {
		resource = ResourceSearch
	} else if strings.HasPrefix(endpoint, "/graphql") {
		resource = ResourceGraphQL
	}
	// If recordRateHeaders already captured a real reset for this bucket on
	// the same response, don't clobber it with the synthetic 1h fallback.
	if existing, ok := c.rateTracker.Snapshot(resource); ok && existing.Exhausted() {
		return
	}
	c.rateTracker.markRateExhausted(resource, time.Time{})
}

// setGitHubHeaders sets the common Authorization, Accept, and API version headers.
func (c *PATClient) setGitHubHeaders(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
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
		Login string `json:"login"`
	}
	if err := c.get(ctx, "/user", &user); err != nil {
		return "", err
	}
	c.username = user.Login
	return c.username, nil
}

func (c *PATClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	var raw patPR
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}
	return convertPatPR(&raw, owner, repo), nil
}

func (c *PATClient) FindPRByBranch(ctx context.Context, owner, repo, branch string) (*PR, error) {
	var raw []patPR
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?head=%s:%s&state=open&per_page=1",
		owner, repo, owner, branch)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("find PR by branch: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return convertPatPR(&raw[0], owner, repo), nil
}

func (c *PATClient) ListAuthoredPRs(ctx context.Context, owner, repo string) ([]*PR, error) {
	user, err := c.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	var raw []patPR
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=open&per_page=100", owner, repo)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list PRs: %w", err)
	}
	var result []*PR
	for i := range raw {
		if raw[i].User.Login == user {
			result = append(result, convertPatPR(&raw[i], owner, repo))
		}
	}
	return result, nil
}

func (c *PATClient) ListReviewRequestedPRs(ctx context.Context, scope, filter, customQuery string) ([]*PR, error) {
	query := buildReviewSearchQuery(scope, filter, customQuery)
	var result struct {
		Items []patSearchItem `json:"items"`
	}
	endpoint := "/search/issues?q=" + url.QueryEscape(query) + "&per_page=50"
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("search review-requested: %w", err)
	}
	prs := make([]*PR, len(result.Items))
	for i, item := range result.Items {
		prs[i] = convertSearchItemToPR(
			item.Number, item.Title, item.HTMLURL, item.State,
			item.User.Login, item.RepositoryURL, item.Draft,
			item.CreatedAt, item.UpdatedAt,
		)
	}
	return prs, nil
}

func (c *PATClient) SearchPRs(ctx context.Context, filter, customQuery string) ([]*PR, error) {
	page, err := c.SearchPRsPaged(ctx, filter, customQuery, 1, 50)
	if err != nil {
		return nil, err
	}
	return page.PRs, nil
}

func (c *PATClient) SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildPRSearchQuery(filter, customQuery)
	var result struct {
		TotalCount int             `json:"total_count"`
		Items      []patSearchItem `json:"items"`
	}
	endpoint := fmt.Sprintf("/search/issues?q=%s&per_page=%d&page=%d",
		url.QueryEscape(query), perPage, page)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("search PRs: %w", err)
	}
	prs := make([]*PR, len(result.Items))
	for i, item := range result.Items {
		prs[i] = convertSearchItemToPR(
			item.Number, item.Title, item.HTMLURL, item.State,
			item.User.Login, item.RepositoryURL, item.Draft,
			item.CreatedAt, item.UpdatedAt,
		)
	}
	return &PRSearchPage{PRs: prs, TotalCount: result.TotalCount, Page: page, PerPage: perPage}, nil
}

func (c *PATClient) ListIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error) {
	page, err := c.ListIssuesPaged(ctx, filter, customQuery, 1, 50)
	if err != nil {
		return nil, err
	}
	return page.Issues, nil
}

func (c *PATClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildIssueSearchQuery(filter, customQuery)
	var result struct {
		TotalCount int               `json:"total_count"`
		Items      []issueSearchItem `json:"items"`
	}
	endpoint := fmt.Sprintf("/search/issues?q=%s&per_page=%d&page=%d",
		url.QueryEscape(query), perPage, page)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}
	issues := parseIssueSearchResults(result.Items)
	return &IssueSearchPage{Issues: issues, TotalCount: result.TotalCount, Page: page, PerPage: perPage}, nil
}

func (c *PATClient) GetIssueState(ctx context.Context, owner, repo string, number int) (string, error) {
	var result struct {
		State string `json:"state"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return "", fmt.Errorf("get issue state: %w", err)
	}
	return result.State, nil
}

func (c *PATClient) ListUserOrgs(ctx context.Context) ([]GitHubOrg, error) {
	var raw []struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := c.get(ctx, "/user/orgs?per_page=100", &raw); err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	orgs := make([]GitHubOrg, len(raw))
	for i, r := range raw {
		orgs[i] = GitHubOrg{Login: r.Login, AvatarURL: r.AvatarURL}
	}
	return orgs, nil
}

func (c *PATClient) SearchOrgRepos(ctx context.Context, org, query string, limit int) ([]GitHubRepo, error) {
	q := "org:" + org
	if query != "" {
		q += " " + query
	}
	if limit <= 0 {
		limit = 20
	}
	var result struct {
		Items []struct {
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
			Name    string `json:"name"`
			Private bool   `json:"private"`
		} `json:"items"`
	}
	endpoint := fmt.Sprintf("/search/repositories?q=%s&per_page=%d", url.QueryEscape(q), limit)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("search org repos: %w", err)
	}
	repos := make([]GitHubRepo, len(result.Items))
	for i, item := range result.Items {
		repos[i] = GitHubRepo{
			FullName: item.FullName,
			Owner:    item.Owner.Login,
			Name:     item.Name,
			Private:  item.Private,
		}
	}
	return repos, nil
}

func (c *PATClient) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error) {
	var raw []struct {
		ID          int64     `json:"id"`
		State       string    `json:"state"`
		Body        string    `json:"body"`
		SubmittedAt time.Time `json:"submitted_at"`
		User        struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, number)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	reviews := make([]PRReview, len(raw))
	for i, r := range raw {
		reviews[i] = PRReview{
			ID:           r.ID,
			Author:       r.User.Login,
			AuthorAvatar: r.User.AvatarURL,
			State:        r.State,
			Body:         r.Body,
			CreatedAt:    r.SubmittedAt,
		}
	}
	return reviews, nil
}

func (c *PATClient) ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, number)
	if since != nil {
		endpoint += "&since=" + url.QueryEscape(since.Format(time.RFC3339))
	}
	var reviewRaw []ghComment
	if err := c.get(ctx, endpoint, &reviewRaw); err != nil {
		return nil, err
	}
	issueEndpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", owner, repo, number)
	if since != nil {
		issueEndpoint += "&since=" + url.QueryEscape(since.Format(time.RFC3339))
	}
	var issueRaw []ghIssueComment
	if err := c.get(ctx, issueEndpoint, &issueRaw); err != nil {
		return nil, err
	}
	return mergeAndSortPRComments(convertRawComments(reviewRaw), convertRawIssueComments(issueRaw)), nil
}

func (c *PATClient) ListCheckRuns(ctx context.Context, owner, repo, ref string) ([]CheckRun, error) {
	var checkRunsResult struct {
		CheckRuns []ghCheckRun `json:"check_runs"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", owner, repo, ref)
	if err := c.get(ctx, endpoint, &checkRunsResult); err != nil {
		return nil, err
	}
	var statusResult struct {
		Statuses []ghStatusContext `json:"statuses"`
	}
	statusEndpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/status", owner, repo, ref)
	if err := c.get(ctx, statusEndpoint, &statusResult); err != nil {
		return nil, err
	}
	return mergeChecks(
		convertRawCheckRuns(checkRunsResult.CheckRuns),
		convertRawStatusContexts(statusResult.Statuses),
	), nil
}

func (c *PATClient) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	return getPRFeedback(ctx, c, owner, repo, number)
}

func (c *PATClient) GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error) {
	return getPRStatus(ctx, c, owner, repo, number)
}

func (c *PATClient) ListPRFiles(ctx context.Context, owner, repo string, number int) ([]PRFile, error) {
	var raw []ghPRFile
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=100", owner, repo, number)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list PR files: %w", err)
	}
	return convertRawPRFiles(raw), nil
}

func (c *PATClient) ListPRCommits(ctx context.Context, owner, repo string, number int) ([]PRCommitInfo, error) {
	var raw []ghPRCommit
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?per_page=100", owner, repo, number)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("list PR commits: %w", err)
	}
	return convertRawPRCommits(raw), nil
}

func (c *PATClient) ListRepoBranches(ctx context.Context, owner, repo string) ([]RepoBranch, error) {
	var branches []RepoBranch
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?per_page=100", owner, repo)
	for endpoint != "" {
		var page []struct {
			Name string `json:"name"`
		}
		nextLink, err := c.getPaginated(ctx, endpoint, &page)
		if err != nil {
			return nil, fmt.Errorf("list repo branches: %w", err)
		}
		for _, b := range page {
			branches = append(branches, RepoBranch{Name: b.Name})
		}
		endpoint = nextLink
	}
	return branches, nil
}

func (c *PATClient) SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, number)
	payload := map[string]string{"event": event}
	if body != "" {
		payload["body"] = body
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal review payload: %w", err)
	}
	return c.post(ctx, endpoint, jsonBody)
}

func (c *PATClient) post(ctx context.Context, endpoint string, body []byte) error {
	u := githubAPIBase + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setGitHubHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request POST %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.recordRateHeaders(resp, endpoint)

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		c.maybeMarkRateExhaustedFromBody(endpoint, resp.StatusCode, respBody)
		return fmt.Errorf("GitHub API POST %s returned %d: %s", endpoint, resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *PATClient) get(ctx context.Context, endpoint string, result interface{}) error {
	url := githubAPIBase + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.setGitHubHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.recordRateHeaders(resp, endpoint)

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		c.maybeMarkRateExhaustedFromBody(endpoint, resp.StatusCode, body)
		return &GitHubAPIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// getPaginated is like get but also returns the "next" link from the Link header, if any.
func (c *PATClient) getPaginated(ctx context.Context, endpoint string, result interface{}) (string, error) {
	u := githubAPIBase + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.setGitHubHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.recordRateHeaders(resp, endpoint)

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		c.maybeMarkRateExhaustedFromBody(endpoint, resp.StatusCode, body)
		return "", &GitHubAPIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return "", err
	}
	return parseNextLink(resp.Header.Get("Link")), nil
}

// parseNextLink extracts the "next" URL path+query from a GitHub Link header.
func parseNextLink(header string) string {
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
		// Strip the base URL to return just the path+query for our get method
		if strings.HasPrefix(link, githubAPIBase) {
			return link[len(githubAPIBase):]
		}
		return link
	}
	return ""
}

// FetchBranchProtection looks up the branch protection rule on a repo's base
// branch via the REST API. A 404 is treated as "no rule configured" and a 403
// is treated as "no rule we can see" (token lacks Administration: Read scope).
// Both return HasRule=false with no error so the cache stores the negative
// result and we don't burn rate-limit quota retrying every poll cycle.
// Other errors propagate so callers can decide whether to retry.
func (c *PATClient) FetchBranchProtection(ctx context.Context, owner, repo, branch string) (BranchProtection, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, branch)
	var raw struct {
		RequiredPullRequestReviews *struct {
			RequiredApprovingReviewCount int `json:"required_approving_review_count"`
		} `json:"required_pull_request_reviews"`
	}
	if err := c.get(ctx, endpoint, &raw); err != nil {
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusForbidden {
				return BranchProtection{HasRule: false}, nil
			}
		}
		return BranchProtection{}, err
	}
	if raw.RequiredPullRequestReviews == nil {
		return BranchProtection{HasRule: true}, nil
	}
	return BranchProtection{
		HasRule:                      true,
		RequiredApprovingReviewCount: raw.RequiredPullRequestReviews.RequiredApprovingReviewCount,
	}, nil
}

// patPR is the JSON shape from the GitHub REST API for PRs.
type patPR struct {
	Number             int       `json:"number"`
	Title              string    `json:"title"`
	HTMLURL            string    `json:"html_url"`
	Body               string    `json:"body"`
	State              string    `json:"state"`
	Draft              bool      `json:"draft"`
	Mergeable          *bool     `json:"mergeable"`
	MergeableState     string    `json:"mergeable_state"`
	Additions          int       `json:"additions"`
	Deletions          int       `json:"deletions"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	MergedAt           *string   `json:"merged_at"`
	ClosedAt           *string   `json:"closed_at"`
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers"`
	RequestedTeams []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"requested_teams"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type patSearchItem struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	HTMLURL       string    `json:"html_url"`
	State         string    `json:"state"`
	Draft         bool      `json:"draft"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	RepositoryURL string    `json:"repository_url"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
}

func convertPatPR(raw *patPR, owner, repo string) *PR {
	state := strings.ToLower(raw.State)
	if raw.MergedAt != nil && *raw.MergedAt != "" {
		state = prStateMerged
	}
	mergeable := false
	if raw.Mergeable != nil {
		mergeable = *raw.Mergeable
	}
	pr := &PR{
		Number:             raw.Number,
		Title:              raw.Title,
		HTMLURL:            raw.HTMLURL,
		Body:               raw.Body,
		State:              state,
		HeadBranch:         raw.Head.Ref,
		HeadSHA:            raw.Head.SHA,
		BaseBranch:         raw.Base.Ref,
		AuthorLogin:        raw.User.Login,
		RepoOwner:          owner,
		RepoName:           repo,
		Draft:              raw.Draft,
		Mergeable:          mergeable,
		MergeableState:     strings.ToLower(raw.MergeableState),
		Additions:          raw.Additions,
		Deletions:          raw.Deletions,
		RequestedReviewers: convertPatRequestedReviewers(raw),
		CreatedAt:          raw.CreatedAt,
		UpdatedAt:          raw.UpdatedAt,
	}
	if raw.MergedAt != nil {
		pr.MergedAt = parseTimePtr(*raw.MergedAt)
	}
	if raw.ClosedAt != nil {
		pr.ClosedAt = parseTimePtr(*raw.ClosedAt)
	}
	return pr
}

func convertPatRequestedReviewers(raw *patPR) []RequestedReviewer {
	reviewers := make([]RequestedReviewer, 0, len(raw.RequestedReviewers)+len(raw.RequestedTeams))
	for _, reviewer := range raw.RequestedReviewers {
		if reviewer.Login == "" {
			continue
		}
		reviewers = append(reviewers, RequestedReviewer{Login: reviewer.Login, Type: reviewerTypeUser})
	}
	for _, team := range raw.RequestedTeams {
		login := team.Slug
		if login == "" {
			login = team.Name
		}
		if login == "" {
			continue
		}
		reviewers = append(reviewers, RequestedReviewer{Login: login, Type: reviewerTypeTeam})
	}
	return reviewers
}
