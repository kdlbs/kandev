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
// are passed to the adapter.
type ACPAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Subprocess stdin/stdout (managed externally)
	stdin  io.Writer
	stdout io.Reader

	// ACP SDK connection
	acpClient *acpclient.Client
	acpConn   *acp.ClientSideConnection
	sessionID string

	// Agent info (populated after Initialize)
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewACPAdapter creates a new ACP protocol adapter.
// stdin and stdout are the subprocess's stdin/stdout pipes (managed by process.Manager).
func NewACPAdapter(stdin io.Writer, stdout io.Reader, cfg *Config, log *logger.Logger) *ACPAdapter {
	return &ACPAdapter{
		cfg:       cfg,
		logger:    log.WithFields(zap.String("adapter", "acp")),
		stdin:     stdin,
		stdout:    stdout,
		updatesCh: make(chan AgentEvent, 100),
	}
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

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    "unknown",
		Version: "unknown",
	}
	if resp.AgentInfo != nil {
		a.agentInfo.Name = resp.AgentInfo.Name
		a.agentInfo.Version = resp.AgentInfo.Version
	}
	a.logger.Info("ACP adapter initialized",
		zap.String("agent_name", a.agentInfo.Name),
		zap.String("agent_version", a.agentInfo.Version))

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
		out = append(out, acp.McpServer{
			Stdio: &acp.McpServerStdio{
				Name:    server.Name,
				Command: server.Command,
				Args:    append([]string{}, server.Args...),
			},
		})
	}
	return out
}

// LoadSession resumes an existing session.
func (a *ACPAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.acpConn == nil {
		return fmt.Errorf("adapter not initialized")
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
func (a *ACPAdapter) Prompt(ctx context.Context, message string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt", zap.String("session_id", sessionID))

	_, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    []acp.ContentBlock{acp.TextBlock(message)},
	})
	return err
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

	// Log what update types we're receiving
	a.logger.Debug("ACP notification received",
		zap.Bool("has_tool_call", u.ToolCall != nil),
		zap.Bool("has_tool_call_update", u.ToolCallUpdate != nil),
		zap.Bool("has_message_chunk", u.AgentMessageChunk != nil),
		zap.Bool("has_thought_chunk", u.AgentThoughtChunk != nil),
		zap.Bool("has_plan", u.Plan != nil))

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
		// Agent thinking/reasoning - map to the new reasoning type
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
	a.mu.RUnlock()

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
