package streamjson

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// PrepareEnvironment performs protocol-specific setup before the agent process starts.
// Stream-json protocol reads MCP configuration from settings files, but we handle MCP via kandev's
// built-in MCP server, so this is a no-op.
func (a *Adapter) PrepareEnvironment() (map[string]string, error) {
	a.logger.Info("PrepareEnvironment called",
		zap.Int("mcp_server_count", len(a.cfg.McpServers)))
	// MCP configuration is handled externally or via CLI flags
	return nil, nil
}

// PrepareCommandArgs returns extra command-line arguments for the agent process.
// For stream-json (Claude Code), MCP configuration is passed via --mcp-config flag.
func (a *Adapter) PrepareCommandArgs() []string {
	if len(a.cfg.McpServers) == 0 {
		return nil
	}

	// Build MCP configuration in Claude Code format
	// Format: { "server-name": { "command": "...", "args": [...] } }
	mcpConfig := make(map[string]interface{})
	for _, server := range a.cfg.McpServers {
		serverDef := make(map[string]interface{})

		// Handle different transport types
		if server.Command != "" {
			// stdio transport
			serverDef["command"] = server.Command
			if len(server.Args) > 0 {
				serverDef["args"] = server.Args
			}
		} else if server.URL != "" {
			// SSE/HTTP transport
			serverDef["url"] = server.URL
			if server.Type != "" {
				serverDef["type"] = server.Type
			}
		}

		mcpConfig[server.Name] = serverDef
	}

	// Wrap in mcpServers key (Claude Code expects this format)
	wrappedConfig := map[string]interface{}{
		"mcpServers": mcpConfig,
	}

	// Convert to JSON string
	configJSON, err := json.Marshal(wrappedConfig)
	if err != nil {
		a.logger.Warn("failed to marshal MCP config, skipping",
			zap.Error(err),
			zap.Int("server_count", len(a.cfg.McpServers)))
		return nil
	}

	a.logger.Info("prepared MCP configuration for Claude Code",
		zap.Int("server_count", len(a.cfg.McpServers)),
		zap.String("config", string(configJSON)))

	// Return --mcp-config flag with JSON string
	return []string{"--mcp-config", string(configJSON)}
}

// Initialize establishes the stream-json connection with the agent subprocess.
func (a *Adapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing stream-json adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Claude Code client
	a.client = claudecode.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetRequestHandler(a.handleControlRequest)
	a.client.SetCancelHandler(a.handleControlCancel)
	a.client.SetMessageHandler(a.handleMessage)

	// Start reading from stdout with the adapter's context
	// Wait for the read loop to be ready before sending initialize
	readyC := a.client.Start(a.ctx)
	select {
	case <-readyC:
		a.logger.Info("read loop is ready")
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		a.logger.Warn("timeout waiting for read loop to start")
	}

	// Store agent info (version will be populated from first message)
	a.agentInfo = &AgentInfo{
		Name:    a.agentID,
		Version: "unknown",
	}

	// Build hooks based on permission policy
	hooks := a.buildHooks()

	// Send initialize control request to get slash commands
	// This is required for streaming mode (input-format=stream-json)
	initCtx, initSpan := shared.TraceProtocolRequest(ctx, shared.ProtocolStreamJSON, a.agentID, "initialize")
	a.traceOutgoingSendCtx(initCtx, "initialize", map[string]any{
		"type":    claudecode.MessageTypeControlRequest,
		"request": map[string]any{"subtype": claudecode.SubtypeInitialize, "hooks": hooks},
	})
	initResp, err := a.client.Initialize(ctx, 60*time.Second, hooks)
	initSpan.End()

	if err != nil {
		a.logger.Warn("failed to initialize (continuing anyway)", zap.Error(err))
	} else if initResp != nil && len(initResp.Commands) > 0 {
		// Store available commands to emit after session is created
		commands := make([]streams.AvailableCommand, len(initResp.Commands))
		for i, cmd := range initResp.Commands {
			commands[i] = streams.AvailableCommand{
				Name:        cmd.Name,
				Description: cmd.Description,
			}
		}
		a.mu.Lock()
		a.pendingAvailableCommands = commands
		a.mu.Unlock()

		a.logger.Info("received slash commands from initialize",
			zap.Int("count", len(commands)))
	}

	a.logger.Info("stream-json adapter initialized")

	return nil
}

// buildHooks constructs hook configuration based on the permission policy.
// Returns nil for autonomous mode (no hooks).
func (a *Adapter) buildHooks() map[string]any {
	policy := a.cfg.PermissionPolicy
	if policy == "" || policy == "autonomous" {
		return nil
	}

	cfg := &claudecode.HookConfig{}

	switch policy {
	case "supervised":
		// Match write/dangerous tools — everything except read-only tools.
		// Non-matching tools (Glob, Grep, Read, Task, TodoWrite, etc.) are
		// implicitly auto-approved by Claude Code when no hook matches.
		cfg.PreToolUse = []claudecode.HookEntry{
			{
				Matcher:         `^(?!(Glob|Grep|NotebookRead|Read|Task|TodoWrite)$).*`,
				HookCallbackIDs: []string{"tool_approval"},
			},
		}
	case "plan":
		// Only ExitPlanMode requires approval — all other tools run freely during planning.
		// The auto_approve callback explicitly approves so Claude doesn't fall through.
		cfg.PreToolUse = []claudecode.HookEntry{
			{
				Matcher:         `^ExitPlanMode$`,
				HookCallbackIDs: []string{"tool_approval"},
			},
			{
				Matcher:         `^(?!ExitPlanMode$).*`,
				HookCallbackIDs: []string{"auto_approve"},
			},
		}
	}

	// Always register a Stop hook to check for uncommitted changes
	cfg.Stop = []claudecode.HookEntry{
		{
			HookCallbackIDs: []string{"stop_git_check"},
		},
	}

	return cfg.ToMap()
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}
