package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// GHClient implements Client using the gh CLI.
type GHClient struct {
	rateTracker *RateTracker
}

// NewGHClient creates a new gh CLI-based client.
func NewGHClient() *GHClient {
	return &GHClient{}
}

// WithRateTracker attaches a rate tracker so stderr output is inspected for
// rate-limit signals after every gh invocation. Returns the client for
// chaining; safe to call before any commands run.
func (c *GHClient) WithRateTracker(t *RateTracker) *GHClient {
	c.rateTracker = t
	return c
}

// reGHRateLimit matches the prose gh prints when a request hit a primary or
// secondary rate limit. The exact text varies by gh version and locale, but
// "rate limit" / "API rate limit" appear consistently.
var ghRateLimitMarkers = []string{"rate limit", "abuse detection"}

func ghStderrIndicatesRateLimit(stderr string) bool {
	if stderr == "" {
		return false
	}
	lower := strings.ToLower(stderr)
	for _, marker := range ghRateLimitMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// inspectRateStderr inspects the stderr from a failed `gh` invocation and
// flags the appropriate resource bucket as exhausted. `gh api <path>` is REST
// (Core) by default, with `api graphql` and `api search/*` as the documented
// exceptions; non-`api` subcommands like `pr`, `issue`, and `repo` are
// implemented against GraphQL by gh itself, so they map to the GraphQL bucket.
func (c *GHClient) inspectRateStderr(args []string, stderr string) {
	if c.rateTracker == nil {
		return
	}
	if !ghStderrIndicatesRateLimit(stderr) {
		return
	}
	c.rateTracker.markRateExhausted(resourceForGHArgs(args), time.Time{})
}

// resourceForGHArgs maps a `gh` argv to the rate-limit bucket the call hits.
// Exposed at package scope so the table-driven test can pin every branch.
func resourceForGHArgs(args []string) Resource {
	if len(args) == 0 {
		return ResourceCore
	}
	if args[0] != "api" {
		// `gh pr`, `gh issue`, `gh repo`, etc. are all GraphQL under the hood.
		return ResourceGraphQL
	}
	if len(args) < 2 {
		return ResourceCore
	}
	switch {
	case args[1] == "graphql":
		return ResourceGraphQL
	case strings.HasPrefix(args[1], "search/"):
		return ResourceSearch
	default:
		return ResourceCore
	}
}

// GHAvailable checks if the gh CLI is installed and accessible.
func GHAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func (c *GHClient) IsAuthenticated(ctx context.Context) (bool, error) {
	// Treat any non-zero exit as "not authenticated". This avoids parsing
	// locale-dependent error messages and handles multi-account scenarios
	// where a secondary account has an invalid token. GHAvailable() already
	// guards binary existence before this method is called.
	_, err := c.run(ctx, "auth", "status", "--hostname", "github.com")
	if err == nil {
		return true, nil
	}
	// Propagate timeout/cancellation errors so callers can distinguish them
	// from a genuine "not authenticated" result. Check the returned error
	// (not ctx.Err()) because run() may apply its own child deadline.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false, err
	}
	return false, nil
}

// RunAuthDiagnostics executes gh auth status and captures the raw output for troubleshooting.
func (c *GHClient) RunAuthDiagnostics(ctx context.Context) *AuthDiagnostics {
	ctx, cancel := withDefaultGHTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "status", "--hostname", "github.com")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}
	return &AuthDiagnostics{
		Command:  "gh auth status --hostname github.com",
		Output:   output,
		ExitCode: exitCode,
	}
}

func (c *GHClient) GetAuthenticatedUser(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "api", "user", "-q", ".login")
	if err != nil {
		return "", fmt.Errorf("get authenticated user: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ghRequestedReviewer is a user/team requested reviewer returned by gh pr view.
type ghRequestedReviewer struct {
	TypeName string `json:"__typename"`
	Login    string `json:"login"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
}

// ghPR is the JSON shape returned by gh pr list/view.
type ghPR struct {
	Number           int                   `json:"number"`
	Title            string                `json:"title"`
	URL              string                `json:"url"`
	State            string                `json:"state"`
	Body             string                `json:"body"`
	HeadRefName      string                `json:"headRefName"`
	HeadRefOid       string                `json:"headRefOid"`
	BaseRefName      string                `json:"baseRefName"`
	IsDraft          bool                  `json:"isDraft"`
	Mergeable        string                `json:"mergeable"`
	MergeStateStatus string                `json:"mergeStateStatus"`
	Additions        int                   `json:"additions"`
	Deletions        int                   `json:"deletions"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
	MergedAt         string                `json:"mergedAt"`
	ClosedAt         string                `json:"closedAt"`
	ReviewRequests   []ghRequestedReviewer `json:"reviewRequests"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (c *GHClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	out, err := c.run(ctx, "pr", "view", fmt.Sprintf("%d", number),
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--json", "number,title,url,state,body,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt,mergedAt,closedAt,reviewRequests")
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
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt",
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
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt")
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

func (c *GHClient) SearchPRs(ctx context.Context, filter, customQuery string) ([]*PR, error) {
	page, err := c.SearchPRsPaged(ctx, filter, customQuery, 1, 50)
	if err != nil {
		return nil, err
	}
	return page.PRs, nil
}

func (c *GHClient) SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildPRSearchQuery(filter, customQuery)
	out, err := c.run(ctx, "api", "search/issues",
		"-X", "GET",
		"-f", "q="+query,
		"-f", fmt.Sprintf("per_page=%d", perPage),
		"-f", fmt.Sprintf("page=%d", page),
	)
	if err != nil {
		return nil, fmt.Errorf("search PRs: %w", err)
	}
	var result struct {
		TotalCount int             `json:"total_count"`
		Items      json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parse PR search response: %w", err)
	}
	prs, err := c.parseSearchResults(string(result.Items))
	if err != nil {
		return nil, err
	}
	return &PRSearchPage{PRs: prs, TotalCount: result.TotalCount, Page: page, PerPage: perPage}, nil
}

func (c *GHClient) ListIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error) {
	page, err := c.ListIssuesPaged(ctx, filter, customQuery, 1, 50)
	if err != nil {
		return nil, err
	}
	return page.Issues, nil
}

func (c *GHClient) ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	page, perPage = clampSearchPage(page, perPage)
	query := buildIssueSearchQuery(filter, customQuery)
	out, err := c.run(ctx, "api", "search/issues",
		"-X", "GET",
		"-f", "q="+query,
		"-f", fmt.Sprintf("per_page=%d", perPage),
		"-f", fmt.Sprintf("page=%d", page),
	)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	var result struct {
		TotalCount int               `json:"total_count"`
		Items      []issueSearchItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parse issue search response: %w", err)
	}
	issues := parseIssueSearchResults(result.Items)
	return &IssueSearchPage{Issues: issues, TotalCount: result.TotalCount, Page: page, PerPage: perPage}, nil
}

func (c *GHClient) GetIssueState(ctx context.Context, owner, repo string, number int) (string, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/issues/%d", owner, repo, number),
		"--jq", ".state")
	if err != nil {
		return "", fmt.Errorf("get issue state: %w", err)
	}
	return strings.TrimSpace(out), nil
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
	SubmittedAt time.Time `json:"submitted_at"`
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
		Type      string `json:"type"`
	} `json:"user"`
}

func (c *GHClient) ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error) {
	reviewEndpoint := appendSinceQuery(fmt.Sprintf("repos/%s/%s/pulls/%d/comments", owner, repo, number), since)
	reviewOut, err := c.run(ctx, "api", reviewEndpoint, "--paginate")
	if err != nil {
		return nil, fmt.Errorf("list PR comments: %w", err)
	}
	var reviewRaw []ghComment
	if err := json.Unmarshal([]byte(reviewOut), &reviewRaw); err != nil {
		return nil, fmt.Errorf("parse comments: %w", err)
	}
	issueEndpoint := appendSinceQuery(fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number), since)
	issueOut, err := c.run(ctx, "api", issueEndpoint, "--paginate")
	if err != nil {
		return nil, fmt.Errorf("list issue comments: %w", err)
	}
	var issueRaw []ghIssueComment
	if err := json.Unmarshal([]byte(issueOut), &issueRaw); err != nil {
		return nil, fmt.Errorf("parse issue comments: %w", err)
	}
	return mergeAndSortPRComments(convertRawComments(reviewRaw), convertRawIssueComments(issueRaw)), nil
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
	checkRunsOut, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/commits/%s/check-runs", owner, repo, ref),
		"--jq", ".check_runs")
	if err != nil {
		return nil, fmt.Errorf("list check runs: %w", err)
	}
	var checkRunsRaw []ghCheckRun
	if err := json.Unmarshal([]byte(checkRunsOut), &checkRunsRaw); err != nil {
		return nil, fmt.Errorf("parse check runs: %w", err)
	}
	statusOut, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/commits/%s/status", owner, repo, ref),
		"--jq", ".statuses")
	if err != nil {
		return nil, fmt.Errorf("list status contexts: %w", err)
	}
	var statusRaw []ghStatusContext
	if err := json.Unmarshal([]byte(statusOut), &statusRaw); err != nil {
		return nil, fmt.Errorf("parse status contexts: %w", err)
	}
	return mergeChecks(convertRawCheckRuns(checkRunsRaw), convertRawStatusContexts(statusRaw)), nil
}

func (c *GHClient) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	return getPRFeedback(ctx, c, owner, repo, number)
}

func (c *GHClient) GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error) {
	return getPRStatus(ctx, c, owner, repo, number)
}

// FetchBranchProtection looks up branch protection via `gh api`. A 404 from
// `gh` (no rule) and a 403 (token lacks Administration: Read scope) are both
// treated as "no rule we can see" so callers cache the negative result and
// don't burn rate-limit quota on every poll. The string match is permissive
// across formats so a future `gh` CLI release that tweaks the wording
// doesn't silently break the cache.
func (c *GHClient) FetchBranchProtection(ctx context.Context, owner, repo, branch string) (BranchProtection, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, repo, branch))
	if err != nil {
		if isNotFoundErr(err) || isForbiddenErr(err) {
			return BranchProtection{HasRule: false}, nil
		}
		return BranchProtection{}, err
	}
	var raw struct {
		RequiredPullRequestReviews *struct {
			RequiredApprovingReviewCount int `json:"required_approving_review_count"`
		} `json:"required_pull_request_reviews"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return BranchProtection{}, fmt.Errorf("decode branch protection: %w", err)
	}
	if raw.RequiredPullRequestReviews == nil {
		return BranchProtection{HasRule: true}, nil
	}
	return BranchProtection{
		HasRule:                      true,
		RequiredApprovingReviewCount: raw.RequiredPullRequestReviews.RequiredApprovingReviewCount,
	}, nil
}

// isNotFoundErr matches the formats the `gh` CLI uses to report a 404 across
// recent versions: "HTTP 404", "HTTP 404: ...", and "404 Not Found".
// Restrictive enough that an unrelated 404 substring (e.g. inside a JSON
// payload accidentally rendered into stderr) is unlikely to false-match.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 404") ||
		strings.Contains(s, "404 Not Found") ||
		strings.Contains(s, "status: 404")
}

// isForbiddenErr matches the formats the `gh` CLI uses to report a 403, used
// by the branch-protection lookup to silently fall back to "no rule" when
// the token lacks Administration: Read scope.
func isForbiddenErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 403") ||
		strings.Contains(s, "403 Forbidden") ||
		strings.Contains(s, "status: 403")
}

func (c *GHClient) ListPRFiles(ctx context.Context, owner, repo string, number int) ([]PRFile, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/files", owner, repo, number),
		"--paginate")
	if err != nil {
		return nil, fmt.Errorf("list PR files: %w", err)
	}
	return parsePRFilesJSON(out)
}

func (c *GHClient) ListPRCommits(ctx context.Context, owner, repo string, number int) ([]PRCommitInfo, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/commits", owner, repo, number),
		"--paginate")
	if err != nil {
		return nil, fmt.Errorf("list PR commits: %w", err)
	}
	return parsePRCommitsJSON(out)
}

func (c *GHClient) SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error {
	args := []string{"api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", owner, repo, number),
		"-X", "POST",
		"-f", "event=" + event,
	}
	if body != "" {
		args = append(args, "-f", "body="+body)
	}
	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("submit review on PR #%d: %w", number, err)
	}
	return nil
}

func (c *GHClient) ListRepoBranches(ctx context.Context, owner, repo string) ([]RepoBranch, error) {
	out, err := c.run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/branches", owner, repo),
		"-X", "GET",
		"-f", "per_page=100",
		"--paginate",
		"--jq", ".[].name")
	if err != nil {
		return nil, fmt.Errorf("list repo branches: %w", err)
	}
	var branches []RepoBranch
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			branches = append(branches, RepoBranch{Name: name})
		}
	}
	return branches, nil
}

const ghCLITimeout = 30 * time.Second

// withDefaultGHTimeout applies a 30s timeout if the context has no deadline.
func withDefaultGHTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, ghCLITimeout)
}

// run executes a gh CLI command and returns its stdout output.
// Stderr is captured separately to avoid contaminating JSON output.
// A default 30s timeout is applied if the context has no deadline.
func (c *GHClient) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := withDefaultGHTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		c.inspectRateStderr(args, stderr.String())
		return stdout.String(), fmt.Errorf("gh %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}

// ghRateLimitResponse mirrors the GET /rate_limit JSON shape so we can seed
// the tracker on startup and after CLI failures without parsing prose.
type ghRateLimitResponse struct {
	Resources struct {
		Core    ghRateLimitBucket `json:"core"`
		GraphQL ghRateLimitBucket `json:"graphql"`
		Search  ghRateLimitBucket `json:"search"`
	} `json:"resources"`
}

type ghRateLimitBucket struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// FetchRateLimit calls `gh api rate_limit` and seeds the tracker with the
// returned snapshots. Best-effort: a failure (e.g. CLI absent, network) is
// logged and ignored.
func (c *GHClient) FetchRateLimit(ctx context.Context) error {
	if c.rateTracker == nil {
		return nil
	}
	out, err := c.run(ctx, "api", "rate_limit")
	if err != nil {
		return fmt.Errorf("fetch rate limit: %w", err)
	}
	var raw ghRateLimitResponse
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return fmt.Errorf("parse rate_limit response: %w", err)
	}
	now := time.Now().UTC()
	record := func(resource Resource, b ghRateLimitBucket) {
		if b.Limit == 0 && b.Remaining == 0 && b.Reset == 0 {
			return
		}
		c.rateTracker.Record(RateSnapshot{
			Resource:  resource,
			Limit:     b.Limit,
			Remaining: b.Remaining,
			ResetAt:   time.Unix(b.Reset, 0).UTC(),
			UpdatedAt: now,
		})
	}
	record(ResourceCore, raw.Resources.Core)
	record(ResourceGraphQL, raw.Resources.GraphQL)
	record(ResourceSearch, raw.Resources.Search)
	return nil
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
		Number:             raw.Number,
		Title:              raw.Title,
		URL:                raw.URL,
		HTMLURL:            raw.URL,
		State:              state,
		Body:               raw.Body,
		HeadBranch:         raw.HeadRefName,
		HeadSHA:            raw.HeadRefOid,
		BaseBranch:         raw.BaseRefName,
		AuthorLogin:        raw.Author.Login,
		RepoOwner:          owner,
		RepoName:           repo,
		Draft:              raw.IsDraft,
		Mergeable:          raw.Mergeable == "MERGEABLE",
		MergeableState:     strings.ToLower(raw.MergeStateStatus),
		Additions:          raw.Additions,
		Deletions:          raw.Deletions,
		RequestedReviewers: convertGHRequestedReviewers(raw.ReviewRequests),
		CreatedAt:          raw.CreatedAt,
		UpdatedAt:          raw.UpdatedAt,
		MergedAt:           parseTimePtr(raw.MergedAt),
		ClosedAt:           parseTimePtr(raw.ClosedAt),
	}
	return pr
}

func convertGHRequestedReviewers(raw []ghRequestedReviewer) []RequestedReviewer {
	reviewers := make([]RequestedReviewer, 0, len(raw))
	for _, req := range raw {
		switch req.TypeName {
		case "Team":
			login := req.Slug
			if login == "" {
				login = req.Name
			}
			if login == "" {
				continue
			}
			reviewers = append(reviewers, RequestedReviewer{Login: login, Type: reviewerTypeTeam})
		default:
			if req.Login == "" {
				continue
			}
			reviewers = append(reviewers, RequestedReviewer{Login: req.Login, Type: reviewerTypeUser})
		}
	}
	return reviewers
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

func appendSinceQuery(endpoint string, since *time.Time) string {
	if since == nil {
		return endpoint
	}
	return endpoint + "?since=" + url.QueryEscape(since.Format(time.RFC3339))
}
