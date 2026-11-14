package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const testCallerCredential = "Bearer mock-credential"

// newCredentialMCPServer serves one tool over SSE and streamable HTTP and
// rejects any request without the expected Authorization header, like an
// agentctl instance that enforces its built-in MCP caller credential.
func newCredentialMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithToolCapabilities(true))
	for _, name := range []string{"report_change_request_auto_fix_outcome_kandev", "list_task_sessions_kandev"} {
		server.AddTool(mcp.NewTool(name), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		})
	}
	var handler http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != testCallerCredential {
			http.Error(w, "credential required", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	mux := http.NewServeMux()
	sse := mcpserver.NewSSEServer(server, mcpserver.WithBaseURL(ts.URL))
	mux.Handle("/sse", sse)
	mux.Handle("/message", sse)
	mux.Handle("/mcp", mcpserver.NewStreamableHTTPServer(server))
	handler = mux
	t.Cleanup(ts.Close)
	return ts
}

func isolateMockMCPClients(t *testing.T) {
	t.Helper()
	previousServers := mcpServers
	mcpServers = nil
	previousClients := mcpClients
	mcpClients = make(map[string]*mcpclient.Client)
	t.Cleanup(func() {
		closeMCPClients()
		mcpServers = previousServers
		mcpClients = previousClients
	})
}

func TestACPSessionHeadersReachEnforcingMCPServer(t *testing.T) {
	ts := newCredentialMCPServer(t)
	isolateMockMCPClients(t)
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	_, err := agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{{Sse: &acp.McpServerSseInline{
			Name:    "kandev",
			Url:     ts.URL + "/sse",
			Headers: []acp.HttpHeader{{Name: "Authorization", Value: testCallerCredential}},
		}}},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	result, err := callMCPTool("kandev", "report_change_request_auto_fix_outcome_kandev", nil)
	if err != nil {
		t.Fatalf("callMCPTool() with session headers error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("callMCPTool() result = %q, want ok", result)
	}
}

func TestACPSessionWithoutHeadersIsRejectedByEnforcingMCPServer(t *testing.T) {
	ts := newCredentialMCPServer(t)
	isolateMockMCPClients(t)
	registerACPMcpServers([]acp.McpServer{{Sse: &acp.McpServerSseInline{Name: "kandev", Url: ts.URL + "/sse"}}})
	if _, err := callMCPTool("kandev", "report_change_request_auto_fix_outcome_kandev", nil); err == nil {
		t.Fatal("callMCPTool() without the credential succeeded against an enforcing server")
	}
}

func TestTUIMCPConfigHeadersReachEnforcingMCPServer(t *testing.T) {
	ts := newCredentialMCPServer(t)
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	config := `{"mcpServers":{"kandev":{"type":"http","url":"` + ts.URL + `/mcp","headers":{"Authorization":"` + testCallerCredential + `"}}}}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	previousArgs := os.Args
	os.Args = []string{"mock-agent", "--mcp-config", configPath}
	t.Cleanup(func() { os.Args = previousArgs })
	result, err := callTUIMCPContext()
	if err != nil {
		t.Fatalf("callTUIMCPContext() error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("callTUIMCPContext() result = %q, want ok", result)
	}
}
