package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GHClient implements Client using the gh CLI.
type GHClient struct{}

// NewGHClient creates a new gh CLI-based client.
func NewGHClient() *GHClient {
	return &GHClient{}
}

// GHAvailable checks if the gh CLI is installed and accessible.
func GHAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func (c *GHClient) IsAuthenticated(ctx context.Context) (bool, error) {
	_, err := c.run(ctx, "auth", "status", "--hostname", "github.com")
	if err != nil {
		// gh auth status exits non-zero when not authenticated.
		// The error message includes stderr from the gh CLI.
		errMsg := err.Error()
		if strings.Contains(errMsg, "not logged") || strings.Contains(errMsg, "no accounts") {
			return false, nil
		}
		// Unexpected error (binary not found, network issue, etc.)
		return false, err
	}
	return true, nil
}

func (c *GHClient) GetAuthenticatedUser(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "api", "user", "-q", ".login")
	if err != nil {
		return "", fmt.Errorf("get authenticated user: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ghPR is the JSON shape returned by gh pr list/view.
type ghPR struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	State       string    `json:"state"`
	HeadRefName string    `json:"headRefName"`
	HeadRefOid  string    `json:"headRefOid"`
	BaseRefName string    `json:"baseRefName"`
	IsDraft     bool      `json:"isDraft"`
	Mergeable   string    `json:"mergeable"`
	Additions   int       `json:"additions"`
	Deletions   int       `json:"deletions"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	MergedAt    string    `json:"mergedAt"`
	ClosedAt    string    `json:"closedAt"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (c *GHClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	out, err := c.run(ctx, "pr", "view", fmt.Sprintf("%d", number),
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,additions,deletions,createdAt,updatedAt,mergedAt,closedAt")
	if err != nil {
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}
	var raw ghPR
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse PR response: %w", err)
	}
	return convertGHPR(&raw, owner, repo), nil
}

func (c *GHClient) FindPRByBranch(ctx context.Context, owner, repo, branch string) (*PR, error) {
	out, err := c.run(ctx, "pr", "list",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--head", branch,
		"--state", "open",
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,additions,deletions,createdAt,updatedAt",
		"--limit", "1")
	if err != nil {
		return nil, fmt.Errorf("find PR by branch %q: %w", branch, err)
	}
	var prs []ghPR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return convertGHPR(&prs[0], owner, repo), nil
}

func (c *GHClient) ListAuthoredPRs(ctx context.Context, owner, repo string) ([]*PR, error) {
	out, err := c.run(ctx, "pr", "list",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--author", "@me",
		"--state", "open",
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,additions,deletions,createdAt,updatedAt")
	if err != nil {
		return nil, fmt.Errorf("list authored PRs: %w", err)
	}
	return c.parsePRList(out, owner, repo)
}

func (c *GHClient) ListReviewRequestedPRs(ctx context.Context, scope, filter, customQuery string) ([]*PR, error) {
	query := buildReviewSearchQuery(scope, filter, customQuery)
	out, err := c.run(ctx, "api", "search/issues",
		"-X", "GET",
		"-f", "q="+query,
		"-f", "per_page=50",
		"--jq", ".items")
	if err != nil {
		return nil, fmt.Errorf("list review-requested PRs: %w", err)
	}
	return c.parseSearchResults(out)
}

func (c *GHClient) ListUserOrgs(ctx context.Context) ([]GitHubOrg, error) {
	out, err := c.run(ctx, "api", "user/orgs", "--paginate")
	if err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	var raw []struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}
	orgs := make([]GitHubOrg, len(raw))
	for i, r := range raw {
		orgs[i] = GitHubOrg{Login: r.Login, AvatarURL: r.AvatarURL}
	}
	return orgs, nil
}

func (c *GHClient) SearchOrgRepos(ctx context.Context, org, query string, limit int) ([]GitHubRepo, error) {
	q := "org:" + org
	if query != "" {
		q += " " + query
	}
	if limit <= 0 {
		limit = 20
	}
	out, err := c.run(ctx, "api", "search/repositories",
		"-X", "GET",
		"-f", "q="+q,
		"-f", fmt.Sprintf("per_page=%d", limit),
		"--jq", ".items")
	if err != nil {
		return nil, fmt.Errorf("search org repos: %w", err)
	}
	return parseGHSearchRepos(out)
}

func parseGHSearchRepos(data string) ([]GitHubRepo, error) {
	var items []struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, fmt.Errorf("parse search repos: %w", err)
	}
	repos := make([]GitHubRepo, len(items))
	for i, item := range items {
		repos[i] = GitHubRepo{
			FullName: item.FullName,
			Owner:    item.Owner.Login,
			Name:     item.Name,
			Private:  item.Private,
		}
	}
	return repos, nil
}

// ghReview is the JSON shape for reviews from gh pr view.
type ghReview struct {
	ID     int64 `json:"id"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	User struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submittedAt"`
}

func (c *GHClient) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", owner, repo, number),
		"--paginate")
	if err != nil {
		return nil, fmt.Errorf("list PR reviews: %w", err)
	}
	var raw []ghReview
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse reviews: %w", err)
	}
	reviews := make([]PRReview, len(raw))
	for i, r := range raw {
		author := r.Author.Login
		avatar := ""
		if r.User.Login != "" {
			author = r.User.Login
			avatar = r.User.AvatarURL
		}
		reviews[i] = PRReview{
			ID:           r.ID,
			Author:       author,
			AuthorAvatar: avatar,
			State:        r.State,
			Body:         r.Body,
			CreatedAt:    r.SubmittedAt,
		}
	}
	return reviews, nil
}

// ghComment is the JSON shape for review comments from the GitHub API.
type ghComment struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	Line      int       `json:"line"`
	Side      string    `json:"side"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	InReplyTo *int64    `json:"in_reply_to_id"`
	User      struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

func (c *GHClient) ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/comments", owner, repo, number)
	if since != nil {
		endpoint += "?since=" + since.Format(time.RFC3339)
	}
	out, err := c.run(ctx, "api", endpoint, "--paginate")
	if err != nil {
		return nil, fmt.Errorf("list PR comments: %w", err)
	}
	var raw []ghComment
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse comments: %w", err)
	}
	return convertRawComments(raw), nil
}

// ghCheckRun is the JSON shape from the check-runs API.
type ghCheckRun struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	HTMLURL     string  `json:"html_url"`
	StartedAt   string  `json:"started_at"`
	CompletedAt string  `json:"completed_at"`
	Output      struct {
		Title   *string `json:"title"`
		Summary *string `json:"summary"`
	} `json:"output"`
}

func (c *GHClient) ListCheckRuns(ctx context.Context, owner, repo, ref string) ([]CheckRun, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/commits/%s/check-runs", owner, repo, ref),
		"--jq", ".check_runs")
	if err != nil {
		return nil, fmt.Errorf("list check runs: %w", err)
	}
	var raw []ghCheckRun
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse check runs: %w", err)
	}
	return convertRawCheckRuns(raw), nil
}

func (c *GHClient) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	return getPRFeedback(ctx, c, owner, repo, number)
}

// run executes a gh CLI command and returns its stdout output.
// Stderr is captured separately to avoid contaminating JSON output.
func (c *GHClient) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("gh %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}

func (c *GHClient) parsePRList(data string, owner, repo string) ([]*PR, error) {
	var raw []ghPR
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	prs := make([]*PR, len(raw))
	for i := range raw {
		prs[i] = convertGHPR(&raw[i], owner, repo)
	}
	return prs, nil
}

// ghSearchItem is a PR item from the GitHub search API.
type ghSearchItem struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	RepositoryURL string `json:"repository_url"`
}

func (c *GHClient) parseSearchResults(data string) ([]*PR, error) {
	var items []ghSearchItem
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}
	prs := make([]*PR, len(items))
	for i, item := range items {
		prs[i] = convertSearchItemToPR(
			item.Number, item.Title, item.HTMLURL, item.State,
			item.User.Login, item.RepositoryURL, item.Draft,
			item.CreatedAt, item.UpdatedAt,
		)
	}
	return prs, nil
}

func convertGHPR(raw *ghPR, owner, repo string) *PR {
	state := strings.ToLower(raw.State)
	if raw.MergedAt != "" {
		state = prStateMerged
	}
	pr := &PR{
		Number:      raw.Number,
		Title:       raw.Title,
		URL:         raw.URL,
		HTMLURL:     raw.URL,
		State:       state,
		HeadBranch:  raw.HeadRefName,
		HeadSHA:     raw.HeadRefOid,
		BaseBranch:  raw.BaseRefName,
		AuthorLogin: raw.Author.Login,
		RepoOwner:   owner,
		RepoName:    repo,
		Draft:       raw.IsDraft,
		Mergeable:   raw.Mergeable == "MERGEABLE",
		Additions:   raw.Additions,
		Deletions:   raw.Deletions,
		CreatedAt:   raw.CreatedAt,
		UpdatedAt:   raw.UpdatedAt,
		MergedAt:    parseTimePtr(raw.MergedAt),
		ClosedAt:    parseTimePtr(raw.ClosedAt),
	}
	return pr
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// parseRepoURL extracts owner and repo from a GitHub API URL like
// "https://api.github.com/repos/owner/repo".
func parseRepoURL(url string) (string, string) {
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}
