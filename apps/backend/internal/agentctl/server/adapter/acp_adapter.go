package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ACPAdapter implements AgentAdapter for agents using the ACP protocol.
// ACP (Agent Communication Protocol) uses JSON-RPC 2.0 over stdin/stdout.
// The subprocess is managed externally (by process.Manager) and stdin/stdout
// are connected via the Connect method after the process starts.
type ACPAdapter struct {
	cfg    *Config
	logger *logger.Logger

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

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewACPAdapter creates a new ACP protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
func NewACPAdapter(cfg *Config, log *logger.Logger) *ACPAdapter {
	return &ACPAdapter{
		cfg:       cfg,
		logger:    log.WithFields(zap.String("adapter", "acp")),
		updatesCh: make(chan AgentEvent, 100),
	}
}

// PrepareEnvironment is a no-op for ACP.
// ACP passes MCP servers through the protocol during session creation.
func (a *ACPAdapter) PrepareEnvironment() (map[string]string, error) {
	return nil, nil
}

// Connect wires up the stdin/stdout pipes from the running agent subprocess.
func (a *ACPAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
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
func (a *ACPAdapter) Initialize(ctx context.Context) error {
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
func (a *ACPAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new agent session.
func (a *ACPAdapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
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
func (a *ACPAdapter) LoadSession(ctx context.Context, sessionID string) error {
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

	return nil
}

// Prompt sends a prompt to the agent.
// If pending context is set (from SetPendingContext), it will be prepended to the message.
func (a *ACPAdapter) Prompt(ctx context.Context, message string) error {
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

	a.logger.Info("sending prompt", zap.String("session_id", sessionID))

	_, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    []acp.ContentBlock{acp.TextBlock(finalMessage)},
	})
	return err
}

// SetPendingContext sets the context to be injected into the next prompt.
// This is used by the fork_session pattern for ACP agents that don't support session/load.
// The context will be prepended to the first prompt sent to this session.
func (a *ACPAdapter) SetPendingContext(context string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingContext = context
}

// Cancel cancels the current operation.
func (a *ACPAdapter) Cancel(ctx context.Context) error {
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
func (a *ACPAdapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *ACPAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation/turn ID.
// ACP protocol doesn't have explicit turn/operation IDs, so this returns empty string.
func (a *ACPAdapter) GetOperationID() string {
	// ACP doesn't have explicit operation/turn IDs
	return ""
}

// SetPermissionHandler sets the handler for permission requests.
func (a *ACPAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *ACPAdapter) Close() error {
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
func (a *ACPAdapter) RequiresProcessKill() bool {
	return false
}

// GetACPConnection returns the underlying ACP connection for advanced usage.
func (a *ACPAdapter) GetACPConnection() *acp.ClientSideConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acpConn
}

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic AgentEvent.
func (a *ACPAdapter) handleACPUpdate(n acp.SessionNotification) {
	event := a.convertNotification(n)
	if event != nil {
		select {
		case a.updatesCh <- *event:
		default:
			a.logger.Warn("updates channel full, dropping notification")
		}
	}
}

// convertNotification converts an ACP SessionNotification to an AgentEvent.
func (a *ACPAdapter) convertNotification(n acp.SessionNotification) *AgentEvent {
	u := n.Update

	switch {
	case u.AgentMessageChunk != nil:
		if u.AgentMessageChunk.Content.Text != nil {
			return &AgentEvent{
				Type:      EventTypeMessageChunk,
				SessionID: string(n.SessionId),
				Text:      u.AgentMessageChunk.Content.Text.Text,
			}
		}

	case u.AgentThoughtChunk != nil:
		// Agent thinking/reasoning - map to the reasoning type
		// Note: Only models with extended thinking (e.g., Opus 4.5) send agent_thought_chunk
		if u.AgentThoughtChunk.Content.Text != nil {
			return &AgentEvent{
				Type:          EventTypeReasoning,
				SessionID:     string(n.SessionId),
				ReasoningText: u.AgentThoughtChunk.Content.Text.Text,
			}
		}

	case u.ToolCall != nil:
		// Extract rich tool call information
		args := map[string]interface{}{}

		// Add tool kind
		if u.ToolCall.Kind != "" {
			args["kind"] = string(u.ToolCall.Kind)
		}

		// Add locations (file paths with line numbers)
		if len(u.ToolCall.Locations) > 0 {
			locations := make([]map[string]interface{}, len(u.ToolCall.Locations))
			for i, loc := range u.ToolCall.Locations {
				locMap := map[string]interface{}{"path": loc.Path}
				if loc.Line != nil {
					locMap["line"] = *loc.Line
				}
				locations[i] = locMap
			}
			args["locations"] = locations

			// Also set primary path for convenience
			args["path"] = u.ToolCall.Locations[0].Path
		}

		// Add raw input if available
		if u.ToolCall.RawInput != nil {
			args["raw_input"] = u.ToolCall.RawInput
		}

		// Normalize status - if empty, default to "running" for tool_call start
		status := string(u.ToolCall.Status)
		if status == "" {
			status = "running"
		}

		return &AgentEvent{
			Type:       EventTypeToolCall,
			SessionID:  string(n.SessionId),
			ToolCallID: string(u.ToolCall.ToolCallId),
			ToolName:   string(u.ToolCall.Kind), // Kind is effectively the tool name
			ToolTitle:  u.ToolCall.Title,
			ToolStatus: status,
			ToolArgs:   args,
		}

	case u.ToolCallUpdate != nil:
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		// Normalize status - "completed" → "complete" for frontend consistency
		if status == "completed" {
			status = "complete"
		}
		return &AgentEvent{
			Type:       EventTypeToolUpdate,
			SessionID:  string(n.SessionId),
			ToolCallID: string(u.ToolCallUpdate.ToolCallId),
			ToolStatus: status,
		}

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
			Type:        EventTypePlan,
			SessionID:   string(n.SessionId),
			PlanEntries: entries,
		}
	}

	return nil
}

// handlePermissionRequest handles permission requests from the agent.
// Since both acpclient and adapter now use the shared types package,
// no conversion is needed - we just forward to the handler.
func (a *ACPAdapter) handlePermissionRequest(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	a.mu.RUnlock()

	// Emit a tool_call event so a message is created in the database.
	// This is needed because permission requests bypass the normal ToolCall notification flow.
	// Without this, when the tool completes (ToolCallUpdate), there's no message to update.
	toolCallEvent := AgentEvent{
		Type:       EventTypeToolCall,
		SessionID:  sessionID,
		ToolCallID: req.ToolCallID,
		ToolName:   req.ActionType, // Use action type as tool name (e.g., "run_shell_command")
		ToolTitle:  req.Title,
		ToolStatus: "pending_permission", // Mark as pending permission
		ToolArgs: map[string]interface{}{
			"permission_request": true,
			"action_type":        req.ActionType,
		},
	}

	// Add action details if available
	if req.ActionDetails != nil {
		for k, v := range req.ActionDetails {
			toolCallEvent.ToolArgs[k] = v
		}
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

// Verify interface implementation
var _ AgentAdapter = (*ACPAdapter)(nil)
