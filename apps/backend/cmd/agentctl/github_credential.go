package main

import (
	"bufio"
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
	"unicode/utf8"

	"github.com/kandev/kandev/internal/githubauth"
)

type githubBrokerResolveRequest struct {
	Lease             string `json:"lease,omitempty"`
	ReissueCapability string `json:"reissue_capability,omitempty"`
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	RepositoryID      string `json:"repository_id"`
	Owner             string `json:"owner"`
	Repo              string `json:"repo"`
	Host              string `json:"host"`
	Path              string `json:"path,omitempty"`
	ProviderID        string `json:"provider_id,omitempty"`
	ParentProviderID  string `json:"parent_provider_id,omitempty"`
}

type githubBrokerCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type githubCredentialBrokerClient struct {
	endpoint string
	request  githubBrokerResolveRequest
	http     *http.Client
}

type githubBrokerHTTPError struct {
	status int
	code   string
	body   string
}

func (e *githubBrokerHTTPError) Error() string {
	return fmt.Sprintf("broker returned HTTP %d%s", e.status, e.body)
}

func newGitHubCredentialBrokerClient(
	getenv func(string) string,
	httpClient *http.Client,
) (*githubCredentialBrokerClient, error) {
	request := githubBrokerResolveRequest{
		Lease:             strings.TrimSpace(getenv(githubauth.CredentialLeaseEnv)),
		ReissueCapability: strings.TrimSpace(getenv(githubauth.CredentialReissueCapabilityEnv)),
		TaskID:            strings.TrimSpace(getenv(githubauth.CredentialTaskIDEnv)),
		SessionID:         strings.TrimSpace(getenv(githubauth.CredentialSessionIDEnv)),
		RepositoryID:      strings.TrimSpace(getenv(githubauth.CredentialRepositoryEnv)),
		Owner:             strings.TrimSpace(getenv(githubauth.CredentialOwnerEnv)),
		Repo:              strings.TrimSuffix(strings.TrimSpace(getenv(githubauth.CredentialRepoEnv)), ".git"),
		Host:              strings.ToLower(strings.TrimSpace(getenv(githubauth.CredentialHostEnv))),
	}
	endpoint := strings.TrimSpace(getenv(githubauth.CredentialBrokerURLEnv))
	for name, value := range map[string]string{
		"broker URL": endpoint, "lease": request.Lease, "task": request.TaskID,
		"session": request.SessionID, "repository": request.RepositoryID,
		"owner": request.Owner, "repo": request.Repo, "host": request.Host,
	} {
		if value == "" {
			return nil, fmt.Errorf("GitHub credential %s is not configured", name)
		}
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("GitHub credential broker URL is invalid")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &githubCredentialBrokerClient{endpoint: endpoint, request: request, http: httpClient}, nil
}

// maxBrokerErrorBodyBytes caps the broker error body appended to a resolve
// failure. Broker error bodies are code/message JSON and carry no credential
// (success bodies, which do carry the token, are never passed through this
// path), but the cap and control-char stripping stay regardless.
const maxBrokerErrorBodyBytes = 512

// formatBrokerErrorBody reads a bounded, sanitized broker error body suitable
// for appending to an error message, e.g. ": {\"error\":\"...\"}" — or "" when
// the body is empty after sanitizing.
func formatBrokerErrorBody(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, maxBrokerErrorBodyBytes))
	for !utf8.Valid(raw) && len(raw) > 0 {
		raw = raw[:len(raw)-1]
	}
	var sanitized strings.Builder
	for _, r := range string(raw) {
		if r < 0x20 {
			continue
		}
		sanitized.WriteRune(r)
	}
	trimmed := strings.TrimSpace(sanitized.String())
	if trimmed == "" {
		return ""
	}
	return ": " + trimmed
}

func (c *githubCredentialBrokerClient) resolve(ctx context.Context) (*githubBrokerCredential, error) {
	body, err := c.post(ctx, c.endpoint, c.request)
	if err != nil {
		return nil, fmt.Errorf("resolve GitHub credential: %w", err)
	}
	var credential githubBrokerCredential
	if err := json.Unmarshal(body, &credential); err != nil {
		return nil, fmt.Errorf("decode GitHub credential: %w", err)
	}
	if credential.Username == "" || credential.Password == "" {
		return nil, fmt.Errorf("resolve GitHub credential: broker returned an empty credential")
	}
	return &credential, nil
}

func (c *githubCredentialBrokerClient) resolveWithReissue(ctx context.Context) (*githubBrokerCredential, error) {
	credential, err := c.resolve(ctx)
	if err == nil || !isReissuableLeaseError(err) || strings.TrimSpace(c.request.ReissueCapability) == "" {
		return credential, err
	}
	if err := c.reissue(ctx); err != nil {
		return nil, err
	}
	return c.resolve(ctx)
}

func (c *githubCredentialBrokerClient) reissue(ctx context.Context) error {
	endpoint, err := githubCredentialReissueEndpoint(c.endpoint)
	if err != nil {
		return err
	}
	request := c.request
	request.Lease = ""
	body, err := c.post(ctx, endpoint, request)
	if err != nil {
		return fmt.Errorf("reissue GitHub credential lease: %w", err)
	}
	var lease struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &lease); err != nil || strings.TrimSpace(lease.Token) == "" {
		return fmt.Errorf("reissue GitHub credential lease: broker returned an invalid lease")
	}
	c.request.Lease = lease.Token
	return nil
}

func (c *githubCredentialBrokerClient) post(
	ctx context.Context,
	endpoint string,
	payload githubBrokerResolveRequest,
) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode GitHub credential request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create GitHub credential request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body := formatBrokerErrorBody(resp.Body)
		return nil, &githubBrokerHTTPError{status: resp.StatusCode, code: brokerErrorCode(body), body: body}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func brokerErrorCode(body string) string {
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(body, ": ")), &payload); err != nil {
		return ""
	}
	return payload.Code
}

func isReissuableLeaseError(err error) bool {
	var brokerErr *githubBrokerHTTPError
	if !errors.As(err, &brokerErr) || brokerErr.status != http.StatusUnauthorized {
		return false
	}
	switch brokerErr.code {
	case "github_credential_lease_invalid", "github_credential_lease_expired", "github_credential_lease_revoked",
		"git_credential_lease_invalid", "git_credential_lease_expired", "git_credential_lease_revoked":
		return true
	default:
		return false
	}
}

func githubCredentialReissueEndpoint(raw string) (string, error) {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Host == "" || !strings.HasSuffix(endpoint.Path, "/resolve") {
		return "", fmt.Errorf("GitHub credential broker URL is invalid")
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/resolve") + "/reissue"
	return endpoint.String(), nil
}

func runGitHubCredentialHelper(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	getenv func(string) string,
	httpClient *http.Client,
) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: agentctl git-credential <get|store|erase>")
	}
	switch args[0] {
	case "store", "erase":
		return nil
	case "get":
	default:
		return fmt.Errorf("unsupported git credential operation %q", args[0])
	}

	input, err := readGitCredentialInput(stdin)
	if err != nil {
		return err
	}
	client, err := newGitHubCredentialBrokerClientForInput(getenv, httpClient, input)
	if err != nil {
		return err
	}
	request, err := gitCredentialRequestForInput(input, client.request)
	if err != nil {
		return err
	}
	client.request = request
	credential, err := client.resolveWithReissue(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "username=%s\npassword=%s\n\n", credential.Username, credential.Password)
	return err
}

func newGitHubCredentialBrokerClientForInput(
	getenv func(string) string,
	httpClient *http.Client,
	input map[string]string,
) (*githubCredentialBrokerClient, error) {
	rawScopes := strings.TrimSpace(getenv(githubauth.CredentialScopesEnv))
	if rawScopes == "" {
		return newGitHubCredentialBrokerClient(getenv, httpClient)
	}
	var scopes []githubBrokerResolveRequest
	if err := json.Unmarshal([]byte(rawScopes), &scopes); err != nil {
		return nil, fmt.Errorf("GitHub credential scopes are invalid: %w", err)
	}
	for _, scope := range scopes {
		request, err := gitCredentialRequestForInput(input, scope)
		if err == nil {
			return newGitHubCredentialBrokerClientForRequest(getenv, httpClient, request)
		}
	}
	return nil, fmt.Errorf("git repository does not match any credential lease scope")
}

func newGitHubCredentialBrokerClientForRequest(
	getenv func(string) string,
	httpClient *http.Client,
	request githubBrokerResolveRequest,
) (*githubCredentialBrokerClient, error) {
	endpoint := strings.TrimSpace(getenv(githubauth.CredentialBrokerURLEnv))
	for name, value := range map[string]string{
		"broker URL": endpoint, "lease": request.Lease, "task": request.TaskID,
		"session": request.SessionID, "repository": request.RepositoryID,
		"owner": request.Owner, "repo": request.Repo, "host": request.Host,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("GitHub credential %s is not configured", name)
		}
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("GitHub credential broker URL is invalid")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &githubCredentialBrokerClient{endpoint: endpoint, request: request, http: httpClient}, nil
}

func readGitCredentialInput(input io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(input, 64<<10))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read git credential request: %w", err)
	}
	return values, nil
}

// gitCredentialRequestForInput validates the Git credential helper request
// against its issued lease scope, then carries the issued repository path
// through to the broker. Git uses owner/repo fields on this compatibility
// endpoint, so Repo retains any provider namespace after the first segment.
func gitCredentialRequestForInput(input map[string]string, scope githubBrokerResolveRequest) (githubBrokerResolveRequest, error) {
	protocol := strings.TrimSpace(input["protocol"])
	if !strings.EqualFold(protocol, "https") {
		return githubBrokerResolveRequest{}, fmt.Errorf("git credential protocol %q is not supported", protocol)
	}
	host := strings.TrimSpace(input["host"])
	if host == "" || !strings.EqualFold(host, scope.Host) {
		return githubBrokerResolveRequest{}, fmt.Errorf("git credential host does not match credential lease scope")
	}
	path, err := url.PathUnescape(strings.TrimSpace(input["path"]))
	if err != nil {
		return githubBrokerResolveRequest{}, fmt.Errorf("decode git credential path: %w", err)
	}
	path = "/" + strings.TrimLeft(path, "/")
	if path == "/" {
		return githubBrokerResolveRequest{}, fmt.Errorf("git repository does not match credential lease scope")
	}
	if scope.Path != "" {
		// The scope path comes from a clone URL and keeps its ".git" suffix,
		// while the gh CLI shim asks for a bare "/<owner>/<repo>". Compare the
		// canonical spelling so both reach the same lease. The broker compares
		// the same way, so the exact issued path is what travels onward.
		if githubauth.CanonicalCredentialPath(path) != githubauth.CanonicalCredentialPath(scope.Path) {
			return githubBrokerResolveRequest{}, fmt.Errorf("git repository does not match credential lease scope")
		}
		path = scope.Path
	} else {
		legacyPath := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
		owner, repo, found := strings.Cut(legacyPath, "/")
		if !found || owner == "" || repo == "" || legacyPath != scope.Owner+"/"+scope.Repo {
			return githubBrokerResolveRequest{}, fmt.Errorf("git repository does not match credential lease scope")
		}
		scope.Owner = owner
		scope.Repo = repo
	}
	scope.Path = path
	return scope, nil
}
