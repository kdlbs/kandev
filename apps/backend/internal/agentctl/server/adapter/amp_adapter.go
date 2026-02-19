package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/amp"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// AmpAdapter implements AgentAdapter for Sourcegraph Amp CLI.
// It uses the stream-json protocol over stdin/stdout, similar to Claude Code.
type AmpAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Normalizer for converting tool data to NormalizedPayload
	normalizer *AmpNormalizer

	// Subprocess stdin/stdout (set via Connect)
	stdin  io.Writer
	stdout io.Reader

	// Amp client for protocol communication
	client *amp.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state
	sessionID         string // Thread ID in Amp terminology
	operationID       string
	sessionStatusSent bool

	// Track pending tool calls and their normalized payloads
	pendingToolCalls    map[string]bool
	pendingToolPayloads map[string]*streams.NormalizedPayload

	// Accumulate text for the complete event
	// This is needed because the manager's buffer may be cleared by session code
	// before events are processed, causing a race condition
	textAccumulator strings.Builder

	// Agent info
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Result completion signaling
	resultCh chan resultComplete

	// Context window tracking
	mainModelName          string
	mainModelContextWindow int64
	contextTokensUsed      int64

	// Track if complete event was already sent for current operation
	// (to avoid duplicates from both stop_reason=end_turn and result message)
	completeSent bool

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// defaultAmpContextWindow is the fallback context window size.
const defaultAmpContextWindow = 200000

// NewAmpAdapter creates a new Amp protocol adapter.
func NewAmpAdapter(cfg *Config, log *logger.Logger) *AmpAdapter {
	ctx, cancel := context.WithCancel(context.Background())

	return &AmpAdapter{
		cfg:                    cfg,
		logger:                 log.WithFields(zap.String("adapter", "amp")),
		normalizer:             NewAmpNormalizer(),
		ctx:                    ctx,
		cancel:                 cancel,
		updatesCh:              make(chan AgentEvent, 100),
		mainModelContextWindow: defaultAmpContextWindow,
		pendingToolCalls:       make(map[string]bool),
		pendingToolPayloads:    make(map[string]*streams.NormalizedPayload),
	}
}

// PrepareEnvironment performs protocol-specific setup before the agent process starts.
// Amp reads configuration from ~/.config/amp/settings.json.
func (a *AmpAdapter) PrepareEnvironment() (map[string]string, error) {
	// Amp MCP configuration is handled via config file
	return nil, nil
}

// PrepareCommandArgs returns extra command-line arguments for the agent process.
// For Amp, no extra args are needed - configuration is via config files.
func (a *AmpAdapter) PrepareCommandArgs() []string {
	return nil
}

// Connect wires up the stdin/stdout pipes from the running agent subprocess.
func (a *AmpAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin != nil || a.stdout != nil {
		return fmt.Errorf("adapter already connected")
	}

	a.stdin = stdin
	a.stdout = stdout
	return nil
}

// Initialize establishes the Amp connection with the agent subprocess.
func (a *AmpAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Amp adapter",
		zap.String("workdir", a.cfg.WorkDir))

	// Create Amp client
	a.client = amp.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetMessageHandler(a.handleMessage)

	// Start reading from stdout
	a.client.Start(a.ctx)

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    "amp",
		Version: "unknown",
	}

	a.logger.Info("Amp adapter initialized")
	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *AmpAdapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// NewSession creates a new Amp session (thread).
// Note: Amp sessions are created implicitly with the first prompt.
func (a *AmpAdapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate a placeholder session ID - Amp will return its thread ID
	sessionID := uuid.New().String()
	a.sessionID = sessionID

	a.logger.Info("created new session placeholder", zap.String("session_id", sessionID))

	return sessionID, nil
}

// LoadSession resumes an existing Amp session (thread).
// The thread ID will be passed to Amp via the process manager which handles
// the "threads fork" and "threads continue" commands.
func (a *AmpAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()

	// Set thread ID in client for tracking
	a.client.SetThreadID(sessionID)

	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	return nil
}

// Prompt sends a prompt to Amp and waits for completion.
// Note: attachments are not yet supported in Amp protocol - they are ignored.
func (a *AmpAdapter) Prompt(ctx context.Context, message string, _ []v1.MessageAttachment) error {
	a.mu.Lock()
	sessionID := a.sessionID
	operationID := uuid.New().String()
	a.operationID = operationID
	a.resultCh = make(chan resultComplete, 1)
	a.completeSent = false // Reset completion flag for new operation
	a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID))

	// Send user message
	if err := a.client.SendUserMessage(message); err != nil {
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
func (a *AmpAdapter) Cancel(ctx context.Context) error {
	// Amp doesn't have a cancel command in stream-json mode
	// Cancellation is handled by killing the process
	a.logger.Info("cancel requested (not supported in stream-json mode)")
	return nil
}

// Updates returns the channel for agent events.
func (a *AmpAdapter) Updates() <-chan AgentEvent {
	return a.updatesCh
}

// GetSessionID returns the current session ID (thread ID).
func (a *AmpAdapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

// GetOperationID returns the current operation ID.
func (a *AmpAdapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.operationID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *AmpAdapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// Close releases resources held by the adapter.
func (a *AmpAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	a.logger.Info("closing Amp adapter")

	// Cancel the context
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
func (a *AmpAdapter) sendUpdate(update AgentEvent) {
	shared.LogNormalizedEvent(shared.ProtocolAmp, a.cfg.AgentID, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
}

// handleMessage processes streaming messages from Amp.
func (a *AmpAdapter) handleMessage(msg *amp.Message) {
	// Log raw event for debugging
	if rawData, err := json.Marshal(msg); err == nil {
		shared.LogRawEvent(shared.ProtocolAmp, a.cfg.AgentID, msg.Type, rawData)
	}

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	// Update session ID from thread ID if provided
	if msg.ThreadID != "" {
		a.mu.Lock()
		a.sessionID = msg.ThreadID
		sessionID = msg.ThreadID
		a.mu.Unlock()
	}

	switch msg.Type {
	case amp.MessageTypeSystem:
		a.handleSystemMessage(msg, sessionID)

	case amp.MessageTypeAssistant:
		a.handleAssistantMessage(msg, sessionID, operationID)

	case amp.MessageTypeUser:
		// Amp sends tool_result blocks in user messages (unlike Claude Code which sends them in assistant messages)
		a.handleUserMessage(msg, sessionID, operationID)

	case amp.MessageTypeResult:
		a.handleResultMessage(msg, sessionID, operationID)

	default:
		a.logger.Debug("unhandled message type", zap.String("type", msg.Type))
	}
}

// handleSystemMessage processes system init messages.
func (a *AmpAdapter) handleSystemMessage(msg *amp.Message, sessionID string) {
	a.logger.Info("received system message",
		zap.String("thread_id", msg.ThreadID))

	a.mu.Lock()
	alreadySent := a.sessionStatusSent
	a.sessionStatusSent = true
	a.mu.Unlock()

	// Only send session status once
	if alreadySent {
		return
	}

	status := "new"
	if msg.ThreadID != "" && msg.ThreadID == sessionID {
		status = "resumed"
	}

	a.sendUpdate(AgentEvent{
		Type:          EventTypeSessionStatus,
		SessionID:     sessionID,
		SessionStatus: status,
		Data: map[string]any{
			"session_status": status,
			"init":           true,
		},
	})
}

// handleUserMessage processes user messages.
// In Amp's protocol, tool_result blocks come in user messages (not assistant messages like Claude Code).
func (a *AmpAdapter) handleUserMessage(msg *amp.Message, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Process content blocks - only interested in tool_result
	for _, block := range msg.Message.Content {
		if block.Type == amp.ContentTypeToolResult {
			a.handleToolResult(&block, sessionID, operationID)
		}
	}
}

// updateMainModel updates agent version and tracks the main model name.
func (a *AmpAdapter) updateMainModel(model string) {
	if model == "" || a.agentInfo == nil {
		return
	}
	a.agentInfo.Version = model
	a.mu.Lock()
	if a.mainModelName == "" {
		a.mainModelName = model
		a.logger.Debug("tracking main model", zap.String("model", model))
	}
	a.mu.Unlock()
}

// emitContextWindow emits a context window event.
func (a *AmpAdapter) emitContextWindow(sessionID, operationID string, contextUsed, contextSize int64) {
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

// trackUsageAndEmitContextWindow updates token tracking and emits a context window event.
func (a *AmpAdapter) trackUsageAndEmitContextWindow(usage *amp.TokenUsage, sessionID, operationID string) {
	if usage == nil {
		return
	}
	contextUsed := usage.InputTokens + usage.OutputTokens +
		usage.CacheCreationInputTokens + usage.CacheReadInputTokens

	a.mu.Lock()
	a.contextTokensUsed = contextUsed
	contextSize := a.mainModelContextWindow
	a.mu.Unlock()

	a.emitContextWindow(sessionID, operationID, contextUsed, contextSize)
}

// processAssistantContentBlock processes a single content block from an assistant message.
func (a *AmpAdapter) processAssistantContentBlock(block *amp.ContentBlock, sessionID, operationID string) {
	switch block.Type {
	case amp.ContentTypeText:
		if block.Text != "" {
			// Accumulate text locally to include in complete event
			// This bypasses the race condition with the manager's buffer
			a.mu.Lock()
			a.textAccumulator.WriteString(block.Text)
			a.mu.Unlock()

			a.sendUpdate(AgentEvent{
				Type:        EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        block.Text,
			})
		}

	case amp.ContentTypeThinking:
		if block.Thinking != "" {
			a.sendUpdate(AgentEvent{
				Type:          EventTypeReasoning,
				SessionID:     sessionID,
				OperationID:   operationID,
				ReasoningText: block.Thinking,
			})
		}

	case amp.ContentTypeToolUse:
		a.handleToolUse(block, sessionID, operationID)

	case amp.ContentTypeToolResult:
		a.handleToolResult(block, sessionID, operationID)
	}
}

// handleAssistantMessage processes assistant messages.
func (a *AmpAdapter) handleAssistantMessage(msg *amp.Message, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Log content block types for debugging
	blockTypes := make([]string, 0, len(msg.Message.Content))
	for _, block := range msg.Message.Content {
		blockTypes = append(blockTypes, block.Type)
	}
	a.logger.Debug("processing assistant message",
		zap.Int("num_blocks", len(msg.Message.Content)),
		zap.Strings("block_types", blockTypes))

	// Update agent version and track main model
	a.updateMainModel(msg.Message.Model)

	// Process content blocks
	for i := range msg.Message.Content {
		a.processAssistantContentBlock(&msg.Message.Content[i], sessionID, operationID)
	}

	// Calculate and emit token usage
	a.trackUsageAndEmitContextWindow(msg.Message.Usage, sessionID, operationID)

	// Check for turn completion - Amp signals this via stop_reason in assistant messages
	// (unlike Claude Code which sends a separate "result" message)
	if msg.Message.StopReason == "end_turn" {
		a.handleTurnComplete(sessionID, operationID)
	}
}

// handleToolUse processes a tool_use content block.
func (a *AmpAdapter) handleToolUse(block *amp.ContentBlock, sessionID, operationID string) {
	// Generate normalized payload using the normalizer
	normalizedPayload := a.normalizer.NormalizeToolCall(block.Name, block.Input)

	// Build human-readable title
	toolTitle := block.Name
	// Amp uses "cmd" for Bash commands, Claude Code uses "command"
	if cmd, ok := block.Input["cmd"].(string); ok && block.Name == AmpToolBash {
		toolTitle = cmd
	} else if cmd, ok := block.Input["command"].(string); ok && block.Name == AmpToolBash {
		toolTitle = cmd
	} else if path, ok := block.Input["path"].(string); ok {
		// Amp uses "path" for Read tool
		toolTitle = path
	} else if path, ok := block.Input["file_path"].(string); ok {
		// Claude Code uses "file_path"
		toolTitle = path
	}

	a.logger.Info("tool_use block received",
		zap.String("tool_call_id", block.ID),
		zap.String("tool_name", block.Name),
		zap.String("title", toolTitle))

	// Track pending tool call and cache the normalized payload for result handling
	a.mu.Lock()
	a.pendingToolCalls[block.ID] = true
	a.pendingToolPayloads[block.ID] = normalizedPayload
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:              EventTypeToolCall,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        block.ID,
		ToolName:          block.Name,
		ToolTitle:         toolTitle,
		ToolStatus:        "running",
		NormalizedPayload: normalizedPayload,
	})
}

// handleToolResult processes a tool_result content block.
func (a *AmpAdapter) handleToolResult(block *amp.ContentBlock, sessionID, operationID string) {
	status := "complete"
	if block.IsError {
		status = "error"
	}

	a.logger.Info("tool_result block received",
		zap.String("tool_call_id", block.ToolUseID),
		zap.String("status", status))

	// Get cached payload and remove from pending
	a.mu.Lock()
	delete(a.pendingToolCalls, block.ToolUseID)
	cachedPayload := a.pendingToolPayloads[block.ToolUseID]
	delete(a.pendingToolPayloads, block.ToolUseID)
	a.mu.Unlock()

	// Normalize the tool result content
	normalizedPayload := a.normalizer.NormalizeToolResult(cachedPayload, block.Content, block.IsError)

	a.sendUpdate(AgentEvent{
		Type:              EventTypeToolUpdate,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        block.ToolUseID,
		ToolStatus:        status,
		NormalizedPayload: normalizedPayload,
	})
}

// handleTurnComplete processes turn completion when stop_reason is "end_turn".
// This is Amp's way of signaling turn completion (instead of a separate "result" message).
func (a *AmpAdapter) handleTurnComplete(sessionID, operationID string) {
	// Check if we already sent completion for this operation
	// (Amp may send both stop_reason=end_turn AND a result message)
	a.mu.Lock()
	if a.completeSent {
		a.mu.Unlock()
		a.logger.Debug("ignoring duplicate turn complete event",
			zap.String("session_id", sessionID),
			zap.String("operation_id", operationID))
		return
	}
	a.completeSent = true

	a.logger.Info("turn complete (stop_reason=end_turn)")

	// Auto-complete any pending tool calls and get accumulated text
	pendingTools := make(map[string]*streams.NormalizedPayload, len(a.pendingToolCalls))
	for toolID := range a.pendingToolCalls {
		pendingTools[toolID] = a.pendingToolPayloads[toolID]
	}
	a.pendingToolCalls = make(map[string]bool)
	a.pendingToolPayloads = make(map[string]*streams.NormalizedPayload)
	contextSize := a.mainModelContextWindow
	contextUsed := a.contextTokensUsed
	// Clear the text accumulator (text was already sent via message_chunk events)
	a.textAccumulator.Reset()
	a.mu.Unlock()

	for toolID, cachedPayload := range pendingTools {
		a.logger.Info("auto-completing pending tool call on turn complete",
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

	// Emit final context window event
	if contextUsed > 0 {
		a.emitContextWindow(sessionID, operationID, contextUsed, contextSize)
	}

	// Send completion event WITHOUT text - text was already sent via message_chunk events
	// Including text here would cause duplicate messages
	a.sendUpdate(AgentEvent{
		Type:        EventTypeComplete,
		SessionID:   sessionID,
		OperationID: operationID,
		Data: map[string]any{
			"stop_reason": "end_turn",
		},
	})

	// Signal completion
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh != nil {
		select {
		case resultCh <- resultComplete{success: true}:
			a.logger.Debug("signaled prompt completion")
		default:
			a.logger.Warn("result channel full, dropping signal")
		}
	}
}

// extractResultErrorMsg extracts the error message from a result message.
func (a *AmpAdapter) extractResultErrorMsg(msg *amp.Message) string {
	if !msg.IsError {
		return ""
	}
	if msg.Error != "" {
		return msg.Error
	}
	if errStr := msg.GetResultString(); errStr != "" {
		return errStr
	}
	if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
		return resultData.Text
	}
	return "prompt failed"
}

// signalResultCompletion sends the result to the result channel.
func (a *AmpAdapter) signalResultCompletion(success bool, errMsg string) {
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh == nil {
		return
	}
	select {
	case resultCh <- resultComplete{success: success, err: errMsg}:
		a.logger.Debug("signaled prompt completion")
	default:
		a.logger.Warn("result channel full, dropping signal")
	}
}

// updateContextWindowFromModelUsage updates the context window size from model_usage.
func (a *AmpAdapter) updateContextWindowFromModelUsage(msg *amp.Message) (contextUsed, contextSize int64) {
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
	contextSize = a.mainModelContextWindow
	contextUsed = a.contextTokensUsed
	a.mu.Unlock()
	return contextUsed, contextSize
}

// drainPendingToolCalls atomically takes all pending tool calls and clears them.
func (a *AmpAdapter) drainPendingToolCalls() map[string]*streams.NormalizedPayload {
	a.mu.Lock()
	pending := make(map[string]*streams.NormalizedPayload, len(a.pendingToolCalls))
	for toolID := range a.pendingToolCalls {
		pending[toolID] = a.pendingToolPayloads[toolID]
	}
	a.pendingToolCalls = make(map[string]bool)
	a.pendingToolPayloads = make(map[string]*streams.NormalizedPayload)
	a.mu.Unlock()
	return pending
}

// handleResultMessage processes result (completion) messages.
func (a *AmpAdapter) handleResultMessage(msg *amp.Message, sessionID, operationID string) {
	a.logger.Info("received result message",
		zap.Bool("is_error", msg.IsError),
		zap.Int("num_turns", msg.NumTurns),
		zap.String("subtype", msg.Subtype),
		zap.String("error", msg.Error))

	// Update session ID from result if provided
	if resultData := msg.GetResultData(); resultData != nil {
		if resultData.ThreadID != "" {
			a.mu.Lock()
			a.sessionID = resultData.ThreadID
			sessionID = resultData.ThreadID
			a.mu.Unlock()
		}
	}

	// Check if complete was already sent (by handleTurnComplete on stop_reason=end_turn)
	a.mu.Lock()
	alreadyCompleted := a.completeSent
	if !alreadyCompleted {
		a.completeSent = true
	}
	a.mu.Unlock()

	// Auto-complete any pending tool calls
	pendingTools := a.drainPendingToolCalls()
	for toolID, cachedPayload := range pendingTools {
		a.logger.Info("auto-completing pending tool call on result",
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

	// Extract context window from model_usage if available
	contextUsed, contextSize := a.updateContextWindowFromModelUsage(msg)

	// Emit final context window event
	if contextUsed > 0 {
		a.emitContextWindow(sessionID, operationID, contextUsed, contextSize)
	}

	// Only send completion event if not already sent by handleTurnComplete
	if !alreadyCompleted {
		// Clear the text accumulator (text was already sent via message_chunk events)
		a.mu.Lock()
		a.textAccumulator.Reset()
		a.mu.Unlock()

		// Send complete event WITHOUT text to avoid duplicates
		a.sendUpdate(AgentEvent{
			Type:        EventTypeComplete,
			SessionID:   sessionID,
			OperationID: operationID,
			Data: map[string]any{
				"cost_usd":      msg.CostUSD,
				"duration_ms":   msg.DurationMS,
				"num_turns":     msg.NumTurns,
				"input_tokens":  msg.TotalInputTokens,
				"output_tokens": msg.TotalOutputTokens,
				"is_error":      msg.IsError,
			},
		})
	}

	// Extract error message if failed
	errorMsg := a.extractResultErrorMsg(msg)

	// Signal completion
	a.signalResultCompletion(!msg.IsError, errorMsg)

	// Send error event if failed
	if msg.IsError {
		a.sendUpdate(AgentEvent{
			Type:        EventTypeError,
			SessionID:   sessionID,
			OperationID: operationID,
			Error:       errorMsg,
		})
	}
}

// RequiresProcessKill returns false because Amp agents exit when stdin is closed.
func (a *AmpAdapter) RequiresProcessKill() bool {
	return false
}

// Verify interface implementation
var _ AgentAdapter = (*AmpAdapter)(nil)
