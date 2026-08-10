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
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Head    string `json:"head"`
	Base    string `json:"base"`
	Draft   bool   `json:"draft"`
}

type rawPullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Merged  bool   `json:"merged"`
	Head    struct {
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
	return &PullRequest{Number: p.Number, Title: p.Title, State: state, HTMLURL: p.HTMLURL, Head: p.Head.Ref, Base: p.Base.Ref, Draft: p.Draft}
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
		Number      int             `json:"number"`
		Title       string          `json:"title"`
		State       string          `json:"state"`
		HTMLURL     string          `json:"html_url"`
		Body        string          `json:"body"`
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
		issues = append(issues, Issue{Number: issue.Number, Title: issue.Title, State: issue.State, HTMLURL: issue.HTMLURL, Body: issue.Body})
	}
	return issues, total, nil
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
