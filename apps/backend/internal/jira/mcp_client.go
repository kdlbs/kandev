package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient implements the Client interface by calling Jira APIs through
// the Atlassian MCP server (mcp.atlassian.com/v1/mcp/authv2) using JSON-RPC
// over the MCP Streamable HTTP transport. The OAuth token from the MCP
// OAuth 2.1 flow authenticates the session — it does NOT work for direct
// Jira REST API calls, so we route through the MCP server.
type MCPClient struct {
	http        *http.Client
	accessToken string
	tokenMu     sync.RWMutex // guards accessToken
	workspaceID string
	siteURL     string
	sessionMu   sync.Mutex // guards SDK client initialization and replacement
	session     *mcpclient.Client
	cloudID     string
	refresher   TokenRefresher
}

const (
	mcpEndpoint = "https://mcp.atlassian.com/v1/mcp/authv2"
	mcpProtocol = "2025-06-18"
)

// NewMCPClient builds an MCP-backed Jira client.
func NewMCPClient(accessToken, cloudID, workspaceID, siteURL string) *MCPClient {
	site := strings.TrimRight(siteURL, "/")
	return &MCPClient{
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &mcpStatusRoundTripper{base: http.DefaultTransport},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		accessToken: accessToken,
		workspaceID: workspaceID,
		siteURL:     site,
		cloudID:     cloudID,
	}
}

func (c *MCPClient) getToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.accessToken
}

func (c *MCPClient) setToken(t string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.accessToken = t
}

// SetRefresher wires the OAuth token refresher for 401 auto-refresh.
func (c *MCPClient) SetRefresher(workspaceID string, r TokenRefresher) {
	c.workspaceID = workspaceID
	c.refresher = r
}

type mcpAccessTokenContextKey struct{}

type mcpStatusRoundTripper struct {
	base http.RoundTripper
}

func (t *mcpStatusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return nil, &APIError{StatusCode: http.StatusUnauthorized, Message: string(body)}
}

func withMCPAccessToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, mcpAccessTokenContextKey{}, token)
}

// call sends a JSON-RPC request. On 401, refreshes token and retries once.
func (c *MCPClient) call(ctx context.Context, method string, params interface{}) (string, error) {
	staleToken := c.getToken()
	requestCtx := withMCPAccessToken(ctx, staleToken)
	result, err := c.callOnce(requestCtx, method, params)
	if err == nil {
		return result, nil
	}
	if errors.Is(err, mcptransport.ErrSessionTerminated) {
		c.resetSession()
		return c.callOnce(requestCtx, method, params)
	}
	if c.tryRefreshOn401(ctx, err, staleToken) {
		return c.callOnce(withMCPAccessToken(ctx, c.getToken()), method, params)
	}
	return "", err
}

// tryRefreshOn401 refreshes the OAuth token on 401 and resets the session.
// Returns true if the caller should retry.
func (c *MCPClient) tryRefreshOn401(ctx context.Context, err error, staleToken string) bool {
	if c.refresher == nil {
		return false
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		return false
	}
	if refreshErr := c.refresher.RefreshOAuthToken(ctx, c.workspaceID, staleToken); refreshErr != nil {
		return false
	}
	newToken, revealErr := c.refresher.RevealAccessToken(ctx, c.workspaceID)
	if revealErr != nil {
		return false
	}
	c.setToken(newToken)
	c.resetSession()
	return true
}

func (c *MCPClient) callOnce(ctx context.Context, method string, params interface{}) (string, error) {
	client, err := c.ensureSession(ctx)
	if err != nil {
		return "", err
	}
	if method != "tools/call" {
		return "", fmt.Errorf("unsupported MCP method %q", method)
	}
	requestParams, ok := params.(map[string]interface{})
	if !ok {
		return "", errors.New("MCP tools/call params must be an object")
	}
	toolName, _ := requestParams["name"].(string)
	if toolName == "" {
		return "", errors.New("MCP tool name is required")
	}
	result, err := client.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name: toolName, Arguments: requestParams["arguments"],
	}})
	if err != nil {
		return "", err
	}
	text := firstMCPText(result)
	if result.IsError {
		if text != "" {
			return "", fmt.Errorf("MCP tool error: %s", text)
		}
		return "", errors.New("MCP tool error (no details)")
	}
	return text, nil
}

func firstMCPText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		switch text := content.(type) {
		case mcp.TextContent:
			return text.Text
		case *mcp.TextContent:
			return text.Text
		}
	}
	return ""
}

// ensureSession creates and initializes the SDK client once. The SDK owns
// protocol negotiation, initialized notification, response demultiplexing,
// and the Streamable HTTP session identifier.
func (c *MCPClient) ensureSession(ctx context.Context) (*mcpclient.Client, error) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	client, err := mcpclient.NewStreamableHttpClient(
		mcpEndpoint,
		mcptransport.WithHTTPBasicClient(c.http),
		mcptransport.WithHTTPHeaderFunc(func(requestCtx context.Context) map[string]string {
			token, _ := requestCtx.Value(mcpAccessTokenContextKey{}).(string)
			if token == "" {
				token = c.getToken()
			}
			return map[string]string{"Authorization": "Bearer " + token}
		}),
	)
	if err != nil {
		return nil, err
	}
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("MCP start: %w", err)
	}
	if _, err := client.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcpProtocol,
		Capabilities:    mcp.ClientCapabilities{},
		ClientInfo:      mcp.Implementation{Name: "kandev", Version: "1.0"},
	}}); err != nil {
		return nil, fmt.Errorf("MCP initialize: %w", err)
	}
	c.session = client
	return client, nil
}

func (c *MCPClient) resetSession() {
	c.sessionMu.Lock()
	previous := c.session
	c.session = nil
	c.sessionMu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
}

// --- Client interface ---

func (c *MCPClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      "atlassianUserInfo",
		"arguments": map[string]interface{}{},
	})
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	var user struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal([]byte(result), &user); err != nil {
		return &TestConnectionResult{OK: true}, nil
	}
	return &TestConnectionResult{OK: true, DisplayName: user.Name, Email: user.Email, AccountID: user.AccountID}, nil
}

func (c *MCPClient) GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error) {
	type issueResult struct {
		ticket *JiraTicket
		err    error
	}
	type transResult struct {
		transitions []JiraTransition
		err         error
	}
	issueCh := make(chan issueResult, 1)
	transCh := make(chan transResult, 1)
	go func() {
		result, err := c.call(ctx, "tools/call", map[string]interface{}{
			"name": "getJiraIssue",
			"arguments": map[string]interface{}{
				"cloudId":      c.cloudID,
				"issueIdOrKey": ticketKey,
			},
		})
		if err != nil {
			issueCh <- issueResult{nil, err}
			return
		}
		t, err := parseMCPIssue(result, c.siteURL)
		issueCh <- issueResult{t, err}
	}()
	go func() {
		trans, err := c.listTransitions(ctx, ticketKey)
		transCh <- transResult{trans, err}
	}()
	ir := <-issueCh
	if ir.err != nil {
		return nil, ir.err
	}
	tr := <-transCh
	if tr.err == nil {
		ir.ticket.Transitions = tr.transitions
	}
	return ir.ticket, nil
}

func (c *MCPClient) listTransitions(ctx context.Context, ticketKey string) ([]JiraTransition, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "getTransitionsForJiraIssue",
		"arguments": map[string]interface{}{
			"cloudId":      c.cloudID,
			"issueIdOrKey": ticketKey,
		},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("decode transitions: %w", err)
	}
	out := make([]JiraTransition, 0, len(resp.Transitions))
	for _, t := range resp.Transitions {
		out = append(out, JiraTransition{
			ID:           t.ID,
			Name:         t.Name,
			ToStatusID:   t.To.ID,
			ToStatusName: t.To.Name,
		})
	}
	return out, nil
}

func (c *MCPClient) ListTransitions(ctx context.Context, ticketKey string) ([]JiraTransition, error) {
	return c.listTransitions(ctx, ticketKey)
}

func (c *MCPClient) DoTransition(ctx context.Context, ticketKey, transitionID string) error {
	_, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "transitionJiraIssue",
		"arguments": map[string]interface{}{
			"cloudId":      c.cloudID,
			"issueIdOrKey": ticketKey,
			"transition":   map[string]string{"id": transitionID},
		},
	})
	return err
}

func (c *MCPClient) ListProjects(ctx context.Context) ([]JiraProject, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "getVisibleJiraProjects",
		"arguments": map[string]interface{}{
			"cloudId": c.cloudID,
		},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Values []struct {
			Key  string `json:"key"`
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("decode projects: %w", err)
	}
	out := make([]JiraProject, 0, len(resp.Values))
	for _, p := range resp.Values {
		out = append(out, JiraProject{Key: p.Key, Name: p.Name, ID: p.ID})
	}
	return out, nil
}

// ListProjectStatuses fetches project workflow statuses. The MCP server
// doesn't expose a direct /project/{key}/statuses endpoint, so we use
// getJiraProjectIssueTypesMetadata which returns issue types with their
// available statuses. We flatten and dedupe like CloudClient does.
func (c *MCPClient) ListProjectStatuses(ctx context.Context, projectKey string) ([]JiraStatus, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "getJiraProjectIssueTypesMetadata",
		"arguments": map[string]interface{}{
			"cloudId":        c.cloudID,
			"projectIdOrKey": projectKey,
		},
	})
	if err != nil {
		return nil, err
	}
	// The response is an array of issue types, each with a "statuses" array.
	var body []struct {
		Statuses []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal([]byte(result), &body); err != nil {
		return nil, fmt.Errorf("decode project statuses: %w", err)
	}
	out := make([]JiraStatus, 0)
	seen := make(map[string]struct{})
	for _, it := range body {
		for _, s := range it.Statuses {
			if _, dup := seen[s.ID]; dup {
				continue
			}
			seen[s.ID] = struct{}{}
			out = append(out, JiraStatus{
				ID:             s.ID,
				Name:           s.Name,
				StatusCategory: s.StatusCategory.Key,
			})
		}
	}
	return out, nil
}

func (c *MCPClient) SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	args := map[string]interface{}{
		"cloudId":    c.cloudID,
		"jql":        jql,
		"maxResults": maxResults,
	}
	if pageToken != "" {
		args["nextPageToken"] = pageToken
	}
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      "searchJiraIssuesUsingJql",
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	return parseMCPSearchResult(result, c.siteURL)
}

// SearchTicketsForWatch uses the same MCP search tool because the remote MCP
// service owns the result field selection. Description may be empty when the
// MCP service does not include it by default; see
// docs/specs/integrations/system-design/jira-watcher-task-prompts.md.
func (c *MCPClient) SearchTicketsForWatch(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	return c.SearchTickets(ctx, jql, pageToken, maxResults)
}

// mcpIssue mirrors the subset of the Jira issue payload from the MCP tool.
type mcpIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary     string      `json:"summary"`
		Description interface{} `json:"description"` // ADF or string
		Updated     string      `json:"updated"`
		Status      struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		Issuetype struct {
			Name    string `json:"name"`
			IconURL string `json:"iconUrl"`
		} `json:"issuetype"`
		Priority struct {
			Name    string `json:"name"`
			IconURL string `json:"iconUrl"`
		} `json:"priority"`
		Assignee *mcpUser `json:"assignee"`
		Reporter *mcpUser `json:"reporter"`
	} `json:"fields"`
}

type mcpUser struct {
	DisplayName string `json:"displayName"`
	AvatarURLs  struct {
		Size24 string `json:"24x24"`
		Size32 string `json:"32x32"`
	} `json:"avatarUrls"`
}

func (u *mcpUser) avatar() string {
	if u == nil {
		return ""
	}
	if u.AvatarURLs.Size24 != "" {
		return u.AvatarURLs.Size24
	}
	return u.AvatarURLs.Size32
}

func (u *mcpUser) name() string {
	if u == nil {
		return ""
	}
	return u.DisplayName
}

// parseMCPIssue converts the MCP tool response into a JiraTicket.
func parseMCPIssue(raw, siteURL string) (*JiraTicket, error) {
	var issue mcpIssue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}
	return &JiraTicket{
		ID:             issue.ID,
		Key:            issue.Key,
		Summary:        issue.Fields.Summary,
		Description:    extractDescription(issue.Fields.Description),
		StatusID:       issue.Fields.Status.ID,
		StatusName:     issue.Fields.Status.Name,
		StatusCategory: issue.Fields.Status.StatusCategory.Key,
		ProjectKey:     issue.Fields.Project.Key,
		IssueType:      issue.Fields.Issuetype.Name,
		IssueTypeIcon:  issue.Fields.Issuetype.IconURL,
		Priority:       issue.Fields.Priority.Name,
		PriorityIcon:   issue.Fields.Priority.IconURL,
		AssigneeName:   issue.Fields.Assignee.name(),
		AssigneeAvatar: issue.Fields.Assignee.avatar(),
		ReporterName:   issue.Fields.Reporter.name(),
		ReporterAvatar: issue.Fields.Reporter.avatar(),
		Updated:        issue.Fields.Updated,
		URL:            siteURL + "/browse/" + issue.Key,
	}, nil
}

func parseMCPSearchResult(raw, siteURL string) (*SearchResult, error) {
	var resp struct {
		Issues        []json.RawMessage `json:"issues"`
		IsLast        bool              `json:"isLast"`
		NextPageToken string            `json:"nextPageToken"`
		MaxResults    int               `json:"maxResults"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("decode search result: %w", err)
	}
	tickets := make([]JiraTicket, 0, len(resp.Issues))
	for _, raw := range resp.Issues {
		ticket, err := parseMCPIssue(string(raw), siteURL)
		if err != nil {
			continue
		}
		tickets = append(tickets, *ticket)
	}
	return &SearchResult{
		Tickets:       tickets,
		MaxResults:    resp.MaxResults,
		IsLast:        resp.IsLast,
		NextPageToken: resp.NextPageToken,
	}, nil
}
