// Package acp implements the ACP (Agent Communication Protocol) transport adapter.
// ACP uses JSON-RPC 2.0 over stdin/stdout for agent communication.
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// Re-export types needed by external packages
type (
	PermissionRequest  = types.PermissionRequest
	PermissionResponse = types.PermissionResponse
	PermissionOption   = streams.PermissionOption
	PermissionHandler  = types.PermissionHandler
	AgentEvent         = streams.AgentEvent
	PlanEntry          = streams.PlanEntry
)

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Adapter implements the transport adapter for agents using the ACP protocol.
// ACP (Agent Communication Protocol) uses JSON-RPC 2.0 over stdin/stdout.
// The subprocess is managed externally (by process.Manager) and stdin/stdout
// are connected via the Connect method after the process starts.
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

	// ACP SDK connection
	acpClient *acpclient.Client
	acpConn   *acp.ClientSideConnection
	sessionID string

	// Agent info (populated after Initialize)
	agentInfo    *AgentInfo
	capabilities acp.AgentCapabilities

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Context injection for fork_session pattern (ACP agents that don't support session/load)
	// When set, this context will be prepended to the first prompt sent to the session.
	pendingContext string

	// Tool call tracking for result normalization
	// Maps toolCallId -> NormalizedPayload so we can update with results
	activeToolCalls map[string]*streams.NormalizedPayload

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewAdapter creates a new ACP protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
// cfg.AgentID is required for debug file naming.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	return &Adapter{
		cfg:             cfg,
		logger:          log.WithFields(zap.String("adapter", "acp"), zap.String("agent_id", cfg.AgentID)),
		agentID:         cfg.AgentID,
		normalizer:      NewNormalizer(),
		updatesCh:       make(chan AgentEvent, 100),
		activeToolCalls: make(map[string]*streams.NormalizedPayload),
	}
}

// PrepareEnvironment is a no-op for ACP.
// ACP passes MCP servers through the protocol during session creation.
func (a *Adapter) PrepareEnvironment() (map[string]string, error) {
	return nil, nil
}

// PrepareCommandArgs returns extra command-line arguments for the agent process.
// For ACP, no extra args are needed - MCP servers are passed through the protocol.
func (a *Adapter) PrepareCommandArgs() []string {
	return nil
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

// Initialize establishes the ACP connection with the agent subprocess.
// The subprocess should already be running (started by process.Manager).
func (a *Adapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing ACP adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create ACP client with update handler that converts to AgentEvent
	a.acpClient = acpclient.NewClient(
		acpclient.WithLogger(a.logger.Zap()),
		acpclient.WithWorkspaceRoot(a.cfg.WorkDir),
		acpclient.WithUpdateHandler(a.handleACPUpdate),
		acpclient.WithPermissionHandler(a.handlePermissionRequest),
	)

	// Create ACP SDK connection
	a.acpConn = acp.NewClientSideConnection(a.acpClient, a.stdin, a.stdout)
	a.acpConn.SetLogger(slog.Default().With("component", "acp-conn"))

	// Perform ACP handshake - this exchanges capabilities with the agent
	resp, err := a.acpConn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "kandev-agentctl",
			Version: "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("ACP initialize handshake failed: %w", err)
	}

	// Store agent info and capabilities
	a.agentInfo = &AgentInfo{
		Name:    "unknown",
		Version: "unknown",
	}
	if resp.AgentInfo != nil {
		a.agentInfo.Name = resp.AgentInfo.Name
		a.agentInfo.Version = resp.AgentInfo.Version
	}
	a.capabilities = resp.AgentCapabilities
	a.logger.Info("ACP adapter initialized",
		zap.String("agent_name", a.agentInfo.Name),
		zap.String("agent_version", a.agentInfo.Version),
		zap.Bool("supports_load_session", a.capabilities.LoadSession))

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new agent session.
func (a *Adapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.acpConn == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	resp, err := a.acpConn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(mcpServers),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	a.sessionID = string(resp.SessionId)
	a.logger.Info("created new session", zap.String("session_id", a.sessionID))

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	select {
	case a.updatesCh <- AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     a.sessionID,
		SessionStatus: streams.SessionStatusNew,
		Data: map[string]any{
			"session_status": streams.SessionStatusNew,
			"init":           true,
		},
	}:
	default:
		a.logger.Warn("updates channel full, could not emit session_status event")
	}

	return a.sessionID, nil
}

func toACPMcpServers(servers []types.McpServer) []acp.McpServer {
	if len(servers) == 0 {
		return []acp.McpServer{}
	}
	out := make([]acp.McpServer, 0, len(servers))
	for _, server := range servers {
		switch server.Type {
		case "sse":
			out = append(out, acp.McpServer{
				Sse: &acp.McpServerSse{
					Name:    server.Name,
					Url:     server.URL,
					Type:    "sse",
					Headers: []acp.HttpHeader{},
				},
			})
		default: // stdio
			out = append(out, acp.McpServer{
				Stdio: &acp.McpServerStdio{
					Name:    server.Name,
					Command: server.Command,
					Args:    append([]string{}, server.Args...),
				},
			})
		}
	}
	return out
}

// LoadSession resumes an existing session.
// Returns an error if the agent does not support session loading (LoadSession capability).
func (a *Adapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.acpConn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Check if the agent supports session loading
	if !a.capabilities.LoadSession {
		return fmt.Errorf("agent does not support session loading (LoadSession capability is false)")
	}

	_, err := a.acpConn.LoadSession(ctx, acp.LoadSessionRequest{
		SessionId: acp.SessionId(sessionID),
	})
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	a.sessionID = sessionID
	a.logger.Info("loaded session", zap.String("session_id", a.sessionID))

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	select {
	case a.updatesCh <- AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     a.sessionID,
		SessionStatus: streams.SessionStatusResumed,
		Data: map[string]any{
			"session_status": streams.SessionStatusResumed,
			"init":           true,
		},
	}:
	default:
		a.logger.Warn("updates channel full, could not emit session_status event")
	}

	return nil
}

// Prompt sends a prompt to the agent.
// If pending context is set (from SetPendingContext), it will be prepended to the message.
// Attachments (images) are converted to ACP ImageBlocks and included in the prompt.
// When the prompt completes, a complete event is emitted via the updates channel.
func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment) error {
	a.mu.Lock()
	conn := a.acpConn
	sessionID := a.sessionID
	pendingContext := a.pendingContext
	a.pendingContext = "" // Clear after use
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Inject pending context if available (fork_session pattern)
	finalMessage := message
	if pendingContext != "" {
		finalMessage = pendingContext
		a.logger.Info("injecting resume context into prompt",
			zap.String("session_id", sessionID),
			zap.Int("context_length", len(pendingContext)))
	}

	// Build content blocks: text first, then images
	contentBlocks := []acp.ContentBlock{acp.TextBlock(finalMessage)}

	// Add image attachments as ImageBlocks
	for _, att := range attachments {
		if att.Type == "image" {
			contentBlocks = append(contentBlocks, acp.ImageBlock(att.Data, att.MimeType))
		}
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.Int("content_blocks", len(contentBlocks)),
		zap.Int("image_attachments", len(attachments)))

	_, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    contentBlocks,
	})
	if err != nil {
		return err
	}

	// Emit complete event via the stream.
	// This normalizes ACP behavior to match other adapters (stream-json, amp, copilot, opencode).
	// All adapters now emit complete events, eliminating the need for ReportsStatusViaStream flag.
	a.logger.Debug("emitting complete event after prompt", zap.String("session_id", sessionID))
	select {
	case a.updatesCh <- AgentEvent{
		Type:      streams.EventTypeComplete,
		SessionID: sessionID,
	}:
	default:
		a.logger.Warn("updates channel full, could not emit complete event")
	}

	return nil
}

// SetPendingContext sets the context to be injected into the next prompt.
// This is used by the fork_session pattern for ACP agents that don't support session/load.
// The context will be prepended to the first prompt sent to this session.
func (a *Adapter) SetPendingContext(context string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingContext = context
}

// Cancel cancels the current operation.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling session", zap.String("session_id", sessionID))

	return conn.Cancel(ctx, acp.CancelNotification{
		SessionId: acp.SessionId(sessionID),
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

// GetOperationID returns the current operation/turn ID.
// ACP protocol doesn't have explicit turn/operation IDs, so this returns empty string.
func (a *Adapter) GetOperationID() string {
	// ACP doesn't have explicit operation/turn IDs
	return ""
}

// SetPermissionHandler sets the handler for permission requests.
func (a *Adapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing ACP adapter")

	// Close update channel
	close(a.updatesCh)

	// Note: We don't close stdin or manage the subprocess here.
	// That's handled by process.Manager which owns the subprocess.

	return nil
}

// RequiresProcessKill returns false because ACP agents exit when stdin is closed.
func (a *Adapter) RequiresProcessKill() bool {
	return false
}

// GetACPConnection returns the underlying ACP connection for advanced usage.
func (a *Adapter) GetACPConnection() *acp.ClientSideConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acpConn
}

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic AgentEvent.
func (a *Adapter) handleACPUpdate(n acp.SessionNotification) {
	// Log raw event for debugging
	if rawData, err := json.Marshal(n); err == nil {
		shared.LogRawEvent(shared.ProtocolACP, a.agentID, "session_notification", rawData)
	}

	event := a.convertNotification(n)
	if event != nil {
		shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, event)
		select {
		case a.updatesCh <- *event:
		default:
			a.logger.Warn("updates channel full, dropping notification")
		}
	}
}

// convertNotification converts an ACP SessionNotification to an AgentEvent.
func (a *Adapter) convertNotification(n acp.SessionNotification) *AgentEvent {
	u := n.Update
	sessionID := string(n.SessionId)

	switch {
	case u.AgentMessageChunk != nil:
		if u.AgentMessageChunk.Content.Text != nil {
			return &AgentEvent{
				Type:      streams.EventTypeMessageChunk,
				SessionID: sessionID,
				Text:      u.AgentMessageChunk.Content.Text.Text,
			}
		}

	case u.AgentThoughtChunk != nil:
		// Agent thinking/reasoning - map to the reasoning type
		// Note: Only models with extended thinking (e.g., Opus 4.5) send agent_thought_chunk
		if u.AgentThoughtChunk.Content.Text != nil {
			return &AgentEvent{
				Type:          streams.EventTypeReasoning,
				SessionID:     sessionID,
				ReasoningText: u.AgentThoughtChunk.Content.Text.Text,
			}
		}

	case u.ToolCall != nil:
		return a.convertToolCallUpdate(sessionID, u.ToolCall)

	case u.ToolCallUpdate != nil:
		return a.convertToolCallResultUpdate(sessionID, u.ToolCallUpdate)

	case u.Plan != nil:
		entries := make([]PlanEntry, len(u.Plan.Entries))
		for i, e := range u.Plan.Entries {
			entries[i] = PlanEntry{
				Description: e.Content,
				Status:      string(e.Status),
				Priority:    string(e.Priority),
			}
		}
		return &AgentEvent{
			Type:        streams.EventTypePlan,
			SessionID:   sessionID,
			PlanEntries: entries,
		}

	case u.AvailableCommandsUpdate != nil:
		commands := make([]streams.AvailableCommand, len(u.AvailableCommandsUpdate.AvailableCommands))
		for i, cmd := range u.AvailableCommandsUpdate.AvailableCommands {
			commands[i] = streams.AvailableCommand{
				Name:        cmd.Name,
				Description: cmd.Description,
			}
		}
		return &AgentEvent{
			Type:              streams.EventTypeAvailableCommands,
			SessionID:         sessionID,
			AvailableCommands: commands,
		}
	}

	return nil
}

// convertToolCallUpdate converts a ToolCall notification to an AgentEvent.
func (a *Adapter) convertToolCallUpdate(sessionID string, tc *acp.SessionUpdateToolCall) *AgentEvent {
	args := map[string]any{}

	if tc.Kind != "" {
		args["kind"] = string(tc.Kind)
	}

	if len(tc.Locations) > 0 {
		locations := make([]map[string]any, len(tc.Locations))
		for i, loc := range tc.Locations {
			locMap := map[string]any{"path": loc.Path}
			if loc.Line != nil {
				locMap["line"] = *loc.Line
			}
			locations[i] = locMap
		}
		args["locations"] = locations
		args["path"] = tc.Locations[0].Path
	}

	if tc.RawInput != nil {
		args["raw_input"] = tc.RawInput
	}

	toolKind := string(tc.Kind)
	normalizedPayload := a.normalizer.NormalizeToolCall(toolKind, args)

	toolCallID := string(tc.ToolCallId)
	a.mu.Lock()
	a.activeToolCalls[toolCallID] = normalizedPayload
	a.mu.Unlock()

	// Detect tool type for logging
	toolType := DetectToolOperationType(toolKind, args)
	_ = toolType // Used for normalization

	status := string(tc.Status)
	if status == "" {
		status = "running"
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolName:          toolKind, // Kind is effectively the tool name
		ToolTitle:         tc.Title,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
	}
}

// convertToolCallResultUpdate converts a ToolCallUpdate notification to an AgentEvent.
func (a *Adapter) convertToolCallResultUpdate(sessionID string, tcu *acp.SessionToolCallUpdate) *AgentEvent {
	toolCallID := string(tcu.ToolCallId)
	status := ""
	if tcu.Status != nil {
		status = string(*tcu.Status)
	}
	// Normalize status - "completed" -> "complete" for frontend consistency
	if status == "completed" {
		status = toolStatusComplete
	}

	// Look up the stored payload and update with result if we have rawOutput
	var normalizedPayload *streams.NormalizedPayload
	if tcu.RawOutput != nil {
		a.mu.Lock()
		if payload, ok := a.activeToolCalls[toolCallID]; ok {
			a.normalizer.NormalizeToolResult(payload, tcu.RawOutput)
			normalizedPayload = payload
			if status == toolStatusComplete || status == toolStatusError {
				delete(a.activeToolCalls, toolCallID)
			}
		}
		a.mu.Unlock()
	} else if status == toolStatusComplete || status == toolStatusError {
		// Clean up even without output
		a.mu.Lock()
		normalizedPayload = a.activeToolCalls[toolCallID]
		delete(a.activeToolCalls, toolCallID)
		a.mu.Unlock()
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
	}
}

// handlePermissionRequest handles permission requests from the agent.
// Since both acpclient and adapter now use the shared types package,
// no conversion is needed - we just forward to the handler.
func (a *Adapter) handlePermissionRequest(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	a.mu.RUnlock()

	// Emit a tool_call event so a message is created in the database.
	// This is needed because permission requests bypass the normal ToolCall notification flow.
	// Without this, when the tool completes (ToolCallUpdate), there's no message to update.
	toolCallEvent := AgentEvent{
		Type:       streams.EventTypeToolCall,
		SessionID:  sessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   req.ActionType, // Use action type as tool name (e.g., "run_shell_command")
		ToolTitle:  req.Title,
		ToolStatus: "pending_permission", // Mark as pending permission
	}

	// Emit the tool_call event
	select {
	case a.updatesCh <- toolCallEvent:
		a.logger.Debug("emitted tool_call event for permission request",
			zap.String("tool_call_id", req.ToolCallID))
	default:
		a.logger.Warn("updates channel full, could not emit tool_call event for permission")
	}

	if handler == nil {
		// Auto-approve if no handler
		if len(req.Options) > 0 {
			return &PermissionResponse{OptionID: req.Options[0].OptionID}, nil
		}
		return &PermissionResponse{Cancelled: true}, nil
	}

	// Forward directly to handler - types are already compatible
	return handler(ctx, req)
}
