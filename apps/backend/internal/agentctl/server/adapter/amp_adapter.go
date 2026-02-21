package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
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
// Amp is a one-shot CLI: each prompt spawns a new process that reads stdin
// until EOF, processes the prompt, writes stream-json to stdout, and exits.
// For multi-turn conversations, follow-up prompts use "threads continue <id>".
type AmpAdapter struct {
	cfg    *Config
	logger *logger.Logger

	// Normalizer for converting tool data to NormalizedPayload
	normalizer *AmpNormalizer

	// One-shot subprocess management.
	// When oneShotCfg is set, the adapter spawns a new process per prompt
	// instead of using externally-provided stdin/stdout pipes.
	oneShotCfg *OneShotConfig
	process    *exec.Cmd  // Current subprocess (nil between prompts)
	processMu  sync.Mutex // Guards subprocess lifecycle

	// Subprocess stdin/stdout (set via Connect for non-one-shot, or per-prompt for one-shot)
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
	hasAmpThreadID    bool // True when sessionID contains a real Amp thread ID (T-xxx), not a placeholder

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

	// Track whether text was streamed this turn to prevent duplicates from result.text
	streamingTextSentThisTurn bool

	// lastRawData holds the raw JSON of the current message being processed.
	// Set in handleMessage before dispatch; used by sendUpdate for OTel tracing.
	lastRawData json.RawMessage

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
		oneShotCfg:             cfg.OneShotConfig,
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
// For one-shot adapters, this is a no-op — the adapter manages its own subprocess.
func (a *AmpAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	if a.oneShotCfg != nil {
		return nil // One-shot adapter manages its own subprocess per prompt
	}

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
// For one-shot adapters, this only sets up agent info — the actual client
// is created per-prompt in Prompt().
func (a *AmpAdapter) Initialize(ctx context.Context) error {
	a.logger.Info("initializing Amp adapter",
		zap.String("workdir", a.cfg.WorkDir),
		zap.Bool("one_shot", a.oneShotCfg != nil))

	// Store agent info
	a.agentInfo = &AgentInfo{
		Name:    "amp",
		Version: "unknown",
	}

	// For one-shot mode, skip client creation — it happens per-prompt
	if a.oneShotCfg != nil {
		a.logger.Info("Amp adapter initialized (one-shot mode)")
		return nil
	}

	// Legacy: create persistent client for non-one-shot mode
	a.client = amp.NewClient(a.stdin, a.stdout, a.logger)
	a.client.SetMessageHandler(a.handleMessage)
	a.client.Start(a.ctx)

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
// For one-shot mode, the session ID is used to build the "threads continue" command.
func (a *AmpAdapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	a.sessionID = sessionID
	a.hasAmpThreadID = sessionID != "" // LoadSession always receives a real Amp thread ID
	a.mu.Unlock()

	// Set thread ID in client for tracking (only for non-one-shot)
	if a.client != nil {
		a.client.SetThreadID(sessionID)
	}

	a.logger.Info("loaded session", zap.String("session_id", sessionID))

	return nil
}

// Prompt sends a prompt to Amp and waits for completion.
// In one-shot mode, this spawns a new subprocess per prompt.
// Note: attachments are not yet supported in Amp protocol - they are ignored.
func (a *AmpAdapter) Prompt(ctx context.Context, message string, _ []v1.MessageAttachment) error {
	if a.oneShotCfg != nil {
		return a.promptOneShot(ctx, message)
	}
	return a.promptLongLived(ctx, message)
}

// oneShotProcess holds the state of a one-shot subprocess spawned for a single prompt.
type oneShotProcess struct {
	cmd    *exec.Cmd
	client *amp.Client
	stdin  io.WriteCloser
}

// promptOneShot spawns a new Amp subprocess for each prompt.
// The process reads stdin until EOF, processes the prompt, writes stream-json
// to stdout, and exits. For follow-up prompts, "threads continue <id>" is used.
func (a *AmpAdapter) promptOneShot(ctx context.Context, message string) error {
	operationID := a.resetOneShotState()

	proc, err := a.spawnOneShotProcess(ctx)
	if err != nil {
		return err
	}
	defer a.clearOneShotProcess()

	// Send user message, then close stdin to signal EOF
	if err := proc.client.SendUserMessage(message); err != nil {
		proc.client.Stop()
		return fmt.Errorf("failed to send user message: %w", err)
	}
	if err := proc.stdin.Close(); err != nil {
		a.logger.Debug("failed to close stdin pipe", zap.Error(err))
	}

	// Wait for result message or context cancellation
	result, err := a.awaitOneShotResult(ctx, proc)
	if err != nil {
		return err
	}

	if !result.success && result.err != "" {
		return fmt.Errorf("prompt failed: %s", result.err)
	}

	a.logger.Info("one-shot prompt completed",
		zap.String("operation_id", operationID),
		zap.Bool("success", result.success))
	return nil
}

// resetOneShotState resets per-prompt state and returns the new operation ID.
func (a *AmpAdapter) resetOneShotState() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	operationID := uuid.New().String()
	a.operationID = operationID
	a.resultCh = make(chan resultComplete, 1)
	a.completeSent = false
	a.streamingTextSentThisTurn = false
	return operationID
}

// spawnOneShotProcess creates and starts a subprocess for a single prompt.
func (a *AmpAdapter) spawnOneShotProcess(ctx context.Context) (*oneShotProcess, error) {
	args := a.buildOneShotArgs()
	a.logger.Info("spawning one-shot Amp process", zap.Strings("args", args))

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = a.oneShotCfg.WorkDir
	cmd.Env = a.oneShotCfg.Env
	setProcGroup(cmd)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Amp process: %w", err)
	}

	a.processMu.Lock()
	a.process = cmd
	a.processMu.Unlock()

	client := amp.NewClient(stdinPipe, stdoutPipe, a.logger)
	client.SetMessageHandler(a.handleMessage)
	client.Start(ctx)

	go a.readOneShotStderr(stderrPipe)

	return &oneShotProcess{cmd: cmd, client: client, stdin: stdinPipe}, nil
}

// clearOneShotProcess clears the tracked subprocess after a prompt completes.
func (a *AmpAdapter) clearOneShotProcess() {
	a.processMu.Lock()
	a.process = nil
	a.processMu.Unlock()
}

// awaitOneShotResult waits for the result message, then cleans up the subprocess.
func (a *AmpAdapter) awaitOneShotResult(ctx context.Context, proc *oneShotProcess) (resultComplete, error) {
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	var result resultComplete
	select {
	case <-ctx.Done():
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		proc.client.Stop()
		return result, ctx.Err()
	case result = <-resultCh:
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
	}

	proc.client.Stop()
	if waitErr := proc.cmd.Wait(); waitErr != nil {
		a.logger.Debug("Amp process exited", zap.Error(waitErr))
	}
	return result, nil
}

// buildOneShotArgs builds the command arguments for a one-shot prompt.
// Uses the continue command with thread ID for follow-up prompts.
func (a *AmpAdapter) buildOneShotArgs() []string {
	a.mu.RLock()
	threadID := a.sessionID
	hasRealThread := a.hasAmpThreadID
	a.mu.RUnlock()

	// Use continue command only when we have a real Amp thread ID (T-xxx),
	// not a UUID placeholder from NewSession().
	if hasRealThread && threadID != "" && len(a.oneShotCfg.ContinueArgs) > 0 {
		args := make([]string, len(a.oneShotCfg.ContinueArgs), len(a.oneShotCfg.ContinueArgs)+1)
		copy(args, a.oneShotCfg.ContinueArgs)
		args = append(args, threadID)
		return args
	}

	// First prompt: use initial command
	args := make([]string, len(a.oneShotCfg.InitialArgs))
	copy(args, a.oneShotCfg.InitialArgs)
	return args
}

// readOneShotStderr reads stderr from the one-shot subprocess for logging.
func (a *AmpAdapter) readOneShotStderr(stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		a.logger.Debug("amp stderr", zap.String("line", scanner.Text()))
	}
}

// promptLongLived sends a prompt to a long-lived Amp process (legacy mode).
func (a *AmpAdapter) promptLongLived(ctx context.Context, message string) error {
	a.mu.Lock()
	operationID := uuid.New().String()
	a.operationID = operationID
	a.resultCh = make(chan resultComplete, 1)
	a.completeSent = false
	a.streamingTextSentThisTurn = false
	a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("sending prompt",
		zap.String("operation_id", operationID))

	if err := a.client.SendUserMessage(message); err != nil {
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to send user message: %w", err)
	}

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
// For one-shot mode, this kills the running subprocess.
func (a *AmpAdapter) Cancel(ctx context.Context) error {
	a.processMu.Lock()
	cmd := a.process
	a.processMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		a.logger.Info("cancelling one-shot Amp process", zap.Int("pid", cmd.Process.Pid))
		if err := killProcessGroup(cmd.Process.Pid); err != nil {
			// Fallback to direct kill if process group kill fails
			a.logger.Debug("process group kill failed, trying direct kill", zap.Error(err))
			_ = cmd.Process.Kill()
		}
		return nil
	}

	a.logger.Info("cancel requested (no running process)")
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

	// Kill any running one-shot subprocess
	a.processMu.Lock()
	if a.process != nil && a.process.Process != nil {
		_ = killProcessGroup(a.process.Process.Pid)
	}
	a.processMu.Unlock()

	// Stop the client (for non-one-shot mode)
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
	shared.TraceProtocolEvent(context.Background(), shared.ProtocolAmp, a.cfg.AgentID,
		update.Type, a.lastRawData, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
}

// handleMessage processes streaming messages from Amp.
func (a *AmpAdapter) handleMessage(msg *amp.Message) {
	// Marshal once for debug logging and tracing
	a.lastRawData, _ = json.Marshal(msg)
	if len(a.lastRawData) > 0 {
		shared.LogRawEvent(shared.ProtocolAmp, a.cfg.AgentID, msg.Type, a.lastRawData)
	}

	a.mu.RLock()
	sessionID := a.sessionID
	operationID := a.operationID
	a.mu.RUnlock()

	// Update session ID from thread ID if provided by Amp
	if msg.ThreadID != "" {
		a.mu.Lock()
		a.sessionID = msg.ThreadID
		a.hasAmpThreadID = true
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

	case amp.MessageTypeRateLimit:
		a.handleRateLimitMessage(msg, sessionID, operationID)

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
func (a *AmpAdapter) processAssistantContentBlock(block *amp.ContentBlock, sessionID, operationID, parentToolUseID string) {
	switch block.Type {
	case amp.ContentTypeText:
		if block.Text != "" {
			// Mark that text was streamed this turn (prevents duplicate from result.text)
			a.mu.Lock()
			a.streamingTextSentThisTurn = true
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
		a.handleToolUse(block, sessionID, operationID, parentToolUseID)

	case amp.ContentTypeToolResult:
		a.handleToolResult(block, sessionID, operationID)
	}
}

// handleAssistantMessage processes assistant messages.
func (a *AmpAdapter) handleAssistantMessage(msg *amp.Message, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Extract parent tool use ID for subagent nesting
	parentToolUseID := msg.ParentToolUseID

	// Log content block types for debugging
	blockTypes := make([]string, 0, len(msg.Message.Content))
	for _, block := range msg.Message.Content {
		blockTypes = append(blockTypes, block.Type)
	}
	a.logger.Debug("processing assistant message",
		zap.Int("num_blocks", len(msg.Message.Content)),
		zap.Strings("block_types", blockTypes),
		zap.String("parent_tool_use_id", parentToolUseID))

	// Update agent version and track main model
	a.updateMainModel(msg.Message.Model)

	// Process content blocks
	for i := range msg.Message.Content {
		a.processAssistantContentBlock(&msg.Message.Content[i], sessionID, operationID, parentToolUseID)
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
func (a *AmpAdapter) handleToolUse(block *amp.ContentBlock, sessionID, operationID, parentToolUseID string) {
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
		ParentToolCallID:  parentToolUseID,
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
	// Check errors array first (Claude Code format, most specific)
	if len(msg.Errors) > 0 {
		return strings.Join(msg.Errors, "; ")
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

// extractResultText extracts text from the result message for non-streamed responses.
func (a *AmpAdapter) extractResultText(msg *amp.Message) string {
	if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
		return resultData.Text
	}
	if resultStr := msg.GetResultString(); resultStr != "" {
		return resultStr
	}
	return ""
}

// handleRateLimitMessage processes rate limit notifications.
func (a *AmpAdapter) handleRateLimitMessage(msg *amp.Message, sessionID, operationID string) {
	message := "Rate limited by API"
	if len(msg.RateLimitInfo) > 0 {
		message = string(msg.RateLimitInfo)
	}

	a.logger.Warn("rate limit event received",
		zap.String("session_id", sessionID),
		zap.String("message", message))

	a.sendUpdate(AgentEvent{
		Type:             EventTypeRateLimit,
		SessionID:        sessionID,
		OperationID:      operationID,
		RateLimitMessage: message,
	})
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
			a.hasAmpThreadID = true
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
		// Check if text was already streamed this turn
		a.mu.RLock()
		textWasStreamed := a.streamingTextSentThisTurn
		a.mu.RUnlock()

		// Emit result.text as message_chunk if no text was streamed this turn
		// (e.g. slash commands or very short responses)
		if !textWasStreamed {
			if resultText := a.extractResultText(msg); resultText != "" {
				a.logger.Debug("sending result text as message_chunk (no streaming text this turn)",
					zap.Int("text_length", len(resultText)))
				a.sendUpdate(AgentEvent{
					Type:        EventTypeMessageChunk,
					SessionID:   sessionID,
					OperationID: operationID,
					Text:        resultText,
				})
			}
		} else {
			a.logger.Debug("skipping result text (streaming text already sent this turn)")
		}

		// Reset text dedup flag and accumulator
		a.mu.Lock()
		a.streamingTextSentThisTurn = false
		a.textAccumulator.Reset()
		a.mu.Unlock()

		// Use GetCostUSD() to handle both cost_usd and total_cost_usd field names
		completeData := map[string]any{
			"cost_usd":      msg.GetCostUSD(),
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
			Type:        EventTypeComplete,
			SessionID:   sessionID,
			OperationID: operationID,
			Data:        completeData,
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

// IsOneShot returns true — Amp spawns a new process per prompt.
func (a *AmpAdapter) IsOneShot() bool {
	return a.oneShotCfg != nil
}

// Verify interface implementation.
var (
	_ AgentAdapter   = (*AmpAdapter)(nil)
	_ OneShotAdapter = (*AmpAdapter)(nil)
)
