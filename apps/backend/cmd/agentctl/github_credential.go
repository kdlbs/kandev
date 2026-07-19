package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/githubauth"
)

type githubBrokerResolveRequest struct {
	Lease        string `json:"lease"`
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Host         string `json:"host"`
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

func newGitHubCredentialBrokerClient(
	getenv func(string) string,
	httpClient *http.Client,
) (*githubCredentialBrokerClient, error) {
	request := githubBrokerResolveRequest{
		Lease:        strings.TrimSpace(getenv(githubauth.CredentialLeaseEnv)),
		TaskID:       strings.TrimSpace(getenv(githubauth.CredentialTaskIDEnv)),
		SessionID:    strings.TrimSpace(getenv(githubauth.CredentialSessionIDEnv)),
		RepositoryID: strings.TrimSpace(getenv(githubauth.CredentialRepositoryEnv)),
		Owner:        strings.TrimSpace(getenv(githubauth.CredentialOwnerEnv)),
		Repo:         strings.TrimSuffix(strings.TrimSpace(getenv(githubauth.CredentialRepoEnv)), ".git"),
		Host:         strings.ToLower(strings.TrimSpace(getenv(githubauth.CredentialHostEnv))),
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

func (c *githubCredentialBrokerClient) resolve(ctx context.Context) (*githubBrokerCredential, error) {
	body, err := json.Marshal(c.request)
	if err != nil {
		return nil, fmt.Errorf("encode GitHub credential request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create GitHub credential request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve GitHub credential: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("resolve GitHub credential: broker returned HTTP %d", resp.StatusCode)
	}
	var credential githubBrokerCredential
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&credential); err != nil {
		return nil, fmt.Errorf("decode GitHub credential: %w", err)
	}
	if credential.Username == "" || credential.Password == "" {
		return nil, fmt.Errorf("resolve GitHub credential: broker returned an empty credential")
	}
	return &credential, nil
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
	if err := validateGitCredentialScope(input, client.request); err != nil {
		return err
	}
	credential, err := client.resolve(ctx)
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
		if validateGitCredentialScope(input, scope) == nil {
			return newGitHubCredentialBrokerClientForRequest(getenv, httpClient, scope)
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

func validateGitCredentialScope(input map[string]string, scope githubBrokerResolveRequest) error {
	protocol := strings.TrimSpace(input["protocol"])
	if !strings.EqualFold(protocol, "https") {
		return fmt.Errorf("git credential protocol %q is not supported", protocol)
	}
	host := strings.TrimSpace(input["host"])
	if host == "" || !strings.EqualFold(host, scope.Host) {
		return fmt.Errorf("git credential host does not match credential lease scope")
	}
	path, err := url.PathUnescape(strings.Trim(input["path"], "/"))
	if err != nil {
		return fmt.Errorf("decode git credential path: %w", err)
	}
	path = strings.TrimSuffix(path, ".git")
	if path == "" || !strings.EqualFold(path, scope.Owner+"/"+scope.Repo) {
		return fmt.Errorf("git repository does not match credential lease scope")
	}
	return nil
}
