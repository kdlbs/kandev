// Package acp implements the ACP (Agent Communication Protocol) transport adapter.
// ACP uses JSON-RPC 2.0 over stdin/stdout for agent communication.
package acp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
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
	contentTypeText     = "text"
	toolContentType     = "content"

	// configOptionIDModel is the well-known ConfigOption ID/Category value used
	// by ACP agents to surface the active model as a selectable option.
	configOptionIDModel = "model"
)

// wakeupPromptTimeout bounds how long a synthetic wakeup prompt can run.
// Wakeup turns can perform real work (the model often runs a few tool calls
// before stopping) so we mirror what a normal user-initiated prompt would
// allow rather than a tight RPC deadline.
const wakeupPromptTimeout = 30 * time.Minute

// notifQueueCapacity sizes the buffered channel that feeds the update
// worker. 4096 covers any realistic session/load replay burst (the failure
// case that motivated this was ~5-10k notifications spread over several
// seconds, well within the worker's sub-microsecond drain rate). This is
// the internal hand-off between the SDK's update handler and our worker;
// it sits in front of the larger acpNotifQueueDefault SDK inbound queue and
// can be smaller because the worker drains it well under SDK fill rate.
const notifQueueCapacity = 4096

// acpNotifQueueDefault is the per-connection capacity passed to the SDK's
// inbound notification queue. Long session/load replays can emit tens of
// thousands of notifications before the response, so default to the configured
// ceiling instead of requiring operators to know about KANDEV_ACP_NOTIF_QUEUE.
// The SDK still bounds memory to (capacity * avg notification size).
const acpNotifQueueDefault = 131072

// acpNotifQueueMin / acpNotifQueueMax clamp KANDEV_ACP_NOTIF_QUEUE so a
// misconfigured value can't either re-introduce the overflow (too low) or
// blow the heap (too high).
const (
	acpNotifQueueMin = 1024
	acpNotifQueueMax = 131072
)

// acpNotifQueueCapacity returns the per-connection inbound notification queue
// capacity. Managed servers pass the resolved value explicitly; the legacy
// environment fallback remains for directly constructed adapters.
func acpNotifQueueCapacity(configured ...int) int {
	if len(configured) > 0 && configured[0] != 0 {
		return clampACPNotifQueueCapacity(configured[0])
	}
	raw := os.Getenv("KANDEV_ACP_NOTIF_QUEUE")
	if raw == "" {
		return acpNotifQueueDefault
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return acpNotifQueueDefault
	}
	return clampACPNotifQueueCapacity(n)
}

func clampACPNotifQueueCapacity(n int) int {
	if n < acpNotifQueueMin {
		return acpNotifQueueMin
	}
	if n > acpNotifQueueMax {
		return acpNotifQueueMax
	}
	return n
}

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

	// promptQueueing caches the negotiated prompt-queueing advertisement from the
	// initialize response. It is derived once under `mu` rather than re-read from
	// `capabilities` on each prompt because the prompt path reads it concurrently
	// with nothing else holding the write side, and a plain read of the untyped
	// `capabilities.Meta` map would race.
	promptQueueing bool

	// Update channel
	updatesCh chan AgentEvent

	// notifQueue decouples the ACP SDK's notification goroutine from the
	// per-notification processing we do in handleACPUpdate. The SDK's inbound
	// channel is a 1024-slot non-blocking send — if our handler ever stalls
	// (slow disk, GC, downstream backpressure on updatesCh), the SDK closes
	// the connection. By front-loading a buffered channel + dedicated worker,
	// the SDK only ever sees nanosecond-scale enqueues. workerWg lets Close
	// wait for in-flight notifications to drain before tearing down updatesCh.
	//
	// Items are notifWork so the queue can also carry barrier-sync requests
	// from syncNotifQueue — the worker closes the barrier's channel in FIFO
	// order, which lets sendPrompt wait for queued text chunks before it
	// emits EventTypeComplete.
	//
	// Drained-by-cancel, not by close: Close cancels lifetimeCtx (which the
	// worker selects on) but never closes notifQueue, so any items queued
	// after Close are dropped on the floor rather than re-delivered. Close
	// is terminal — fine for shutdown but worth knowing when reading the loop.
	notifQueue chan notifWork
	workerWg   sync.WaitGroup

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
	// cursorTaskMetaBySession stores Cursor's non-standard `cursor/task`
	// metadata until the matching subagent tool_call appears, keyed by session
	// then wire tool-call ID. An entry is drained either when its tool_call
	// arrives (applyPendingCursorTaskMetaLocked) or when the session is cleared
	// on turn end / reset (clearCursorTaskMetaLocked), so it never outlives the
	// turn that produced it.
	cursorTaskMetaBySession map[string]map[string]cursorTaskMeta
	// toolCallParents preserves nested tool lineage while a handed-off
	// predecessor continues streaming beside its human successor.
	toolCallParents map[string]string
	// handoffProtectedToolCalls identifies predecessor background work that a
	// successor's prompt-end sweep must not terminate.
	handoffProtectedToolCalls map[string]struct{}
	// codexSubagentCorrelations deduplicates the collaboration and activity
	// tool_call frames codex-acp emits for one logical child, keyed by session,
	// wire tool-call ID, and child session ID. Incomplete entries are retained
	// until completion or session cleanup so delayed lifecycle frames cannot
	// split a live card; only completed tombstones are bounded. Emitted-ID
	// reservations are intentionally retained for the session so a pruned
	// tombstone's ID cannot be reused by a later card.
	codexSubagentCorrelations map[codexSubagentCorrelationKey]*codexSubagentCorrelation
	codexEmittedToolCallIDs   map[string]map[string]*codexSubagentCorrelation
	codexSubagentSequence     uint64

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
	promptTraceCtx  context.Context
	promptTraceTurn *promptTurnState
	promptTraceMu   sync.RWMutex

	// Attachment management
	attachMgr *shared.AttachmentManager

	// Available models from the most recent session creation/load.
	// Used by SetModel to validate the requested model exists.
	availableModels []modelInfo

	// usageBySession tracks the latest and previously consumed cumulative
	// `usage_update` samples. codex-acp DOES emit a typed per-turn usage
	// frame on the prompt response, but it is scoped to the LAST model
	// request of the turn, not the whole turn (see
	// normalizeCodexPromptUsage in dialect_codex.go) — so this cache
	// mainly contributes the provider-reported cost delta for adapters
	// like claude-acp / opencode-acp that report a real `result.usage`.
	// For an adapter with no typed usage frame at all, the prompt-complete
	// handler falls back to nonnegative context-occupancy growth as an
	// estimated input count (fallbackUsageForNilTypedUsage in
	// adapter_prompt.go). It attaches when context occupancy grew
	// (delta > 0) OR a provider-reported cost sample is present —
	// either condition alone is enough.
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

	dialect acpDialect

	// Session configuration changes are serialized across model and option
	// RPCs. configGeneration is incremented when a change begins so an older
	// completion cannot overwrite a newer selection.
	configChangeMu   sync.Mutex
	configGeneration uint64
	contextSamples   map[string]contextWindowSample

	// Synchronization
	mu     sync.RWMutex
	closed bool
	// closedCh is closed as soon as shutdown begins so direct sendUpdate
	// callers stop waiting before updatesCh is closed. updateSendWg lets Close
	// wait for those callers before closing the channel.
	closedCh     chan struct{}
	updateSendWg sync.WaitGroup

	// promptTurn tracks the in-flight session/prompt RPC so Cancel can interrupt it
	// and wait for acknowledgment before reporting success.
	promptTurnMu sync.Mutex
	promptTurn   *promptTurnState

	// promptGate is a 1-slot ownership token for session/prompt calls. Calls are
	// serialized by default so ScheduleWakeup cannot race a user prompt and
	// misalign the bridge's prompt responses. A provider-attested human
	// foreground handoff may transfer the existing token directly to one waiting
	// human successor while the old RPC returns; synthetic wakeups never observe
	// that handoff and continue waiting for the owning RPC to finish.
	promptGate chan struct{}

	// asyncTurnFinalizers synthesize a turn completion for ACP updates that
	// arrive while no session/prompt RPC is active. Claude's Monitor tool can
	// produce assistant chunks from a background callback without returning a
	// prompt response, so sendPrompt's normal complete emission never runs.
	asyncTurnMu         sync.Mutex
	asyncTurnFinalizers map[string]*asyncTurnFinalizer
	asyncTurnEpochs     map[string]uint64

	// lifetimeCtx is cancelled by Close. Background work that may outlive
	// the call site (e.g. the synthetic wakeup prompt goroutine) derives its
	// context from this one so it aborts when the adapter shuts down rather
	// than continuing to drive a dead subprocess.
	lifetimeCtx    context.Context
	lifetimeCancel context.CancelFunc
}

// promptTurnState holds synchronization for one in-flight session/prompt RPC.
type promptTurnState struct {
	endTurn           context.CancelCauseFunc
	rpcDone           chan struct{}
	abortCh           chan struct{}
	handoffCh         chan struct{}
	providerErrorCh   chan openCodeStderrDiagnostic
	promptGeneration  uint64
	evidenceMu        sync.Mutex
	codexSystemError  bool
	codexCapacity     bool
	cursorRetriable   bool
	cursorRetriableAt time.Time
	allowHandoff      bool
	handedOff         bool
	gateOwned         bool
	finishing         bool
}

func (t *promptTurnState) observeCodexEvidence(systemError, capacity bool) {
	if t == nil || (!systemError && !capacity) {
		return
	}
	t.evidenceMu.Lock()
	t.codexSystemError = t.codexSystemError || systemError
	t.codexCapacity = t.codexCapacity || capacity
	t.evidenceMu.Unlock()
}

func (t *promptTurnState) codexCapacityFailure() bool {
	if t == nil {
		return false
	}
	t.evidenceMu.Lock()
	defer t.evidenceMu.Unlock()
	return t.codexSystemError && t.codexCapacity
}

func (t *promptTurnState) hasCodexSystemError() bool {
	if t == nil {
		return false
	}
	t.evidenceMu.Lock()
	defer t.evidenceMu.Unlock()
	return t.codexSystemError
}

func (t *promptTurnState) setCursorRetriable() {
	if t == nil {
		return
	}
	t.evidenceMu.Lock()
	if !t.cursorRetriable {
		t.cursorRetriableAt = time.Now().UTC()
	}
	t.cursorRetriable = true
	t.evidenceMu.Unlock()
}

func (t *promptTurnState) clearCursorRetriable() {
	if t == nil {
		return
	}
	t.evidenceMu.Lock()
	t.cursorRetriable = false
	t.cursorRetriableAt = time.Time{}
	t.evidenceMu.Unlock()
}

func (t *promptTurnState) cursorRetriableFailure() bool {
	failure, _ := t.cursorRetriableFailureAt()
	return failure
}

func (t *promptTurnState) cursorRetriableFailureAt() (bool, time.Time) {
	if t == nil {
		return false, time.Time{}
	}
	t.evidenceMu.Lock()
	defer t.evidenceMu.Unlock()
	return t.cursorRetriable, t.cursorRetriableAt
}

type asyncTurnFinalizer struct {
	timer       *time.Timer
	seq         uint64
	promptEpoch uint64
}

// promptCancelJoinTimeout bounds how long Cancel and sendPrompt wait for a stuck
// session/prompt RPC to end after a user cancel. Exposed as a var for tests.
var promptCancelJoinTimeout = 3 * time.Second

// NewAdapter creates a new ACP protocol adapter.
// Call Connect() after starting the subprocess to wire up stdin/stdout.
// cfg.AgentID is required for debug file naming.
func NewAdapter(cfg *shared.Config, log *logger.Logger) *Adapter {
	l := log.WithFields(zap.String("adapter", "acp"), zap.String("agent_id", cfg.AgentID))
	ctx, cancel := context.WithCancel(context.Background())
	a := &Adapter{
		cfg:                       cfg,
		logger:                    l,
		agentID:                   cfg.AgentID,
		normalizer:                NewNormalizer(cfg.AgentID),
		dialect:                   newACPDialect(cfg.AgentID),
		updatesCh:                 make(chan AgentEvent, 100),
		notifQueue:                make(chan notifWork, notifQueueCapacity),
		activeToolCalls:           make(map[string]*streams.NormalizedPayload),
		cursorTaskMetaBySession:   make(map[string]map[string]cursorTaskMeta),
		toolCallParents:           make(map[string]string),
		handoffProtectedToolCalls: make(map[string]struct{}),
		activeMonitors:            make(map[string]map[string]string),
		pendingWakeups:            make(map[string]*pendingWakeup),
		usageBySession:            make(map[string]*usageTracker),
		contextSamples:            make(map[string]contextWindowSample),
		attachMgr:                 shared.NewAttachmentManager(cfg.WorkDir, l.Zap()),
		promptGate:                make(chan struct{}, 1),
		asyncTurnFinalizers:       make(map[string]*asyncTurnFinalizer),
		asyncTurnEpochs:           make(map[string]uint64),
		lifetimeCtx:               ctx,
		lifetimeCancel:            cancel,
		closedCh:                  make(chan struct{}),
	}
	a.wakeup = newWakeupScheduler(l, a.fireWakeup)
	// Start the update worker before returning so any caller that connects
	// the SDK (Initialize, or any future direct wiring) always has a live
	// consumer for notifQueue. Moving this out of Initialize closes a latent
	// footgun where a future caller bypassing Initialize would silently fill
	// the queue and lose notifications.
	a.startUpdateWorker()
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

	// Create ACP client with update handler that enqueues for the worker.
	a.acpClient = acpclient.NewClient(
		acpclient.WithLogger(a.logger.Zap()),
		acpclient.WithWorkspaceRoot(a.cfg.WorkDir),
		acpclient.WithUpdateHandler(a.enqueueACPUpdate),
		acpclient.WithPermissionHandler(a.handlePermissionRequest),
		acpclient.WithCursorTaskHandler(a.handleCursorTask),
	)

	// Create ACP SDK connection. Raise the inbound notification queue cap
	// to acpNotifQueueDefault (well above the SDK's built-in default) so
	// long session/load replays don't overflow and tear the connection
	// down. The internal notifQueueCapacity channel sits in front of this
	// queue and is drained by our update worker. Requires a coder/acp-go-sdk
	// fork with WithMaxQueuedNotifications; see go.mod replace directive.
	notifQueueCap := acpNotifQueueCapacity(a.cfg.NotificationQueueCapacity)
	a.acpConn = acp.NewClientSideConnection(a.acpClient, a.stdin, a.stdout,
		acp.WithMaxQueuedNotifications(notifQueueCap))
	a.acpConn.SetLogger(slog.Default().With("component", "acp-conn"))
	a.logger.Debug("ACP connection notification queue sized",
		zap.Int("capacity", notifQueueCap))

	// Perform ACP handshake - this exchanges capabilities with the agent
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "initialize")
	defer span.End()

	resp, err := a.acpConn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: clientCapabilitiesForAgent(a.agentID),
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
	promptQueueing := agentAdvertisesPromptQueueing(resp.AgentCapabilities)

	span.SetAttributes(
		attribute.String("agent_name", a.agentInfo.Name),
		attribute.String("agent_version", a.agentInfo.Version),
		attribute.Bool("supports_load_session", a.capabilities.LoadSession),
		attribute.Bool("supports_prompt_queueing", promptQueueing),
	)

	a.logger.Info("ACP adapter initialized",
		zap.String("agent_name", a.agentInfo.Name),
		zap.String("agent_version", a.agentInfo.Version),
		zap.Bool("supports_load_session", a.capabilities.LoadSession),
		zap.Bool("supports_prompt_queueing", promptQueueing))

	// Cache auth methods so we can re-emit them on auth_required without re-running initialize.
	authMethods := convertAuthMethods(resp.AuthMethods)
	a.mu.Lock()
	a.availableAuthMethods = authMethods
	a.promptQueueing = promptQueueing
	a.mu.Unlock()

	// Emit agent capabilities event with prompt capabilities and auth methods
	a.sendUpdate(AgentEvent{
		Type:                    streams.EventTypeAgentCapabilities,
		SupportsImage:           a.capabilities.PromptCapabilities.Image,
		SupportsAudio:           a.capabilities.PromptCapabilities.Audio,
		SupportsEmbeddedContext: a.capabilities.PromptCapabilities.EmbeddedContext,
		SupportsPromptQueueing:  promptQueueing,
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

// GetSessionModelState returns the latest session model snapshot while holding
// the adapter lock. The snapshot is used in the synchronous session response;
// the normal session_models stream event remains the long-lived cache update.
func (a *Adapter) GetSessionModelState() *streams.SessionModelState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.availableModels) == 0 && len(a.availableConfigOptions) == 0 {
		return nil
	}
	return &streams.SessionModelState{
		CurrentModelID: currentModelFromConfig(a.availableConfigOptions),
		Models:         cloneSessionModels(convertSessionModels(a.availableModels)),
		ConfigOptions:  cloneConfigOptions(a.availableConfigOptions),
	}
}

func cloneSessionModels(models []streams.SessionModelInfo) []streams.SessionModelInfo {
	if len(models) == 0 {
		return nil
	}
	cloned := append([]streams.SessionModelInfo(nil), models...)
	for i, model := range cloned {
		if model.Meta != nil {
			cloned[i].Meta = make(map[string]any, len(model.Meta))
			for key, value := range model.Meta {
				cloned[i].Meta[key] = value
			}
		}
	}
	return cloned
}

func cloneConfigOptions(options []streams.ConfigOption) []streams.ConfigOption {
	if len(options) == 0 {
		return nil
	}
	cloned := append([]streams.ConfigOption(nil), options...)
	for i, option := range cloned {
		cloned[i].Options = append([]streams.ConfigOptionValue(nil), option.Options...)
	}
	return cloned
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
//
// Stream events are lossless at this boundary: when the per-agent consumer is
// slower than the ACP producer, the ACP update worker applies backpressure to
// that agent instead of silently dropping transcript content. The lifetime
// context makes the wait cancelable during shutdown. Do not hold a.mu while
// waiting; Close needs the write lock to cancel the context and unblock us.
func (a *Adapter) sendUpdate(event AgentEvent) {
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return
	}
	updatesCh := a.updatesCh
	lifetimeCtx := a.lifetimeCtx
	if lifetimeCtx == nil {
		lifetimeCtx = context.Background()
	}
	event.NormalizedPayload = event.NormalizedPayload.Snapshot()
	closedCh := a.closedCh
	if closedCh != nil {
		a.updateSendWg.Add(1)
	}
	a.mu.RUnlock()
	if closedCh != nil {
		defer a.updateSendWg.Done()
	}

	select {
	case updatesCh <- event:
	case <-closedCh:
	case <-lifetimeCtx.Done():
		// Shutdown cancels the wait. The event is intentionally discarded after
		// the adapter has become terminal; no later transcript can be applied.
	}
}

// sendUpdateLocked enqueues an event without acquiring a.mu. Callers must
// hold a.mu for reading or writing.
func (a *Adapter) sendUpdateLocked(event AgentEvent) bool {
	if a.closed {
		return false
	}
	event.NormalizedPayload = event.NormalizedPayload.Snapshot()
	select {
	case a.updatesCh <- event:
		return true
	default:
		return false
	}
}

// Close releases resources held by the adapter.
func (a *Adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	if a.closedCh != nil {
		close(a.closedCh)
	}
	a.mu.Unlock()

	a.logger.Info("closing ACP adapter")

	// Stop any pending ScheduleWakeup timer so it doesn't fire after close,
	// and cancel the lifetime context so any in-flight wakeup prompt aborts.
	// Cancelling lifetimeCtx also unblocks enqueueACPUpdate senders and
	// signals the update worker to exit.
	if a.wakeup != nil {
		a.wakeup.cancel()
	}
	a.cancelAllAsyncTurnCompletes()
	if a.lifetimeCancel != nil {
		a.lifetimeCancel()
	}

	// Wait for the update worker to exit before closing updatesCh.
	// handleACPUpdate may call sendUpdate, so updatesCh must remain open
	// until the worker is gone.
	a.workerWg.Wait()
	a.updateSendWg.Wait()
	a.mu.Lock()
	a.clearCodexSubagentCorrelationsLocked("")
	a.clearCursorTaskMetaLocked("")
	a.clearPromptHandoffToolTrackingLocked()
	clear(a.usageBySession)
	a.mu.Unlock()

	// Clean up any saved attachments
	a.attachMgr.Cleanup()

	// Close update channel
	close(a.updatesCh)

	// Note: We don't close stdin or manage the subprocess here.
	// That's handled by process.Manager which owns the subprocess.

	return nil
}

// RequiresProcessKill reports whether the agent's process group must be killed
// on shutdown. Most ACP agents exit cleanly when stdin closes, but some (notably
// opencode acp) keep an HTTP server and MCP child tree alive after EOF —
// those agents set cfg.RequiresProcessKill so the process manager reaps the
// entire group instead of waiting for a graceful exit that never comes.
// See GH issue #1247.
func (a *Adapter) RequiresProcessKill() bool {
	return a.cfg != nil && a.cfg.RequiresProcessKill
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
func (a *Adapter) setPromptTraceCtx(turn *promptTurnState, ctx context.Context) {
	a.promptTraceMu.Lock()
	defer a.promptTraceMu.Unlock()
	a.promptTraceCtx = ctx
	a.promptTraceTurn = turn
}

// clearPromptTraceCtx clears the prompt span context only when turn still owns
// it. A handed-off RPC may return after its successor installed a new trace.
func (a *Adapter) clearPromptTraceCtx(turn *promptTurnState) {
	a.promptTraceMu.Lock()
	defer a.promptTraceMu.Unlock()
	if a.promptTraceTurn == turn {
		a.promptTraceCtx = nil
		a.promptTraceTurn = nil
	}
}

// GetACPConnection returns the underlying ACP connection for advanced usage.
func (a *Adapter) GetACPConnection() *acp.ClientSideConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.acpConn
}
