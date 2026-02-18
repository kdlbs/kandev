package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// PrepareEnvironment is a no-op for Codex. MCP servers and sandbox settings
// are now passed via command-line -c flags through PrepareCommandArgs().
func (a *Adapter) PrepareEnvironment() (map[string]string, error) {
	a.logger.Info("PrepareEnvironment called (no-op for Codex)")
	return nil, nil
}

// PrepareCommandArgs returns extra command-line arguments for the Codex process.
// This includes -c flags for MCP servers and sandbox configuration.
// Codex uses -c key=value flags to override config at runtime.
func (a *Adapter) PrepareCommandArgs() []string {
	var args []string

	// Set sandbox_mode to workspace-write to enable file editing
	args = append(args, "-c", "sandbox_mode=\"workspace-write\"")

	// Enable network access in sandbox
	args = append(args, "-c", "sandbox_workspace_write.network_access=true")

	// Add MCP servers as -c flags
	for _, server := range a.cfg.McpServers {
		safeName := sanitizeCodexServerName(server.Name)

		if server.Type == "sse" || server.Type == "http" {
			// HTTP/SSE transport - use url field
			// Convert SSE URLs (/sse) to streamable HTTP URLs (/mcp) for Codex compatibility
			url := server.URL
			if url != "" {
				url = convertSSEToStreamableHTTP(url)
				args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.url=\"%s\"", safeName, url))
			}
		} else if server.Command != "" {
			// STDIO transport - use command field
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.command=\"%s\"", safeName, server.Command))
			// Add args if present
			if len(server.Args) > 0 {
				// TOML array format: ["arg1", "arg2"]
				quotedArgs := make([]string, len(server.Args))
				for i, arg := range server.Args {
					quotedArgs[i] = fmt.Sprintf("\"%s\"", arg)
				}
				argsStr := "[" + strings.Join(quotedArgs, ", ") + "]"
				args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.args=%s", safeName, argsStr))
			}
		}
	}

	a.logger.Info("PrepareCommandArgs",
		zap.Int("mcp_server_count", len(a.cfg.McpServers)),
		zap.Strings("extra_args", args))

	return args
}

// sanitizeCodexServerName converts a server name to a valid TOML table name.
// Replaces spaces and special characters with underscores.
func sanitizeCodexServerName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	sanitized := result.String()
	if sanitized == "" {
		return "server"
	}
	return sanitized
}

// convertSSEToStreamableHTTP converts an SSE endpoint URL to a streamable HTTP endpoint URL.
// Codex doesn't support SSE transport - it uses streamable HTTP which requires POST requests.
// This converts URLs ending in /sse to /mcp for Kandev MCP server compatibility.
// Example: http://localhost:9090/sse -> http://localhost:9090/mcp
func convertSSEToStreamableHTTP(url string) string {
	if strings.HasSuffix(url, "/sse") {
		return strings.TrimSuffix(url, "/sse") + "/mcp"
	}
	return url
}

// Initialize establishes the Codex connection with the agent subprocess.
func (a *Adapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Codex adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Codex client
	a.client = codex.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetNotificationHandler(a.handleNotification)
	a.client.SetRequestHandler(a.handleRequest)

	// Start reading from stdout with the adapter's context
	// The readLoop needs to stay alive for the entire lifecycle of the adapter,
	// not just the initialize HTTP request. It will be cancelled when Close() is called.
	a.client.Start(a.ctx)

	// Perform Codex initialize handshake
	resp, err := a.client.Call(ctx, codex.MethodInitialize, &codex.InitializeParams{
		ClientInfo: &codex.ClientInfo{
			Name:    "kandev-agentctl",
			Title:   "Kandev Agent Controller",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("codex initialize handshake failed: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("codex initialize error: %s", resp.Error.Message)
	}

	// Parse initialize result
	var initResult codex.InitializeResult
	if resp.Result != nil {
		if err := json.Unmarshal(resp.Result, &initResult); err != nil {
			a.logger.Warn("failed to parse initialize result", zap.Error(err))
		}
	}

	// Send initialized notification
	if err := a.client.Notify(codex.MethodInitialized, nil); err != nil {
		return fmt.Errorf("failed to send initialized notification: %w", err)
	}

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    a.agentID,
		Version: initResult.UserAgent,
	}

	a.logger.Info("Codex adapter initialized",
		zap.String("user_agent", initResult.UserAgent))

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}
