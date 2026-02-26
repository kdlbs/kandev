package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const githubAPIBase = "https://api.github.com"

// PATClient implements Client using a GitHub Personal Access Token.
type PATClient struct {
	token      string
	httpClient *http.Client
	username   string // cached after first GetAuthenticatedUser call
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
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request POST %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GitHub API %s returned %d: %s", endpoint, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(result)
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
