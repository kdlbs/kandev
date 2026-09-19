package main

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestNewSessionPrimesKandevMCPToolCatalog(t *testing.T) {
	server := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithToolCapabilities(true))
	server.AddTool(mcp.NewTool("report_change_request_auto_fix_outcome_kandev"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	testServer := mcpserver.NewTestServer(server)

	previousServers := mcpServers
	mcpServers = nil
	previousClients := mcpClients
	mcpClients = make(map[string]*mcpclient.Client)
	t.Cleanup(func() {
		closeMCPClients()
		testServer.Close()
		mcpServers = previousServers
		mcpClients = previousClients
	})

	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	_, err := agent.NewSession(sessionCtx, acp.NewSessionRequest{
		McpServers: []acp.McpServer{{Sse: &acp.McpServerSseInline{
			Name: "kandev",
			Url:  testServer.URL + "/sse",
		}}},
	})
	cancelSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	if _, ok := mcpClients["kandev"]; !ok {
		t.Fatal("NewSession() did not prime the Kandev MCP client")
	}
	if result, err := callMCPTool("kandev", "report_change_request_auto_fix_outcome_kandev", nil); err != nil {
		t.Fatalf("callMCPTool() after NewSession context cancellation error = %v", err)
	} else if result != "ok" {
		t.Fatalf("callMCPTool() result = %q, want %q", result, "ok")
	}
}
