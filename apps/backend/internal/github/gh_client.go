package github

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
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

// ghSearchReposPath is the GitHub REST search-repositories path that gh CLI
// invocations target via `gh api`. Centralised to keep the search-bucket
// rate-limit dispatch (`resourceForGHArgs`) and the repo search helpers in
// sync.
const ghSearchReposPath = "search/repositories"

// ghAccessibleReposPath is the GET /user/repos endpoint (with affiliation +
// sort + per_page baked in) that backs ListAccessibleRepos. It returns a flat
// JSON array on the core REST quota, replacing the per-org search fan-out.
const ghAccessibleReposPathFmt = "/user/repos?affiliation=%s&sort=pushed&per_page=%d"

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
	var stdout, stderr bytes.Buffer
	runErr, execCtxErr := subproc.RunGHAfterAcquire(ctx, resolveGHExecTimeout(ctx), func(execCtx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(execCtx, "gh", "auth", "status", "--hostname", "github.com")
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		return cmd
	})
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}
	// When acquire/context fails before gh runs (or the exec is killed by
	// the per-command deadline), both buffers can be empty. Surface the
	// underlying error so the diagnostics panel doesn't show a blank
	// "command failed with exit code -1" with no explanation.
	if output == "" && runErr != nil {
		if execCtxErr != nil {
			output = fmt.Sprintf("gh auth status did not run: %v (context: %v)", runErr, execCtxErr)
		} else {
			output = fmt.Sprintf("gh auth status did not run: %v", runErr)
		}
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

type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// The gh CLI currently emits an empty string for open issues.
	ClosedAt string `json:"closedAt"`
	Author   struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
}

func (c *GHClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	out, err := c.run(ctx, "pr", "view", fmt.Sprintf("%d", number),
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--json", "number,title,url,state,body,headRefName,headRefOid,baseRefName,author,isDraft,mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt,mergedAt,closedAt,reviewRequests")
	if err != nil {
		if isNotFoundErr(err) {
			return nil, &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number),
				Body:       err.Error(),
			}
		}
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}
	var raw ghPR
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse PR response: %w", err)
	}
	return convertGHPR(&raw, owner, repo), nil
}

func (c *GHClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	out, err := c.run(ctx, "issue", "view", fmt.Sprintf("%d", number),
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--json", "number,title,url,state,body,author,labels,assignees,createdAt,updatedAt,closedAt")
	if err != nil {
		if isNotFoundErr(err) {
			return nil, &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number),
				Body:       err.Error(),
			}
		}
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
	}
	var raw ghIssue
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse issue response: %w", err)
	}
	return convertGHIssue(&raw, owner, repo), nil
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
	limit = clampRepoSearchLimit(limit)
	out, err := c.run(ctx, "api", ghSearchReposPath,
		"-X", "GET",
		"-f", "q="+q,
		"-f", fmt.Sprintf("per_page=%d", limit),
		"--jq", ".items")
	if err != nil {
		return nil, fmt.Errorf("search org repos: %w", err)
	}
	return parseGHSearchRepos(out)
}

func (c *GHClient) ListUserRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	limit = clampRepoSearchLimit(limit)
	login, err := c.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user repos: %w", err)
	}
	args := buildUserReposGHArgs(login, query, limit)
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("list user repos: %w", err)
	}
	return parseGHSearchRepos(out)
}

// buildUserReposGHArgs constructs the `gh api search/repositories` argv used
// to list repos for the authenticated user. Keeping this pure makes the
// endpoint + qualifier construction unit-testable without spawning gh. The
// `q` value mirrors the PAT path (`user:<login> <query>`) so both clients
// honour the same GitHub search syntax (e.g. `language:go`, `in:name`).
// Callers must clamp `limit` via clampRepoSearchLimit beforehand.
func buildUserReposGHArgs(login, query string, limit int) []string {
	q := "user:" + login
	if query != "" {
		q += " " + query
	}
	return []string{
		"api", ghSearchReposPath,
		"-X", "GET",
		"-f", "q=" + q,
		"-f", fmt.Sprintf("per_page=%d", limit),
		"--jq", ".items",
	}
}

// ListAccessibleRepos lists every repo the authenticated user can access via a
// single `gh api /user/repos` call on the core REST quota. The endpoint returns
// a flat JSON array (parsed by parseGHSearchRepos, which already decodes a
// top-level array). No --paginate: one page of up to 100 is what the picker
// needs (the frontend caps at 100 and filters client-side).
//
// query is a BEST-EFFORT substring filter over only that single un-paginated
// page — a match beyond the cap returns nothing. The frontend does the
// canonical client-side filtering, so this server filter is just optional
// narrowing; do not rely on it for completeness.
func (c *GHClient) ListAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error) {
	limit = clampRepoSearchLimit(limit)
	out, err := c.run(ctx, buildAccessibleReposGHArgs(limit)...)
	if err != nil {
		return nil, fmt.Errorf("list accessible repos: %w", err)
	}
	repos, err := parseGHSearchRepos(out)
	if err != nil {
		return nil, err
	}
	return filterReposByQuery(repos, query), nil
}

// buildAccessibleReposGHArgs constructs the `gh api /user/repos?...` argv for
// ListAccessibleRepos. Keeping it pure makes the endpoint/query construction
// unit-testable without spawning gh. Callers must clamp `limit` via
// clampRepoSearchLimit beforehand.
func buildAccessibleReposGHArgs(limit int) []string {
	endpoint := fmt.Sprintf(ghAccessibleReposPathFmt, accessibleReposAffiliation, limit)
	return []string{"api", endpoint}
}

func parseGHSearchRepos(data string) ([]GitHubRepo, error) {
	var items []struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name          string    `json:"name"`
		Private       bool      `json:"private"`
		DefaultBranch string    `json:"default_branch"`
		Description   string    `json:"description"`
		PushedAt      time.Time `json:"pushed_at"`
	}
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, fmt.Errorf("parse search repos: %w", err)
	}
	repos := make([]GitHubRepo, len(items))
	for i, item := range items {
		repos[i] = GitHubRepo{
			FullName:      item.FullName,
			Owner:         item.Owner.Login,
			Name:          item.Name,
			Private:       item.Private,
			DefaultBranch: item.DefaultBranch,
			Description:   item.Description,
		}
		if !item.PushedAt.IsZero() {
			t := item.PushedAt
			repos[i].PushedAt = &t
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
		"--paginate",
		fmt.Sprintf("repos/%s/%s/commits/%s/check-runs?per_page=100", owner, repo, ref),
		"--jq", ".check_runs[]")
	if err != nil {
		return nil, fmt.Errorf("list check runs: %w", err)
	}
	checkRunsRaw, err := decodeGHCheckRuns(checkRunsOut)
	if err != nil {
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

// decodeGHCheckRuns decodes whitespace-separated JSON check-run objects
// emitted by gh --paginate --jq '.check_runs[]'.
func decodeGHCheckRuns(out string) ([]ghCheckRun, error) {
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var checkRuns []ghCheckRun
	for {
		var checkRun ghCheckRun
		if err := dec.Decode(&checkRun); err != nil {
			if errors.Is(err, io.EOF) {
				return checkRuns, nil
			}
			return nil, err
		}
		checkRuns = append(checkRuns, checkRun)
	}
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

func (c *GHClient) MergePR(ctx context.Context, owner, repo string, number int, mergeMethod string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/merge", owner, repo, number)
	args := []string{"api", endpoint, "-X", "PUT"}
	if mergeMethod != "" {
		args = append(args, "-f", "merge_method="+mergeMethod)
	}
	_, err := c.run(ctx, args...)
	if err != nil {
		// Surface status-based errors as GitHubAPIError so httpMergePR can
		// translate 405 (not mergeable) / 409 (conflict) to HTTP 409 for
		// gh CLI users too, matching the PAT path.
		if code, ok := ghMergeStatusCode(err); ok {
			return &GitHubAPIError{StatusCode: code, Endpoint: endpoint, Body: err.Error()}
		}
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	return nil
}

// ghMergeStatusCode extracts the HTTP status code from a gh CLI merge error.
// Returns the code and true when the stderr matches one of GitHub's
// merge-API failure shapes (404/403/405/409); false otherwise so the caller
// falls back to a generic wrap.
func ghMergeStatusCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	if isNotFoundErr(err) {
		return http.StatusNotFound, true
	}
	if isForbiddenErr(err) {
		return http.StatusForbidden, true
	}
	s := err.Error()
	if strings.Contains(s, "HTTP 405") || strings.Contains(s, "status: 405") ||
		strings.Contains(s, "405 Method Not Allowed") {
		return http.StatusMethodNotAllowed, true
	}
	if strings.Contains(s, "HTTP 409") || strings.Contains(s, "status: 409") ||
		strings.Contains(s, "409 Conflict") {
		return http.StatusConflict, true
	}
	return 0, false
}

func (c *GHClient) GetRepoMergeMethods(ctx context.Context, owner, repo string) (RepoMergeMethods, error) {
	out, err := c.run(ctx, "api", fmt.Sprintf("repos/%s/%s", owner, repo))
	if err != nil {
		return RepoMergeMethods{}, fmt.Errorf("get repo merge methods: %w", err)
	}
	var raw struct {
		AllowMergeCommit *bool `json:"allow_merge_commit"`
		AllowSquashMerge *bool `json:"allow_squash_merge"`
		AllowRebaseMerge *bool `json:"allow_rebase_merge"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return RepoMergeMethods{}, fmt.Errorf("parse repo: %w", err)
	}
	// Conservative read: missing field → false. A permission-gated response
	// that omits allow_* would otherwise let us pick a disallowed method
	// (e.g. "merge" on a rebase-only repo), reproducing the 405 this fix is
	// designed to prevent. Callers that fall back to GitHub's default on an
	// empty pick get a meaningful error instead of a wrong-method 405.
	allowed := func(p *bool) bool { return p != nil && *p }
	return RepoMergeMethods{
		Merge:  allowed(raw.AllowMergeCommit),
		Squash: allowed(raw.AllowSquashMerge),
		Rebase: allowed(raw.AllowRebaseMerge),
	}, nil
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

func (c *GHClient) CreateGist(ctx context.Context, in CreateGistInput) (*GistResponse, error) {
	payload := struct {
		Description string                 `json:"description,omitempty"`
		Public      bool                   `json:"public"`
		Files       map[string]gistFileDTO `json:"files"`
	}{
		Description: in.Description,
		Public:      in.Public,
		Files:       make(map[string]gistFileDTO, len(in.Files)),
	}
	for name, f := range in.Files {
		payload.Files[name] = gistFileDTO(f)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal gist payload: %w", err)
	}
	args := []string{"api", "gists",
		"-X", "POST",
		"-H", "Accept: " + githubAccept,
		"--input", "-",
	}
	out, err := c.runWithStdin(ctx, body, args...)
	if err != nil {
		return nil, fmt.Errorf("create gist: %w", err)
	}
	var resp GistResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("decode gist response: %w", err)
	}
	return &resp, nil
}

func (c *GHClient) DeleteGist(ctx context.Context, gistID string) error {
	if gistID == "" {
		return fmt.Errorf("delete gist: empty id")
	}
	_, err := c.run(ctx, "api", "gists/"+gistID, "-X", "DELETE")
	if err != nil {
		// gh exits non-zero for HTTP errors; stderr contains the status
		// line (e.g. "HTTP 404: Not Found"). Promote 404 to *GitHubAPIError
		// so share.IsAlreadyGone matches consistently — PATClient.delete()
		// already returns a typed error for the same case, and the share
		// service uses errors.As to detect "gist already revoked upstream"
		// and treat it as a soft success rather than a 502. Use isNotFoundErr
		// so every gh-CLI 404 format ("HTTP 404", "404 Not Found", "status: 404")
		// is recognised, not just the most common one.
		if isNotFoundErr(err) {
			return &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   "/gists/" + gistID,
				Body:       err.Error(),
			}
		}
		return fmt.Errorf("delete gist %s: %w", gistID, err)
	}
	return nil
}

// ListRepoDirectory lists the entries of a directory in a repository at the
// given ref via `gh api repos/{owner}/{repo}/contents/{path}`. A 404 (missing
// directory) is promoted to a *GitHubAPIError, mirroring PATClient.
func (c *GHClient) ListRepoDirectory(ctx context.Context, owner, repo, dir, ref string) ([]RepoContentEntry, error) {
	args := ghContentsArgs(owner, repo, dir, ref)
	out, err := c.run(ctx, args...)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, repoContentsPath(dir)),
				Body:       err.Error(),
			}
		}
		return nil, fmt.Errorf("list repo directory: %w", err)
	}
	var entries []RepoContentEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("list repo directory: decode: %w", err)
	}
	return entries, nil
}

// GetRepoFileContent fetches the raw decoded content of a single file via
// `gh api repos/{owner}/{repo}/contents/{path}`. A 404 (missing file) is
// promoted to a *GitHubAPIError, mirroring PATClient.
func (c *GHClient) GetRepoFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	args := ghContentsArgs(owner, repo, path, ref)
	out, err := c.run(ctx, args...)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, repoContentsPath(path)),
				Body:       err.Error(),
			}
		}
		return nil, fmt.Errorf("get repo file content: %w", err)
	}
	var raw ghRepoFileContent
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("get repo file content: decode: %w", err)
	}
	if raw.Encoding != "base64" {
		return nil, fmt.Errorf("get repo file content: unsupported encoding %q", raw.Encoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(stripBase64Whitespace(raw.Content))
	if err != nil {
		return nil, fmt.Errorf("get repo file content: decode base64: %w", err)
	}
	return decoded, nil
}

// ghContentsArgs builds the `gh api` argv for the repo contents endpoint,
// shared by ListRepoDirectory and GetRepoFileContent. ref, when non-empty, is
// passed as a `-f` field (requiring the explicit `-X GET`, since `gh api`
// otherwise switches to POST once any `-f`/`-F` field is supplied).
func ghContentsArgs(owner, repo, path, ref string) []string {
	endpoint := "repos/" + owner + "/" + repo + "/contents"
	if p := repoContentsPath(path); p != "" {
		endpoint += "/" + p
	}
	args := []string{"api", endpoint, "-X", "GET"}
	if ref != "" {
		args = append(args, "-f", "ref="+ref)
	}
	return args
}

const ghCLITimeout = 30 * time.Second

// resolveGHExecTimeout returns the per-command exec budget used by
// run / runWithStdin. Honours the caller's ctx deadline when set (so a
// short-lived WS request stays bounded by the request deadline), but
// caps the budget at ghCLITimeout when the deadline is further out or
// absent. Used by the acquire-then-derive-timeout path so a queued
// waiter's exec timer starts AFTER it gets the throttle slot.
func resolveGHExecTimeout(ctx context.Context) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < ghCLITimeout {
			return remaining
		}
	}
	return ghCLITimeout
}

// run executes a gh CLI command and returns its stdout output.
// Stderr is captured separately to avoid contaminating JSON output.
// A 30s per-command exec timeout is applied AFTER the gh throttle slot
// is acquired (acquire-then-derive-timeout). Pre-fix the timer started
// before Acquire, so a queued waiter inherited a deadline that had
// already partly elapsed against throttle queue time — producing the
// `signal: killed` + `context deadline exceeded` cascade in the
// SyncWatchesBatched storm logs.
func (c *GHClient) run(ctx context.Context, args ...string) (string, error) {
	return c.runGH(ctx, nil, args...)
}

// runWithStdin is like run but pipes stdin into the gh CLI process.
// Useful for `gh api --input -` calls where the body comes from JSON.
func (c *GHClient) runWithStdin(ctx context.Context, stdin []byte, args ...string) (string, error) {
	return c.runGH(ctx, stdin, args...)
}

// runGH is the shared body of run / runWithStdin — both apply the same
// acquire-then-derive-timeout ordering, the same stderr/rate-limit
// inspection, and the same error wrap. The only delta is whether stdin
// is wired (`stdin != nil`).
func (c *GHClient) runGH(ctx context.Context, stdin []byte, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	execTimeout := resolveGHExecTimeout(ctx)
	runErr, execCtxErr := subproc.RunGHAfterAcquire(ctx, execTimeout, func(execCtx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(execCtx, "gh", args...)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		return cmd
	})
	if runErr != nil {
		// Surface execCtx timeouts AND cancellations as the canonical
		// context error so callers (FetchRateLimit, IsAuthenticated,
		// share.IsAlreadyGone) can still errors.Is(err, context.X). The
		// runErr from cmd.Run on a killed child is `signal: killed`,
		// which loses both classifier signals that pre-fix code relied on.
		if execCtxErr != nil && (errors.Is(execCtxErr, context.DeadlineExceeded) || errors.Is(execCtxErr, context.Canceled)) {
			return stdout.String(), fmt.Errorf("gh %s: %w", firstArg(args), execCtxErr)
		}
		c.inspectRateStderr(args, stderr.String())
		return stdout.String(), fmt.Errorf("gh %s: %w: %s", firstArg(args), runErr, stderr.String())
	}
	return stdout.String(), nil
}

// firstArg returns args[0] when present, else "<no-args>". Used for error
// messages when the semaphore acquisition fails before we even exec gh.
func firstArg(args []string) string {
	if len(args) == 0 {
		return "<no-args>"
	}
	return args[0]
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
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
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
		URL      string `json:"url"`
		MergedAt string `json:"merged_at"`
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
			item.ID, item.NodeID,
			item.Number, item.Title, item.HTMLURL, item.State,
			item.User.Login, item.RepositoryURL, item.PullRequest.MergedAt,
			item.Draft, item.CreatedAt, item.UpdatedAt,
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

func convertGHIssue(raw *ghIssue, owner, repo string) *Issue {
	labels := make([]string, len(raw.Labels))
	for i, label := range raw.Labels {
		labels[i] = label.Name
	}
	assignees := make([]string, len(raw.Assignees))
	for i, assignee := range raw.Assignees {
		assignees[i] = assignee.Login
	}
	return &Issue{
		Number:      raw.Number,
		Title:       raw.Title,
		URL:         raw.URL,
		HTMLURL:     raw.URL,
		State:       strings.ToLower(raw.State),
		Body:        raw.Body,
		AuthorLogin: raw.Author.Login,
		RepoOwner:   owner,
		RepoName:    repo,
		Labels:      labels,
		Assignees:   assignees,
		CreatedAt:   raw.CreatedAt,
		UpdatedAt:   raw.UpdatedAt,
		ClosedAt:    parseTimePtr(raw.ClosedAt),
	}
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
