package adapter

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// ClaudeCodeAdapter implements AgentAdapter for Claude Code CLI using stream-json protocol.
// Claude Code uses a streaming JSON format over stdin/stdout with control requests for permissions.
type ClaudeCodeAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Subprocess stdin/stdout (set via Connect)
	stdin  io.Writer
	stdout io.Reader

	// Claude Code client for protocol communication
	client *claudecode.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state
	sessionID         string
	operationID       string // Current prompt operation
	sessionStatusSent bool   // Whether we've sent the session status event

	// Track pending tool calls to auto-complete on result
	pendingToolCalls map[string]bool

	// Agent info
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Stderr provider for error context
	stderrProvider StderrProvider

	// Result completion signaling
	resultCh chan resultComplete

	// Dynamic context window tracking
	mainModelName          string // Model name from assistant messages (excludes subagents)
	mainModelContextWindow int64  // Context window size (updated from result's model_usage)
	contextTokensUsed      int64  // Total tokens used (input + output + cache)

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// defaultContextWindow is the fallback context window size for Claude models
const defaultContextWindow = 200000

// resultComplete holds the result of a completed prompt
type resultComplete struct {
	success bool
	err     string
}

// NewClaudeCodeAdapter creates a new Claude Code protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
func NewClaudeCodeAdapter(cfg *Config, log *logger.Logger) *ClaudeCodeAdapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &ClaudeCodeAdapter{
		cfg:                    cfg,
		logger:                 log.WithFields(zap.String("adapter", "claude-code")),
		ctx:                    ctx,
		cancel:                 cancel,
		updatesCh:              make(chan AgentEvent, 100),
		mainModelContextWindow: defaultContextWindow,
		pendingToolCalls:       make(map[string]bool),
	}
}

// PrepareEnvironment performs protocol-specific setup before the agent process starts.
// Claude Code reads MCP configuration from settings files, but we handle MCP via kandev's
// built-in MCP server, so this is a no-op.
func (a *ClaudeCodeAdapter) PrepareEnvironment() (map[string]string, error) {
	a.logger.Info("PrepareEnvironment called",
		zap.Int("mcp_server_count", len(a.cfg.McpServers)))
	// Claude Code MCP configuration is handled externally or via CLI flags
	return nil, nil
}

// Connect wires up the stdin/stdout pipes from the running agent subprocess.
func (a *ClaudeCodeAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin != nil || a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdin = stdin
	a.stdout = stdout
	return nil
}

// Initialize establishes the Claude Code connection with the agent subprocess.
func (a *ClaudeCodeAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Claude Code adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Claude Code client
	a.client = claudecode.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetRequestHandler(a.handleControlRequest)
	a.client.SetMessageHandler(a.handleMessage)

	// Start reading from stdout with the adapter's context
	a.client.Start(a.ctx)

	// Store agent info (version will be populated from first message)
	a.agentInfo = &AgentInfo{
		Name:    "claude-code",
		Version: "unknown",
	}

	a.logger.Info("Claude Code adapter initialized")

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *ClaudeCodeAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new Claude Code session.
// Note: Claude Code sessions are created implicitly with the first prompt.
// The mcpServers parameter is ignored as Claude Code handles MCP separately.
func (a *ClaudeCodeAdapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate a session ID - Claude Code will return its own session ID
	// which we'll update when we receive the system message
	sessionID := uuid.New().String()
	a.sessionID = sessionID

	a.logger.Info("created new session placeholder", zap.String("session_id", sessionID))

	return sessionID, nil
}

// LoadSession resumes an existing Claude Code session.
// The session ID will be passed to Claude Code via --resume flag (handled by process manager).
func (a *ClaudeCodeAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()

	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	return nil
}

// Prompt sends a prompt to Claude Code and waits for completion.
func (a *ClaudeCodeAdapter) Prompt(ctx context.Context, message string) error {
	a.mu.Lock()
	client := a.client
	sessionID := a.sessionID
	operationID := uuid.New().String()
	a.operationID = operationID
	// Create channel to wait for result
	a.resultCh = make(chan resultComplete, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID))

	// Send user message
	if err := client.SendUserMessage(message); err != nil {
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to send user message: %w", err)
	}

	// Wait for result or context cancellation
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	select {
	case <-ctx.Done():
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return ctx.Err()
	case result := <-resultCh:
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		if !result.success && result.err != "" {
			return fmt.Errorf("prompt failed: %s", result.err)
		}
		a.logger.Info("prompt completed",
			zap.String("operation_id", operationID),
			zap.Bool("success", result.success))
		return nil
	}
}

// Cancel interrupts the current operation.
func (a *ClaudeCodeAdapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling operation")

	// Send interrupt control request
	return client.SendControlRequest(&claudecode.SDKControlRequest{
		Type:      claudecode.MessageTypeControlRequest,
		RequestID: uuid.New().String(),
		Request: claudecode.SDKControlRequestBody{
			Subtype: claudecode.SubtypeInterrupt,
		},
	})
}

// Updates returns the channel for agent events.
func (a *ClaudeCodeAdapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *ClaudeCodeAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation ID.
func (a *ClaudeCodeAdapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.operationID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *ClaudeCodeAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// SetStderrProvider sets the provider for recent stderr output.
func (a *ClaudeCodeAdapter) SetStderrProvider(provider StderrProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stderrProvider = provider
}

// Close releases resources held by the adapter.
func (a *ClaudeCodeAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing Claude Code adapter")

	// Cancel the context to stop the read loop
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

// RequiresProcessKill returns false because Claude Code agents exit when stdin is closed.
func (a *ClaudeCodeAdapter) RequiresProcessKill() bool {
	return false
}

// sendUpdate safely sends an event to the updates channel.
func (a *ClaudeCodeAdapter) sendUpdate(update AgentEvent) {
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
}

// handleControlRequest processes control requests (permission requests) from Claude Code.
func (a *ClaudeCodeAdapter) handleControlRequest(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received control request",
		zap.String("request_id", requestID),
		zap.String("subtype", req.Subtype),
		zap.String("tool_name", req.ToolName))

	switch req.Subtype {
	case claudecode.SubtypeCanUseTool:
		a.handleToolPermission(requestID, req)
	case claudecode.SubtypeHookCallback:
		a.handleHookCallback(requestID, req)
	default:
		a.logger.Warn("unhandled control request subtype",
			zap.String("subtype", req.Subtype))
		// Send error response
		if err := a.client.SendControlResponse(&claudecode.ControlResponseMessage{
			Type:      claudecode.MessageTypeControlResponse,
			RequestID: requestID,
			Response: &claudecode.ControlResponse{
				Subtype: "error",
				Error:   fmt.Sprintf("unhandled subtype: %s", req.Subtype),
			},
		}); err != nil {
			a.logger.Warn("failed to send error response", zap.Error(err))
		}
	}
}

// handleToolPermission processes can_use_tool permission requests.
func (a *ClaudeCodeAdapter) handleToolPermission(requestID string, req *claudecode.ControlRequest) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	// Determine action type based on tool name
	actionType := types.ActionTypeOther
	switch req.ToolName {
	case claudecode.ToolBash:
		actionType = types.ActionTypeCommand
	case claudecode.ToolWrite, claudecode.ToolEdit, claudecode.ToolNotebookEdit:
		actionType = types.ActionTypeFileWrite
	case claudecode.ToolRead, claudecode.ToolGlob, claudecode.ToolGrep:
		actionType = types.ActionTypeFileRead
	case claudecode.ToolWebFetch, claudecode.ToolWebSearch:
		actionType = types.ActionTypeNetwork
	}

	// Build title from tool name and key input
	title := req.ToolName
	if cmd, ok := req.Input["command"].(string); ok && req.ToolName == claudecode.ToolBash {
		title = cmd
	} else if path, ok := req.Input["file_path"].(string); ok {
		title = fmt.Sprintf("%s: %s", req.ToolName, path)
	}

	// Build permission options
	options := []PermissionOption{
		{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		{OptionID: "allowAlways", Name: "Allow Always", Kind: "allow_always"},
		{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
	}

	permReq := &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    req.ToolUseID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: req.Input,
	}

	// If no handler, auto-allow
	if handler == nil {
		a.logger.Info("auto-allowing tool (no handler)",
			zap.String("tool", req.ToolName))
		a.sendPermissionResponse(requestID, claudecode.BehaviorAllow)
		return
	}

	// Send permission notification through updates channel
	a.sendUpdate(AgentEvent{
		Type:              EventTypePermissionRequest,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        req.ToolUseID,
		PendingID:         requestID,
		PermissionTitle:   title,
		PermissionOptions: options,
		ActionType:        actionType,
		ActionDetails:     req.Input,
	})

	// Call permission handler (blocking)
	ctx := context.Background()
	resp, err := handler(ctx, permReq)
	if err != nil {
		a.logger.Error("permission handler error", zap.Error(err))
		a.sendPermissionResponse(requestID, claudecode.BehaviorDeny)
		return
	}

	// Map response to behavior
	behavior := claudecode.BehaviorAllow
	if resp.Cancelled {
		behavior = claudecode.BehaviorDeny
	} else {
		switch resp.OptionID {
		case "allow", "allowAlways", "approve", "approveAlways":
			behavior = claudecode.BehaviorAllow
		case "deny", "reject", "decline":
			behavior = claudecode.BehaviorDeny
		}
	}

	a.sendPermissionResponse(requestID, behavior)
}

// sendPermissionResponse sends a permission response to Claude Code.
func (a *ClaudeCodeAdapter) sendPermissionResponse(requestID string, behavior string) {
	resp := &claudecode.ControlResponseMessage{
		Type:      claudecode.MessageTypeControlResponse,
		RequestID: requestID,
		Response: &claudecode.ControlResponse{
			Subtype: "success",
			Result: &claudecode.PermissionResult{
				Behavior: behavior,
			},
		},
	}

	if err := a.client.SendControlResponse(resp); err != nil {
		a.logger.Warn("failed to send permission response", zap.Error(err))
	}
}

// handleHookCallback processes hook callback requests.
func (a *ClaudeCodeAdapter) handleHookCallback(requestID string, req *claudecode.ControlRequest) {
	a.logger.Info("received hook callback",
		zap.String("request_id", requestID),
		zap.String("hook_name", req.HookName))

	// For now, acknowledge hook callbacks with success
	if err := a.client.SendControlResponse(&claudecode.ControlResponseMessage{
		Type:      claudecode.MessageTypeControlResponse,
		RequestID: requestID,
		Response: &claudecode.ControlResponse{
			Subtype: "success",
		},
	}); err != nil {
		a.logger.Warn("failed to send hook callback response", zap.Error(err))
	}
}

// handleMessage processes streaming messages from Claude Code.
func (a *ClaudeCodeAdapter) handleMessage(msg *claudecode.CLIMessage) {
	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	switch msg.Type {
	case claudecode.MessageTypeSystem:
		a.handleSystemMessage(msg)

	case claudecode.MessageTypeAssistant:
		a.handleAssistantMessage(msg, sessionID, operationID)

	case claudecode.MessageTypeResult:
		a.handleResultMessage(msg, sessionID, operationID)

	default:
		a.logger.Debug("unhandled message type", zap.String("type", msg.Type))
	}
}

// handleSystemMessage processes system init messages.
// Note: System messages are session initialization, NOT turn completion.
// Turn completion is signaled by result messages.
func (a *ClaudeCodeAdapter) handleSystemMessage(msg *claudecode.CLIMessage) {
	a.logger.Info("received system message",
		zap.String("session_id", msg.SessionID),
		zap.String("status", msg.SessionStatus))

	// Update session ID if provided
	a.mu.Lock()
	if msg.SessionID != "" {
		a.sessionID = msg.SessionID
	}
	alreadySent := a.sessionStatusSent
	a.sessionStatusSent = true
	a.mu.Unlock()

	// Only send session status event once per session (on first prompt)
	// Claude Code sends system messages on every prompt, but we only want to
	// show "New session started" or "Session resumed" once
	if alreadySent {
		return
	}

	// Send session status event (NOT complete - that's only for result messages)
	a.sendUpdate(AgentEvent{
		Type:      EventTypeSessionStatus,
		SessionID: msg.SessionID,
		Data: map[string]any{
			"session_status": msg.SessionStatus,
			"init":           true,
		},
	})
}

// handleAssistantMessage processes assistant messages (text, thinking, tool calls).
func (a *ClaudeCodeAdapter) handleAssistantMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Log content block types for debugging
	blockTypes := make([]string, 0, len(msg.Message.Content))
	for _, block := range msg.Message.Content {
		blockTypes = append(blockTypes, block.Type)
	}
	a.logger.Info("processing assistant message",
		zap.Int("num_blocks", len(msg.Message.Content)),
		zap.Strings("block_types", blockTypes))

	// Update agent version and track main model name from model info
	if msg.Message.Model != "" && a.agentInfo != nil {
		a.agentInfo.Version = msg.Message.Model

		// Track the main model name for context window lookup
		// Only set if not already set (first model we see is the main one)
		a.mu.Lock()
		if a.mainModelName == "" {
			a.mainModelName = msg.Message.Model
			a.logger.Debug("tracking main model", zap.String("model", msg.Message.Model))
		}
		a.mu.Unlock()
	}

	// Process content blocks
	for _, block := range msg.Message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				a.sendUpdate(AgentEvent{
					Type:        EventTypeMessageChunk,
					SessionID:   sessionID,
					OperationID: operationID,
					Text:        block.Text,
				})
			}

		case "thinking":
			if block.Thinking != "" {
				a.sendUpdate(AgentEvent{
					Type:          EventTypeReasoning,
					SessionID:     sessionID,
					OperationID:   operationID,
					ReasoningText: block.Thinking,
				})
			}

		case "tool_use":
			// Build a human-readable title for the tool call
			toolTitle := block.Name
			if cmd, ok := block.Input["command"].(string); ok && block.Name == claudecode.ToolBash {
				toolTitle = cmd
			} else if path, ok := block.Input["file_path"].(string); ok {
				toolTitle = fmt.Sprintf("%s: %s", block.Name, path)
			}
			a.logger.Info("tool_use block received",
				zap.String("tool_call_id", block.ID),
				zap.String("tool_name", block.Name),
				zap.String("title", toolTitle))

			// Track this tool call as pending
			a.mu.Lock()
			a.pendingToolCalls[block.ID] = true
			a.mu.Unlock()

			a.sendUpdate(AgentEvent{
				Type:        EventTypeToolCall,
				SessionID:   sessionID,
				OperationID: operationID,
				ToolCallID:  block.ID,
				ToolName:    block.Name,
				ToolTitle:   toolTitle,
				ToolArgs:    block.Input,
				ToolStatus:  "running",
			})

		case "tool_result":
			status := "complete"
			if block.IsError {
				status = "error"
			}
			a.logger.Info("tool_result block received",
				zap.String("tool_call_id", block.ToolUseID),
				zap.String("status", status))

			// Remove from pending
			a.mu.Lock()
			delete(a.pendingToolCalls, block.ToolUseID)
			a.mu.Unlock()

			a.sendUpdate(AgentEvent{
				Type:        EventTypeToolUpdate,
				SessionID:   sessionID,
				OperationID: operationID,
				ToolCallID:  block.ToolUseID,
				ToolResult:  block.Content,
				ToolStatus:  status,
			})
		}
	}

	// Calculate and emit token usage as context window event
	if msg.Message.Usage != nil {
		usage := msg.Message.Usage

		// Calculate total tokens used (including cache tokens)
		contextUsed := usage.InputTokens + usage.OutputTokens +
			usage.CacheCreationInputTokens + usage.CacheReadInputTokens

		// Update tracked token usage
		a.mu.Lock()
		a.contextTokensUsed = contextUsed
		contextSize := a.mainModelContextWindow
		a.mu.Unlock()

		remaining := contextSize - contextUsed
		if remaining < 0 {
			remaining = 0
		}

		a.sendUpdate(AgentEvent{
			Type:                   EventTypeContextWindow,
			SessionID:              sessionID,
			OperationID:            operationID,
			ContextWindowSize:      contextSize,
			ContextWindowUsed:      contextUsed,
			ContextWindowRemaining: remaining,
			ContextEfficiency:      float64(contextUsed) / float64(contextSize) * 100,
		})
	}
}

// handleResultMessage processes result (completion) messages.
func (a *ClaudeCodeAdapter) handleResultMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	a.logger.Info("received result message",
		zap.Bool("is_error", msg.IsError),
		zap.Int("num_turns", msg.NumTurns))

	// Update session ID from result if provided
	if resultData := msg.GetResultData(); resultData != nil && resultData.SessionID != "" {
		a.mu.Lock()
		a.sessionID = resultData.SessionID
		sessionID = resultData.SessionID
		a.mu.Unlock()
	}

	// Auto-complete any pending tool calls that didn't receive explicit tool_result
	// This can happen if the tool result is not sent as a separate assistant message
	a.mu.Lock()
	pendingTools := make([]string, 0, len(a.pendingToolCalls))
	for toolID := range a.pendingToolCalls {
		pendingTools = append(pendingTools, toolID)
	}
	a.pendingToolCalls = make(map[string]bool) // Clear pending
	a.mu.Unlock()

	for _, toolID := range pendingTools {
		a.logger.Info("auto-completing pending tool call on result",
			zap.String("tool_call_id", toolID))
		a.sendUpdate(AgentEvent{
			Type:        EventTypeToolUpdate,
			SessionID:   sessionID,
			OperationID: operationID,
			ToolCallID:  toolID,
			ToolStatus:  "complete",
		})
	}

	// Extract actual context window from model_usage if available
	// This gives us the real context window size for the model
	a.mu.Lock()
	modelName := a.mainModelName
	if msg.ModelUsage != nil && modelName != "" {
		if modelStats, ok := msg.ModelUsage[modelName]; ok && modelStats.ContextWindow != nil {
			a.mainModelContextWindow = *modelStats.ContextWindow
			a.logger.Debug("updated context window from model_usage",
				zap.String("model", modelName),
				zap.Int64("context_window", a.mainModelContextWindow))
		}
	}
	contextSize := a.mainModelContextWindow
	contextUsed := a.contextTokensUsed
	a.mu.Unlock()

	// Emit final accurate context window event
	if contextUsed > 0 {
		remaining := contextSize - contextUsed
		if remaining < 0 {
			remaining = 0
		}
		a.sendUpdate(AgentEvent{
			Type:                   EventTypeContextWindow,
			SessionID:              sessionID,
			OperationID:            operationID,
			ContextWindowSize:      contextSize,
			ContextWindowUsed:      contextUsed,
			ContextWindowRemaining: remaining,
			ContextEfficiency:      float64(contextUsed) / float64(contextSize) * 100,
		})
	}

	// Send completion event
	a.sendUpdate(AgentEvent{
		Type:        EventTypeComplete,
		SessionID:   sessionID,
		OperationID: operationID,
		Data: map[string]any{
			"cost_usd":       msg.CostUSD,
			"duration_ms":    msg.DurationMS,
			"num_turns":      msg.NumTurns,
			"input_tokens":   msg.TotalInputTokens,
			"output_tokens":  msg.TotalOutputTokens,
			"is_error":       msg.IsError,
		},
	})

	// If there's result text, send it as a message chunk
	// Result can be either a ResultData object or an error string
	if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
		a.sendUpdate(AgentEvent{
			Type:        EventTypeMessageChunk,
			SessionID:   sessionID,
			OperationID: operationID,
			Text:        resultData.Text,
		})
	}

	// Signal completion
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh != nil {
		select {
		case resultCh <- resultComplete{success: !msg.IsError}:
			a.logger.Debug("signaled prompt completion")
		default:
			a.logger.Warn("result channel full, dropping signal")
		}
	}

	// Send error event if failed
	if msg.IsError {
		errorMsg := "prompt failed"
		// Error result can be a string (API error) or ResultData with Text
		if errStr := msg.GetResultString(); errStr != "" {
			errorMsg = errStr
		} else if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
			errorMsg = resultData.Text
		}
		a.sendUpdate(AgentEvent{
			Type:        EventTypeError,
			SessionID:   sessionID,
			OperationID: operationID,
			Error:       errorMsg,
		})
	}
}

// Verify interface implementation
var _ AgentAdapter = (*ClaudeCodeAdapter)(nil)
