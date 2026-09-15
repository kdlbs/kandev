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
	defer testServer.Close()

	previousServers := mcpServers
	mcpServers = nil
	previousClients := mcpClients
	mcpClients = make(map[string]*mcpclient.Client)
	t.Cleanup(func() {
		closeMCPClients()
		mcpServers = previousServers
		mcpClients = previousClients
	})

	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	_, err := agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{{Sse: &acp.McpServerSseInline{
			Name: "kandev",
			Url:  testServer.URL + "/sse",
		}}},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	if _, ok := mcpClients["kandev"]; !ok {
		t.Fatal("NewSession() did not prime the Kandev MCP client")
	}
}
