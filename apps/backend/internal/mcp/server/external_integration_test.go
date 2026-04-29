package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalMCP_ToolsListOverHTTP boots an external MCP server with the same
// transport configuration as the backend (gin + RegisterBackendRoutes) and confirms
// a JSON-RPC tools/list call returns the expected tool surface.
func TestExternalMCP_ToolsListOverHTTP(t *testing.T) {
	log := newTestLogger(t)

	dispatcher := ws.NewDispatcher()
	backendClient := NewDispatcherBackendClient(dispatcher, log)
	srv := NewExternal(backendClient, "http://localhost:38429", log, "")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	srv.RegisterBackendRoutes(router)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Initialize the MCP session over Streamable HTTP.
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	resp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, resp.statusCode, "init response: %s", resp.body)

	// tools/list — verify external mode tool surface is exposed.
	listReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	resp = postJSONRPC(t, ts.URL+"/mcp", listReq, resp.sessionID)
	require.Equal(t, http.StatusOK, resp.statusCode, "list response: %s", resp.body)

	// Streamable HTTP wraps responses in SSE-style "data:" lines; extract the JSON.
	jsonLine := extractDataLine(resp.body)
	require.NotEmpty(t, jsonLine, "no data line in response: %s", resp.body)

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonLine), &rpcResp))

	names := make([]string, 0, len(rpcResp.Result.Tools))
	for _, tool := range rpcResp.Result.Tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "create_task_kandev", "external mode must expose create_task_kandev")
	assert.Contains(t, names, "list_workspaces_kandev")
	assert.Contains(t, names, "get_mcp_config_kandev")
	assert.NotContains(t, names, "ask_user_question_kandev", "external mode must not expose session-scoped tools")
	assert.NotContains(t, names, "create_task_plan_kandev")
}

// TestExternalMCP_ToolsCallDispatchesToBackend verifies that calling a tool via
// MCP routes ends up invoking the registered ws.Dispatcher handler.
func TestExternalMCP_ToolsCallDispatchesToBackend(t *testing.T) {
	log := newTestLogger(t)

	called := make(chan struct{}, 1)
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPListWorkspaces, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		called <- struct{}{}
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			"workspaces": []map[string]string{{"id": "ws-1", "name": "Test"}},
			"total":      1,
		})
	})

	backendClient := NewDispatcherBackendClient(dispatcher, log)
	srv := NewExternal(backendClient, "http://localhost:38429", log, "")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	srv.RegisterBackendRoutes(router)

	ts := httptest.NewServer(router)
	defer ts.Close()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	resp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, resp.statusCode)

	callReq := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_workspaces_kandev","arguments":{}}}`
	resp = postJSONRPC(t, ts.URL+"/mcp", callReq, resp.sessionID)
	require.Equal(t, http.StatusOK, resp.statusCode, "call response: %s", resp.body)

	select {
	case <-called:
	default:
		t.Fatal("dispatcher handler was not invoked")
	}

	jsonLine := extractDataLine(resp.body)
	require.NotEmpty(t, jsonLine)
	assert.Contains(t, jsonLine, "ws-1")
}

type httpResp struct {
	statusCode int
	body       string
	sessionID  string
}

func postJSONRPC(t *testing.T, url, body, sessionID string) httpResp {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return httpResp{
		statusCode: resp.StatusCode,
		body:       buf.String(),
		sessionID:  resp.Header.Get("Mcp-Session-Id"),
	}
}

func extractDataLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return strings.TrimSpace(body)
}
