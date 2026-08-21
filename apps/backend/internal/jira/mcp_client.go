package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
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
	sessionMu   sync.Mutex // guards session init
	sessionID   string
	initialized bool
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
			Timeout: 30 * time.Second,
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

type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
}

type mcpResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
}

// call sends a JSON-RPC request. On 401, refreshes token and retries once.
func (c *MCPClient) call(ctx context.Context, method string, params interface{}) (string, error) {
	result, err := c.callOnce(ctx, method, params)
	if err == nil {
		return result, nil
	}
	if c.tryRefreshOn401(ctx, err) {
		return c.callOnce(ctx, method, params)
	}
	return "", err
}

// tryRefreshOn401 refreshes the OAuth token on 401 and resets the session.
// Returns true if the caller should retry.
func (c *MCPClient) tryRefreshOn401(ctx context.Context, err error) bool {
	if c.refresher == nil {
		return false
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		return false
	}
	if refreshErr := c.refresher.RefreshOAuthToken(ctx, c.workspaceID); refreshErr != nil {
		return false
	}
	newToken, revealErr := c.refresher.RevealAccessToken(ctx, c.workspaceID)
	if revealErr != nil {
		return false
	}
	c.setToken(newToken)
	c.sessionMu.Lock()
	c.sessionID = ""
	c.initialized = false
	c.sessionMu.Unlock()
	return true
}

func (c *MCPClient) callOnce(ctx context.Context, method string, params interface{}) (string, error) {
	if err := c.ensureSession(ctx); err != nil {
		return "", err
	}
	reqBody, _ := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      nextMCPID(),
	})
	req, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.getToken())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	req.Header.Set("Mcp-Session-Id", c.sessionID)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 401 {
		c.sessionMu.Lock()
		c.sessionID = ""
		c.initialized = false
		c.sessionMu.Unlock()
		return "", &APIError{StatusCode: 401, Message: "MCP session expired"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{StatusCode: resp.StatusCode, Message: string(raw)}
	}
	data := parseSSE(raw)
	if data == "" {
		return "", fmt.Errorf("MCP server returned empty response")
	}
	var mcpr mcpResponse
	if err := json.Unmarshal([]byte(data), &mcpr); err != nil {
		return "", fmt.Errorf("decode MCP response: %w", err)
	}
	if mcpr.Error != nil {
		return "", fmt.Errorf("MCP error %d: %s", mcpr.Error.Code, mcpr.Error.Message)
	}
	if mcpr.Result.IsError {
		if len(mcpr.Result.Content) > 0 {
			return "", fmt.Errorf("MCP tool error: %s", mcpr.Result.Content[0].Text)
		}
		return "", errors.New("MCP tool error (no details)")
	}
	if len(mcpr.Result.Content) == 0 {
		return "", nil
	}
	return mcpr.Result.Content[0].Text, nil
}

func (c *MCPClient) ensureSession(ctx context.Context) error {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.initialized && c.sessionID != "" {
		return nil
	}
	reqBody, _ := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": mcpProtocol,
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]string{"name": "kandev", "version": "1.0"},
		},
		ID: nextMCPID(),
	})
	req, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getToken())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("MCP initialize: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		// Surface a 401 as APIError so tryRefreshOn401 can refresh + retry;
		// a plain error here would strand an expired token unrefreshed.
		if resp.StatusCode == 401 {
			return &APIError{StatusCode: 401, Message: "MCP initialize unauthorized: " + string(raw)}
		}
		return fmt.Errorf("MCP initialize failed (%d): %s", resp.StatusCode, string(raw))
	}
	c.sessionID = resp.Header.Get("Mcp-Session-Id")
	if c.sessionID == "" {
		return errors.New("MCP initialize: no session ID in response")
	}
	c.initialized = true
	return nil
}

func parseSSE(raw []byte) string {
	var sb strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			sb.WriteString(strings.TrimPrefix(line, "data: "))
		}
	}
	return sb.String()
}

var mcpIDCounter struct {
	sync.Mutex
	n int
}

func nextMCPID() int {
	mcpIDCounter.Lock()
	defer mcpIDCounter.Unlock()
	mcpIDCounter.n++
	return mcpIDCounter.n
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
