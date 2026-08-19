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
//
// Each MCPClient holds one MCP session (initialize → tools/call). Sessions
// are per-workspace and cached in the service's client map, so the session
// is reused across requests until the client is invalidated (config change,
// token refresh).
type MCPClient struct {
	http       *http.Client
	accessToken string
	workspaceID string
	sessionMu  sync.Mutex // guards session init only
	mu         sync.Mutex // guards session reset on 401
	sessionID  string
	initialized bool
	cloudID    string
}

const (
	mcpEndpoint   = "https://mcp.atlassian.com/v1/mcp/authv2"
	mcpProtocol   = "2025-06-18"
)

// NewMCPClient builds an MCP-backed Jira client.
func NewMCPClient(accessToken, cloudID, workspaceID string) *MCPClient {
	return &MCPClient{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		accessToken: accessToken,
		workspaceID: workspaceID,
		cloudID:     cloudID,
	}
}

// mcpRequest is a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
}

// mcpResponse is the parsed SSE event data.
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

// call sends a JSON-RPC request to the MCP server and parses the SSE response.
func (c *MCPClient) call(ctx context.Context, method string, params interface{}) (string, error) {
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
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
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
		// Session expired — reset so next call re-initializes.
		c.mu.Lock()
		c.sessionID = ""
		c.initialized = false
		c.mu.Unlock()
		return "", &APIError{StatusCode: 401, Message: "MCP session expired"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{StatusCode: resp.StatusCode, Message: string(raw)}
	}
	// Parse SSE response: lines starting with "data: "
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

// ensureSession initializes the MCP session if not already done. Uses a
// separate lock so concurrent API calls don't serialize on session init.
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
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
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
		return fmt.Errorf("MCP initialize failed (%d): %s", resp.StatusCode, string(raw))
	}
	c.sessionID = resp.Header.Get("Mcp-Session-Id")
	if c.sessionID == "" {
		return errors.New("MCP initialize: no session ID in response")
	}
	c.initialized = true
	return nil
}

// parseSSE extracts the data from an SSE response body.
func parseSSE(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}

var mcpIDCounter int

func nextMCPID() int {
	mcpIDCounter++
	return mcpIDCounter
}

// --- Client interface implementation ---

func (c *MCPClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "atlassianUserInfo",
		"arguments": map[string]interface{}{},
	})
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	var user struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal([]byte(result), &user); err != nil {
		return &TestConnectionResult{OK: true}, nil
	}
	return &TestConnectionResult{OK: true, DisplayName: user.Name, Email: user.Email, AccountID: user.AccountID}, nil
}

func (c *MCPClient) GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "getJiraIssue",
		"arguments": map[string]interface{}{
			"cloudId":      c.cloudID,
			"issueIdOrKey": ticketKey,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseMCPIssue(result, ticketKey)
}

func (c *MCPClient) ListTransitions(ctx context.Context, ticketKey string) ([]JiraTransition, error) {
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
			ID           string `json:"id"`
			Name         string `json:"name"`
			ToStatusID   string `json:"toStatusId"`
			ToStatusName string `json:"toStatusName"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("decode transitions: %w", err)
	}
	transitions := make([]JiraTransition, len(resp.Transitions))
	for i, t := range resp.Transitions {
		transitions[i] = JiraTransition{
			ID:           t.ID,
			Name:         t.Name,
			ToStatusID:   t.ToStatusID,
			ToStatusName: t.ToStatusName,
		}
	}
	return transitions, nil
}

func (c *MCPClient) DoTransition(ctx context.Context, ticketKey, transitionID string) error {
	_, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "transitionJiraIssue",
		"arguments": map[string]interface{}{
			"cloudId":      c.cloudID,
			"issueIdOrKey": ticketKey,
			"transitionId": transitionID,
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
	projects := make([]JiraProject, len(resp.Values))
	for i, p := range resp.Values {
		projects[i] = JiraProject{Key: p.Key, Name: p.Name, ID: p.ID}
	}
	return projects, nil
}

func (c *MCPClient) ListProjectStatuses(ctx context.Context, projectKey string) ([]JiraStatus, error) {
	result, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name": "getJiraProjectIssueTypesMetadata",
		"arguments": map[string]interface{}{
			"cloudId":     c.cloudID,
			"projectIdOrKey": projectKey,
		},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Statuses []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory string `json:"statusCategory"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("decode project statuses: %w", err)
	}
	statuses := make([]JiraStatus, len(resp.Statuses))
	for i, s := range resp.Statuses {
		statuses[i] = JiraStatus{
			ID:             s.ID,
			Name:           s.Name,
			StatusCategory: s.StatusCategory,
		}
	}
	return statuses, nil
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
	return parseMCPSearchResult(result)
}

// parseMCPIssue parses a Jira issue from the MCP tool response text.
func parseMCPIssue(raw, ticketKey string) (*JiraTicket, error) {
	var issue struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Status      struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				StatusCategory struct {
					Name string `json:"name"`
				} `json:"statusCategory"`
			} `json:"status"`
			Project struct {
				Key string `json:"key"`
			} `json:"project"`
			Issuetype struct {
				Name string `json:"name"`
				IconURL string `json:"iconUrl"`
			} `json:"issuetype"`
			Priority struct {
				Name    string `json:"name"`
				IconURL string `json:"iconUrl"`
			} `json:"priority"`
			Assignee struct {
				DisplayName string `json:"displayName"`
				AvatarUrls  map[string]string `json:"avatarUrls"`
			} `json:"assignee"`
			Reporter struct {
				DisplayName string `json:"displayName"`
				AvatarUrls  map[string]string `json:"avatarUrls"`
			} `json:"reporter"`
			Updated string `json:"updated"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}
	category := ""
	if issue.Fields.Status.StatusCategory.Name != "" {
		cat := strings.ToLower(issue.Fields.Status.StatusCategory.Name)
		switch {
		case strings.Contains(cat, "new"):
			category = "new"
		case strings.Contains(cat, "done"):
			category = "done"
		case strings.Contains(cat, "indeterminate"):
			category = "indeterminate"
		}
	}
	var assigneeAvatar, reporterAvatar string
	if len(issue.Fields.Assignee.AvatarUrls) > 0 {
		for _, v := range issue.Fields.Assignee.AvatarUrls {
			assigneeAvatar = v
			break
		}
	}
	if len(issue.Fields.Reporter.AvatarUrls) > 0 {
		for _, v := range issue.Fields.Reporter.AvatarUrls {
			reporterAvatar = v
			break
		}
	}
	return &JiraTicket{
		Key:            issue.Key,
		Summary:        issue.Fields.Summary,
		StatusID:       issue.Fields.Status.ID,
		StatusName:     issue.Fields.Status.Name,
		StatusCategory: category,
		ProjectKey:     issue.Fields.Project.Key,
		IssueType:      issue.Fields.Issuetype.Name,
		IssueTypeIcon:  issue.Fields.Issuetype.IconURL,
		Priority:       issue.Fields.Priority.Name,
		PriorityIcon:   issue.Fields.Priority.IconURL,
		AssigneeName:   issue.Fields.Assignee.DisplayName,
		AssigneeAvatar: assigneeAvatar,
		ReporterName:   issue.Fields.Reporter.DisplayName,
		ReporterAvatar: reporterAvatar,
		Updated:        issue.Fields.Updated,
		URL:            fmt.Sprintf("https://salla-dev.atlassian.net/browse/%s", issue.Key),
	}, nil
}

// parseMCPSearchResult parses the search response from the MCP tool.
func parseMCPSearchResult(raw string) (*SearchResult, error) {
	var resp struct {
		Issues []json.RawMessage `json:"issues"`
		IsLast bool              `json:"isLast"`
		NextPageToken string    `json:"nextPageToken"`
		MaxResults int          `json:"maxResults"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("decode search result: %w", err)
	}
	tickets := make([]JiraTicket, 0, len(resp.Issues))
	for _, raw := range resp.Issues {
		ticket, err := parseMCPIssue(string(raw), "")
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
