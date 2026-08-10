// Package forgejo provides a workspace-scoped Forgejo REST API integration.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	apiPrefix       = "/api/v1"
	requestTimeout  = 30 * time.Second
	maxResponseSize = 2 << 20
)

var ErrInvalidOrigin = errors.New("forgejo: invalid origin")

type APIError struct {
	StatusCode int
	Endpoint   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Forgejo API %s returned status %d", e.Endpoint, e.StatusCode)
}

type User struct {
	Login string `json:"login"`
}

type Repository struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

type Issue struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	HTMLURL string   `json:"html_url"`
	Body    string   `json:"body"`
	Labels  []string `json:"labels"`
}

type PullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	State          string `json:"state"`
	HTMLURL        string `json:"html_url"`
	Head           string `json:"head"`
	Base           string `json:"base"`
	Draft          bool   `json:"draft"`
	Mergeable      bool   `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
}

type PullRequestCommit struct{ SHA, Message, Author string }
type PullRequestFile struct {
	Filename, Status, PreviousFilename, Patch string
	Additions, Deletions, Changes             int
}
type PullRequestComment struct {
	ID                          int64
	Body, Author, HTMLURL, Path string
	CreatedAt                   time.Time
}
type PullRequestReview struct {
	ID                    int64
	State, Body, Reviewer string
	SubmittedAt           *time.Time
}

type ActionRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Event      string `json:"event"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
}

type SubmitPullRequestReviewInput struct {
	Owner, Repo, Body, Event string
	Number                   int
}

// PullRequestDetailsClient is available when the configured Forgejo server
// exposes the standard review endpoints. Callers may report ErrUnsupported
// when a different client implementation does not provide it.
type PullRequestDetailsClient interface {
	ListPullRequestCommits(context.Context, string, string, int) ([]PullRequestCommit, error)
	ListPullRequestFiles(context.Context, string, string, int) ([]PullRequestFile, error)
	ListPullRequestComments(context.Context, string, string, int) ([]PullRequestComment, error)
	ListPullRequestReviews(context.Context, string, string, int) ([]PullRequestReview, error)
}

type PullRequestReviewWriter interface {
	CreatePullRequestComment(context.Context, string, string, int, string) (*PullRequestComment, error)
	SubmitPullRequestReview(context.Context, SubmitPullRequestReviewInput) (*PullRequestReview, error)
}

type ActionsClient interface {
	ListActionRuns(context.Context, string, string, int, int) ([]ActionRun, int, error)
}

// BranchLookupClient proves a branch exists on Forgejo before a PR is created.
// This deliberately asks the remote provider rather than trusting a local
// worktree ref, which may be stale or absent for remote executors.
type BranchLookupClient interface {
	GetBranch(context.Context, string, string, string) error
}

type rawPullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	State          string `json:"state"`
	HTMLURL        string `json:"html_url"`
	Draft          bool   `json:"draft"`
	Merged         bool   `json:"merged"`
	Mergeable      bool   `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
	Head           struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (p rawPullRequest) pullRequest() *PullRequest {
	state := p.State
	if p.Merged {
		state = "merged"
	}
	return &PullRequest{Number: p.Number, Title: p.Title, State: state, HTMLURL: p.HTMLURL, Head: p.Head.Ref, Base: p.Base.Ref, Draft: p.Draft, Mergeable: p.Mergeable, MergeableState: p.MergeableState}
}

type CreatePullRequestInput struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string
	Base  string
	Draft bool
}

type Client interface {
	GetAuthenticatedUser(context.Context) (*User, error)
	ListRepositories(context.Context, int, int) ([]Repository, int, error)
	ListIssues(context.Context, string, string, int, int) ([]Issue, int, error)
	ListPullRequests(context.Context, string, string, int, int) ([]PullRequest, int, error)
	GetIssue(context.Context, string, string, int) (*Issue, error)
	GetPullRequest(context.Context, string, string, int) (*PullRequest, error)
	CreatePullRequest(context.Context, CreatePullRequestInput) (*PullRequest, error)
}

type PATClient struct {
	origin     *url.URL
	token      string
	httpClient *http.Client
}

func NewPATClient(origin, token string) (*PATClient, error) {
	parsed, err := normalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	return &PATClient{origin: parsed, token: strings.TrimSpace(token), httpClient: &http.Client{Timeout: requestTimeout}}, nil
}

func normalizeOrigin(origin string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, ErrInvalidOrigin
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, ErrInvalidOrigin
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func (c *PATClient) GetAuthenticatedUser(ctx context.Context) (*User, error) {
	var user User
	if _, err := c.get(ctx, "/user", nil, &user); err != nil {
		return nil, err
	}
	if user.Login == "" {
		return nil, errors.New("Forgejo returned an empty login")
	}
	return &user, nil
}

func (c *PATClient) ListRepositories(ctx context.Context, page, limit int) ([]Repository, int, error) {
	var raw []struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
		Owner         User   `json:"owner"`
	}
	total, err := c.get(ctx, "/user/repos", pagination(page, limit), &raw)
	if err != nil {
		return nil, 0, err
	}
	repositories := make([]Repository, 0, len(raw))
	for _, repository := range raw {
		repositories = append(repositories, Repository{Owner: repository.Owner.Login, Name: repository.Name, FullName: repository.FullName, DefaultBranch: repository.DefaultBranch, HTMLURL: repository.HTMLURL})
	}
	return repositories, total, nil
}

func (c *PATClient) ListIssues(ctx context.Context, owner, repo string, page, limit int) ([]Issue, int, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil, 0, errors.New("Forgejo owner and repository are required")
	}
	var raw []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
		PullRequest json.RawMessage `json:"pull_request"`
	}
	total, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/issues", url.PathEscape(owner), url.PathEscape(repo)), pagination(page, limit), &raw)
	if err != nil {
		return nil, 0, err
	}
	issues := make([]Issue, 0, len(raw))
	for _, issue := range raw {
		if len(issue.PullRequest) != 0 && string(issue.PullRequest) != "null" {
			continue
		}
		labels := make([]string, len(issue.Labels))
		for i := range issue.Labels {
			labels[i] = issue.Labels[i].Name
		}
		issues = append(issues, Issue{Number: issue.Number, Title: issue.Title, State: issue.State, HTMLURL: issue.HTMLURL, Body: issue.Body, Labels: labels})
	}
	return issues, total, nil
}

func (c *PATClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || number < 1 {
		return nil, errors.New("Forgejo issue identity required")
	}
	var raw struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if _, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(owner), url.PathEscape(repo), number), nil, &raw); err != nil {
		return nil, err
	}
	labels := make([]string, len(raw.Labels))
	for i := range raw.Labels {
		labels[i] = raw.Labels[i].Name
	}
	return &Issue{Number: raw.Number, Title: raw.Title, State: raw.State, HTMLURL: raw.HTMLURL, Body: raw.Body, Labels: labels}, nil
}

func (c *PATClient) ListPullRequests(ctx context.Context, owner, repo string, page, limit int) ([]PullRequest, int, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil, 0, errors.New("Forgejo owner and repository are required")
	}
	var raw []rawPullRequest
	total, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo)), pagination(page, limit), &raw)
	if err != nil {
		return nil, 0, err
	}
	pulls := make([]PullRequest, len(raw))
	for i := range raw {
		pulls[i] = *raw[i].pullRequest()
	}
	return pulls, total, nil
}

func (c *PATClient) CreatePullRequest(ctx context.Context, input CreatePullRequestInput) (*PullRequest, error) {
	if strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Repo) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Head) == "" || strings.TrimSpace(input.Base) == "" {
		return nil, errors.New("Forgejo pull request owner, repository, title, head, and base are required")
	}
	var raw rawPullRequest
	if err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(input.Owner), url.PathEscape(input.Repo)), map[string]any{"title": input.Title, "body": input.Body, "head": input.Head, "base": input.Base, "draft": input.Draft}, &raw); err != nil {
		return nil, err
	}
	return raw.pullRequest(), nil
}

func (c *PATClient) GetBranch(ctx context.Context, owner, repo, branch string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || strings.TrimSpace(branch) == "" {
		return errors.New("Forgejo branch identity required")
	}
	var ignored json.RawMessage
	_, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/branches/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch)), nil, &ignored)
	return err
}

func (c *PATClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || number < 1 {
		return nil, errors.New("Forgejo pull request identity required")
	}
	var raw rawPullRequest
	if _, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number), nil, &raw); err != nil {
		return nil, err
	}
	return raw.pullRequest(), nil
}

func (c *PATClient) ListActionRuns(ctx context.Context, owner, repo string, page, limit int) ([]ActionRun, int, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil, 0, errors.New("Forgejo owner and repository are required")
	}
	var raw struct {
		WorkflowRuns []ActionRun `json:"workflow_runs"`
		TotalCount   int         `json:"total_count"`
	}
	total, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(owner), url.PathEscape(repo)), pagination(page, limit), &raw)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		total = raw.TotalCount
	}
	return raw.WorkflowRuns, total, nil
}

func (c *PATClient) ListPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]PullRequestCommit, error) {
	var raw []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := c.getPullRequestResource(ctx, owner, repo, number, "commits", &raw); err != nil {
		return nil, err
	}
	result := make([]PullRequestCommit, len(raw))
	for i := range raw {
		result[i] = PullRequestCommit{SHA: raw[i].SHA, Message: raw[i].Commit.Message, Author: raw[i].Commit.Author.Name}
	}
	return result, nil
}

func (c *PATClient) ListPullRequestFiles(ctx context.Context, owner, repo string, number int) ([]PullRequestFile, error) {
	var raw []struct {
		Filename         string `json:"filename"`
		Status           string `json:"status"`
		PreviousFilename string `json:"previous_filename"`
		Patch            string `json:"patch"`
		Additions        int    `json:"additions"`
		Deletions        int    `json:"deletions"`
		Changes          int    `json:"changes"`
	}
	if err := c.getPullRequestResource(ctx, owner, repo, number, "files", &raw); err != nil {
		return nil, err
	}
	result := make([]PullRequestFile, len(raw))
	for i := range raw {
		result[i] = PullRequestFile{Filename: raw[i].Filename, Status: raw[i].Status, PreviousFilename: raw[i].PreviousFilename, Patch: raw[i].Patch, Additions: raw[i].Additions, Deletions: raw[i].Deletions, Changes: raw[i].Changes}
	}
	return result, nil
}

func (c *PATClient) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]PullRequestComment, error) {
	var raw []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		HTMLURL   string    `json:"html_url"`
		Path      string    `json:"path"`
		CreatedAt time.Time `json:"created_at"`
		User      User      `json:"user"`
	}
	if err := c.getPullRequestResource(ctx, owner, repo, number, "comments", &raw); err != nil {
		return nil, err
	}
	result := make([]PullRequestComment, len(raw))
	for i := range raw {
		result[i] = PullRequestComment{ID: raw[i].ID, Body: raw[i].Body, Author: raw[i].User.Login, HTMLURL: raw[i].HTMLURL, Path: raw[i].Path, CreatedAt: raw[i].CreatedAt}
	}
	return result, nil
}

func (c *PATClient) ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]PullRequestReview, error) {
	var raw []struct {
		ID          int64      `json:"id"`
		State       string     `json:"state"`
		Body        string     `json:"body"`
		SubmittedAt *time.Time `json:"submitted_at"`
		User        User       `json:"user"`
	}
	if err := c.getPullRequestResource(ctx, owner, repo, number, "reviews", &raw); err != nil {
		return nil, err
	}
	result := make([]PullRequestReview, len(raw))
	for i := range raw {
		result[i] = PullRequestReview{ID: raw[i].ID, State: raw[i].State, Body: raw[i].Body, Reviewer: raw[i].User.Login, SubmittedAt: raw[i].SubmittedAt}
	}
	return result, nil
}

func (c *PATClient) CreatePullRequestComment(ctx context.Context, owner, repo string, number int, body string) (*PullRequestComment, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || number < 1 || strings.TrimSpace(body) == "" {
		return nil, errors.New("Forgejo pull request comment identity and body are required")
	}
	var raw struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		HTMLURL   string    `json:"html_url"`
		Path      string    `json:"path"`
		CreatedAt time.Time `json:"created_at"`
		User      User      `json:"user"`
	}
	if err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(repo), number), map[string]any{"body": body}, &raw); err != nil {
		return nil, err
	}
	return &PullRequestComment{ID: raw.ID, Body: raw.Body, Author: raw.User.Login, HTMLURL: raw.HTMLURL, Path: raw.Path, CreatedAt: raw.CreatedAt}, nil
}

func (c *PATClient) SubmitPullRequestReview(ctx context.Context, input SubmitPullRequestReviewInput) (*PullRequestReview, error) {
	if strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Repo) == "" || input.Number < 1 || strings.TrimSpace(input.Event) == "" {
		return nil, errors.New("Forgejo pull request review identity and event are required")
	}
	event := strings.ToUpper(strings.TrimSpace(input.Event))
	if event != "APPROVE" && event != "REQUEST_CHANGES" && event != "COMMENT" {
		return nil, errors.New("Forgejo pull request review event must be APPROVE, REQUEST_CHANGES, or COMMENT")
	}
	var raw struct {
		ID          int64      `json:"id"`
		State       string     `json:"state"`
		Body        string     `json:"body"`
		SubmittedAt *time.Time `json:"submitted_at"`
		User        User       `json:"user"`
	}
	if err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(input.Owner), url.PathEscape(input.Repo), input.Number), map[string]any{"body": input.Body, "event": event}, &raw); err != nil {
		return nil, err
	}
	return &PullRequestReview{ID: raw.ID, State: raw.State, Body: raw.Body, Reviewer: raw.User.Login, SubmittedAt: raw.SubmittedAt}, nil
}

func (c *PATClient) getPullRequestResource(ctx context.Context, owner, repo string, number int, resource string, target any) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || number < 1 {
		return errors.New("Forgejo pull request identity required")
	}
	_, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d/%s", url.PathEscape(owner), url.PathEscape(repo), number, resource), nil, target)
	return err
}

func pagination(page, limit int) url.Values {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 30
	}
	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	values.Set("limit", strconv.Itoa(limit))
	values.Set("state", "open")
	return values
}

func (c *PATClient) get(ctx context.Context, endpoint string, query url.Values, target any) (int, error) {
	request, err := c.newRequest(ctx, http.MethodGet, endpoint, query, nil)
	if err != nil {
		return 0, err
	}
	return c.do(request, target)
}

func (c *PATClient) post(ctx context.Context, endpoint string, body any, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := c.newRequest(ctx, http.MethodPost, endpoint, nil, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	_, err = c.do(request, target)
	return err
}

func (c *PATClient) newRequest(ctx context.Context, method, endpoint string, query url.Values, body io.Reader) (*http.Request, error) {
	requestURL := *c.origin
	requestURL.Path = path.Join(c.origin.Path, apiPrefix, endpoint)
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "token "+c.token)
	}
	return request, nil
}

func (c *PATClient) do(request *http.Request, target any) (int, error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseSize))
		return 0, &APIError{StatusCode: response.StatusCode, Endpoint: request.URL.Path}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseSize)).Decode(target); err != nil {
		return 0, err
	}
	total, _ := strconv.Atoi(response.Header.Get("x-total-count"))
	return total, nil
}
