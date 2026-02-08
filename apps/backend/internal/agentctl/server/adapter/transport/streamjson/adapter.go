// Package streamjson implements the stream-json transport adapter.
// This is the protocol used by Claude Code CLI (--output-format stream-json).
package streamjson

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// Re-export types needed by external packages
type (
	PermissionRequest  = types.PermissionRequest
	PermissionResponse = types.PermissionResponse
	PermissionOption   = streams.PermissionOption
	PermissionHandler  = types.PermissionHandler
	AgentEvent         = streams.AgentEvent
)

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// StderrProvider provides access to recent stderr output for error context.
type StderrProvider interface {
	GetRecentStderr() []string
}

// Adapter implements the transport adapter for agents using the stream-json protocol.
// Claude Code uses a streaming JSON format over stdin/stdout with control requests for permissions.
type Adapter struct {
	cfg    *shared.Config
	logger *logger.Logger

	// Agent identity (from config, for logging)
	agentID string

	// Normalizer for converting tool data to NormalizedPayload
	normalizer *Normalizer

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
	// Maps tool_use_id to the NormalizedPayload for enrichment with results
	pendingToolCalls map[string]*streams.NormalizedPayload

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

	// Available commands from initialize response (stored until session is created)
	pendingAvailableCommands []streams.AvailableCommand

	// Track whether text was streamed this turn to prevent duplicates from result.text
	// This is set to true when we send message_chunk events from assistant messages,
	// and reset to false at the start of each prompt.
	streamingTextSentThisTurn bool

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

// NewAdapter creates a new stream-json protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
// cfg.AgentID is required for debug file naming.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		cfg:                    cfg,
		logger:                 log.WithFields(zap.String("adapter", "stream-json"), zap.String("agent_id", cfg.AgentID)),
		agentID:                cfg.AgentID,
		normalizer:             NewNormalizer(),
		ctx:                    ctx,
		cancel:                 cancel,
		updatesCh:              make(chan AgentEvent, 100),
		mainModelContextWindow: defaultContextWindow,
		pendingToolCalls:       make(map[string]*streams.NormalizedPayload),
	}
}

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

	// Convert to JSON string
	configJSON, err := json.Marshal(mcpConfig)
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

// Connect wires up the stdin/stdout pipes from the running agent subprocess.
func (a *Adapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin != nil || a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdin = stdin
	a.stdout = stdout
	return nil
}

// Initialize establishes the stream-json connection with the agent subprocess.
func (a *Adapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing stream-json adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Claude Code client
	a.client = claudecode.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetRequestHandler(a.handleControlRequest)
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

	// Send initialize control request to get slash commands
	// This is required for streaming mode (input-format=stream-json)
	initResp, err := a.client.Initialize(ctx, 60*time.Second)
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

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// emitPendingCommands emits any pending available commands for the given session.
// Must be called after the mutex is unlocked.
func (a *Adapter) emitPendingCommands(sessionID string, commands []streams.AvailableCommand) {
	if len(commands) == 0 {
		return
	}
	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeAvailableCommands,
		SessionID:         sessionID,
		AvailableCommands: commands,
	})
	a.logger.Debug("emitted pending slash commands",
		zap.String("session_id", sessionID),
		zap.Int("count", len(commands)))
}

// takePendingCommands atomically takes and clears pending commands.
// Must be called with the mutex held.
func (a *Adapter) takePendingCommands() []streams.AvailableCommand {
	commands := a.pendingAvailableCommands
	a.pendingAvailableCommands = nil
	return commands
}

// NewSession creates a new stream-json session.
// Note: Sessions are created implicitly with the first prompt.
// The mcpServers parameter is ignored as this protocol handles MCP separately.
func (a *Adapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	a.mu.Lock()
	sessionID := uuid.New().String()
	a.sessionID = sessionID
	pendingCommands := a.takePendingCommands()
	a.mu.Unlock()

	a.logger.Info("created new session placeholder", zap.String("session_id", sessionID))
	a.emitPendingCommands(sessionID, pendingCommands)

	return sessionID, nil
}

// LoadSession resumes an existing stream-json session.
// The session ID will be passed to the agent via --resume flag (handled by process manager).
func (a *Adapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	a.sessionID = sessionID
	pendingCommands := a.takePendingCommands()
	a.mu.Unlock()

	a.logger.Info("loaded session", zap.String("session_id", sessionID))
	a.emitPendingCommands(sessionID, pendingCommands)

	return nil
}

// Prompt sends a prompt and waits for completion.
// Since stream-json protocol doesn't support multimodal content blocks, image attachments
// are saved to temp files in the workspace and the prompt is modified to reference them.
// Claude Code can then read these files with its Read tool.
func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment) error {
	a.mu.Lock()
	client := a.client
	sessionID := a.sessionID
	operationID := uuid.New().String()
	a.operationID = operationID
	workDir := a.cfg.WorkDir
	// Create channel to wait for result
	a.resultCh = make(chan resultComplete, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Handle image attachments by saving to temp files
	finalMessage := message
	if len(attachments) > 0 {
		imagePaths, err := a.saveImageAttachments(workDir, attachments)
		if err != nil {
			a.logger.Warn("failed to save image attachments", zap.Error(err))
		} else if len(imagePaths) > 0 {
			finalMessage = a.buildMessageWithImages(message, imagePaths)
		}
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID),
		zap.Int("attachments", len(attachments)))

	// Send user message
	if err := client.SendUserMessage(finalMessage); err != nil {
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

// saveImageAttachments saves image attachments to temp files in the workspace.
// Returns the relative paths to the saved files.
func (a *Adapter) saveImageAttachments(workDir string, attachments []v1.MessageAttachment) ([]string, error) {
	var imagePaths []string

	// Create temp directory for images
	tempDir := filepath.Join(workDir, ".kandev", "temp", "images")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	for _, att := range attachments {
		if att.Type != "image" {
			continue
		}

		// Decode base64 data
		imageData, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			a.logger.Warn("failed to decode image attachment", zap.Error(err))
			continue
		}

		// Determine file extension from mime type
		ext := ".png" // default
		switch att.MimeType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "image/png":
			ext = ".png"
		}

		// Generate unique filename
		filename := fmt.Sprintf("image-%s%s", uuid.New().String()[:8], ext)
		filePath := filepath.Join(tempDir, filename)

		// Write image to file
		if err := os.WriteFile(filePath, imageData, 0644); err != nil {
			a.logger.Warn("failed to write image file", zap.Error(err), zap.String("path", filePath))
			continue
		}

		// Store relative path from workspace root
		relPath, err := filepath.Rel(workDir, filePath)
		if err != nil {
			relPath = filePath // Fall back to absolute path
		}

		imagePaths = append(imagePaths, relPath)
		a.logger.Info("saved image attachment",
			zap.String("path", relPath),
			zap.Int("size", len(imageData)))
	}

	return imagePaths, nil
}

// buildMessageWithImages prepends image file references to the user's message.
func (a *Adapter) buildMessageWithImages(message string, imagePaths []string) string {
	var sb strings.Builder

	// Add image references at the beginning
	if len(imagePaths) == 1 {
		sb.WriteString(fmt.Sprintf("I've attached an image file that you should read and analyze: %s\n\n", imagePaths[0]))
	} else {
		sb.WriteString("I've attached image files that you should read and analyze:\n")
		for _, path := range imagePaths {
			sb.WriteString(fmt.Sprintf("- %s\n", path))
		}
		sb.WriteString("\n")
	}

	// Add the original message
	sb.WriteString(message)

	return sb.String()
}

// Cancel interrupts the current operation.
func (a *Adapter) Cancel(ctx context.Context) error {
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
func (a *Adapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *Adapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation ID.
func (a *Adapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.operationID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *Adapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// SetStderrProvider sets the provider for recent stderr output.
func (a *Adapter) SetStderrProvider(provider StderrProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stderrProvider = provider
}

// Close releases resources held by the adapter.
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing stream-json adapter")

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
func (a *Adapter) RequiresProcessKill() bool {
	return false
}

// sendUpdate safely sends an event to the updates channel.
func (a *Adapter) sendUpdate(update AgentEvent) {
	shared.LogNormalizedEvent(shared.ProtocolStreamJSON, a.agentID, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
}

// handleControlRequest processes control requests (permission requests) from the agent.
func (a *Adapter) handleControlRequest(requestID string, req *claudecode.ControlRequest) {
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
func (a *Adapter) handleToolPermission(requestID string, req *claudecode.ControlRequest) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
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

	// Build permission request with Claude Code's requestID.
	// The handler (process manager's handlePermissionRequest) will:
	// 1. Send the permission_request notification to the frontend
	// 2. Block waiting for user response
	// 3. Return the response
	// We pass PendingID so the handler uses Claude Code's requestID
	// instead of generating a new one - this ensures the frontend and backend
	// use the same ID for response lookup.
	permReq := &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    req.ToolUseID,
		Title:         title,
		Options:       options,
		ActionType:    actionType,
		ActionDetails: req.Input,
		PendingID:     requestID, // Use Claude Code's requestID so response lookup works
	}

	// If no handler, auto-allow
	if handler == nil {
		a.logger.Debug("auto-allowing tool (no handler)",
			zap.String("tool", req.ToolName))
		a.sendPermissionResponse(requestID, claudecode.BehaviorAllow)
		return
	}

	// Call permission handler (blocking) - it will send the notification
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

// sendPermissionResponse sends a permission response to the agent.
func (a *Adapter) sendPermissionResponse(requestID string, behavior string) {
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
func (a *Adapter) handleHookCallback(requestID string, req *claudecode.ControlRequest) {
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

// handleMessage processes streaming messages from the agent.
func (a *Adapter) handleMessage(msg *claudecode.CLIMessage) {
	// Log raw event for debugging
	if rawData, err := json.Marshal(msg); err == nil {
		shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, msg.Type, rawData)
	}

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	switch msg.Type {
	case claudecode.MessageTypeSystem:
		a.handleSystemMessage(msg)

	case claudecode.MessageTypeAssistant:
		a.handleAssistantMessage(msg, sessionID, operationID)

	case claudecode.MessageTypeUser:
		a.handleUserMessage(msg, sessionID, operationID)

	case claudecode.MessageTypeResult:
		a.handleResultMessage(msg, sessionID, operationID)

	default:
		a.logger.Debug("unhandled message type", zap.String("type", msg.Type))
	}
}

// handleSystemMessage processes system init messages.
// Note: System messages are session initialization, NOT turn completion.
// Turn completion is signaled by result messages.
func (a *Adapter) handleSystemMessage(msg *claudecode.CLIMessage) {
	a.logger.Info("received system message",
		zap.String("session_id", msg.SessionID),
		zap.String("status", msg.SessionStatus),
		zap.Int("slash_commands_count", len(msg.SlashCommands)))

	// Update session ID if provided
	a.mu.Lock()
	if msg.SessionID != "" {
		a.sessionID = msg.SessionID
	}
	alreadySent := a.sessionStatusSent
	a.sessionStatusSent = true
	a.mu.Unlock()

	// Emit available commands if present (do this on every system message,
	// not just the first, in case commands change)
	// Note: System message slash_commands is just an array of names (strings),
	// so we only have the name, not description. The initialize response has full details.
	if len(msg.SlashCommands) > 0 {
		commands := make([]streams.AvailableCommand, len(msg.SlashCommands))
		for i, name := range msg.SlashCommands {
			commands[i] = streams.AvailableCommand{
				Name:        name,
				Description: "", // System message only has names, not descriptions
			}
		}
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeAvailableCommands,
			SessionID:         msg.SessionID,
			AvailableCommands: commands,
		})
	}

	// Only send session status event once per session (on first prompt)
	// The agent sends system messages on every prompt, but we only want to
	// show "New session started" or "Session resumed" once
	if alreadySent {
		return
	}

	// Send session status event (NOT complete - that's only for result messages)
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeSessionStatus,
		SessionID: msg.SessionID,
		Data: map[string]any{
			"session_status": msg.SessionStatus,
			"init":           true,
		},
	})
}

// handleAssistantMessage processes assistant messages (text, thinking, tool calls).
func (a *Adapter) handleAssistantMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Extract parent tool use ID for subagent nesting
	parentToolUseID := msg.ParentToolUseID

	// Get content blocks (may be nil if content is a string)
	contentBlocks := msg.Message.GetContentBlocks()

	// Log content block types for debugging
	blockTypes := make([]string, 0, len(contentBlocks))
	for _, block := range contentBlocks {
		blockTypes = append(blockTypes, block.Type)
	}
	a.logger.Debug("processing assistant message",
		zap.Int("num_blocks", len(contentBlocks)),
		zap.Strings("block_types", blockTypes),
		zap.String("parent_tool_use_id", parentToolUseID))

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
	for _, block := range contentBlocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				// Mark that we've sent streaming text this turn
				// This prevents duplicate content from result.text
				a.mu.Lock()
				a.streamingTextSentThisTurn = true
				a.mu.Unlock()

				a.sendUpdate(AgentEvent{
					Type:        streams.EventTypeMessageChunk,
					SessionID:   sessionID,
					OperationID: operationID,
					Text:        block.Text,
				})
			}

		case "thinking":
			if block.Thinking != "" {
				a.sendUpdate(AgentEvent{
					Type:          streams.EventTypeReasoning,
					SessionID:     sessionID,
					OperationID:   operationID,
					ReasoningText: block.Thinking,
				})
			}

		case "tool_use":
			// Generate normalized payload using the normalizer
			normalizedPayload := a.normalizer.NormalizeToolCall(block.Name, block.Input)

			// Detect specific tool operation type for logging
			toolType := DetectStreamJSONToolType(block.Name)

			// Build a human-readable title for the tool call
			toolTitle := block.Name
			if cmd, ok := block.Input["command"].(string); ok && block.Name == claudecode.ToolBash {
				toolTitle = cmd
			} else if path, ok := block.Input["file_path"].(string); ok {
				toolTitle = fmt.Sprintf("%s: %s", block.Name, path)
			}
			a.logger.Debug("tool_use block received",
				zap.String("tool_call_id", block.ID),
				zap.String("tool_name", block.Name),
				zap.String("tool_type", toolType),
				zap.String("title", toolTitle))

			// Track this tool call as pending with its payload for result enrichment
			a.mu.Lock()
			a.pendingToolCalls[block.ID] = normalizedPayload
			a.mu.Unlock()

			a.sendUpdate(AgentEvent{
				Type:              streams.EventTypeToolCall,
				SessionID:         sessionID,
				OperationID:       operationID,
				ToolCallID:        block.ID,
				ParentToolCallID:  parentToolUseID,
				ToolName:          block.Name,
				ToolTitle:         toolTitle,
				ToolStatus:        "running",
				NormalizedPayload: normalizedPayload,
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
			Type:                   streams.EventTypeContextWindow,
			SessionID:              sessionID,
			OperationID:            operationID,
			ContextWindowSize:      contextSize,
			ContextWindowUsed:      contextUsed,
			ContextWindowRemaining: remaining,
			ContextEfficiency:      float64(contextUsed) / float64(contextSize) * 100,
		})
	}
}

// handleUserMessage processes user messages containing tool results or slash command output.
// Claude Code sends tool results back as user messages with tool_result content blocks.
// For slash commands, content may be a plain string wrapped in <local-command-stdout> tags.
func (a *Adapter) handleUserMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Check if content is a string (slash command output)
	if contentStr := msg.Message.GetContentString(); contentStr != "" {
		// Extract text from <local-command-stdout> tags if present
		text := contentStr
		if strings.HasPrefix(text, "<local-command-stdout>") && strings.HasSuffix(text, "</local-command-stdout>") {
			text = strings.TrimPrefix(text, "<local-command-stdout>")
			text = strings.TrimSuffix(text, "</local-command-stdout>")
		}

		if text != "" {
			a.logger.Info("received user message with string content (slash command output)",
				zap.String("session_id", sessionID),
				zap.Int("content_length", len(text)))

			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        text,
			})
		}
		return
	}

	// Process content blocks looking for tool_result
	contentBlocks := msg.Message.GetContentBlocks()
	for _, block := range contentBlocks {
		if block.Type != "tool_result" {
			continue
		}

		// Get and enrich the pending payload with result content
		a.mu.Lock()
		payload := a.pendingToolCalls[block.ToolUseID]
		delete(a.pendingToolCalls, block.ToolUseID)
		a.mu.Unlock()

		// Enrich payload with result content
		if payload != nil && block.Content != "" {
			a.normalizer.NormalizeToolResult(payload, block.Content)
		}

		// If there's an error, set the error flag on the payload
		// This ensures the frontend can display error messages properly
		if payload != nil && block.IsError {
			if payload.HttpRequest() != nil {
				payload.HttpRequest().IsError = true
			}
		}

		// Determine status
		status := "complete"
		if block.IsError {
			status = "error"
		}

		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			OperationID:       operationID,
			ToolCallID:        block.ToolUseID,
			ToolStatus:        status,
			NormalizedPayload: payload,
		})
	}
}

// handleResultMessage processes result (completion) messages.
func (a *Adapter) handleResultMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
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
	a.pendingToolCalls = make(map[string]*streams.NormalizedPayload) // Clear pending
	a.mu.Unlock()

	for _, toolID := range pendingTools {
		a.logger.Debug("auto-completing pending tool call on result",
			zap.String("tool_call_id", toolID))
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypeToolUpdate,
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
			Type:                   streams.EventTypeContextWindow,
			SessionID:              sessionID,
			OperationID:            operationID,
			ContextWindowSize:      contextSize,
			ContextWindowUsed:      contextUsed,
			ContextWindowRemaining: remaining,
			ContextEfficiency:      float64(contextUsed) / float64(contextSize) * 100,
		})
	}

	// Check if text was already streamed this turn via assistant messages
	a.mu.RLock()
	textWasStreamed := a.streamingTextSentThisTurn
	a.mu.RUnlock()

	// Only send result.text if NO text was streamed this turn.
	// If text was already streamed via assistant messages, result.text is a duplicate.
	// This prevents the same content from appearing twice in the conversation.
	if !textWasStreamed {
		var resultText string
		if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
			resultText = resultData.Text
		} else if resultStr := msg.GetResultString(); resultStr != "" {
			resultText = resultStr
		}
		if resultText != "" {
			a.logger.Debug("sending result text as message_chunk (no streaming text this turn)",
				zap.Int("text_length", len(resultText)))
			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        resultText,
			})
		}
	} else {
		a.logger.Debug("skipping result text (streaming text already sent this turn)")
	}

	// Reset the streaming flag for next turn
	a.mu.Lock()
	a.streamingTextSentThisTurn = false
	a.mu.Unlock()

	// Build error message from errors array if available
	var errorMsg string
	if msg.IsError && len(msg.Errors) > 0 {
		errorMsg = strings.Join(msg.Errors, "; ")
	}

	// Send completion event AFTER any result text chunk
	completeData := map[string]any{
		"cost_usd":      msg.CostUSD,
		"duration_ms":   msg.DurationMS,
		"num_turns":     msg.NumTurns,
		"input_tokens":  msg.TotalInputTokens,
		"output_tokens": msg.TotalOutputTokens,
		"is_error":      msg.IsError,
	}
	if len(msg.Errors) > 0 {
		completeData["errors"] = msg.Errors
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeComplete,
		SessionID:   sessionID,
		OperationID: operationID,
		Error:       errorMsg,
		Data:        completeData,
	})

	// Signal completion
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh != nil {
		select {
		case resultCh <- resultComplete{success: !msg.IsError, err: errorMsg}:
			a.logger.Debug("signaled prompt completion")
		default:
			a.logger.Warn("result channel full, dropping signal")
		}
	}

	// Send error event if failed
	if msg.IsError {
		errMsg := "prompt failed"
		// Use errors array first (most specific), then fallback to result string/data
		if len(msg.Errors) > 0 {
			errMsg = strings.Join(msg.Errors, "; ")
		} else if errStr := msg.GetResultString(); errStr != "" {
			errMsg = errStr
		} else if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
			errMsg = resultData.Text
		}
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypeError,
			SessionID:   sessionID,
			OperationID: operationID,
			Error:       errMsg,
		})
	}
}
