// Package streamjson implements the stream-json transport adapter.
// This is the protocol used by Claude Code CLI (--output-format stream-json).
package streamjson

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
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

	// Message UUID tracking for --resume-session-at support
	lastMessageUUID      string // Latest committed message UUID (safe to resume from)
	pendingAssistantUUID string // UUID from assistant message, committed on result

	// lastRawData holds the raw JSON of the current message being processed.
	// Set in handleMessage before dispatch; used by sendUpdate for OTel tracing.
	lastRawData json.RawMessage

	// OTel tracing: active prompt span context.
	// Notification spans become children of the prompt span for visual grouping.
	promptTraceCtx context.Context
	promptTraceMu  sync.RWMutex

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

// RequiresProcessKill returns false because Claude Code agents exit when stdin is closed.
func (a *Adapter) RequiresProcessKill() bool {
	return false
}

// sendUpdate safely sends an event to the updates channel.
func (a *Adapter) sendUpdate(update AgentEvent) {
	shared.LogNormalizedEvent(shared.ProtocolStreamJSON, a.agentID, &update)
	shared.TraceProtocolEvent(a.getPromptTraceCtx(), shared.ProtocolStreamJSON, a.agentID,
		update.Type, a.lastRawData, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping event")
	}
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

// handleMessage processes streaming messages from the agent.
func (a *Adapter) handleMessage(msg *claudecode.CLIMessage) {
	// Marshal once for debug logging and tracing
	a.lastRawData, _ = json.Marshal(msg)
	if len(a.lastRawData) > 0 {
		shared.LogRawEvent(shared.ProtocolStreamJSON, a.agentID, msg.Type, a.lastRawData)
	}

	// Update session ID from any message type.
	// Claude Code can change session_id mid-conversation (e.g. on compact).
	if msg.SessionID != "" {
		a.mu.Lock()
		if a.sessionID != msg.SessionID {
			a.logger.Info("session ID updated",
				zap.String("old", a.sessionID),
				zap.String("new", msg.SessionID),
				zap.String("message_type", msg.Type))
			a.sessionID = msg.SessionID
		}
		a.mu.Unlock()
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

	case claudecode.MessageTypeRateLimit:
		a.handleRateLimitMessage(msg, sessionID, operationID)

	default:
		a.logger.Debug("unhandled message type", zap.String("type", msg.Type))
	}
}
