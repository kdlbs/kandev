package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	mcpClients       = map[string]*mcpclient.Client{}
	mcpClientCancels = map[string]context.CancelFunc{}
	mcpClientsMu     sync.Mutex
)

// mcpSSEProtocolVersion keeps the mock agent on the legacy SSE protocol.
const mcpSSEProtocolVersion = mcp.ProtocolVersion20241105

// getMCPClient returns (or creates) an initialized MCP client for the named server.
func getMCPClient(serverName string) (*mcpclient.Client, error) {
	return getMCPClientCtx(context.Background(), serverName)
}

// getMCPClientCtx is the global-server variant used by catalog priming and
// command-line scenarios.
func getMCPClientCtx(ctx context.Context, serverName string) (*mcpclient.Client, error) {
	return getMCPClientForServersCtx(ctx, serverName, nil)
}

func getMCPClientForServersCtx(ctx context.Context, serverName string, sessionServers map[string]mcpServerDef) (*mcpclient.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	mcpClientsMu.Lock()
	defer mcpClientsMu.Unlock()

	srv, ok := sessionServers[serverName]
	if !ok {
		srv, ok = mcpServers[serverName]
	}
	if !ok {
		return nil, fmt.Errorf("unknown MCP server: %s", serverName)
	}
	cacheKey := serverName + "\x00" + srv.URL
	if c, ok := mcpClients[cacheKey]; ok {
		return c, nil
	}

	c, err := mcpclient.NewSSEMCPClient(srv.URL)
	if err != nil {
		return nil, fmt.Errorf("create SSE client for %s: %w", serverName, err)
	}

	// Start owns the lifetime of the SSE stream. Keep it independent from the
	// request context so a successful catalog probe remains usable by later
	// prompts after ACP returns from NewSession or LoadSession, while still
	// allowing the caller's probe deadline to cancel startup.
	startCtx, cancelStart := context.WithCancel(context.Background())
	startErr := make(chan error, 1)
	go func() {
		startErr <- c.Start(startCtx)
	}()
	select {
	case err := <-startErr:
		if err != nil {
			cancelStart()
			return nil, fmt.Errorf("start MCP client %s: %w", serverName, err)
		}
	case <-ctx.Done():
		cancelStart()
		<-startErr
		_ = c.Close()
		return nil, fmt.Errorf("start MCP client %s: %w", serverName, ctx.Err())
	}

	if err := ctx.Err(); err != nil {
		cancelStart()
		_ = c.Close()
		return nil, fmt.Errorf("start MCP client %s: %w", serverName, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpSSEProtocolVersion
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "mock-agent",
		Version: "1.0",
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		cancelStart()
		_ = c.Close()
		return nil, fmt.Errorf("initialize MCP client %s: %w", serverName, err)
	}
	if _, err := c.ListTools(ctx, mcp.ListToolsRequest{}); err != nil {
		cancelStart()
		_ = c.Close()
		return nil, fmt.Errorf("list MCP tools %s: %w", serverName, err)
	}

	if mcpClients == nil {
		mcpClients = make(map[string]*mcpclient.Client)
	}
	if mcpClientCancels == nil {
		mcpClientCancels = make(map[string]context.CancelFunc)
	}
	mcpClients[cacheKey] = c
	mcpClientCancels[cacheKey] = cancelStart
	// Keep the legacy global lookup visible for callers and tests that inspect
	// the command-line configured client directly. Session calls always use the
	// URL-qualified cache key above.
	if sessionServers == nil {
		mcpClients[serverName] = c
		mcpClientCancels[serverName] = cancelStart
	}
	return c, nil
}

// primeKandevMCPToolCatalog mirrors an MCP-capable agent's initial tools/list
// request. CI auto-fix dispatch requires persisted evidence of the current
// catalog before it adds the outcome protocol to a prompt, so the mock agent
// must observe the injected Kandev tools even when the scenario does not call a
// tool during its first turn.
func primeKandevMCPToolCatalog(_ context.Context) {
	if _, configured := mcpServers["kandev"]; !configured {
		return
	}
	// The MCP connection outlives the ACP request that triggers priming. ACP
	// cancels NewSession's context after the response, while later prompts use
	// the cached SSE client and need its session to remain open.
	primeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := getMCPClientCtx(primeCtx, "kandev"); err != nil {
		_, _ = fmt.Fprintf(logOutput, "mock-agent: failed to prime Kandev MCP tool catalog: %v\n", err)
	}
}

// callMCPTool calls a tool on the named MCP server and returns the result text.
func callMCPTool(serverName, toolName string, args map[string]any) (string, error) {
	return callMCPToolCtx(context.Background(), serverName, toolName, args)
}

// callMCPToolCtx calls a tool on the named MCP server with a caller-provided context.
// Use this when the caller needs to impose a timeout on the MCP call.
func callMCPToolCtx(ctx context.Context, serverName, toolName string, args map[string]any) (string, error) {
	return callMCPToolCtxForServers(ctx, nil, serverName, toolName, args)
}

// callMCPToolCtxForServers calls a tool using the current ACP session's MCP
// server definitions. A nil map keeps the command-line configured behavior.
func callMCPToolCtxForServers(ctx context.Context, sessionServers map[string]mcpServerDef, serverName, toolName string, args map[string]any) (string, error) {
	c, err := getMCPClientForServersCtx(ctx, serverName, sessionServers)
	if err != nil {
		return "", err
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	result, err := c.CallTool(ctx, req)
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

// registerACPMcpServers adds SSE MCP servers from an ACP NewSessionRequest to
// the global map and returns a session-owned copy for callers that need to
// preserve the endpoint selected for that ACP session.
func registerACPMcpServers(servers []acp.McpServer) map[string]mcpServerDef {
	registered := make(map[string]mcpServerDef)
	for _, s := range servers {
		if s.Sse != nil && s.Sse.Name != "" && s.Sse.Url != "" {
			registered[s.Sse.Name] = mcpServerDef{URL: s.Sse.Url, Type: "sse"}
		}
	}

	mcpClientsMu.Lock()
	defer mcpClientsMu.Unlock()
	if mcpServers == nil {
		mcpServers = make(map[string]mcpServerDef)
	}
	for name, server := range registered {
		mcpServers[name] = server
		_, _ = fmt.Fprintf(logOutput, "mock-agent: registered ACP MCP server %s at %s\n", name, server.URL)
	}
	return registered
}

func cloneMCPServerDefs(defs map[string]mcpServerDef) map[string]mcpServerDef {
	if len(defs) == 0 {
		return nil
	}
	cloned := make(map[string]mcpServerDef, len(defs))
	for name, def := range defs {
		cloned[name] = def
	}
	return cloned
}

// closeMCPClients closes all open MCP clients (called on shutdown).
func closeMCPClients() {
	mcpClientsMu.Lock()
	defer mcpClientsMu.Unlock()
	closed := make(map[*mcpclient.Client]struct{}, len(mcpClients))
	for key, c := range mcpClients {
		if cancel, ok := mcpClientCancels[key]; ok {
			cancel()
		}
		if _, ok := closed[c]; ok {
			continue
		}
		closed[c] = struct{}{}
		_ = c.Close()
	}
	mcpClients = make(map[string]*mcpclient.Client)
	mcpClientCancels = make(map[string]context.CancelFunc)
}
