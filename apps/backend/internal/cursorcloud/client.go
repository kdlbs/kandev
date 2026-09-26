package cursorcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const APIOrigin = "https://api.cursor.com"

const (
	defaultRequestTimeout = 30 * time.Second
	maxResponseBytes      = 4 << 20
	maxReadRetries        = 2
	schemeHTTP            = "http"
	agentIDConflictCode   = "agent_id_conflict"
)

var (
	ErrOutcomeUnknown      = errors.New("cursor cloud submission outcome is unknown")
	ErrUnsupportedContract = errors.New("cursor cloud API contract is unsupported")
	agentIDPattern         = regexp.MustCompile(`^bc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type Client struct {
	apiKey       string
	httpClient   *http.Client
	streamClient *http.Client
	baseURL      string
	configError  error
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
}

type ClientConfig struct {
	APIKey     string
	APIOrigin  string
	HTTPClient *http.Client
	Now        func() time.Time
	Sleep      func(context.Context, time.Duration) error
}

type Prompt struct {
	Text string `json:"text"`
}

type ModelSelection struct {
	ID     string       `json:"id"`
	Params []ModelParam `json:"params,omitempty"`
}

type ModelParam struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type Repository struct {
	URL         string `json:"url"`
	StartingRef string `json:"startingRef,omitempty"`
}

type MCPServer struct {
	Name    string            `json:"name"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

type CreateAgentRequest struct {
	AgentID      string          `json:"agentId"`
	Prompt       Prompt          `json:"prompt"`
	Model        *ModelSelection `json:"model,omitempty"`
	Name         string          `json:"name,omitempty"`
	Repos        []Repository    `json:"repos"`
	AutoCreatePR bool            `json:"autoCreatePR"`
	MCPServers   []MCPServer     `json:"mcpServers"`
}

type CreateAgentResponse struct {
	Agent Agent `json:"agent"`
	Run   Run   `json:"run"`
}

type CreateRunRequest struct {
	Prompt     Prompt      `json:"prompt"`
	MCPServers []MCPServer `json:"mcpServers"`
}

type Agent struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Status              string       `json:"status"`
	URL                 string       `json:"url"`
	LatestRunID         string       `json:"latestRunId"`
	Repos               []Repository `json:"repos,omitempty"`
	WorkOnCurrentBranch bool         `json:"workOnCurrentBranch"`
	AutoCreatePR        bool         `json:"autoCreatePR"`
}

type Run struct {
	ID         string     `json:"id"`
	AgentID    string     `json:"agentId"`
	Status     string     `json:"status"`
	CreatedAt  string     `json:"createdAt"`
	UpdatedAt  string     `json:"updatedAt"`
	DurationMs int64      `json:"durationMs,omitempty"`
	Result     string     `json:"result,omitempty"`
	Git        *GitResult `json:"git,omitempty"`
}

type RunPage struct {
	Items      []Run  `json:"items"`
	NextCursor string `json:"nextCursor"`
}

type Model struct {
	ID          string           `json:"id"`
	DisplayName string           `json:"displayName"`
	Description string           `json:"description,omitempty"`
	Aliases     []string         `json:"aliases,omitempty"`
	Parameters  []ModelParameter `json:"parameters,omitempty"`
	Variants    []ModelVariant   `json:"variants,omitempty"`
}

type ModelParameter struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"displayName,omitempty"`
	Values      []ModelParameterValue `json:"values,omitempty"`
}

type ModelParameterValue struct {
	Value       string `json:"value"`
	DisplayName string `json:"displayName,omitempty"`
}

type ModelVariant struct {
	Params      []ModelParam `json:"params"`
	DisplayName string       `json:"displayName"`
	Description string       `json:"description,omitempty"`
	IsDefault   bool         `json:"isDefault,omitempty"`
}

type ModelCatalog struct {
	Items []Model `json:"items"`
}

type RepositoryCatalog struct {
	Items []Repository `json:"items"`
}

type GitResult struct {
	Branches []GitBranch `json:"branches"`
}

type GitBranch struct {
	RepositoryURL string `json:"repoUrl"`
	Branch        string `json:"branch,omitempty"`
	PRURL         string `json:"prUrl,omitempty"`
}

func NewClient(apiKey string, httpClient *http.Client) *Client {
	client, err := NewClientWithConfig(ClientConfig{APIKey: apiKey, HTTPClient: httpClient})
	if err != nil {
		return &Client{configError: err}
	}
	return client
}

// NewClientWithConfig injects transport and time dependencies. APIOrigin is
// normally omitted; backend composition may set it only for its gated E2E
// fixture transport.
func NewClientWithConfig(config ClientConfig) (*Client, error) {
	origin := config.APIOrigin
	if origin == "" {
		origin = APIOrigin
	}
	client, err := newClient(config.APIKey, config.HTTPClient, origin)
	if err != nil {
		return nil, err
	}
	if config.Now != nil {
		client.now = config.Now
	}
	if config.Sleep != nil {
		client.sleep = config.Sleep
	}
	return client, nil
}

func newClient(apiKey string, httpClient *http.Client, baseURL string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid Cursor Cloud API origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("cursor cloud API origin must not include a path")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != schemeHTTP || !isLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("cursor cloud API origin must use HTTPS")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	baseURL = strings.TrimSuffix(parsed.String(), "/")

	baseClient := httpClient
	if baseClient == nil {
		baseClient = &http.Client{Timeout: defaultRequestTimeout}
	}
	requestClient := cloneHTTPClient(baseClient)
	streamClient := cloneHTTPClient(baseClient)
	streamClient.Timeout = 0
	return &Client{
		apiKey:       apiKey,
		httpClient:   requestClient,
		streamClient: streamClient,
		baseURL:      baseURL,
		now:          time.Now,
		sleep:        sleepContext,
	}, nil
}

func cloneHTTPClient(client *http.Client) *http.Client {
	clone := *client
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) CreateAgent(ctx context.Context, input CreateAgentRequest) (CreateAgentResponse, error) {
	if c.configError != nil {
		return CreateAgentResponse{}, c.configError
	}
	if !agentIDPattern.MatchString(input.AgentID) {
		return CreateAgentResponse{}, errors.New("cursor cloud agent ID must use the bc-UUID format")
	}
	if strings.TrimSpace(input.Prompt.Text) == "" {
		return CreateAgentResponse{}, errors.New("cursor cloud prompt text is required")
	}
	if len(input.Repos) != 1 {
		return CreateAgentResponse{}, errors.New("cursor cloud requires exactly one repository")
	}
	if len(input.MCPServers) == 0 {
		return CreateAgentResponse{}, errors.New("cursor cloud requires a scoped MCP server")
	}
	body := struct {
		AgentID             string          `json:"agentId"`
		Prompt              Prompt          `json:"prompt"`
		Model               *ModelSelection `json:"model,omitempty"`
		Name                string          `json:"name,omitempty"`
		Repos               []Repository    `json:"repos"`
		WorkOnCurrentBranch bool            `json:"workOnCurrentBranch"`
		AutoCreatePR        bool            `json:"autoCreatePR"`
		MCPServers          []MCPServer     `json:"mcpServers"`
	}{
		AgentID:             input.AgentID,
		Prompt:              input.Prompt,
		Model:               input.Model,
		Name:                input.Name,
		Repos:               input.Repos,
		WorkOnCurrentBranch: false,
		AutoCreatePR:        input.AutoCreatePR,
		MCPServers:          input.MCPServers,
	}
	var result CreateAgentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/agents", body, &result, true); err != nil {
		return CreateAgentResponse{}, err
	}
	if result.Agent.ID != input.AgentID || result.Run.AgentID != input.AgentID || result.Agent.ID == "" || result.Run.ID == "" {
		return CreateAgentResponse{}, &ContractError{Detail: "create response did not confirm the submitted agent and run identities", Unknown: true}
	}
	return result, nil
}

func (c *Client) CreateRun(ctx context.Context, agentID string, input CreateRunRequest) (Run, error) {
	if !agentIDPattern.MatchString(agentID) {
		return Run{}, errors.New("cursor cloud agent ID must use the bc-UUID format")
	}
	if strings.TrimSpace(input.Prompt.Text) == "" {
		return Run{}, errors.New("cursor cloud prompt text is required")
	}
	if input.MCPServers == nil {
		return Run{}, errors.New("follow-up MCP servers must be supplied to replace the previous grant")
	}
	var response struct {
		Run Run `json:"run"`
	}
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs"
	if err := c.doJSON(ctx, http.MethodPost, path, input, &response, true); err != nil {
		return Run{}, err
	}
	if response.Run.ID == "" || response.Run.AgentID != agentID {
		return Run{}, &ContractError{Detail: "follow-up response did not confirm the requested agent and run identities", Unknown: true}
	}
	return response.Run, nil
}

func (c *Client) GetRun(ctx context.Context, agentID, runID string) (Run, error) {
	if !agentIDPattern.MatchString(agentID) || runID == "" {
		return Run{}, errors.New("cursor cloud agent and run IDs are required")
	}
	var run Run
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &run, false); err != nil {
		return Run{}, err
	}
	if run.ID != runID || run.AgentID != agentID {
		return Run{}, &ContractError{Detail: "run response identity did not match the requested run"}
	}
	return run, nil
}

func (c *Client) GetAgent(ctx context.Context, agentID string) (Agent, error) {
	if !agentIDPattern.MatchString(agentID) {
		return Agent{}, errors.New("cursor cloud agent ID must use the bc-UUID format")
	}
	var agent Agent
	path := "/v1/agents/" + url.PathEscape(agentID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &agent, false); err != nil {
		return Agent{}, err
	}
	if agent.ID != agentID {
		return Agent{}, &ContractError{Detail: "agent response identity did not match the requested agent"}
	}
	return agent, nil
}

func (c *Client) CancelRun(ctx context.Context, agentID, runID string) error {
	if !agentIDPattern.MatchString(agentID) || runID == "" {
		return errors.New("cursor cloud agent and run IDs are required")
	}
	var response struct {
		ID string `json:"id"`
	}
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID) + "/cancel"
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &response, true); err != nil {
		return err
	}
	if response.ID != runID {
		return &ContractError{Detail: "cancel response did not confirm the requested run", Unknown: true}
	}
	return nil
}

func (c *Client) ListRuns(ctx context.Context, agentID string, limit int, cursor string) (RunPage, error) {
	if !agentIDPattern.MatchString(agentID) {
		return RunPage{}, errors.New("cursor cloud agent ID must use the bc-UUID format")
	}
	if limit < 0 || limit > 100 {
		return RunPage{}, errors.New("cursor cloud run limit must be between 1 and 100")
	}
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var page RunPage
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &page, false); err != nil {
		return RunPage{}, err
	}
	for _, run := range page.Items {
		if run.AgentID != agentID || run.ID == "" {
			return RunPage{}, &ContractError{Detail: "run list contained an invalid run identity"}
		}
	}
	return page, nil
}

func (c *Client) ListModels(ctx context.Context) (ModelCatalog, error) {
	var catalog ModelCatalog
	if err := c.doJSON(ctx, http.MethodGet, "/v1/models", nil, &catalog, false); err != nil {
		return ModelCatalog{}, err
	}
	for _, model := range catalog.Items {
		if model.ID == "" || model.DisplayName == "" {
			return ModelCatalog{}, &ContractError{Detail: "model catalog contained a model without an identity or display name"}
		}
	}
	return catalog, nil
}

func (c *Client) ListRepositories(ctx context.Context) (RepositoryCatalog, error) {
	var catalog RepositoryCatalog
	if err := c.doJSON(ctx, http.MethodGet, "/v1/repositories", nil, &catalog, false); err != nil {
		return RepositoryCatalog{}, err
	}
	for _, repository := range catalog.Items {
		if repository.URL == "" {
			return RepositoryCatalog{}, &ContractError{Detail: "repository catalog contained an entry without a URL"}
		}
	}
	return catalog, nil
}

type ContractError struct {
	Detail  string
	Unknown bool
}

type APIError struct {
	StatusCode int
	Code       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("Cursor Cloud API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("Cursor Cloud API returned HTTP %d (%s)", e.StatusCode, e.Code)
}

func (e *APIError) Is(target error) bool {
	return target == ErrOutcomeUnknown && e.StatusCode == http.StatusConflict && e.Code == agentIDConflictCode
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnsupportedContract, e.Detail)
}

func (e *ContractError) Is(target error) bool {
	return target == ErrUnsupportedContract || e.Unknown && target == ErrOutcomeUnknown
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, output any, mutation bool) error {
	if c.configError != nil {
		return c.configError
	}
	if c.apiKey == "" {
		return errors.New("cursor cloud API key is required")
	}
	var encoded []byte
	hasBody := body != nil
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Cursor Cloud request: %w", err)
		}
	}
	safeRead := !mutation && method == http.MethodGet
	for attempt := 0; ; attempt++ {
		delay, retry, err := c.doJSONAttempt(ctx, method, path, encoded, hasBody, output, mutation, safeRead, attempt, attempt < maxReadRetries)
		if err != nil || !retry {
			return err
		}
		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func (c *Client) doJSONAttempt(ctx context.Context, method, path string, encoded []byte, hasBody bool, output any, mutation, safeRead bool, attempt int, canRetry bool) (time.Duration, bool, error) {
	request, err := newJSONRequest(ctx, method, c.baseURL+path, encoded, hasBody, c.apiKey)
	if err != nil {
		return 0, false, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return handleJSONTransportError(ctx, method, path, err, mutation, safeRead, attempt, canRetry)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := readBounded(response.Body, maxResponseBytes)
	if err != nil {
		if mutation {
			return 0, false, unknownSubmissionError(method, path)
		}
		return 0, false, fmt.Errorf("read Cursor Cloud response: %w", err)
	}
	return c.handleJSONResponse(response, responseBody, method, path, output, mutation, safeRead, attempt, canRetry)
}

func newJSONRequest(ctx context.Context, method, url string, encoded []byte, hasBody bool, apiKey string) (*http.Request, error) {
	var requestBody io.Reader
	if encoded != nil {
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("create Cursor Cloud request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func handleJSONTransportError(
	ctx context.Context, method, path string, err error, mutation, safeRead bool, attempt int, canRetry bool,
) (time.Duration, bool, error) {
	if safeRead && canRetry && ctx.Err() == nil {
		return retryBackoff(attempt), true, nil
	}
	if mutation {
		return 0, false, unknownSubmissionError(method, path)
	}
	return 0, false, fmt.Errorf("cursor cloud request failed: %w", err)
}

func (c *Client) handleJSONResponse(
	response *http.Response, responseBody []byte, method, path string, output any,
	mutation, safeRead bool, attempt int, canRetry bool,
) (time.Duration, bool, error) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return c.handleJSONStatus(response, responseBody, method, path, mutation, safeRead, attempt, canRetry)
	}
	if output == nil || len(responseBody) == 0 {
		return 0, false, nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		unknown := mutation
		return 0, false, &ContractError{Detail: "successful response was not valid JSON", Unknown: unknown}
	}
	return 0, false, nil
}

func (c *Client) handleJSONStatus(
	response *http.Response, responseBody []byte, method, path string,
	mutation, safeRead bool, attempt int, canRetry bool,
) (time.Duration, bool, error) {
	apiErr := decodeAPIError(response.StatusCode, response.Header, responseBody, c.now())
	if safeRead && canRetry && retryableStatus(response.StatusCode) {
		return retryDelay(attempt, response.Header.Get("Retry-After"), c.now()), true, nil
	}
	if mutation && response.StatusCode >= http.StatusInternalServerError {
		return 0, false, unknownSubmissionError(method, path)
	}
	return 0, false, apiErr
}

func unknownSubmissionError(method, path string) error {
	return fmt.Errorf("%w: %s", ErrOutcomeUnknown, operationName(method, path))
}

func readBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("response exceeds configured limit")
	}
	return data, nil
}

func operationName(method, path string) string {
	if method == http.MethodPost && path == "/v1/agents" {
		return "create agent"
	}
	if method == http.MethodPost && strings.HasSuffix(path, "/runs") {
		return "create run"
	}
	return "submit request"
}

func decodeAPIError(status int, headers http.Header, body []byte, now time.Time) *APIError {
	var payload struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &payload)
	return &APIError{
		StatusCode: status,
		Code:       safeAPIErrorCode(payload.Code),
		RetryAfter: parseRetryAfter(headers.Get("Retry-After"), now),
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryDelay(attempt int, retryAfter string, now time.Time) time.Duration {
	if strings.TrimSpace(retryAfter) != "" {
		if delay := parseRetryAfter(retryAfter, now); delay > 0 || strings.TrimSpace(retryAfter) == "0" {
			return delay
		}
	}
	return retryBackoff(attempt)
}

func retryBackoff(attempt int) time.Duration {
	return 200 * time.Millisecond * time.Duration(1<<attempt)
}

func safeAPIErrorCode(code string) string {
	switch code {
	case agentIDConflictCode, "agent_busy", "run_not_cancellable", "stream_expired", "invalid_last_event_id", "not_found", "unauthorized", "forbidden", "rate_limited", "invalid_request", "model_not_found", "repository_not_found":
		return code
	default:
		return ""
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
