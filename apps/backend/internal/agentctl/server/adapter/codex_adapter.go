package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/codex"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// CodexAdapter implements AgentAdapter for agents using the OpenAI Codex app-server protocol.
// Codex uses a JSON-RPC 2.0 variant over stdio (omitting the jsonrpc field).
type CodexAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Subprocess stdin/stdout (set via Connect)
	stdin  io.Writer
	stdout io.Reader

	// Codex client for JSON-RPC communication
	client *codex.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state - Thread maps to Session, Turn maps to Operation
	threadID string // session ID
	turnID   string // operation ID

	// Agent info (populated after Initialize)
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Accumulators for streaming content
	messageBuffer   string
	reasoningBuffer string
	summaryBuffer   string

	// Turn completion signaling
	turnCompleteCh chan turnCompleteResult

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// turnCompleteResult holds the result of a completed turn
type turnCompleteResult struct {
	success bool
	err     string
}

// NewCodexAdapter creates a new Codex protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
func NewCodexAdapter(cfg *Config, log *logger.Logger) *CodexAdapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &CodexAdapter{
		cfg:       cfg,
		logger:    log.WithFields(zap.String("adapter", "codex")),
		ctx:       ctx,
		cancel:    cancel,
		updatesCh: make(chan AgentEvent, 100),
	}
}

// PrepareEnvironment writes MCP servers to the Codex config file.
// Codex reads MCP configuration from ~/.codex/config.toml at startup time.
func (a *CodexAdapter) PrepareEnvironment() error {
	a.logger.Info("PrepareEnvironment called",
		zap.Int("mcp_server_count", len(a.cfg.McpServers)))
	for i, srv := range a.cfg.McpServers {
		a.logger.Info("MCP server config",
			zap.Int("index", i),
			zap.String("name", srv.Name),
			zap.String("url", srv.URL),
			zap.String("type", srv.Type),
			zap.String("command", srv.Command))
	}
	return WriteCodexMcpConfig(a.cfg.McpServers, "", a.logger)
}

// Connect wires up the stdin/stdout pipes from the running agent subprocess.
func (a *CodexAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin != nil || a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdin = stdin
	a.stdout = stdout
	return nil
}

// WriteCodexMcpConfig writes MCP server configuration to Codex's config.toml.
// It merges with existing config, preserving other settings and existing MCP servers.
// Codex reads MCP servers from ~/.codex/config.toml at startup time, not through the protocol.
// This function should be called before starting the Codex process.
// If homeDir is empty, it uses os.UserHomeDir().
func WriteCodexMcpConfig(mcpServers []McpServerConfig, homeDir string, log *logger.Logger) error {
	if len(mcpServers) == 0 {
		return nil // Nothing to configure
	}

	// Determine config directory
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}
	}
	configDir := filepath.Join(homeDir, ".codex")
	configPath := filepath.Join(configDir, "config.toml")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create codex config directory: %w", err)
	}

	// Read existing config if it exists
	existingData, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing codex config: %w", err)
	}

	// Parse existing config into a generic map to preserve all fields
	var rawConfig map[string]interface{}
	if len(existingData) > 0 {
		if err := toml.Unmarshal(existingData, &rawConfig); err != nil {
			if log != nil {
				log.Warn("failed to parse existing codex config, will create new",
					zap.String("path", configPath),
					zap.Error(err))
			}
			rawConfig = make(map[string]interface{})
		}
	} else {
		rawConfig = make(map[string]interface{})
	}

	// Get or create mcp_servers section
	mcpServersSection, ok := rawConfig["mcp_servers"].(map[string]interface{})
	if !ok {
		mcpServersSection = make(map[string]interface{})
	}

	// Add/update our MCP servers
	for _, server := range mcpServers {
		safeName := sanitizeCodexServerName(server.Name)
		serverConfig := make(map[string]interface{})

		if server.Type == "sse" || server.Type == "http" {
			// HTTP/SSE transport - use url field
			// Codex doesn't support SSE transport - it uses streamable HTTP which requires POST requests.
			// Convert SSE URLs (/sse) to streamable HTTP URLs (/mcp) for Codex compatibility.
			url := server.URL
			if url != "" {
				url = convertSSEToStreamableHTTP(url)
				serverConfig["url"] = url
			}
		} else {
			// STDIO transport - use command and args
			if server.Command != "" {
				serverConfig["command"] = server.Command
			}
			if len(server.Args) > 0 {
				serverConfig["args"] = server.Args
			}
		}

		mcpServersSection[safeName] = serverConfig
	}

	// Update the mcp_servers section in the config
	rawConfig["mcp_servers"] = mcpServersSection

	// Marshal back to TOML
	output, err := toml.Marshal(rawConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal codex config: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write codex config: %w", err)
	}

	if log != nil {
		log.Info("wrote Codex MCP config",
			zap.String("path", configPath),
			zap.Int("server_count", len(mcpServers)),
			zap.Int("total_mcp_servers", len(mcpServersSection)))
	}

	return nil
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
func (a *CodexAdapter) Initialize(ctx context.Context) error {
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
		Name:    "codex",
		Version: initResult.UserAgent,
	}

	a.logger.Info("Codex adapter initialized",
		zap.String("user_agent", initResult.UserAgent))

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *CodexAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new Codex thread (session).
// Note: The mcpServers parameter is ignored because Codex reads MCP configuration from
// ~/.codex/config.toml at startup time, not through the protocol. MCP servers are written
// to the config file by PrepareEnvironment() before the Codex process starts.
func (a *CodexAdapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	// Check client under lock, but don't hold lock during Call() to avoid deadlock
	// with handleNotification which also needs the lock
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	// Determine approval policy - default to "untrusted" if not specified.
	// "untrusted" forces Codex to request approval for all commands/writes.
	// Other options: "on-failure", "on-request", "never"
	approvalPolicy := a.cfg.ApprovalPolicy
	if approvalPolicy == "" {
		approvalPolicy = "untrusted"
	}

	a.logger.Info("starting codex thread with approval policy",
		zap.String("approval_policy", approvalPolicy),
		zap.String("work_dir", a.cfg.WorkDir))

	resp, err := client.Call(ctx, codex.MethodThreadStart, &codex.ThreadStartParams{
		Cwd:            a.cfg.WorkDir,
		ApprovalPolicy: approvalPolicy, // "untrusted", "on-failure", "on-request", "never"
		SandboxPolicy: &codex.SandboxPolicy{
			Type:          "workspaceWrite",        // Sandbox to workspace only
			WritableRoots: []string{a.cfg.WorkDir}, // Allow writing to workspace
			NetworkAccess: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to start thread: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("thread start error: %s", resp.Error.Message)
	}

	var result codex.ThreadStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse thread start result: %w", err)
	}

	a.mu.Lock()
	a.threadID = result.Thread.ID
	a.mu.Unlock()

	a.logger.Info("created new thread", zap.String("thread_id", a.threadID))

	return a.threadID, nil
}

// LoadSession resumes an existing Codex thread.
// It passes the same approval policy and sandbox settings as NewSession to ensure
// permission requirements are preserved across resume (see openai/codex#5322).
func (a *CodexAdapter) LoadSession(ctx context.Context, sessionID string) error {
	// Check client under lock, but don't hold lock during Call() to avoid deadlock
	// with handleNotification which also needs the lock
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Determine approval policy - same logic as NewSession
	// "untrusted" forces Codex to request approval for all commands/writes.
	approvalPolicy := a.cfg.ApprovalPolicy
	if approvalPolicy == "" {
		approvalPolicy = "untrusted"
	}

	a.logger.Info("resuming codex thread with approval policy",
		zap.String("thread_id", sessionID),
		zap.String("approval_policy", approvalPolicy),
		zap.String("work_dir", a.cfg.WorkDir))

	resp, err := client.Call(ctx, codex.MethodThreadResume, &codex.ThreadResumeParams{
		ThreadID:       sessionID,
		Cwd:            a.cfg.WorkDir,
		ApprovalPolicy: approvalPolicy,
		SandboxPolicy: &codex.SandboxPolicy{
			Type:          "workspaceWrite",
			WritableRoots: []string{a.cfg.WorkDir},
			NetworkAccess: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to resume thread: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("thread resume error: %s", resp.Error.Message)
	}

	a.mu.Lock()
	a.threadID = sessionID
	a.mu.Unlock()

	a.logger.Info("resumed thread", zap.String("thread_id", a.threadID))

	return nil
}

// Prompt sends a prompt to the agent, starting a new turn.
// This method blocks until the turn completes (turn/completed notification received).
func (a *CodexAdapter) Prompt(ctx context.Context, message string) error {
	a.mu.Lock()
	client := a.client
	threadID := a.threadID
	// Reset accumulators for new turn
	a.messageBuffer = ""
	a.reasoningBuffer = ""
	a.summaryBuffer = ""
	// Create channel to wait for turn completion
	a.turnCompleteCh = make(chan turnCompleteResult, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt", zap.String("thread_id", threadID))

	params := &codex.TurnStartParams{
		ThreadID: threadID,
		Input: []codex.UserInput{
			{Type: "text", Text: message},
		},
	}

	resp, err := client.Call(ctx, codex.MethodTurnStart, params)
	if err != nil {
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to start turn: %w", err)
	}

	if resp.Error != nil {
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return fmt.Errorf("turn start error: %s", resp.Error.Message)
	}

	var result codex.TurnStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		a.logger.Warn("failed to parse turn start result", zap.Error(err), zap.String("raw", string(resp.Result)))
	}

	turnID := ""
	if result.Turn != nil {
		turnID = result.Turn.ID
	}

	a.mu.Lock()
	a.turnID = turnID
	completeCh := a.turnCompleteCh
	a.mu.Unlock()

	if result.Turn != nil {
		a.logger.Info("started turn, waiting for completion", zap.String("turn_id", turnID), zap.String("status", result.Turn.Status))
	} else {
		a.logger.Info("started turn, waiting for completion", zap.String("turn_id", turnID))
	}

	// Wait for turn completion or context cancellation
	select {
	case <-ctx.Done():
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return ctx.Err()
	case completeResult := <-completeCh:
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		if !completeResult.success && completeResult.err != "" {
			return fmt.Errorf("turn failed: %s", completeResult.err)
		}
		a.logger.Info("turn completed", zap.String("turn_id", turnID), zap.Bool("success", completeResult.success))
		return nil
	}
}

// Cancel interrupts the current turn.
func (a *CodexAdapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling turn", zap.String("thread_id", threadID), zap.String("turn_id", turnID))

	// Codex uses turn/interrupt to cancel
	_, err := client.Call(ctx, codex.MethodTurnInterrupt, map[string]string{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return err
}

// Updates returns the channel for agent events.
func (a *CodexAdapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current thread ID (session).
func (a *CodexAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.threadID
}

// GetOperationID returns the current turn ID (operation).
func (a *CodexAdapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.turnID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *CodexAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *CodexAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing Codex adapter")

	// Cancel the context to stop the read loop goroutine
	if a.cancel != nil {
		a.cancel()
	}

	// Stop the client
	if a.client != nil {
		a.client.Stop()
	}

	// Close update channel
	close(a.updatesCh)

	return nil
}

// sendUpdate safely sends an event to the updates channel.
func (a *CodexAdapter) sendUpdate(update AgentEvent) {
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping notification")
	}
}

// handleNotification processes Codex notifications and emits AgentEvents.
func (a *CodexAdapter) handleNotification(method string, params json.RawMessage) {
	a.mu.RLock()
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	switch method {
	// Standard notifications
	case codex.NotifyItemAgentMessageDelta:
		var p codex.AgentMessageDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse agent message delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.messageBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(AgentEvent{
			Type:        EventTypeMessageChunk,
			SessionID:   threadID,
			OperationID: turnID,
			Text:        p.Delta,
		})

	case codex.NotifyItemReasoningTextDelta:
		var p codex.ReasoningDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse reasoning text delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.reasoningBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(AgentEvent{
			Type:          EventTypeReasoning,
			SessionID:     threadID,
			OperationID:   turnID,
			ReasoningText: p.Delta,
		})

	case codex.NotifyItemReasoningSummaryDelta:
		var p codex.ReasoningDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse reasoning summary delta", zap.Error(err))
			return
		}
		a.mu.Lock()
		a.summaryBuffer += p.Delta
		a.mu.Unlock()
		a.sendUpdate(AgentEvent{
			Type:             EventTypeReasoning,
			SessionID:        threadID,
			OperationID:      turnID,
			ReasoningSummary: p.Delta,
		})

	case codex.NotifyTurnCompleted:
		var p codex.TurnCompletedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn completed", zap.Error(err))
			return
		}

		// Signal turn completion to the waiting Prompt() call
		a.mu.RLock()
		completeCh := a.turnCompleteCh
		a.mu.RUnlock()

		if completeCh != nil {
			select {
			case completeCh <- turnCompleteResult{success: p.Success, err: p.Error}:
				a.logger.Debug("signaled turn completion", zap.String("turn_id", p.TurnID), zap.Bool("success", p.Success))
			default:
				a.logger.Warn("turn complete channel full, dropping signal")
			}
		}

		// Send error event if the turn failed (for UI notification)
		if !p.Success && p.Error != "" {
			a.sendUpdate(AgentEvent{
				Type:        EventTypeError,
				SessionID:   threadID,
				OperationID: p.TurnID,
				Error:       p.Error,
			})
		}

	case codex.NotifyTurnDiffUpdated:
		var p codex.TurnDiffUpdatedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn diff updated", zap.Error(err))
			return
		}
		a.sendUpdate(AgentEvent{
			Type:        EventTypeMessageChunk,
			SessionID:   threadID,
			OperationID: p.TurnID,
			Diff:        p.Diff,
		})

	case codex.NotifyTurnPlanUpdated:
		var p codex.TurnPlanUpdatedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse turn plan updated", zap.Error(err))
			return
		}
		entries := make([]PlanEntry, len(p.Plan))
		for i, e := range p.Plan {
			entries[i] = PlanEntry{
				Description: e.Description,
				Status:      e.Status,
			}
		}
		a.sendUpdate(AgentEvent{
			Type:        EventTypePlan,
			SessionID:   threadID,
			OperationID: p.TurnID,
			PlanEntries: entries,
		})

	case codex.NotifyError:
		var p codex.ErrorParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse error notification", zap.Error(err))
			return
		}
		a.sendUpdate(AgentEvent{
			Type:      EventTypeError,
			SessionID: threadID,
			Error:     p.Message,
		})

	case codex.NotifyItemStarted:
		var p codex.ItemStartedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse item started", zap.Error(err))
			return
		}
		if p.Item == nil {
			return
		}
		// Map Codex item types to tool call updates
		// Item types: "userMessage", "agentMessage", "commandExecution", "fileChange", "reasoning"
		switch p.Item.Type {
		case "commandExecution":
			a.sendUpdate(AgentEvent{
				Type:        EventTypeToolCall,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolName:    "commandExecution",
				ToolTitle:   p.Item.Command,
				ToolStatus:  "running",
				ToolArgs: map[string]interface{}{
					"command": p.Item.Command,
					"cwd":     p.Item.Cwd,
				},
			})
		case "fileChange":
			// Build title from file paths
			var title string
			if len(p.Item.Changes) > 0 {
				title = p.Item.Changes[0].Path
				if len(p.Item.Changes) > 1 {
					title += fmt.Sprintf(" (+%d more)", len(p.Item.Changes)-1)
				}
			}
			a.sendUpdate(AgentEvent{
				Type:        EventTypeToolCall,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolName:    "fileChange",
				ToolTitle:   title,
				ToolStatus:  "running",
			})
		}

	case codex.NotifyItemCompleted:
		var p codex.ItemCompletedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse item completed", zap.Error(err))
			return
		}
		if p.Item == nil {
			return
		}
		// Only send updates for tool-like items
		if p.Item.Type == "commandExecution" || p.Item.Type == "fileChange" {
			status := "complete"
			if p.Item.Status == "failed" {
				status = "error"
			}
			update := AgentEvent{
				Type:        EventTypeToolUpdate,
				SessionID:   threadID,
				OperationID: turnID,
				ToolCallID:  p.Item.ID,
				ToolStatus:  status,
			}
			// Include output for commands
			if p.Item.Type == "commandExecution" && p.Item.AggregatedOutput != "" {
				update.ToolResult = p.Item.AggregatedOutput
			}
			// Include diff for file changes
			if p.Item.Type == "fileChange" && len(p.Item.Changes) > 0 {
				var diffs []string
				for _, c := range p.Item.Changes {
					if c.Diff != "" {
						diffs = append(diffs, c.Diff)
					}
				}
				if len(diffs) > 0 {
					update.Diff = strings.Join(diffs, "\n")
				}
			}
			a.sendUpdate(update)
		}

	case codex.NotifyItemCmdExecOutputDelta:
		var p codex.CommandOutputDeltaParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse command output delta", zap.Error(err))
			return
		}
		a.sendUpdate(AgentEvent{
			Type:        EventTypeToolUpdate,
			SessionID:   threadID,
			OperationID: turnID,
			ToolCallID:  p.ItemID,
			ToolResult:  p.Delta,
		})

	case codex.NotifyThreadTokenUsageUpdated:
		var p codex.ThreadTokenUsageUpdatedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse thread token usage updated notification", zap.Error(err))
			return
		}
		// Extract context window information from the token usage update
		if p.TokenUsage != nil && p.TokenUsage.ModelContextWindow > 0 {
			contextWindowSize := p.TokenUsage.ModelContextWindow
			contextWindowUsed := int64(p.TokenUsage.Last.TotalTokens)

			remaining := contextWindowSize - contextWindowUsed
			if remaining < 0 {
				remaining = 0
			}
			efficiency := float64(contextWindowUsed) / float64(contextWindowSize) * 100

			a.logger.Debug("emitting context window event",
				zap.Int64("size", contextWindowSize),
				zap.Int64("used", contextWindowUsed),
				zap.Int64("remaining", remaining),
				zap.Float64("efficiency", efficiency))

			a.sendUpdate(AgentEvent{
				Type:                   EventTypeContextWindow,
				SessionID:              threadID,
				OperationID:            turnID,
				ContextWindowSize:      contextWindowSize,
				ContextWindowUsed:      contextWindowUsed,
				ContextWindowRemaining: remaining,
				ContextEfficiency:      efficiency,
			})
		}

	case codex.NotifyTokenCount:
		// Legacy token_count notification - ignore as we now use thread/tokenUsage/updated
		a.logger.Debug("ignoring legacy token_count notification")

	case codex.NotifyContextCompacted:
		var p codex.ContextCompactedParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse context compacted notification", zap.Error(err))
			return
		}
		a.logger.Info("context compacted",
			zap.String("thread_id", p.ThreadID),
			zap.String("turn_id", p.TurnID))
		// We could emit an event here if we want to notify the frontend about compaction

	default:
		// Log unhandled notifications at debug level
		a.logger.Debug("unhandled notification", zap.String("method", method))
	}
}

// handleRequest processes Codex requests (approval requests) and calls permissionHandler.
func (a *CodexAdapter) handleRequest(id interface{}, method string, params json.RawMessage) {
	a.logger.Debug("codex: received request",
		zap.Any("id", id),
		zap.String("method", method))

	a.mu.RLock()
	handler := a.permissionHandler
	a.mu.RUnlock()

	switch method {
	case codex.NotifyItemCmdExecRequestApproval:
		var p codex.CommandApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse command approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, p.ThreadID, p.ItemID, types.ActionTypeCommand, p.Command, map[string]interface{}{
			"command":   p.Command,
			"cwd":       p.Cwd,
			"reasoning": p.Reasoning,
		}, p.Options)

	case codex.NotifyItemFileChangeRequestApproval:
		var p codex.FileChangeApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse file change approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, p.ThreadID, p.ItemID, types.ActionTypeFileWrite, p.Path, map[string]interface{}{
			"path":      p.Path,
			"diff":      p.Diff,
			"reasoning": p.Reasoning,
		}, p.Options)

	default:
		a.logger.Warn("unhandled request", zap.String("method", method))
		if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.MethodNotFound, Message: "method not found"}); err != nil {
			a.logger.Warn("failed to send method not found response", zap.Error(err))
		}
	}
}

// handleApprovalRequest handles permission request logic for both command and file change approvals.
func (a *CodexAdapter) handleApprovalRequest(
	id interface{},
	handler PermissionHandler,
	threadID string,
	itemID string,
	actionType string,
	title string,
	details map[string]interface{},
	optionStrings []string,
) {
	// Build permission options from Codex options
	options := make([]PermissionOption, len(optionStrings))
	for i, opt := range optionStrings {
		kind := "allow_once"
		switch opt {
		case "approveAlways":
			kind = "allow_always"
		case "reject":
			kind = "reject_once"
		}
		options[i] = PermissionOption{
			OptionID: opt,
			Name:     opt,
			Kind:     kind,
		}
	}

	// If no options provided, use defaults
	if len(options) == 0 {
		options = []PermissionOption{
			{OptionID: "approve", Name: "Approve", Kind: "allow_once"},
			{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
		}
	}

	req := &PermissionRequest{
		SessionID:     threadID,
		ToolCallID:    itemID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: details,
	}

	if handler == nil {
		// Auto-approve if no handler
		if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
			Decision: "accept",
		}, nil); err != nil {
			a.logger.Warn("failed to send approval response", zap.Error(err))
		}
		return
	}

	ctx := context.Background()
	resp, err := handler(ctx, req)
	if err != nil {
		a.logger.Error("permission handler error", zap.Error(err))
		if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
			Decision: "decline",
		}, nil); err != nil {
			a.logger.Warn("failed to send decline response", zap.Error(err))
		}
		return
	}

	// Map frontend option to Codex decision
	// Codex accepts: "accept", "acceptForSession", "decline", "cancel"
	decision := "accept"
	if resp.Cancelled {
		decision = "cancel"
	} else {
		switch resp.OptionID {
		case "approve", "allow", "accept":
			decision = "accept"
		case "approveAlways", "allowAlways", "acceptForSession":
			decision = "acceptForSession"
		case "reject", "deny", "decline":
			decision = "decline"
		case "cancel":
			decision = "cancel"
		default:
			if resp.OptionID != "" {
				decision = resp.OptionID
			}
		}
	}

	if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
		Decision: decision,
	}, nil); err != nil {
		a.logger.Warn("failed to send approval response", zap.Error(err))
	}
}

// Verify interface implementation
var _ AgentAdapter = (*CodexAdapter)(nil)
