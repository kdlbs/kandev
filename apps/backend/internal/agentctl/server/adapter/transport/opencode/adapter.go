// Package opencode implements the OpenCode transport adapter.
// OpenCode uses a REST/SSE protocol over HTTP, spawning its own HTTP server.
package opencode

import (
	"bufio"
	"context"
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
	"github.com/kandev/kandev/pkg/opencode"
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

// Adapter implements the transport adapter for OpenCode.
// OpenCode spawns an HTTP server and communicates via REST API calls and SSE for events.
type Adapter struct {
	cfg    *shared.Config
	logger *logger.Logger

	// Agent identity (from config, for logging)
	agentID string

	// Normalizer for converting tool data to NormalizedPayload
	normalizer *Normalizer

	// Subprocess stdout (for parsing server URL)
	stdout io.Reader

	// HTTP client (created after server starts)
	client *opencode.Client

	// Server configuration
	serverURL string
	password  string
	directory string

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state
	sessionID   string
	operationID string

	// Agent info
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Pending permission requests
	pendingPermissions map[string]*permissionRequest

	// Result completion signaling
	resultCh chan resultComplete

	// Token tracking
	totalTokens        int
	modelContextWindow int

	// Tool call deduplication - track which tool calls we've seen to avoid duplicates
	seenToolCalls map[string]bool

	// Text part tracking - track text parts by part ID to compute incremental text
	textParts map[string]*textPartState

	// Message role tracking - track message roles (user/assistant) by messageID
	// Used to filter out user messages from being processed as agent output
	messageRoles map[string]string

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// textPartState tracks the state of a text part for incremental text computation
type textPartState struct {
	lastTextLen int // Length of text we've already sent
}

// permissionRequest tracks a pending permission
type permissionRequest struct {
	requestID  string
	toolCallID string
	toolName   string
}

// resultComplete holds the result of a completed prompt
// Note: Fields are currently unused but kept for future implementation
type resultComplete struct {
	_ bool   // success placeholder
	_ string // err placeholder
}

// NewAdapter creates a new OpenCode protocol adapter.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())
	agentID := cfg.AgentID
	if agentID == "" {
		agentID = "opencode" // Fallback for unknown agents using this protocol
	}
	return &Adapter{
		cfg:                cfg,
		logger:             log.WithFields(zap.String("adapter", "opencode"), zap.String("agent_id", agentID)),
		agentID:            agentID,
		normalizer:         NewNormalizer(),
		ctx:                ctx,
		cancel:             cancel,
		updatesCh:          make(chan AgentEvent, 100),
		pendingPermissions: make(map[string]*permissionRequest),
		seenToolCalls:      make(map[string]bool),
		password:           opencode.GenerateServerPassword(),
		directory:          cfg.WorkDir,
		modelContextWindow: 200000, // Default
	}
}

// PrepareEnvironment performs protocol-specific setup before the agent process starts.
// For OpenCode, we write MCP servers to ~/.config/opencode/opencode.json and return
// environment variables for server authentication.
func (a *Adapter) PrepareEnvironment() (map[string]string, error) {
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

	// Write MCP servers to OpenCode config file
	if err := WriteOpenCodeMcpConfig(a.cfg.McpServers, "", a.logger); err != nil {
		a.logger.Warn("failed to write OpenCode MCP config", zap.Error(err))
		// Continue - MCP servers are optional
	}

	env := map[string]string{
		"OPENCODE_SERVER_PASSWORD": a.password,
	}

	// Set up permissions based on config
	if a.cfg.AutoApprove {
		env["OPENCODE_PERMISSION"] = `{"question":"deny"}`
	} else {
		env["OPENCODE_PERMISSION"] = `{"edit":"ask","bash":"ask","webfetch":"ask","doom_loop":"ask","external_directory":"ask","question":"deny"}`
	}

	return env, nil
}

// Connect wires up the stdout pipe from the running agent subprocess.
// For OpenCode, we only need stdout to parse the server URL.
func (a *Adapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdout = stdout
	return nil
}

// Initialize establishes the OpenCode connection by waiting for the server URL.
func (a *Adapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing OpenCode adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Wait for server to print listening URL
	serverURL, err := a.waitForServerURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to get server URL: %w", err)
	}

	a.mu.Lock()
	a.serverURL = serverURL
	a.mu.Unlock()

	a.logger.Info("OpenCode server started", zap.String("url", serverURL))

	// Create HTTP client
	a.client = opencode.NewClient(serverURL, a.directory, a.password, a.logger)

	// Set up event handler
	a.client.SetEventHandler(a.handleSDKEvent)

	// Wait for server to be healthy
	healthCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := a.client.WaitForHealth(healthCtx); err != nil {
		return fmt.Errorf("server health check failed: %w", err)
	}

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    a.agentID,
		Version: "unknown",
	}

	a.logger.Info("OpenCode adapter initialized")

	return nil
}

// waitForServerURL reads stdout until we find the server URL
func (a *Adapter) waitForServerURL(ctx context.Context) (string, error) {
	deadline := time.Now().Add(180 * time.Second)

	scanner := bufio.NewScanner(a.stdout)
	var capturedLines []string

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Try to scan with a short timeout
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read stdout: %w", err)
			}
			// EOF - server exited
			tail := strings.Join(capturedLines, "\n")
			return "", fmt.Errorf("server exited before printing URL\nOutput:\n%s", tail)
		}

		line := scanner.Text()

		// Capture for error reporting
		if len(capturedLines) < 64 {
			capturedLines = append(capturedLines, line)
		}

		// Check for the listening URL
		if url, found := strings.CutPrefix(line, "opencode server listening on "); found {
			url = strings.TrimSpace(url)
			a.logger.Info("found server URL", zap.String("url", url))

			// Continue draining stdout in background
			go func() {
				for scanner.Scan() {
					// Just drain
				}
			}()

			return url, nil
		}
	}

	tail := strings.Join(capturedLines[max(0, len(capturedLines)-12):], "\n")
	return "", fmt.Errorf("timeout waiting for server URL\nOutput tail:\n%s", tail)
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new OpenCode session.
func (a *Adapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	// Clear any stale state from previous sessions
	a.clearSessionState()

	sessionID, err := a.client.CreateSession(ctx)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()

	a.logger.Info("created new session", zap.String("session_id", sessionID))

	// Emit session status event for resume token storage
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeSessionStatus,
		SessionID: sessionID,
		Data: map[string]any{
			"session_status": "active",
			"init":           true,
		},
	})

	return sessionID, nil
}

// LoadSession resumes an existing OpenCode session by forking it.
func (a *Adapter) LoadSession(ctx context.Context, sessionID string) error {
	// Clear any stale state from previous sessions
	a.clearSessionState()

	// Fork the session to create a new one with the same history
	newSessionID, err := a.client.ForkSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("fork session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = newSessionID
	a.mu.Unlock()

	a.logger.Info("loaded session (forked)",
		zap.String("original_session_id", sessionID),
		zap.String("new_session_id", newSessionID))

	return nil
}

// Prompt sends a prompt to OpenCode and waits for completion.
func (a *Adapter) Prompt(ctx context.Context, message string) error {
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

	// Start event stream before sending prompt
	if err := client.StartEventStream(a.ctx, sessionID); err != nil {
		a.logger.Warn("failed to start event stream", zap.Error(err))
	}

	// Send prompt
	// TODO: Support model selection from config
	if err := client.SendPrompt(ctx, sessionID, message, nil, "", ""); err != nil {
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	// Wait for result via control channel or context cancellation
	controlCh := client.ControlChannel()

	for {
		select {
		case <-ctx.Done():
			// Request cancellation
			_ = a.Cancel(context.Background())
			return ctx.Err()

		case control, ok := <-controlCh:
			if !ok {
				// Channel closed
				a.sendUpdate(AgentEvent{
					Type:      streams.EventTypeComplete,
					SessionID: sessionID,
				})
				return nil
			}

			switch control.Type {
			case "idle":
				a.logger.Info("session idle, prompt complete")
				a.sendUpdate(AgentEvent{
					Type:      streams.EventTypeComplete,
					SessionID: sessionID,
				})
				return nil

			case "auth_required":
				a.sendUpdate(AgentEvent{
					Type:      streams.EventTypeError,
					SessionID: sessionID,
					Error:     fmt.Sprintf("Authentication required: %s", control.Message),
				})
				return fmt.Errorf("auth required: %s", control.Message)

			case "session_error":
				a.sendUpdate(AgentEvent{
					Type:      streams.EventTypeError,
					SessionID: sessionID,
					Error:     control.Message,
				})
				// Continue - might recover

			case "disconnected":
				a.sendUpdate(AgentEvent{
					Type:      streams.EventTypeComplete,
					SessionID: sessionID,
				})
				return nil
			}
		}
	}
}

// Cancel cancels the current operation.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	sessionID := a.sessionID
	a.mu.RUnlock()

	if client == nil {
		return nil
	}

	a.logger.Info("cancelling operation", zap.String("session_id", sessionID))

	return client.Abort(ctx, sessionID)
}

// Updates returns a channel that receives agent events.
func (a *Adapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *Adapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation/turn ID.
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

// Close releases resources held by the adapter.
func (a *Adapter) Close() error {
	a.logger.Info("OpenCode adapter Close() called")

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		a.logger.Debug("OpenCode adapter already closed")
		return nil
	}
	a.closed = true

	a.logger.Debug("OpenCode adapter: cancelling context")
	a.cancel()

	a.logger.Debug("OpenCode adapter: closing HTTP client")
	if a.client != nil {
		a.client.Close()
	}

	a.logger.Debug("OpenCode adapter: closing updates channel")
	close(a.updatesCh)

	a.logger.Info("OpenCode adapter closed successfully")
	return nil
}

// RequiresProcessKill returns true because OpenCode runs as an HTTP server
// and does not exit when stdin is closed.
func (a *Adapter) RequiresProcessKill() bool {
	return true
}

// clearSessionState clears session-specific tracking state.
// This should be called when starting a new session to prevent memory leaks
// and stale data from previous sessions.
func (a *Adapter) clearSessionState() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.textParts = make(map[string]*textPartState)
	a.messageRoles = make(map[string]string)
	a.seenToolCalls = make(map[string]bool)
}

// sendUpdate sends an event to the updates channel
func (a *Adapter) sendUpdate(event AgentEvent) {
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()

	if closed {
		return
	}

	shared.LogNormalizedEvent(shared.ProtocolOpenCode, a.agentID, &event)
	select {
	case a.updatesCh <- event:
	default:
		a.logger.Warn("update channel full, dropping event",
			zap.String("event_type", event.Type))
	}
}

// handleSDKEvent processes events from the OpenCode SSE stream
func (a *Adapter) handleSDKEvent(event *opencode.SDKEventEnvelope) {
	// Log raw event for debugging
	shared.LogRawEvent(shared.ProtocolOpenCode, a.agentID, event.Type, event.Properties)

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	switch event.Type {
	case opencode.SDKEventMessagePartUpdated:
		a.handleMessagePartUpdated(event.Properties, sessionID, operationID)

	case opencode.SDKEventMessageUpdated:
		a.handleMessageUpdated(event.Properties, sessionID)

	case opencode.SDKEventPermissionAsked:
		a.handlePermissionAsked(event.Properties, sessionID)

	case opencode.SDKEventSessionError:
		props, err := opencode.ParseSessionError(event.Properties)
		if err != nil {
			return
		}
		if props.Error != nil {
			a.sendUpdate(AgentEvent{
				Type:      streams.EventTypeError,
				SessionID: sessionID,
				Error:     props.Error.GetMessage(),
			})
		}
	}
}

// handleMessagePartUpdated processes message.part.updated events
func (a *Adapter) handleMessagePartUpdated(props json.RawMessage, sessionID, operationID string) {
	parsed, err := opencode.ParseMessagePartUpdated(props)
	if err != nil {
		a.logger.Warn("failed to parse message.part.updated", zap.Error(err))
		return
	}

	part := parsed.Part

	// Filter out user messages - only process assistant messages
	// The role is tracked from message.updated events
	if part.MessageID != "" {
		a.mu.RLock()
		role := a.messageRoles[part.MessageID]
		a.mu.RUnlock()
		if role == "user" {
			// Skip user message parts - we don't want to echo user input as agent output
			return
		}
	}

	switch part.Type {
	case opencode.PartTypeText:
		// Track text parts by ID to compute incremental text
		// IMPORTANT: Always prefer cumulative part.Text over delta to avoid duplication
		// The delta may be sent multiple times with the same content
		partID := part.ID
		if partID == "" {
			// Fallback: use MessageID + type as unique key
			partID = part.MessageID + ":text"
		}

		a.mu.Lock()
		if a.textParts == nil {
			a.textParts = make(map[string]*textPartState)
		}
		state := a.textParts[partID]
		if state == nil {
			state = &textPartState{lastTextLen: 0}
			a.textParts[partID] = state
		}

		// Determine what text to send - prefer cumulative text to avoid duplication
		var textToSend string
		if part.Text != "" {
			// Use cumulative text - compute what's new since last time
			if len(part.Text) > state.lastTextLen {
				textToSend = part.Text[state.lastTextLen:]
				state.lastTextLen = len(part.Text)
			}
		} else if parsed.Delta != "" && state.lastTextLen == 0 {
			// No cumulative text available, use delta only for first chunk
			// This handles streaming before cumulative text is populated
			textToSend = parsed.Delta
		}
		a.mu.Unlock()

		if textToSend != "" {
			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        textToSend,
			})
		}

	case opencode.PartTypeReasoning:
		// Track reasoning parts by ID to compute incremental text
		// IMPORTANT: Always prefer cumulative part.Text over delta to avoid duplication
		partID := part.ID
		if partID == "" {
			// Fallback: use MessageID + type as unique key
			partID = part.MessageID + ":reasoning"
		}

		a.mu.Lock()
		if a.textParts == nil {
			a.textParts = make(map[string]*textPartState)
		}
		state := a.textParts[partID]
		if state == nil {
			state = &textPartState{lastTextLen: 0}
			a.textParts[partID] = state
		}

		// Determine what text to send - prefer cumulative text to avoid duplication
		var textToSend string
		if part.Text != "" {
			// Use cumulative text - compute what's new since last time
			if len(part.Text) > state.lastTextLen {
				textToSend = part.Text[state.lastTextLen:]
				state.lastTextLen = len(part.Text)
			}
		} else if parsed.Delta != "" && state.lastTextLen == 0 {
			// No cumulative text available, use delta only for first chunk
			textToSend = parsed.Delta
		}
		a.mu.Unlock()

		if textToSend != "" {
			a.sendUpdate(AgentEvent{
				Type:          streams.EventTypeReasoning,
				SessionID:     sessionID,
				OperationID:   operationID,
				ReasoningText: textToSend,
			})
		}

	case opencode.PartTypeTool:
		// Tool call
		if part.State == nil {
			return
		}

		toolCallID := part.CallID
		toolName := part.Tool
		state := part.State

		// Determine status
		var status string
		switch state.Status {
		case opencode.ToolStatusPending:
			status = "pending"
		case opencode.ToolStatusRunning:
			status = "running"
		case opencode.ToolStatusCompleted:
			status = "complete" // Normalized to "complete" for frontend consistency
		case opencode.ToolStatusError:
			status = "error"
		default:
			status = state.Status
		}

		title := state.Title
		if title == "" {
			title = toolName
		}

		var args map[string]any
		if state.Input != nil {
			_ = json.Unmarshal(state.Input, &args) // Ignore error - args are optional
		}

		// Generate normalized payload
		normalizedPayload := a.normalizer.NormalizeToolCall(toolName, args)

		// Track if we've seen this tool call before
		a.mu.Lock()
		if a.seenToolCalls == nil {
			a.seenToolCalls = make(map[string]bool)
		}
		isFirstEvent := !a.seenToolCalls[toolCallID]
		a.seenToolCalls[toolCallID] = true
		a.mu.Unlock()

		// Emit tool_call for the first event we see for this tool call
		// This creates the message in the database
		if isFirstEvent {
			a.sendUpdate(AgentEvent{
				Type:              streams.EventTypeToolCall,
				SessionID:         sessionID,
				OperationID:       operationID,
				ToolCallID:        toolCallID,
				ToolName:          toolName,
				ToolTitle:         title,
				ToolStatus:        status,
				NormalizedPayload: normalizedPayload,
			})
		} else {
			// For subsequent events, emit tool_update to update the existing message
			a.sendUpdate(AgentEvent{
				Type:              streams.EventTypeToolUpdate,
				SessionID:         sessionID,
				OperationID:       operationID,
				ToolCallID:        toolCallID,
				ToolName:          toolName,
				ToolTitle:         title,
				ToolStatus:        status,
				Error:             state.Error,
				NormalizedPayload: normalizedPayload,
			})
		}
	}
}

// handleMessageUpdated processes message.updated events (for token tracking)
func (a *Adapter) handleMessageUpdated(props json.RawMessage, sessionID string) {
	parsed, err := opencode.ParseMessageUpdated(props)
	if err != nil {
		return
	}

	info := parsed.Info

	// Track message role (user vs assistant) for filtering in handleMessagePartUpdated
	if info.ID != "" && info.Role != "" {
		a.mu.Lock()
		if a.messageRoles == nil {
			a.messageRoles = make(map[string]string)
		}
		a.messageRoles[info.ID] = info.Role
		a.mu.Unlock()
	}

	if info.Tokens == nil {
		return
	}

	// Calculate total tokens
	totalTokens := info.Tokens.Input + info.Tokens.Output
	if info.Tokens.Cache != nil {
		totalTokens += info.Tokens.Cache.Read
	}

	a.mu.Lock()
	a.totalTokens = totalTokens
	a.mu.Unlock()

	// Emit context window event
	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeContextWindow,
		SessionID:         sessionID,
		ContextWindowUsed: int64(totalTokens),
		ContextWindowSize: int64(a.modelContextWindow),
	})
}

// handlePermissionAsked processes permission.asked events
func (a *Adapter) handlePermissionAsked(props json.RawMessage, sessionID string) {
	parsed, err := opencode.ParsePermissionAsked(props)
	if err != nil {
		a.logger.Warn("failed to parse permission.asked", zap.Error(err))
		return
	}

	requestID := parsed.ID
	permission := parsed.Permission
	toolCallID := requestID
	if parsed.Tool != nil {
		toolCallID = parsed.Tool.CallID
	}

	a.mu.RLock()
	handler := a.permissionHandler
	autoApprove := a.cfg.AutoApprove
	a.mu.RUnlock()

	// If auto-approve is enabled, immediately approve
	if autoApprove {
		go func() {
			if err := a.client.ReplyPermission(a.ctx, requestID, opencode.PermissionReplyOnce, nil); err != nil {
				a.logger.Warn("failed to auto-approve permission", zap.Error(err))
			}
		}()
		return
	}

	// Store pending permission
	a.mu.Lock()
	a.pendingPermissions[requestID] = &permissionRequest{
		requestID:  requestID,
		toolCallID: toolCallID,
		toolName:   permission,
	}
	a.mu.Unlock()

	// Format title from metadata if available
	title := fmt.Sprintf("Permission: %s", permission)
	if parsed.Metadata != nil {
		if cmd, ok := parsed.Metadata["command"].(string); ok {
			title = fmt.Sprintf("Execute: %s", cmd)
		} else if path, ok := parsed.Metadata["path"].(string); ok {
			title = fmt.Sprintf("%s: %s", permission, path)
		}
	}

	// Emit permission request event
	a.sendUpdate(AgentEvent{
		Type:            streams.EventTypePermissionRequest,
		SessionID:       sessionID,
		PendingID:       requestID,
		ToolCallID:      toolCallID,
		PermissionTitle: title,
		PermissionOptions: []PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
			{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
		},
		ActionType:    permission,
		ActionDetails: parsed.Metadata,
	})

	// If we have a handler, call it
	if handler != nil {
		go func() {
			req := &PermissionRequest{
				SessionID:     sessionID,
				ToolCallID:    toolCallID,
				Title:         title,
				ActionType:    permission,
				ActionDetails: parsed.Metadata,
				Options: []PermissionOption{
					{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
					{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
				},
			}
			resp, err := handler(a.ctx, req)
			if err != nil {
				a.logger.Warn("permission handler error", zap.Error(err))
				return
			}
			if resp == nil {
				return
			}

			// Respond based on handler result
			approved := resp.OptionID == "allow"
			if err := a.RespondToPermission(a.ctx, requestID, approved, ""); err != nil {
				a.logger.Warn("failed to respond to permission", zap.Error(err))
			}
		}()
	}
}

// RespondToPermission responds to a permission request (called via handler)
func (a *Adapter) RespondToPermission(ctx context.Context, requestID string, approved bool, message string) error {
	a.mu.Lock()
	delete(a.pendingPermissions, requestID)
	a.mu.Unlock()

	reply := opencode.PermissionReplyOnce
	var msg *string

	if !approved {
		reply = opencode.PermissionReplyReject
		if message != "" {
			msg = &message
		}
	}

	return a.client.ReplyPermission(ctx, requestID, reply, msg)
}

// WriteOpenCodeMcpConfig writes MCP server configuration to OpenCode's config file.
// It merges with existing config, preserving other settings and existing MCP servers.
// OpenCode reads MCP servers from ~/.config/opencode/opencode.json at startup time.
// This function should be called before starting the OpenCode process.
// If homeDir is empty, it uses os.UserHomeDir().
func WriteOpenCodeMcpConfig(mcpServers []shared.McpServerConfig, homeDir string, log *logger.Logger) error {
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
	configDir := filepath.Join(homeDir, ".config", "opencode")
	configPath := filepath.Join(configDir, "opencode.json")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create opencode config directory: %w", err)
	}

	// Read existing config if it exists
	existingData, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing opencode config: %w", err)
	}

	// Parse existing config into a generic map to preserve all fields
	var rawConfig map[string]interface{}
	if len(existingData) > 0 {
		if err := json.Unmarshal(existingData, &rawConfig); err != nil {
			if log != nil {
				log.Warn("failed to parse existing opencode config, will create new",
					zap.String("path", configPath),
					zap.Error(err))
			}
			rawConfig = make(map[string]interface{})
		}
	} else {
		rawConfig = make(map[string]interface{})
	}

	// Get or create mcp section
	mcpSection, ok := rawConfig["mcp"].(map[string]interface{})
	if !ok {
		mcpSection = make(map[string]interface{})
	}

	// Add/update our MCP servers
	for _, server := range mcpServers {
		serverConfig := make(map[string]interface{})

		if server.Type == "sse" || server.Type == "http" || server.Type == "remote" {
			// Remote transport - use url field
			serverConfig["type"] = "remote"
			if server.URL != "" {
				serverConfig["url"] = server.URL
			}
		} else {
			// Local/stdio transport - use command and args
			serverConfig["type"] = "local"
			if server.Command != "" {
				// OpenCode expects command as an array: ["command", "arg1", "arg2"]
				cmdArray := []string{server.Command}
				cmdArray = append(cmdArray, server.Args...)
				serverConfig["command"] = cmdArray
			}
		}

		// Always enable the server
		serverConfig["enabled"] = true

		mcpSection[server.Name] = serverConfig
	}

	// Update the mcp section in the config
	rawConfig["mcp"] = mcpSection

	// Marshal back to JSON with indentation
	output, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal opencode config: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write opencode config: %w", err)
	}

	if log != nil {
		log.Info("wrote opencode MCP config",
			zap.String("path", configPath),
			zap.Int("server_count", len(mcpServers)))
	}

	return nil
}
