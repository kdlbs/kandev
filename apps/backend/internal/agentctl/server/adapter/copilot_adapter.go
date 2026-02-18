package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/copilot"
	"go.uber.org/zap"
)

// CopilotAdapter implements AgentAdapter for GitHub Copilot using the official SDK.
//
// The process manager spawns the Copilot CLI in server mode (--server), which
// listens on a random TCP port and prints "listening on port <port>" to stdout.
// Connect() captures the stdout pipe, and Initialize() scans it for the port,
// then creates an SDK client that connects to the external server via CLIUrl.
type CopilotAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Normalizer for converting tool data to NormalizedPayload
	normalizer *CopilotNormalizer

	// stdout from the process manager (used to detect the listening port)
	stdout io.Reader

	// Copilot SDK client (connects to externally managed CLI server via TCP)
	client *copilot.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state
	sessionID   string
	operationID string

	// Track pending tool calls and their normalized payloads
	pendingToolPayloads map[string]*streams.NormalizedPayload

	// Accumulate text for the complete event
	textAccumulator strings.Builder

	// Agent info
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Result completion signaling
	resultCh chan resultComplete

	// Context window tracking (from usage events)
	contextWindowSize int64
	contextTokensUsed int64

	// Track if completion was already sent for current operation
	completeSent bool

	// Track if we received streaming deltas for this operation
	// (to avoid double-accumulating from both delta and full message events)
	receivedDeltas bool

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// resultComplete holds the result of a completed prompt
type resultComplete struct {
	success bool
	err     string
}

// defaultCopilotContextWindow is the fallback context window size.
const defaultCopilotContextWindow = 128000

// mcpServerTypeHTTP is the HTTP transport type identifier for MCP servers.
const mcpServerTypeHTTP = "http"

// NewCopilotAdapter creates a new Copilot protocol adapter.
func NewCopilotAdapter(cfg *Config, log *logger.Logger) *CopilotAdapter {
	ctx, cancel := context.WithCancel(context.Background())

	return &CopilotAdapter{
		cfg:                 cfg,
		logger:              log.WithFields(zap.String("adapter", "copilot")),
		normalizer:          NewCopilotNormalizer(),
		ctx:                 ctx,
		cancel:              cancel,
		updatesCh:           make(chan AgentEvent, 100),
		contextWindowSize:   defaultCopilotContextWindow,
		pendingToolPayloads: make(map[string]*streams.NormalizedPayload),
	}
}

// PrepareEnvironment performs protocol-specific setup before the agent process starts.
// No special environment is needed; the process manager spawns the CLI in server mode.
func (a *CopilotAdapter) PrepareEnvironment() (map[string]string, error) {
	return nil, nil
}

// PrepareCommandArgs returns extra command-line arguments for the agent process.
// For Copilot, no extra args are needed - the CLI is spawned in server mode.
func (a *CopilotAdapter) PrepareCommandArgs() []string {
	return nil
}

// Connect stores the stdout pipe from the process manager.
// The adapter reads stdout during Initialize to detect the TCP port
// the Copilot CLI server is listening on.
func (a *CopilotAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdout = stdout
	a.logger.Info("Connect: stored stdout pipe for port detection")
	return nil
}

// Initialize waits for the Copilot CLI server to print its listening port,
// then creates an SDK client that connects to it via TCP.
func (a *CopilotAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Copilot adapter (server mode)",
		zap.String("workdir", a.cfg.WorkDir))

	if a.stdout == nil {
		return fmt.Errorf("stdout not connected; call Connect() before Initialize()")
	}

	// Scan stdout for "listening on port <port>" from the CLI server
	port, scanner, err := a.waitForPort(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect Copilot CLI server port: %w", err)
	}

	cliURL := fmt.Sprintf("localhost:%d", port)
	a.logger.Info("detected Copilot CLI server",
		zap.Int("port", port),
		zap.String("cli_url", cliURL))

	// Create SDK client pointing at the external server
	a.client = copilot.NewClient(copilot.ClientConfig{
		CLIUrl: cliURL,
	}, a.logger)

	// Set event handler before starting
	a.client.SetEventHandler(a.handleEvent)

	// Set permission handler before starting
	a.client.SetPermissionHandler(a.handlePermissionRequest)

	// Start the SDK client (connects to external server via TCP)
	if err := a.client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Copilot SDK client: %w", err)
	}

	// Drain remaining stdout in background so the process doesn't block
	go func() {
		for scanner.Scan() {
			// discard
		}
	}()

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    "copilot",
		Version: "sdk",
	}

	a.logger.Info("Copilot adapter initialized (server mode)")
	return nil
}

// portPattern matches "listening on port <number>" printed by the Copilot CLI in server mode.
var portPattern = regexp.MustCompile(`listening on port (\d+)`)

// waitForPort scans stdout line-by-line until it finds the listening port.
// Returns the detected port and the scanner (for background draining).
func (a *CopilotAdapter) waitForPort(ctx context.Context) (int, *bufio.Scanner, error) {
	const timeout = 180 * time.Second

	scanner := bufio.NewScanner(a.stdout)
	portCh := make(chan int, 1)
	errCh := make(chan error, 1)

	go func() {
		var capturedLines []string
		for scanner.Scan() {
			line := scanner.Text()
			a.logger.Debug("CLI stdout", zap.String("line", line))

			if len(capturedLines) < 64 {
				capturedLines = append(capturedLines, line)
			}

			if m := portPattern.FindStringSubmatch(line); m != nil {
				port, err := strconv.Atoi(m[1])
				if err != nil {
					errCh <- fmt.Errorf("invalid port number %q: %w", m[1], err)
					return
				}
				portCh <- port
				return
			}
		}
		// EOF before finding port
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("error reading stdout: %w", err)
		} else {
			var detail strings.Builder
			if len(capturedLines) > 0 {
				start := 0
				if len(capturedLines) > 12 {
					start = len(capturedLines) - 12
				}
				for _, l := range capturedLines[start:] {
					detail.WriteString("\n  ")
					detail.WriteString(l)
				}
			}
			errCh <- fmt.Errorf("CLI exited before printing listening port%s", detail.String())
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case port := <-portCh:
		return port, scanner, nil
	case err := <-errCh:
		return 0, nil, err
	case <-timer.C:
		return 0, nil, fmt.Errorf("timeout (%s) waiting for Copilot CLI to print listening port", timeout)
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

// GetAgentInfo returns information about the connected agent.
func (a *CopilotAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// mcpServersToCopilotConfig converts agentctl MCP server list to copilot SDK format.
func mcpServersToCopilotConfig(servers []types.McpServer) map[string]copilot.MCPServerConfig {
	if len(servers) == 0 {
		return nil
	}
	result := make(map[string]copilot.MCPServerConfig, len(servers))
	for _, srv := range servers {
		cfg := copilot.MCPServerConfig{
			"tools": []string{"*"},
		}
		switch srv.Type {
		case "sse":
			cfg["type"] = "sse"
			cfg["url"] = srv.URL
		case mcpServerTypeHTTP:
			cfg["type"] = mcpServerTypeHTTP
			cfg["url"] = srv.URL
		default: // stdio / local
			cfg["type"] = "local"
			cfg["command"] = srv.Command
			if srv.Args != nil {
				cfg["args"] = srv.Args
			}
		}
		result[srv.Name] = cfg
	}
	return result
}

// NewSession creates a new Copilot session via the SDK.
func (a *CopilotAdapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	mcpConfig := mcpServersToCopilotConfig(mcpServers)
	sessionID, err := a.client.CreateSession(ctx, mcpConfig)
	if err != nil {
		// If session creation fails, generate a placeholder ID
		sessionID = uuid.New().String()
		a.logger.Warn("failed to create SDK session, using placeholder",
			zap.Error(err),
			zap.String("session_id", sessionID))
	}

	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()

	a.logger.Info("created new session", zap.String("session_id", sessionID))

	// Send session status event
	a.sendUpdate(AgentEvent{
		Type:          EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: "new",
		Data: map[string]any{
			"session_status": "new",
			"init":           true,
		},
	})

	return sessionID, nil
}

// LoadSession resumes an existing Copilot session via the SDK.
func (a *CopilotAdapter) LoadSession(ctx context.Context, sessionID string) error {
	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	if err := a.client.ResumeSession(ctx, sessionID, nil); err != nil {
		a.logger.Warn("failed to resume SDK session, continuing anyway",
			zap.Error(err),
			zap.String("session_id", sessionID))
	}

	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()

	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	// Send session status event
	a.sendUpdate(AgentEvent{
		Type:          EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: "resumed",
		Data: map[string]any{
			"session_status": "resumed",
			"init":           true,
		},
	})

	return nil
}

// Prompt sends a prompt to Copilot and waits for completion.
// Note: attachments are not yet supported in Copilot protocol - they are ignored.
func (a *CopilotAdapter) Prompt(ctx context.Context, message string, _ []v1.MessageAttachment) error {
	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.mu.Lock()
	sessionID := a.sessionID
	operationID := uuid.New().String()
	a.operationID = operationID
	a.resultCh = make(chan resultComplete, 1)
	a.completeSent = false    // Reset completion flag for new operation
	a.receivedDeltas = false  // Reset delta tracking for new operation
	a.mu.Unlock()

	a.logger.Info("sending prompt via SDK",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID))

	// Send message (non-blocking, events come via handler)
	if _, err := a.client.Send(ctx, message); err != nil {
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	// Wait for idle (completion) or context cancellation
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
func (a *CopilotAdapter) Cancel(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling operation via SDK")
	return a.client.Abort(ctx)
}

// Updates returns the channel for agent events.
func (a *CopilotAdapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID.
func (a *CopilotAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation ID.
func (a *CopilotAdapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.operationID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *CopilotAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *CopilotAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing Copilot SDK adapter")

	// Cancel the context
	if a.cancel != nil {
		a.cancel()
	}

	// Stop the SDK client (disconnects from the CLI server)
	if a.client != nil {
		if err := a.client.Stop(); err != nil {
			a.logger.Warn("error stopping SDK client", zap.Error(err))
		}
		a.client = nil
	}

	// Close update channel
	close(a.updatesCh)

	return nil
}

// sendUpdate safely sends an event to the updates channel.
func (a *CopilotAdapter) sendUpdate(update AgentEvent) {
	shared.LogNormalizedEvent(shared.ProtocolCopilot, a.cfg.AgentID, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
}

// handleEvent processes events from the Copilot SDK client.
func (a *CopilotAdapter) handleEvent(evt copilot.SessionEvent) {
	// Log raw event for debugging
	if rawData, err := json.Marshal(evt); err == nil {
		shared.LogRawEvent(shared.ProtocolCopilot, a.cfg.AgentID, string(evt.Type), rawData)
	}

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	// Use session ID from event data if provided
	if evt.Data.SessionID != nil && *evt.Data.SessionID != "" {
		sessionID = *evt.Data.SessionID
	}

	if a.handleLifecycleEvent(evt, sessionID, operationID) {
		return
	}
	a.handleContentEvent(evt, sessionID, operationID)
}

// handleLifecycleEvent handles session lifecycle events.
// Returns true if the event was handled.
func (a *CopilotAdapter) handleLifecycleEvent(evt copilot.SessionEvent, sessionID, operationID string) bool {
	switch evt.Type {
	case copilot.EventTypeSessionStart:
		a.logger.Info("session started", zap.String("session_id", sessionID))
	case copilot.EventTypeSessionResume:
		a.logger.Info("session resumed", zap.String("session_id", sessionID))
	case copilot.EventTypeAssistantTurnStart:
		a.logger.Debug("turn started", zap.String("operation_id", operationID))
	case copilot.EventTypeAssistantTurnEnd:
		a.logger.Debug("turn ended", zap.String("operation_id", operationID))
	case copilot.EventTypeAbort:
		a.logger.Info("operation aborted")
		a.signalCompletion(false, "operation aborted")
	default:
		return false
	}
	return true
}

// handleContentEvent handles content and tool events.
func (a *CopilotAdapter) handleContentEvent(evt copilot.SessionEvent, sessionID, operationID string) {
	switch evt.Type {
	case copilot.EventTypeAssistantMessage:
		a.handleAssistantMessage(evt, sessionID, operationID)
	case copilot.EventTypeAssistantMessageDelta:
		a.handleAssistantMessageDelta(evt, sessionID, operationID)
	case copilot.EventTypeAssistantReasoning, copilot.EventTypeAssistantReasoningDelta:
		a.handleAssistantReasoning(evt, sessionID, operationID)
	case copilot.EventTypeToolStart:
		a.handleToolStart(evt, sessionID, operationID)
	case copilot.EventTypeToolComplete:
		a.handleToolComplete(evt, sessionID, operationID)
	case copilot.EventTypeToolProgress:
		a.handleToolProgress(evt, sessionID, operationID)
	case copilot.EventTypeSessionIdle:
		a.handleSessionIdle(sessionID, operationID)
	case copilot.EventTypeSessionError:
		a.handleSessionError(evt, sessionID, operationID)
	case copilot.EventTypeSessionUsageInfo, copilot.EventTypeAssistantUsage:
		a.handleUsageInfo(evt, sessionID, operationID)
	default:
		a.logger.Debug("unhandled SDK event", zap.String("type", string(evt.Type)))
	}
}

// handleAssistantMessage handles a full (non-streaming) assistant message event.
// It skips accumulation if streaming deltas were already received to avoid duplication.
func (a *CopilotAdapter) handleAssistantMessage(evt copilot.SessionEvent, sessionID, operationID string) {
	if evt.Data.Content == nil || *evt.Data.Content == "" {
		return
	}
	a.mu.Lock()
	alreadyReceivedDeltas := a.receivedDeltas
	if !alreadyReceivedDeltas {
		// Non-streaming mode: accumulate the full message
		a.textAccumulator.WriteString(*evt.Data.Content)
	}
	a.mu.Unlock()

	// Only send message chunk if we haven't been streaming
	if !alreadyReceivedDeltas {
		a.sendUpdate(AgentEvent{
			Type:        EventTypeMessageChunk,
			SessionID:   sessionID,
			OperationID: operationID,
			Text:        *evt.Data.Content,
		})
	}
}

// handleAssistantMessageDelta handles a streaming message delta event.
func (a *CopilotAdapter) handleAssistantMessageDelta(evt copilot.SessionEvent, sessionID, operationID string) {
	if evt.Data.DeltaContent == nil || *evt.Data.DeltaContent == "" {
		return
	}
	text := *evt.Data.DeltaContent
	// Accumulate text locally to include in complete event
	a.mu.Lock()
	a.textAccumulator.WriteString(text)
	a.receivedDeltas = true // Mark that we've received streaming deltas
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:        EventTypeMessageChunk,
		SessionID:   sessionID,
		OperationID: operationID,
		Text:        text,
	})
}

// handleAssistantReasoning handles reasoning/thinking content events.
func (a *CopilotAdapter) handleAssistantReasoning(evt copilot.SessionEvent, sessionID, operationID string) {
	var content string
	if evt.Data.Content != nil {
		content = *evt.Data.Content
	}
	if content == "" && evt.Data.DeltaContent != nil {
		content = *evt.Data.DeltaContent
	}
	if content != "" {
		a.sendUpdate(AgentEvent{
			Type:          EventTypeReasoning,
			SessionID:     sessionID,
			OperationID:   operationID,
			ReasoningText: content,
		})
	}
}

// handleToolProgress handles tool progress update events.
func (a *CopilotAdapter) handleToolProgress(evt copilot.SessionEvent, sessionID, operationID string) {
	toolCallID := ""
	if evt.Data.ToolCallID != nil {
		toolCallID = *evt.Data.ToolCallID
	}
	a.sendUpdate(AgentEvent{
		Type:        EventTypeToolUpdate,
		SessionID:   sessionID,
		OperationID: operationID,
		ToolCallID:  toolCallID,
		ToolStatus:  "running",
	})
}

// handleToolStart processes tool execution start events.
func (a *CopilotAdapter) handleToolStart(evt copilot.SessionEvent, sessionID, operationID string) {
	toolCallID := ""
	toolName := ""
	var toolArgs any

	if evt.Data.ToolCallID != nil {
		toolCallID = *evt.Data.ToolCallID
	}
	if evt.Data.ToolName != nil {
		toolName = *evt.Data.ToolName
	}
	toolArgs = evt.Data.Arguments

	// Generate normalized payload using the normalizer
	normalizedPayload := a.normalizer.NormalizeToolCall(toolName, toolArgs)

	// Build human-readable title
	toolTitle := toolName
	if argsMap, ok := toolArgs.(map[string]any); ok && argsMap != nil {
		if cmd, ok := argsMap["command"].(string); ok && strings.ToLower(toolName) == "bash" {
			toolTitle = cmd
		} else if path, ok := argsMap["file_path"].(string); ok {
			toolTitle = fmt.Sprintf("%s: %s", toolName, path)
		}
	}

	a.logger.Info("tool execution started",
		zap.String("tool_call_id", toolCallID),
		zap.String("tool_name", toolName))

	// Track pending tool call and cache the normalized payload for result handling
	a.mu.Lock()
	a.pendingToolPayloads[toolCallID] = normalizedPayload
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:              EventTypeToolCall,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        toolCallID,
		ToolName:          toolName,
		ToolTitle:         toolTitle,
		ToolStatus:        "running",
		NormalizedPayload: normalizedPayload,
	})
}

// handleToolComplete processes tool execution complete events.
func (a *CopilotAdapter) handleToolComplete(evt copilot.SessionEvent, sessionID, operationID string) {
	toolCallID := ""
	if evt.Data.ToolCallID != nil {
		toolCallID = *evt.Data.ToolCallID
	}

	status := "complete"

	a.logger.Info("tool execution completed",
		zap.String("tool_call_id", toolCallID),
		zap.String("status", status))

	// Get cached payload and remove from pending
	a.mu.Lock()
	cachedPayload := a.pendingToolPayloads[toolCallID]
	delete(a.pendingToolPayloads, toolCallID)
	a.mu.Unlock()

	// Normalize the tool result if we have result data
	var normalizedPayload *streams.NormalizedPayload
	if cachedPayload != nil {
		// Use the result from the event if available
		normalizedPayload = a.normalizer.NormalizeToolResult(cachedPayload, evt.Data.Result, false)
	}

	a.sendUpdate(AgentEvent{
		Type:              EventTypeToolUpdate,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        toolCallID,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
	})
}

// handleSessionIdle processes session idle (completion) events.
func (a *CopilotAdapter) handleSessionIdle(sessionID, operationID string) {
	// Check if we already sent completion for this operation (SDK may send multiple idle events)
	a.mu.Lock()
	if a.completeSent {
		a.mu.Unlock()
		a.logger.Debug("ignoring duplicate session idle event",
			zap.String("session_id", sessionID),
			zap.String("operation_id", operationID))
		return
	}
	a.completeSent = true

	// Clear the text accumulator - text was already sent via message_chunk events
	// so we should NOT include it in the complete event to avoid duplicates
	a.textAccumulator.Reset()

	// Auto-complete any pending tool calls
	pendingTools := make(map[string]*streams.NormalizedPayload, len(a.pendingToolPayloads))
	for toolID, payload := range a.pendingToolPayloads {
		pendingTools[toolID] = payload
	}
	a.pendingToolPayloads = make(map[string]*streams.NormalizedPayload)
	a.mu.Unlock()

	a.logger.Info("session idle (operation completed)",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID))

	// Auto-complete pending tool calls
	for toolID, cachedPayload := range pendingTools {
		a.logger.Info("auto-completing pending tool call on idle",
			zap.String("tool_call_id", toolID))
		a.sendUpdate(AgentEvent{
			Type:              EventTypeToolUpdate,
			SessionID:         sessionID,
			OperationID:       operationID,
			ToolCallID:        toolID,
			ToolStatus:        "complete",
			NormalizedPayload: cachedPayload,
		})
	}

	// Send completion event WITHOUT text - text was already sent via message_chunk events.
	// Including text here would cause duplicate messages.
	a.sendUpdate(AgentEvent{
		Type:        EventTypeComplete,
		SessionID:   sessionID,
		OperationID: operationID,
	})

	// Signal completion
	a.signalCompletion(true, "")
}

// handleSessionError processes session error events.
func (a *CopilotAdapter) handleSessionError(evt copilot.SessionEvent, sessionID, operationID string) {
	errorMsg := "unknown error"
	if evt.Data.Message != nil {
		errorMsg = *evt.Data.Message
	}

	errorType := ""
	if evt.Data.ErrorType != nil {
		errorType = *evt.Data.ErrorType
	}

	a.logger.Error("session error",
		zap.String("error", errorMsg),
		zap.String("error_type", errorType))

	a.sendUpdate(AgentEvent{
		Type:        EventTypeError,
		SessionID:   sessionID,
		OperationID: operationID,
		Error:       errorMsg,
	})

	// Signal failure
	a.signalCompletion(false, errorMsg)
}

// handleUsageInfo processes usage info events.
// session.usage_info events provide CurrentTokens/TokenLimit (session-level tracking).
// assistant.usage events provide InputTokens/OutputTokens (per-call tracking).
func (a *CopilotAdapter) handleUsageInfo(evt copilot.SessionEvent, sessionID, operationID string) {
	var contextUsed, contextSize int64

	switch evt.Type {
	case copilot.EventTypeSessionUsageInfo:
		// session.usage_info: uses CurrentTokens/TokenLimit fields
		if evt.Data.CurrentTokens != nil {
			contextUsed = int64(*evt.Data.CurrentTokens)
		}
		if evt.Data.TokenLimit != nil {
			contextSize = int64(*evt.Data.TokenLimit)
		}
	case copilot.EventTypeAssistantUsage:
		// assistant.usage: uses InputTokens/OutputTokens fields
		var inputTokens, outputTokens int64
		if evt.Data.InputTokens != nil {
			inputTokens = int64(*evt.Data.InputTokens)
		}
		if evt.Data.OutputTokens != nil {
			outputTokens = int64(*evt.Data.OutputTokens)
		}
		contextUsed = inputTokens + outputTokens
	}

	a.mu.Lock()
	if contextUsed > 0 {
		a.contextTokensUsed = contextUsed
	}
	if contextSize > 0 {
		a.contextWindowSize = contextSize
	}
	// Read final values after update
	contextUsed = a.contextTokensUsed
	contextSize = a.contextWindowSize
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

// signalCompletion signals the result channel.
func (a *CopilotAdapter) signalCompletion(success bool, errMsg string) {
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh != nil {
		select {
		case resultCh <- resultComplete{success: success, err: errMsg}:
		default:
		}
	}
}

// handlePermissionRequest handles permission requests from the Copilot SDK.
// This is called by the SDK when the agent needs permission for an action.
func (a *CopilotAdapter) handlePermissionRequest(
	request copilot.PermissionRequest,
	invocation copilot.PermissionInvocation,
) (copilot.PermissionRequestResult, error) {
	a.mu.RLock()
	handler := a.permissionHandler
	sessionID := a.sessionID
	a.mu.RUnlock()

	// If no handler is set, auto-approve
	if handler == nil {
		a.logger.Debug("auto-approving permission (no handler)",
			zap.String("kind", request.Kind),
			zap.String("tool_call_id", request.ToolCallID))
		return copilot.PermissionRequestResult{Kind: "approved"}, nil
	}

	permReq := a.buildPermissionRequest(request, sessionID)

	a.logger.Info("requesting permission from user",
		zap.String("kind", request.Kind),
		zap.String("tool_call_id", request.ToolCallID),
		zap.String("session_id", sessionID))

	// Call the permission handler (blocking - waits for user response)
	ctx := context.Background()
	resp, err := handler(ctx, permReq)
	if err != nil {
		a.logger.Warn("permission handler error", zap.Error(err))
		return copilot.PermissionRequestResult{Kind: "denied-interactively-by-user"}, nil
	}

	if resp == nil || resp.Cancelled {
		a.logger.Info("permission denied (cancelled)")
		return copilot.PermissionRequestResult{Kind: "denied-interactively-by-user"}, nil
	}

	// Convert response
	if resp.OptionID == "allow" {
		a.logger.Info("permission approved")
		return copilot.PermissionRequestResult{Kind: "approved"}, nil
	}

	a.logger.Info("permission denied")
	return copilot.PermissionRequestResult{Kind: "denied-interactively-by-user"}, nil
}

// buildPermissionRequest constructs a PermissionRequest from a Copilot SDK request,
// waiting briefly for cached tool payload data to populate action details.
func (a *CopilotAdapter) buildPermissionRequest(request copilot.PermissionRequest, sessionID string) *PermissionRequest {
	// Build a human-readable title using cached tool data if available.
	// The pendingToolPayloads map is populated by handleToolStart events
	// which run in a separate goroutine from the permission handler.
	// Wait briefly for the tool_call event to be processed first, ensuring
	// the tool_call message is created in the DB before the permission message
	// (needed for the frontend merge logic to work).
	title := request.Kind
	actionType := request.Kind
	actionDetails := request.Extra

	var cachedPayload *streams.NormalizedPayload
	for i := 0; i < 10; i++ {
		a.mu.RLock()
		cachedPayload = a.pendingToolPayloads[request.ToolCallID]
		a.mu.RUnlock()
		if cachedPayload != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if cachedPayload != nil {
		var newDetails interface{}
		title, actionType, newDetails = a.resolvePermissionActionDetails(cachedPayload, title, actionType, actionDetails)
		if m, ok := newDetails.(map[string]interface{}); ok {
			actionDetails = m
		}
	}

	return &PermissionRequest{
		SessionID:     sessionID,
		ToolCallID:    request.ToolCallID,
		Title:         title,
		ActionType:    actionType,
		ActionDetails: actionDetails,
		PendingID:     request.ToolCallID,
		Options: []PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
			{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
		},
	}
}

// resolvePermissionActionDetails maps a cached tool payload to a human-readable title,
// action type, and action details for use in a permission request.
func (a *CopilotAdapter) resolvePermissionActionDetails(
	cachedPayload *streams.NormalizedPayload,
	title, actionType string,
	actionDetails map[string]interface{},
) (string, string, map[string]interface{}) {
	switch cachedPayload.Kind() {
	case streams.ToolKindShellExec:
		if se := cachedPayload.ShellExec(); se != nil {
			return se.Command, types.ActionTypeCommand, map[string]interface{}{
				"command": se.Command,
				"cwd":    se.WorkDir,
			}
		}
	case streams.ToolKindModifyFile:
		if mf := cachedPayload.ModifyFile(); mf != nil {
			return fmt.Sprintf("Write: %s", mf.FilePath), types.ActionTypeFileWrite, map[string]interface{}{
				"path": mf.FilePath,
			}
		}
	case streams.ToolKindReadFile:
		if rf := cachedPayload.ReadFile(); rf != nil {
			return fmt.Sprintf("Read: %s", rf.FilePath), types.ActionTypeFileRead, map[string]interface{}{
				"path": rf.FilePath,
			}
		}
	default:
		// For other tool kinds, use a generic label
		return string(cachedPayload.Kind()), actionType, actionDetails
	}
	return title, actionType, actionDetails
}

// RequiresProcessKill returns true because the Copilot CLI server doesn't exit on stdin close.
// The CLI runs as an HTTP server that must be explicitly killed.
func (a *CopilotAdapter) RequiresProcessKill() bool {
	return true
}

// Verify interface implementation
var _ AgentAdapter = (*CopilotAdapter)(nil)
