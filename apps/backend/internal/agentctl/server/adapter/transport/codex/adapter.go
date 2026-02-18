// Package codex implements the Codex transport adapter.
// Codex uses a JSON-RPC 2.0 variant over stdio (omitting the jsonrpc field).
package codex

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
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// Codex decision constants for approval responses.
const (
	decisionAccept        = "accept"
	decisionAcceptSession = "acceptForSession"
	decisionDecline       = "decline"
	decisionCancel        = "cancel"
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

// StderrProvider provides access to recent stderr output for error context.
type StderrProvider interface {
	GetRecentStderr() []string
}

// Adapter implements the transport adapter for agents using the Codex protocol.
// Codex uses a JSON-RPC 2.0 variant over stdio (omitting the jsonrpc field).
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

	// Codex client for JSON-RPC communication
	client *codex.Client

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Session state - Thread maps to Session, Turn maps to Operation
	threadID string // session ID
	turnID   string // operation ID

	// Agent info (populated after Initialize)
	agentInfo *AgentInfo

	// Update channel
	updatesCh chan AgentEvent

	// Permission handler
	permissionHandler PermissionHandler

	// Stderr provider for error context
	stderrProvider StderrProvider

	// Accumulators for streaming content
	messageBuffer          string
	reasoningBuffer        string
	currentReasoningItemID string // track current reasoning item for separator insertion

	// Turn completion signaling
	turnCompleteCh chan turnCompleteResult

	// Synchronization
	mu     sync.RWMutex
	closed bool
}

// turnCompleteResult holds the result of a completed turn
type turnCompleteResult struct {
	success bool
	err     string
}

// NewAdapter creates a new Codex protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &Adapter{
		cfg:        cfg,
		logger:     log.WithFields(zap.String("adapter", "codex"), zap.String("agent_id", cfg.AgentID)),
		agentID:    cfg.AgentID,
		normalizer: NewNormalizer(),
		ctx:        ctx,
		cancel:     cancel,
		updatesCh:  make(chan AgentEvent, 100),
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

	a.logger.Info("closing Codex adapter")

	// Cancel the context to stop the read loop goroutine
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

// GetSessionID returns the current thread ID (session).
func (a *Adapter) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.threadID
}

// GetOperationID returns the current turn ID (operation).
func (a *Adapter) GetOperationID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.turnID
}

// SetPermissionHandler sets the handler for permission requests.
func (a *Adapter) SetPermissionHandler(handler PermissionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionHandler = handler
}

// SetStderrProvider sets the provider for recent stderr output.
// This is used to include stderr in error events when the agent
// reports an error without a detailed message.
func (a *Adapter) SetStderrProvider(provider StderrProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stderrProvider = provider
}

// RequiresProcessKill returns false because Codex agents exit when stdin is closed.
func (a *Adapter) RequiresProcessKill() bool {
	return false
}

// sendUpdate safely sends an event to the updates channel.
func (a *Adapter) sendUpdate(update AgentEvent) {
	shared.LogNormalizedEvent(shared.ProtocolCodex, a.agentID, &update)
	select {
	case a.updatesCh <- update:
	default:
		a.logger.Warn("updates channel full, dropping notification")
	}
}

// handleNotification processes Codex notifications and emits AgentEvents.
func (a *Adapter) handleNotification(method string, params json.RawMessage) {
	// Log raw event for debugging
	shared.LogRawEvent(shared.ProtocolCodex, a.agentID, method, params)

	a.mu.RLock()
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	switch method {
	case codex.NotifyItemAgentMessageDelta:
		a.handleAgentMessageDelta(params, threadID, turnID)
	case codex.NotifyItemReasoningTextDelta:
		a.handleReasoningDelta(params, threadID, turnID, "reasoning text delta")
	case codex.NotifyItemReasoningSummaryDelta:
		a.handleReasoningDelta(params, threadID, turnID, "reasoning summary delta")
	case codex.NotifyTurnCompleted:
		a.handleTurnCompleted(params, threadID)
	case codex.NotifyTurnDiffUpdated:
		a.handleTurnDiffUpdated(params, threadID)
	case codex.NotifyTurnPlanUpdated:
		a.handleTurnPlanUpdated(params, threadID)
	case codex.NotifyError:
		a.handleErrorNotification(params, threadID)
	case codex.NotifyItemStarted:
		a.handleItemStarted(params, threadID, turnID)
	case codex.NotifyItemCompleted:
		a.handleItemCompleted(params, threadID, turnID)
	case codex.NotifyItemCmdExecOutputDelta:
		a.handleCmdExecOutputDelta(params, threadID, turnID)
	case codex.NotifyThreadTokenUsageUpdated:
		a.handleTokenUsageUpdated(params, threadID, turnID)
	case codex.NotifyTokenCount:
		// Legacy token_count notification - ignore as we now use thread/tokenUsage/updated
		a.logger.Debug("ignoring legacy token_count notification")
	case codex.NotifyContextCompacted:
		a.handleContextCompacted(params)
	default:
		// Log unhandled notifications at debug level
		a.logger.Debug("unhandled notification", zap.String("method", method))
	}
}
