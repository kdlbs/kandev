// Package acp implements the ACP (Agent Communication Protocol) transport adapter.
// ACP uses JSON-RPC 2.0 over stdin/stdout for agent communication.
package acp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/attribute"
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

// Content block type constants.
const (
	contentTypeImage    = "image"
	contentTypeAudio    = "audio"
	contentTypeResource = "resource"
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

	// isLoadingSession is true during LoadSession() to suppress history replay notifications.
	// ACP agents stream the entire conversation history during session/load which should
	// not be emitted as new message events.
	isLoadingSession bool

	// Tool call tracking for result normalization
	// Maps toolCallId -> NormalizedPayload so we can update with results
	activeToolCalls map[string]*streams.NormalizedPayload

	// OTel tracing: active prompt span context.
	// Notification spans become children of the prompt span for visual grouping.
	promptTraceCtx context.Context
	promptTraceMu  sync.RWMutex

	// Attachment management
	attachMgr *shared.AttachmentManager

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// NewAdapter creates a new ACP protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
// cfg.AgentID is required for debug file naming.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	l := log.WithFields(zap.String("adapter", "acp"), zap.String("agent_id", cfg.AgentID))
	return &Adapter{
		cfg:             cfg,
		logger:          l,
		agentID:         cfg.AgentID,
		normalizer:      NewNormalizer(),
		updatesCh:       make(chan AgentEvent, 100),
		activeToolCalls: make(map[string]*streams.NormalizedPayload),
		attachMgr:       shared.NewAttachmentManager(cfg.WorkDir, l.Zap()),
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
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "initialize")
	defer span.End()

	resp, err := a.acpConn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "kandev-agentctl",
			Version: "1.0.0",
		},
	})
	if err != nil {
		span.RecordError(err)
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

	span.SetAttributes(
		attribute.String("agent_name", a.agentInfo.Name),
		attribute.String("agent_version", a.agentInfo.Version),
		attribute.Bool("supports_load_session", a.capabilities.LoadSession),
	)

	a.logger.Info("ACP adapter initialized",
		zap.String("agent_name", a.agentInfo.Name),
		zap.String("agent_version", a.agentInfo.Version),
		zap.Bool("supports_load_session", a.capabilities.LoadSession))

	// Emit agent capabilities event with prompt capabilities and auth methods
	a.sendUpdate(AgentEvent{
		Type:                    streams.EventTypeAgentCapabilities,
		SupportsImage:           a.capabilities.PromptCapabilities.Image,
		SupportsAudio:           a.capabilities.PromptCapabilities.Audio,
		SupportsEmbeddedContext: a.capabilities.PromptCapabilities.EmbeddedContext,
		AuthMethods:             convertAuthMethods(resp.AuthMethods),
	})

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new agent session.
func (a *Adapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	a.mu.Lock()
	conn := a.acpConn
	a.mu.Unlock()

	if conn == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.new")
	defer span.End()

	filteredServers := filterMcpServersByCapabilities(mcpServers, a.capabilities.McpCapabilities, a.logger)
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        a.cfg.WorkDir,
		McpServers: toACPMcpServers(filteredServers),
	})
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = string(resp.SessionId)
	sessionID := a.sessionID
	a.mu.Unlock()
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("created new session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(resp.Modes)
	}

	// Emit session models if the agent returned model state
	if resp.Models != nil {
		a.emitSessionModels(sessionID, resp.Models, resp.Meta)
	}

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: streams.SessionStatusNew,
		Data: map[string]any{
			"session_status": streams.SessionStatusNew,
			"init":           true,
		},
	})

	return sessionID, nil
}

// filterMcpServersByCapabilities removes MCP servers that the agent doesn't support.
// Stdio servers are always allowed; SSE/HTTP servers require the corresponding capability.
func filterMcpServersByCapabilities(servers []types.McpServer, caps acp.McpCapabilities, logger *logger.Logger) []types.McpServer {
	filtered := make([]types.McpServer, 0, len(servers))
	for _, s := range servers {
		switch s.Type {
		case "sse":
			if !caps.Sse {
				logger.Warn("filtering out SSE MCP server (agent does not support SSE)", zap.String("name", s.Name))
				continue
			}
		case "http":
			if !caps.Http {
				logger.Warn("filtering out HTTP MCP server (agent does not support HTTP)", zap.String("name", s.Name))
				continue
			}
		}
		filtered = append(filtered, s)
	}
	return filtered
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
	conn := a.acpConn
	supportsLoad := a.capabilities.LoadSession
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Check if the agent supports session loading
	if !supportsLoad {
		a.logger.Debug("session/load rejected: agent does not advertise LoadSession capability",
			zap.String("session_id", sessionID))
		return fmt.Errorf("agent does not support session loading (LoadSession capability is false)")
	}

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.load")
	defer span.End()

	// Suppress history replay notifications during load.
	// ACP session/load replays the entire conversation history asynchronously.
	// We set a flag to suppress these notifications to avoid duplicating messages in the database.
	// The flag will be cleared when we send the next prompt (see Prompt method).
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	resp, err := conn.LoadSession(ctx, acp.LoadSessionRequest{
		SessionId:  acp.SessionId(sessionID),
		Cwd:        a.cfg.WorkDir,
		McpServers: []acp.McpServer{}, // MCPs configured via CLI args; empty satisfies the required field
	})

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to load session: %w", err)
	}

	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()
	a.attachMgr.SetSessionID(sessionID)

	span.SetAttributes(attribute.String("session_id", sessionID))
	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	// Emit initial session mode if the agent returned mode state
	if resp.Modes != nil {
		a.emitInitialModeState(resp.Modes)
	}

	// Emit session models if the agent returned model state
	if resp.Models != nil {
		a.emitSessionModels(sessionID, resp.Models, resp.Meta)
	}

	// Emit session status event to normalize with other adapters.
	// This eliminates the need for ReportsStatusViaStream flag.
	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: streams.SessionStatusResumed,
		Data: map[string]any{
			"session_status": streams.SessionStatusResumed,
			"init":           true,
		},
	})

	return nil
}

// ResetSession creates a new session on the existing connection, effectively resetting
// the agent's conversation context without restarting the subprocess. This is much faster
// than a full process restart since the ACP protocol supports multiple sessions per connection.
func (a *Adapter) ResetSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	return a.NewSession(ctx, mcpServers)
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

	// Add media attachments as typed content blocks
	for _, att := range attachments {
		switch att.Type {
		case contentTypeImage:
			contentBlocks = append(contentBlocks, acp.ImageBlock(att.Data, att.MimeType))
		case contentTypeAudio:
			contentBlocks = append(contentBlocks, acp.AudioBlock(att.Data, att.MimeType))
		case contentTypeResource:
			if a.capabilities.PromptCapabilities.EmbeddedContext {
				contentBlocks = append(contentBlocks, buildResourceBlock(att))
			} else {
				// Agent doesn't support embedded resources — save to workspace and reference in text
				saved, saveErr := a.attachMgr.SaveAttachments([]v1.MessageAttachment{att})
				if saveErr != nil || len(saved) == 0 {
					a.logger.Warn("failed to save attachment to workspace, falling back to resource block",
						zap.String("name", att.Name), zap.Error(saveErr))
					contentBlocks = append(contentBlocks, buildResourceBlock(att))
				} else {
					contentBlocks = append(contentBlocks, acp.TextBlock(shared.BuildAttachmentPrompt(saved)))
				}
			}
		}
	}

	// Start prompt span — notification spans become children via getPromptTraceCtx()
	promptCtx, promptSpan := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "prompt")
	promptSpan.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.Int("prompt_length", len(finalMessage)),
		attribute.Int("image_count", len(attachments)),
	)
	a.setPromptTraceCtx(promptCtx)

	// Clear the loading flag before sending the prompt.
	// If we're resuming a session, history replay is complete by the time we send a new prompt.
	a.mu.Lock()
	wasLoading := a.isLoadingSession
	a.isLoadingSession = false
	a.mu.Unlock()

	if wasLoading {
		a.logger.Info("cleared session load suppression flag before sending new prompt",
			zap.String("session_id", sessionID))
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.Int("content_blocks", len(contentBlocks)),
		zap.Int("image_attachments", len(attachments)))

	resp, err := conn.Prompt(promptCtx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    contentBlocks,
	})

	// Clear prompt context and end span regardless of outcome
	a.clearPromptTraceCtx()
	stopReason := ""
	if err != nil {
		promptSpan.RecordError(err)
	} else {
		stopReason = string(resp.StopReason)
		promptSpan.SetAttributes(attribute.String("stop_reason", stopReason))
	}
	promptSpan.End()

	if err != nil {
		return err
	}

	// Emit complete event via the stream, including the StopReason from the agent.
	// This normalizes ACP behavior to match other adapters (stream-json, amp, copilot, opencode).
	a.logger.Debug("emitting complete event after prompt",
		zap.String("session_id", sessionID),
		zap.String("stop_reason", stopReason))
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeComplete,
		SessionID: sessionID,
		Data:      map[string]any{"stop_reason": stopReason},
		Usage:     extractPromptUsage(resp.Meta),
	})

	// Clean up saved attachments — agent has finished reading them
	a.attachMgr.Cleanup()

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
// Per ACP spec, the client must immediately mark non-finished tool calls as cancelled.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "cancel")
	defer span.End()
	span.SetAttributes(attribute.String("session_id", sessionID))

	a.logger.Info("cancelling session", zap.String("session_id", sessionID))

	// Mark all active tool calls as cancelled before sending cancel to agent.
	a.cancelActiveToolCalls(sessionID)

	err := conn.Cancel(ctx, acp.CancelNotification{
		SessionId: acp.SessionId(sessionID),
	})
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// cancelActiveToolCalls emits cancelled tool_update events for all in-flight tool calls
// and clears the activeToolCalls map.
func (a *Adapter) cancelActiveToolCalls(sessionID string) {
	a.mu.Lock()
	toolCalls := a.activeToolCalls
	a.activeToolCalls = make(map[string]*streams.NormalizedPayload)
	a.mu.Unlock()

	for toolCallID, normalized := range toolCalls {
		a.logger.Debug("cancelling active tool call",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID))
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			ToolCallID:        toolCallID,
			ToolStatus:        "cancelled",
			NormalizedPayload: normalized,
		})
	}
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

// sendUpdate safely sends an event to the updates channel.
// It checks the closed flag under read-lock to prevent panics on closed channels.
func (a *Adapter) sendUpdate(event AgentEvent) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return
	}
	select {
	case a.updatesCh <- event:
	default:
		a.logger.Warn("updates channel full, dropping event", zap.String("type", event.Type))
	}
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

	// Clean up any saved attachments
	a.attachMgr.Cleanup()

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

// getPromptTraceCtx returns the current prompt span context for child-span linking.
// Returns context.Background() if no prompt is active.
func (a *Adapter) getPromptTraceCtx() context.Context {
	a.promptTraceMu.RLock()
	defer a.promptTraceMu.RUnlock()
	if a.promptTraceCtx != nil {
		return a.promptTraceCtx
	}
	return context.Background()
}

// setPromptTraceCtx stores the prompt span context.
func (a *Adapter) setPromptTraceCtx(ctx context.Context) {
	a.promptTraceMu.Lock()
	defer a.promptTraceMu.Unlock()
	a.promptTraceCtx = ctx
}

// clearPromptTraceCtx clears the prompt span context.
func (a *Adapter) clearPromptTraceCtx() {
	a.promptTraceMu.Lock()
	defer a.promptTraceMu.Unlock()
	a.promptTraceCtx = nil
}

// GetACPConnection returns the underlying ACP connection for advanced usage.
func (a *Adapter) GetACPConnection() *acp.ClientSideConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acpConn
}

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic AgentEvent.
func (a *Adapter) handleACPUpdate(n acp.SessionNotification) {
	// Marshal once for both debug logging and tracing
	rawData, _ := json.Marshal(n)

	// Log raw event for debugging
	if len(rawData) > 0 {
		shared.LogRawEvent(shared.ProtocolACP, a.agentID, "session_notification", rawData)
	}

	// During session/load, suppress history replay notifications.
	// ACP agents stream the entire conversation history during load, which should not
	// be emitted as new message events to avoid duplicating messages in the UI.
	// We suppress: message chunks, thinking chunks, tool calls, and tool updates.
	a.mu.RLock()
	isLoading := a.isLoadingSession
	a.mu.RUnlock()

	if isLoading {
		u := n.Update
		// Suppress all conversation history events during load
		if u.AgentMessageChunk != nil || u.UserMessageChunk != nil || u.AgentThoughtChunk != nil ||
			u.ToolCall != nil || u.ToolCallUpdate != nil {
			a.logger.Debug("suppressing history replay notification during session load",
				zap.String("session_id", string(n.SessionId)),
				zap.Bool("is_message", u.AgentMessageChunk != nil || u.UserMessageChunk != nil),
				zap.Bool("is_thinking", u.AgentThoughtChunk != nil),
				zap.Bool("is_tool_call", u.ToolCall != nil),
				zap.Bool("is_tool_update", u.ToolCallUpdate != nil))
			return
		}
	}

	event := a.convertNotification(n)
	if event != nil {
		shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, event)
		shared.TraceProtocolEvent(a.getPromptTraceCtx(), shared.ProtocolACP, a.agentID,
			event.Type, rawData, event)
		a.sendUpdate(*event)
	} else if updateJSON, err := json.Marshal(n.Update); err == nil {
		a.logger.Warn("unhandled ACP session notification",
			zap.String("session_id", string(n.SessionId)),
			zap.String("update_json", string(updateJSON)))
	}
}

// convertNotification converts an ACP SessionNotification to an AgentEvent.
func (a *Adapter) convertNotification(n acp.SessionNotification) *AgentEvent {
	u := n.Update
	sessionID := string(n.SessionId)

	switch {
	case u.AgentMessageChunk != nil:
		return a.convertMessageChunk(sessionID, u.AgentMessageChunk.Content, "assistant")

	case u.UserMessageChunk != nil:
		return a.convertMessageChunk(sessionID, u.UserMessageChunk.Content, "user")

	case u.AgentThoughtChunk != nil:
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
		return a.convertAvailableCommands(sessionID, u.AvailableCommandsUpdate)

	case u.CurrentModeUpdate != nil:
		return &AgentEvent{
			Type:          streams.EventTypeSessionMode,
			SessionID:     sessionID,
			CurrentModeID: string(u.CurrentModeUpdate.CurrentModeId),
		}
	}

	return nil
}

// convertMessageChunk converts an ACP ContentBlock to an AgentEvent, handling multimodal content.
// For text-only messages, sets the Text field for backward compatibility.
// For non-text content, populates ContentBlocks.
func (a *Adapter) convertMessageChunk(sessionID string, content acp.ContentBlock, role string) *AgentEvent {
	event := &AgentEvent{
		Type:      streams.EventTypeMessageChunk,
		SessionID: sessionID,
	}

	// Only set Role for user messages (assistant is the default)
	if role == "user" {
		event.Role = role
	}

	// Text content goes directly into the Text field for backward compatibility
	if content.Text != nil {
		event.Text = content.Text.Text
		return event
	}

	// Non-text content uses the shared converter
	cb := a.convertContentBlockToStreams(content)
	if cb == nil {
		return nil
	}
	event.ContentBlocks = []streams.ContentBlock{*cb}
	return event
}

// convertAvailableCommands converts an ACP AvailableCommandsUpdate to an AgentEvent,
// including input hints when available.
func (a *Adapter) convertAvailableCommands(sessionID string, update *acp.SessionAvailableCommandsUpdate) *AgentEvent {
	commands := make([]streams.AvailableCommand, len(update.AvailableCommands))
	for i, cmd := range update.AvailableCommands {
		ac := streams.AvailableCommand{
			Name:        cmd.Name,
			Description: cmd.Description,
		}
		if cmd.Input != nil && cmd.Input.UnstructuredCommandInput != nil {
			ac.InputHint = cmd.Input.UnstructuredCommandInput.Hint
		}
		commands[i] = ac
	}
	return &AgentEvent{
		Type:              streams.EventTypeAvailableCommands,
		SessionID:         sessionID,
		AvailableCommands: commands,
	}
}

// emitInitialModeState emits a session_mode event from the session response's Modes field.
// Called after session/new and session/load to provide the initial mode state.
func (a *Adapter) emitInitialModeState(modes *acp.SessionModeState) {
	availModes := make([]streams.SessionModeInfo, 0, len(modes.AvailableModes))
	for _, m := range modes.AvailableModes {
		availModes = append(availModes, streams.SessionModeInfo{
			ID:          string(m.Id),
			Name:        m.Name,
			Description: derefStr(m.Description),
		})
	}
	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionMode,
		SessionID:      a.sessionID,
		CurrentModeID:  string(modes.CurrentModeId),
		AvailableModes: availModes,
	})
}

// emitSessionModels emits a session_models event from the session response.
func (a *Adapter) emitSessionModels(sessionID string, models *acp.SessionModelState, meta any) {
	a.sendUpdate(AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      sessionID,
		CurrentModelID: string(models.CurrentModelId),
		SessionModels:  convertSessionModels(models.AvailableModels),
		ConfigOptions:  extractConfigOptions(meta),
	})
}

// SetMode changes the agent's session mode via ACP session/set_mode.
func (a *Adapter) SetMode(ctx context.Context, modeID string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: acp.SessionId(sessionID),
		ModeId:    acp.SessionModeId(modeID),
	})
	if err != nil {
		return fmt.Errorf("set session mode failed: %w", err)
	}

	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeSessionMode,
		SessionID:     sessionID,
		CurrentModeID: modeID,
	})
	return nil
}

// SetModel changes the agent's model via ACP session/set_model (unstable SDK method).
func (a *Adapter) SetModel(ctx context.Context, modelID string) error {
	a.mu.RLock()
	conn := a.acpConn
	sessionID := a.sessionID
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := conn.SetSessionModel(ctx, acp.SetSessionModelRequest{
		SessionId: acp.SessionId(sessionID),
		ModelId:   acp.ModelId(modelID),
	})
	if err != nil {
		return fmt.Errorf("set session model failed: %w", err)
	}
	return nil
}

// Authenticate triggers ACP session/authenticate for a given auth method.
func (a *Adapter) Authenticate(ctx context.Context, methodID string) error {
	a.mu.RLock()
	conn := a.acpConn
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("adapter not initialized")
	}

	_, err := conn.Authenticate(ctx, acp.AuthenticateRequest{
		MethodId: acp.AuthMethodId(methodID),
	})
	if err != nil {
		return fmt.Errorf("authenticate failed: %w", err)
	}
	return nil
}

// derefStr safely dereferences a string pointer, returning empty string if nil.
func derefStr(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// buildResourceBlock constructs an ACP ResourceBlock from a MessageAttachment.
// Text-based MIME types use TextResourceContents; everything else uses BlobResourceContents.
func buildResourceBlock(att v1.MessageAttachment) acp.ContentBlock {
	uri := att.Name // Use filename as URI if no explicit URI
	if uri == "" {
		uri = "attachment"
	}
	if isTextMimeType(att.MimeType) {
		text := att.Data
		if decoded, err := base64.StdEncoding.DecodeString(att.Data); err == nil {
			text = string(decoded)
		}
		return acp.ResourceBlock(acp.EmbeddedResourceResource{
			TextResourceContents: &acp.TextResourceContents{
				Uri:      uri,
				Text:     text,
				MimeType: acp.Ptr(att.MimeType),
			},
		})
	}
	return acp.ResourceBlock(acp.EmbeddedResourceResource{
		BlobResourceContents: &acp.BlobResourceContents{
			Uri:      uri,
			Blob:     att.Data,
			MimeType: acp.Ptr(att.MimeType),
		},
	})
}

// isTextMimeType returns true for MIME types that represent text content.
func isTextMimeType(mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/json", "application/xml", "application/javascript",
		"application/typescript", "application/x-yaml", "application/toml",
		"application/x-sh", "application/sql":
		return true
	}
	return false
}

// convertToolCallContents converts ACP ToolCallContent items to our protocol-agnostic type.
func (a *Adapter) convertToolCallContents(contents []acp.ToolCallContent) []streams.ToolCallContentItem {
	if len(contents) == 0 {
		return nil
	}
	items := make([]streams.ToolCallContentItem, 0, len(contents))
	for _, c := range contents {
		switch {
		case c.Diff != nil:
			items = append(items, streams.ToolCallContentItem{
				Type:    "diff",
				Path:    c.Diff.Path,
				OldText: c.Diff.OldText,
				NewText: c.Diff.NewText,
			})
		case c.Content != nil:
			cb := a.convertContentBlockToStreams(c.Content.Content)
			if cb != nil {
				items = append(items, streams.ToolCallContentItem{
					Type:    "content",
					Content: cb,
				})
			}
		case c.Terminal != nil:
			items = append(items, streams.ToolCallContentItem{
				Type:       "terminal",
				TerminalID: c.Terminal.TerminalId,
			})
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// convertContentBlockToStreams converts an ACP ContentBlock to a streams.ContentBlock.
func (a *Adapter) convertContentBlockToStreams(cb acp.ContentBlock) *streams.ContentBlock {
	switch {
	case cb.Text != nil:
		return &streams.ContentBlock{Type: "text", Text: cb.Text.Text}
	case cb.Image != nil:
		return &streams.ContentBlock{Type: contentTypeImage, Data: cb.Image.Data, MimeType: cb.Image.MimeType, URI: derefStr(cb.Image.Uri)}
	case cb.Audio != nil:
		return &streams.ContentBlock{Type: contentTypeAudio, Data: cb.Audio.Data, MimeType: cb.Audio.MimeType}
	case cb.ResourceLink != nil:
		return &streams.ContentBlock{
			Type: "resource_link", URI: cb.ResourceLink.Uri, Name: cb.ResourceLink.Name,
			MimeType: derefStr(cb.ResourceLink.MimeType), Title: derefStr(cb.ResourceLink.Title),
			Description: derefStr(cb.ResourceLink.Description), Size: cb.ResourceLink.Size,
		}
	case cb.Resource != nil:
		block := &streams.ContentBlock{Type: "resource"}
		res := cb.Resource.Resource
		switch {
		case res.TextResourceContents != nil:
			block.URI = res.TextResourceContents.Uri
			block.Text = res.TextResourceContents.Text
			block.MimeType = derefStr(res.TextResourceContents.MimeType)
		case res.BlobResourceContents != nil:
			block.URI = res.BlobResourceContents.Uri
			block.Data = res.BlobResourceContents.Blob
			block.MimeType = derefStr(res.BlobResourceContents.MimeType)
		}
		return block
	default:
		return nil
	}
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
		status = "in_progress"
	}

	return &AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolName:          toolKind, // Kind is effectively the tool name
		ToolTitle:         tc.Title,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
		ToolCallContents:  a.convertToolCallContents(tc.Content),
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
		ToolCallContents:  a.convertToolCallContents(tcu.Content),
	}
}

// handlePermissionRequest handles permission requests from the agent.
// Since both acpclient and adapter now use the shared types package,
// no conversion is needed - we just forward to the handler.
func (a *Adapter) handlePermissionRequest(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	fallbackSessionID := a.sessionID
	a.mu.RUnlock()

	// Prefer session ID from the request; fall back to adapter-level session ID
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fallbackSessionID
	}

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
	a.sendUpdate(toolCallEvent)
	a.logger.Debug("emitted tool_call event for permission request",
		zap.String("tool_call_id", req.ToolCallID))

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
