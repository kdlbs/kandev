// Package acp implements the ACP (Agent Communication Protocol) transport adapter.
// ACP uses JSON-RPC 2.0 over stdin/stdout for agent communication.
package acp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
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

	// configOptionIDModel is the well-known ConfigOption ID/Category value used
	// by ACP agents to surface the active model as a selectable option.
	configOptionIDModel = "model"
)

// wakeupPromptTimeout bounds how long a synthetic wakeup prompt can run.
// Wakeup turns can perform real work (the model often runs a few tool calls
// before stopping) so we mirror what a normal user-initiated prompt would
// allow rather than a tight RPC deadline.
const wakeupPromptTimeout = 30 * time.Minute

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
	// During load, we capture the last Plan so we can re-emit it after load completes.
	// AvailableCommandsUpdate is NOT suppressed — it may arrive after the replay as a
	// "ready" signal, and the last one always wins in the frontend.
	isLoadingSession bool
	loadReplayPlan   *acp.SessionUpdatePlan

	// Tool call tracking for result normalization
	// Maps toolCallId -> NormalizedPayload so we can update with results
	activeToolCalls map[string]*streams.NormalizedPayload

	// Active Monitor tools, keyed by sessionID -> taskID -> toolCallID.
	// Claude-acp's Monitor tool runs a background script that streams events
	// back to the LLM as `<task-notification>` envelopes. We hold this map so
	// later agent_message_chunks carrying those envelopes can be routed back
	// to the originating Monitor's tool_call card. Cleared on prompt completion
	// and rebuilt during session/load replay.
	activeMonitors map[string]map[string]string

	// ScheduleWakeup tracking. The Claude Agent SDK's ScheduleWakeup tool
	// fires its timer inside the SDK's async-iterator, but the upstream
	// @agentclientprotocol/claude-agent-acp bridge only drains that iterator
	// inside its prompt() handler — so a wakeup that fires while no prompt
	// is in flight produces no output. wakeup re-injects the wakeup as a
	// synthetic session/prompt at fire time. pendingWakeups tracks per-tool-
	// call info (prompt + scheduledFor) since these arrive in separate
	// notifications.
	wakeup         *wakeupScheduler
	pendingWakeups map[string]*pendingWakeup

	// OTel tracing: active prompt span context.
	// Notification spans become children of the prompt span for visual grouping.
	promptTraceCtx context.Context
	promptTraceMu  sync.RWMutex

	// Attachment management
	attachMgr *shared.AttachmentManager

	// Available models from the most recent session creation/load.
	// Used by SetModel to validate the requested model exists.
	availableModels []acp.ModelInfo

	// usageDelta tracks the running cumulative `usage_update.used` and
	// the most recent USD cost reported per session. codex-acp emits no
	// per-turn usage frame; the prompt-complete handler consumes the
	// delta here when resp.Usage is empty and flags the row estimated.
	// claude-acp / opencode-acp report a real `result.usage` so this
	// cache is only ever read for codex-acp turns. Reset to 0 once
	// consumed so the next turn starts from a fresh delta.
	usageBySession map[string]*usageTracker

	// Available auth methods captured from the ACP initialize response.
	// Re-emitted via EventTypeAuthRequired when session/new fails with the
	// AuthenticationRequired error code so the frontend can surface a picker
	// without re-running initialize.
	availableAuthMethods []streams.AuthMethodInfo

	// Available modes from the most recent session creation/load.
	// Used by SetMode to include cached modes in the event so the
	// frontend mode selector can render available options.
	availableModes []streams.SessionModeInfo

	// Available config options from the most recent session creation/load.
	// Used by emitSetModelEvent to include cached options in the convergence
	// event emitted after SetModel succeeds so the frontend doesn't lose
	// the options list when the model is changed.
	availableConfigOptions []streams.ConfigOption

	// Synchronization
	mu     sync.RWMutex
	closed bool

	// promptGate is a 1-slot semaphore that serializes session/prompt calls so
	// at most one is in flight against the bridge at a time. The ScheduleWakeup
	// path injects a synthetic prompt via fireWakeup; without this gate it can
	// race a user prompt, and the claude-agent-acp bridge then returns each
	// prompt's stop_reason against the wrong turn — shifting chat turns one
	// prompt behind. A queued synthetic prompt waits here and drains the wakeup
	// turn once the in-flight prompt finishes. It is a channel rather than a
	// sync.Mutex so the wait honours the caller's context (a wakeup whose
	// timeout/lifetime context is cancelled while queued aborts instead of
	// blocking on a stuck turn).
	promptGate chan struct{}

	// lifetimeCtx is cancelled by Close. Background work that may outlive
	// the call site (e.g. the synthetic wakeup prompt goroutine) derives its
	// context from this one so it aborts when the adapter shuts down rather
	// than continuing to drive a dead subprocess.
	lifetimeCtx    context.Context
	lifetimeCancel context.CancelFunc
}

// NewAdapter creates a new ACP protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
// cfg.AgentID is required for debug file naming.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	l := log.WithFields(zap.String("adapter", "acp"), zap.String("agent_id", cfg.AgentID))
	ctx, cancel := context.WithCancel(context.Background())
	a := &Adapter{
		cfg:             cfg,
		logger:          l,
		agentID:         cfg.AgentID,
		normalizer:      NewNormalizer(),
		updatesCh:       make(chan AgentEvent, 100),
		activeToolCalls: make(map[string]*streams.NormalizedPayload),
		activeMonitors:  make(map[string]map[string]string),
		pendingWakeups:  make(map[string]*pendingWakeup),
		usageBySession:  make(map[string]*usageTracker),
		attachMgr:       shared.NewAttachmentManager(cfg.WorkDir, l.Zap()),
		promptGate:      make(chan struct{}, 1),
		lifetimeCtx:     ctx,
		lifetimeCancel:  cancel,
	}
	a.wakeup = newWakeupScheduler(l, a.fireWakeup)
	return a
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

	// Cache auth methods so we can re-emit them on auth_required without re-running initialize.
	authMethods := convertAuthMethods(resp.AuthMethods)
	a.mu.Lock()
	a.availableAuthMethods = authMethods
	a.mu.Unlock()

	// Emit agent capabilities event with prompt capabilities and auth methods
	a.sendUpdate(AgentEvent{
		Type:                    streams.EventTypeAgentCapabilities,
		SupportsImage:           a.capabilities.PromptCapabilities.Image,
		SupportsAudio:           a.capabilities.PromptCapabilities.Audio,
		SupportsEmbeddedContext: a.capabilities.PromptCapabilities.EmbeddedContext,
		AuthMethods:             authMethods,
	})

	return nil
}

// GetAgentInfo returns information about the connected agent.
func (a *Adapter) GetAgentInfo() *AgentInfo {
	return a.agentInfo
}

// SetPendingContext sets the context to be injected into the next prompt.
// This is used by the fork_session pattern for ACP agents that don't support session/load.
// The context will be prepended to the first prompt sent to this session.
func (a *Adapter) SetPendingContext(context string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingContext = context
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

	// Stop any pending ScheduleWakeup timer so it doesn't fire after close,
	// and cancel the lifetime context so any in-flight wakeup prompt aborts.
	if a.wakeup != nil {
		a.wakeup.cancel()
	}
	if a.lifetimeCancel != nil {
		a.lifetimeCancel()
	}

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
