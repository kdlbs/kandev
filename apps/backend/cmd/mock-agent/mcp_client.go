package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	mcpClients   = map[string]*mcpclient.Client{}
	mcpClientsMu sync.Mutex
)

// getMCPClient returns (or creates) an initialized MCP client for the named server.
func getMCPClient(serverName string) (*mcpclient.Client, error) {
	mcpClientsMu.Lock()
	defer mcpClientsMu.Unlock()

	if c, ok := mcpClients[serverName]; ok {
		return c, nil
	}

	srv, ok := mcpServers[serverName]
	if !ok {
		return nil, fmt.Errorf("unknown MCP server: %s", serverName)
	}

	c, err := mcpclient.NewSSEMCPClient(srv.URL)
	if err != nil {
		return nil, fmt.Errorf("create SSE client for %s: %w", serverName, err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start MCP client %s: %w", serverName, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "mock-agent",
		Version: "1.0",
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialize MCP client %s: %w", serverName, err)
	}

	mcpClients[serverName] = c
	return c, nil
}

// callMCPTool calls a tool on the named MCP server and returns the result text.
func callMCPTool(serverName, toolName string, args map[string]any) (string, error) {
	c, err := getMCPClient(serverName)
	if err != nil {
		return "", err
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	result, err := c.CallTool(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("call tool %s/%s: %w", serverName, toolName, err)
	}

	return extractMCPResultText(result), nil
}

// extractMCPResultText extracts text from an MCP CallToolResult.
func extractMCPResultText(result *mcp.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// closeMCPClients closes all open MCP clients (called on shutdown).
func closeMCPClients() {
	mcpClientsMu.Lock()
	defer mcpClientsMu.Unlock()
	for _, c := range mcpClients {
		_ = c.Close()
	}
}
