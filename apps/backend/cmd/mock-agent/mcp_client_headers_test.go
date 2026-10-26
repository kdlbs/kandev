package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// authRecorder serves an MCP SSE endpoint and records the Authorization header
// of every request it receives.
func authRecorder(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	server := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithToolCapabilities(true))
	server.AddTool(mcp.NewTool("ping"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	sse := mcpserver.NewSSEServer(server)
	var mu sync.Mutex
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		sse.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func resetMCPState(t *testing.T) {
	t.Helper()
	previousServers, previousClients := mcpServers, mcpClients
	mcpServers, mcpClients = nil, make(map[string]*mcpclient.Client)
	t.Cleanup(func() {
		closeMCPClients()
		mcpServers, mcpClients = previousServers, previousClients
	})
}

func requireAllAuthorized(t *testing.T, seen []string, want string) {
	t.Helper()
	if len(seen) == 0 {
		t.Fatal("no request reached the MCP server")
	}
	for i, got := range seen {
		if got != want {
			t.Fatalf("request %d Authorization = %q, want %q", i, got, want)
		}
	}
}

// The ACP carrier: headers on the session/new server entry reach the MCP
// server on every request the mock agent makes.
func TestACPSessionMCPServerHeadersAreSent(t *testing.T) {
	ts, seen := authRecorder(t)
	resetMCPState(t)
	const bearer = "Bearer acp-token"

	servers := registerACPMcpServers([]acp.McpServer{{Sse: &acp.McpServerSseInline{
		Name:    "kandev",
		Url:     ts.URL + "/sse",
		Headers: []acp.HttpHeader{{Name: "Authorization", Value: bearer}},
	}}})
	if _, err := callMCPToolCtxForServers(context.Background(), servers, "kandev", "ping", nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	requireAllAuthorized(t, seen(), bearer)
}

// The TUI carrier: the headers object of a --mcp-config entry reaches the MCP
// server the same way.
func TestTUIMCPConfigHeadersAreSent(t *testing.T) {
	ts, seen := authRecorder(t)
	resetMCPState(t)
	const bearer = "Bearer tui-token"

	raw, err := json.Marshal(mcpConfigPayload{MCPServers: map[string]mcpServerDef{
		"kandev": {URL: ts.URL + "/sse", Type: "sse", Headers: map[string]string{"Authorization": bearer}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var payload mcpConfigPayload
	if err := json.Unmarshal([]byte(parseMCPConfigFromArgs([]string{"mock-agent", "--tui", "--mcp-config", string(raw), "--model", "m"})), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := callMCPToolCtxForServers(context.Background(), payload.MCPServers, "kandev", "ping", nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	requireAllAuthorized(t, seen(), bearer)
}
