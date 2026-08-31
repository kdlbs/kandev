// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/ports"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

// AgentCtlPort is the default agentctl control port.
const AgentCtlPort = ports.AgentCtl

// AgentExecution represents a running agent execution
type AgentExecution struct {
	ID string
	// RunID identifies the Office run that launched this execution. It is
	// retained after runtime environment cleanup so delayed stop events can
	// still be attributed to the correct run.
	RunID             string
	TaskID            string
	SessionID         string
	TaskEnvironmentID string // Env owning this execution; sessions in the same task share one env
	// AgentProfileID is the concrete profile used by the running CLI. The
	// historical name is retained inside lifecycle because profile resolution,
	// MCP, env, and command construction all consume this value.
	AgentProfileID string
	// OfficeAgentProfileID is the stable Office identity. Empty for non-Office
	// launches, where AgentProfileID owns both identity and execution config.
	OfficeAgentProfileID string
	AgentID              string // Agent type ID (e.g., "claude-acp", "codex") — used for fallback auth methods
	ContainerID          string
	ContainerIP          string               // IP address of the container for agentctl communication
	WorkspacePath        string               // Path to the workspace (worktree or repository path)
	WorkspaceSourceRoots []string             // Canonical durable source roots permitted by agentctl file operations
	ACPSessionID         string               // ACP session ID to resume, if available
	AgentCommand         string               // Command to start the agent subprocess
	ContinueCommand      string               // Command for follow-up prompts (one-shot agents)
	AgentArgs            []string             // Structured argv for AgentCommand
	ContinueArgs         []string             // Structured argv for ContinueCommand
	RuntimeName          agentruntime.Runtime // Name of the runtime used (e.g., "docker", "standalone")
	Status               v1.AgentStatus
	StartedAt            time.Time
	FinishedAt           *time.Time
	ExitCode             *int
	ErrorMessage         string
	// FailureCode and FailureDetails carry a bounded, structured startup
	// diagnostic to the orchestrator. They remain separate from the generic
	// error message so user-facing recovery can choose a stable presentation.
	FailureCode    string
	FailureDetails string
	ProviderError  *streams.ProviderError
	// metadata is unexported on purpose: it is touched from the launch, prompt
	// and stop paths concurrently, so all access must go through the metadataMu
	// helpers in execution_metadata.go.
	metadata   map[string]interface{}
	metadataMu sync.RWMutex
	// runtimeEnv is the effective environment used to create the task's
	// runtime instance. It is kept in memory only so authorized task-scoped
	// terminals and passthrough processes can inherit the same credentials and
	// PATH as the agent subprocess without persisting secrets in metadata.
	runtimeEnv       map[string]string
	runtimeEnvMu     sync.RWMutex
	promptGeneration uint64
	// promptCompletionGeneration prevents duplicate terminal events for the
	// same prompt from replacing the first terminal outcome or provider error.
	promptCompletionGeneration uint64
	// dispatchedPromptGeneration is the generation of the prompt that has been
	// accepted by agentctl and is still in flight. It is set only after the
	// ordinary prompt's triggerPrompt succeeds and reset by beginExecutionPrompt.
	// A mid-turn steer reuses this exact generation, so the getter must not hand
	// out a generation that was merely admitted (not yet dispatched — its buffers
	// are still being reset) or already completed.
	dispatchedPromptGeneration uint64
	// promptTurnID is the durable Kandev turn bound to the currently
	// dispatched prompt. It is snapshotted onto terminal AgentEvents before
	// AgentReady is published, so a queued successor cannot overwrite the
	// completion's attribution while its stream frame is in flight.
	promptTurnID      string
	promptLifecycleMu sync.Mutex

	// PrepareResult carries the environment preparation result back to the caller
	// so it can be persisted synchronously before UpdateTaskSession clobbers metadata.
	PrepareResult *EnvPrepareResult `json:"-"`

	// agentctl client for this execution
	agentctl *agentctl.Client
	// agentctlReady records the successful health check independently of the
	// agent process and workspace stream lifecycles. Prepared sessions have a
	// healthy agentctl before either of those is started or attached.
	agentctlReady atomic.Bool

	// Unified workspace stream for shell I/O, git status, and file changes
	workspaceStream   *agentctl.WorkspaceStream
	workspaceStreamMu sync.RWMutex

	// Standalone mode info (when not using Docker)
	standaloneInstanceID string // Instance ID in standalone agentctl
	standalonePort       int    // Port of the standalone execution

	// IsPassthrough captures the session's mode as decided at session-creation
	// time (TaskSession.IsPassthrough snapshot). StartAgentProcess uses this
	// instead of re-resolving the live profile so a profile that toggles
	// CLIPassthrough after the session was created cannot strand existing
	// sessions in the wrong launch path.
	IsPassthrough bool

	// Passthrough mode info (CLI passthrough without ACP)
	// passthroughLifecycleMu serializes process replacement paths. A workflow
	// context reset and a delayed exit auto-restart must not both replace the
	// same PTY, or the execution can point at the wrong process.
	passthroughLifecycleMu sync.Mutex
	PassthroughProcessID   string    // Process ID in the interactive runner (empty if not in passthrough mode)
	PassthroughStartedAt   time.Time // When the current passthrough process was launched; used to detect fast-fail exits and skip auto-restart loops
	// passthroughLaunchUsedResume is true if the current passthrough process was
	// launched via ResumePassthroughSession with the resume flag attached. The
	// fast-fail handler reads this to decide whether to retry once with a fresh
	// command (no resume flag) — covers the "No conversation found to continue"
	// case where the CLI's local conversation history is gone after a backend
	// restart.
	passthroughLaunchUsedResume bool
	// passthroughResumeFailed sticks once a resume launch exits non-zero, so that
	// subsequent ResumePassthroughSession calls (e.g. from EnsurePassthroughExecution
	// when the frontend reconnects its terminal WS) build a fresh command
	// instead of thrashing on the same broken resume flag.
	passthroughResumeFailed bool

	// isResumedSession is true when this execution was created as part of a session resume
	// (e.g., after backend restart). Used by StartAgentProcess to route passthrough sessions
	// to ResumePassthroughSession instead of startPassthroughSession.
	isResumedSession bool

	// Buffers for accumulating agent response during a prompt
	messageBuffer  strings.Builder
	thinkingBuffer strings.Builder
	messageMu      sync.Mutex
	streamMu       sync.Mutex
	stream         *streamCoalescer

	// Legacy streaming message tracking for agents that omit protocol message IDs.
	// These are set when we create a streaming message and cleared on tool_call/complete.
	currentMessageID  string
	currentThinkingID string

	// Protocol message correlation maps source-protocol IDs to Kandev message
	// record IDs. Assistant and thought IDs use separate namespaces so an agent
	// reusing the same source ID cannot merge visible and reasoning content.
	protocolMessageIDs  map[string]string
	protocolThinkingIDs map[string]string
	// assistantHistoryBuffer accumulates assistant chunks in wire order for
	// history-context injection. Tool and completion boundaries persist it as
	// one segment before recording the boundary event.
	assistantHistoryBuffer strings.Builder

	// History-based context injection for agents without native session resume (e.g. Auggie).
	// historyEnabled gates recording and injection; it is enabled by either
	// HistoryContextInjection or NewSessionOnWorkspaceRebind.
	// needsResumeContext is set to true when the session has history that should be injected.
	// resumeContextInjected is set to true after context has been injected into a prompt.
	historyEnabled        bool
	needsResumeContext    bool
	resumeContextInjected bool

	// sessionInitialized is set to true after InitializeAndPrompt completes successfully.
	// Used to distinguish launch-phase failures from normal prompt failures.
	sessionInitialized   bool
	sessionInitializedMu sync.RWMutex

	// Available commands from the agent (for slash command menu)
	availableCommands   []streams.AvailableCommand
	availableCommandsMu sync.RWMutex

	// Cached session mode state (for re-sending on subscribe after page refresh)
	modeState   *CachedModeState
	modeStateMu sync.RWMutex

	// Cached session model state (for re-sending on subscribe after page refresh)
	modelState                   *CachedModelState
	providerDefaultModelState    *CachedModelState
	authoritativeConfigResponses map[string]*CachedModelState
	pendingConfigSettlement      *configSettlement
	modelStateMu                 sync.RWMutex

	// Cached auth methods from agent_capabilities (for error recovery metadata)
	authMethods   []streams.AuthMethodInfo
	authMethodsMu sync.RWMutex

	// Channel signaled by handleAgentEvent(complete) or stream disconnect to unblock SendPrompt.
	// Buffered (size 1) so the sender never blocks.
	promptDoneCh chan PromptCompletionSignal
	// promptMu keeps exactly one SendPrompt completion waiter and one set of
	// response buffers active for an execution. Agentctl accepts prompt requests
	// asynchronously, so its transport-level gate alone cannot provide this.
	promptMu                sync.Mutex
	dispatchedPromptPending atomic.Bool

	// Closed when the current SendPrompt returns, so CancelAgent can wait
	// for the in-flight prompt to finish before the caller retries.
	promptFinished   chan struct{}
	promptFinishedMu sync.Mutex

	// Last time an agent event was received (for stall detection)
	lastActivityAt time.Time
	// agentEventSincePrompt is armed (false) on each prompt dispatch and set
	// true by the first genuine agent event (recordActivity/handleCompleteEvent)
	// that follows. It lets the stall watchdog distinguish "the agent never
	// produced a single frame for this prompt" from "it worked, then paused" —
	// both cases otherwise bump the same lastActivityAt timestamp.
	agentEventSincePrompt bool
	// promptActivityEpoch changes when a prompt is armed or a genuine agent
	// event arrives. Stall consumers use it to reject a snapshot that became
	// stale while the event was crossing the bus.
	promptActivityEpoch uint64
	lastActivityAtMu    sync.Mutex
	activeTool          *activeTopLevelTool
	activeToolMu        sync.RWMutex

	// Fires once on the first agent event to publish AgentRunning.
	firstActivityOnce sync.Once

	// Session-level trace span for grouping all operations under one trace
	sessionSpan   trace.Span
	sessionSpanMu sync.RWMutex

	// Startup attempts are generation-bearing so callbacks from the first
	// process cannot fail a replacement process after npm recovery starts.
	// This state has its own mutex because stream setup can run while
	// promptLifecycleMu is held by workspace rebind waiting for readiness.
	startupAttemptGeneration uint64
	startupRecoveryStarted   bool
	startupLifecycleMu       sync.Mutex
}

func (e *AgentExecution) isSessionInitialized() bool {
	e.sessionInitializedMu.RLock()
	defer e.sessionInitializedMu.RUnlock()
	return e.sessionInitialized
}

func (e *AgentExecution) setSessionInitialized(value bool) {
	e.sessionInitializedMu.Lock()
	e.sessionInitialized = value
	e.sessionInitializedMu.Unlock()
}

type activeTopLevelTool struct {
	ToolCallID string
	Name       string
	Title      string
	Status     string
}

func (e *AgentExecution) setActiveTool(tool activeTopLevelTool) {
	e.activeToolMu.Lock()
	e.activeTool = &tool
	e.activeToolMu.Unlock()
}

func (e *AgentExecution) clearActiveTool(toolCallID string) {
	e.activeToolMu.Lock()
	if e.activeTool != nil && e.activeTool.ToolCallID == toolCallID {
		e.activeTool = nil
	}
	e.activeToolMu.Unlock()
}

func (e *AgentExecution) resetActiveTool() {
	e.activeToolMu.Lock()
	e.activeTool = nil
	e.activeToolMu.Unlock()
}

func (e *AgentExecution) activeToolSnapshot() *activeTopLevelTool {
	e.activeToolMu.RLock()
	defer e.activeToolMu.RUnlock()
	if e.activeTool == nil {
		return nil
	}
	tool := *e.activeTool
	return &tool
}

// setRuntimeEnvironment stores a defensive copy of the effective task runtime
// environment. Callers must use RuntimeEnvironment when reading it.
func (e *AgentExecution) setRuntimeEnvironment(env map[string]string) {
	if e == nil {
		return
	}
	e.runtimeEnvMu.Lock()
	e.runtimeEnv = cloneStringMap(env)
	e.runtimeEnvMu.Unlock()
}

// RuntimeEnvironment returns a defensive copy of the effective task runtime
// environment. The lifecycle manager only exposes an execution after the
// caller's task/session ownership check has succeeded.
func (e *AgentExecution) RuntimeEnvironment() map[string]string {
	if e == nil {
		return nil
	}
	e.runtimeEnvMu.RLock()
	env := cloneStringMap(e.runtimeEnv)
	e.runtimeEnvMu.RUnlock()
	return env
}

func (e *AgentExecution) clearRuntimeEnvironment() {
	if e == nil {
		return
	}
	e.runtimeEnvMu.Lock()
	e.runtimeEnv = nil
	e.runtimeEnvMu.Unlock()
}

func (e *AgentExecution) officeProfileID() string {
	if e == nil {
		return ""
	}
	if e.OfficeAgentProfileID != "" {
		return e.OfficeAgentProfileID
	}
	return e.AgentProfileID
}

// PromptCompletionSignal carries the result from a complete event or disconnect.
type PromptCompletionSignal struct {
	StopReason        string
	IsError           bool
	Error             string
	PromptGeneration  uint64
	StartupGeneration uint64
}

func (e *AgentExecution) promptGenerationSnapshot() uint64 {
	e.promptLifecycleMu.Lock()
	defer e.promptLifecycleMu.Unlock()
	return e.promptGeneration
}

func (e *AgentExecution) armPromptActivity() {
	e.lastActivityAtMu.Lock()
	e.lastActivityAt = time.Now()
	e.agentEventSincePrompt = false
	e.promptActivityEpoch++
	e.lastActivityAtMu.Unlock()
}

func (e *AgentExecution) markAgentActivity() {
	e.lastActivityAtMu.Lock()
	e.lastActivityAt = time.Now()
	e.agentEventSincePrompt = true
	e.promptActivityEpoch++
	e.lastActivityAtMu.Unlock()
}

func (e *AgentExecution) promptActivitySnapshot() (time.Time, bool, uint64) {
	e.lastActivityAtMu.Lock()
	defer e.lastActivityAtMu.Unlock()
	return e.lastActivityAt, e.agentEventSincePrompt, e.promptActivityEpoch
}

func (e *AgentExecution) promptActivityEpochSnapshot() uint64 {
	e.lastActivityAtMu.Lock()
	defer e.lastActivityAtMu.Unlock()
	return e.promptActivityEpoch
}

// beginStartupAttempt starts a generation for a new ACP process. Generation
// zero is reserved for executions that predate startup tracking.
func (e *AgentExecution) beginStartupAttempt() uint64 {
	e.startupLifecycleMu.Lock()
	defer e.startupLifecycleMu.Unlock()
	e.startupAttemptGeneration++
	e.startupRecoveryStarted = false
	return e.startupAttemptGeneration
}

// beginStartupRecovery advances the startup generation exactly once. The
// caller uses the returned generation when wiring the replacement streams.
func (e *AgentExecution) beginStartupRecovery() (uint64, bool) {
	e.startupLifecycleMu.Lock()
	defer e.startupLifecycleMu.Unlock()
	if e.startupRecoveryStarted {
		return e.startupAttemptGeneration, false
	}
	e.startupRecoveryStarted = true
	e.startupAttemptGeneration++
	return e.startupAttemptGeneration, true
}

func (e *AgentExecution) finishStartupRecovery() {
	e.startupLifecycleMu.Lock()
	e.startupRecoveryStarted = false
	e.startupLifecycleMu.Unlock()
}

func (e *AgentExecution) startupAttemptSnapshot() uint64 {
	e.startupLifecycleMu.Lock()
	defer e.startupLifecycleMu.Unlock()
	return e.startupAttemptGeneration
}

func (e *AgentExecution) acceptsStartupAttempt(generation uint64) bool {
	e.startupLifecycleMu.Lock()
	defer e.startupLifecycleMu.Unlock()
	return e.startupAttemptGeneration == generation
}

// signalPromptCompletionForStartupGeneration claims the current startup
// generation and enqueues its completion signal as one ownership operation.
// A stream can finish its generation check just before recovery advances the
// execution, so checking and sending under the same mutex prevents an old
// stream from publishing into the replacement prompt's channel.
func (e *AgentExecution) signalPromptCompletionForStartupGeneration(
	startupGeneration uint64,
	signal PromptCompletionSignal,
) bool {
	if e == nil {
		return false
	}
	e.startupLifecycleMu.Lock()
	defer e.startupLifecycleMu.Unlock()
	if e.startupAttemptGeneration != startupGeneration {
		return false
	}
	signal.StartupGeneration = startupGeneration
	select {
	case e.promptDoneCh <- signal:
	default:
	}
	return true
}

func (e *AgentExecution) promptTurnIDSnapshot() string {
	if e == nil {
		return ""
	}
	e.promptLifecycleMu.Lock()
	defer e.promptLifecycleMu.Unlock()
	return e.promptTurnID
}

func (e *AgentExecution) setPromptTurnID(turnID string) {
	if e == nil {
		return
	}
	e.promptLifecycleMu.Lock()
	e.promptTurnID = turnID
	e.promptLifecycleMu.Unlock()
}

// GetAgentCtlClient returns the agentctl client for this execution
func (ae *AgentExecution) GetAgentCtlClient() *agentctl.Client {
	return ae.agentctl
}

// MarkAgentctlReady records that agentctl passed its startup health check.
func (ae *AgentExecution) MarkAgentctlReady() {
	ae.agentctlReady.Store(true)
}

// IsAgentctlReady reports whether agentctl passed its startup health check.
func (ae *AgentExecution) IsAgentctlReady() bool {
	return ae.agentctlReady.Load()
}

// AgentctlURL returns the base URL of the agentctl HTTP server for this
// execution. Returns an empty string when no agentctl client is set (e.g.
// before the execution has been wired to an agentctl instance).
func (ae *AgentExecution) AgentctlURL() string {
	if ae.agentctl == nil {
		return ""
	}
	return ae.agentctl.BaseURL()
}

// SetWorkspaceStream sets the unified workspace stream for this execution
func (ae *AgentExecution) SetWorkspaceStream(ws *agentctl.WorkspaceStream) {
	ae.workspaceStreamMu.Lock()
	defer ae.workspaceStreamMu.Unlock()
	ae.workspaceStream = ws
}

// ClearWorkspaceStream clears the workspace stream if it is still the active stream.
func (ae *AgentExecution) ClearWorkspaceStream(ws *agentctl.WorkspaceStream) {
	ae.workspaceStreamMu.Lock()
	defer ae.workspaceStreamMu.Unlock()
	if ae.workspaceStream == ws {
		ae.workspaceStream = nil
	}
}

// GetWorkspaceStream returns the unified workspace stream for this execution
func (ae *AgentExecution) GetWorkspaceStream() *agentctl.WorkspaceStream {
	ae.workspaceStreamMu.RLock()
	defer ae.workspaceStreamMu.RUnlock()
	return ae.workspaceStream
}

// SetAvailableCommands sets the available commands for this execution
func (ae *AgentExecution) SetAvailableCommands(commands []streams.AvailableCommand) {
	ae.availableCommandsMu.Lock()
	defer ae.availableCommandsMu.Unlock()
	ae.availableCommands = commands
}

// GetAvailableCommands returns the available commands for this execution
func (ae *AgentExecution) GetAvailableCommands() []streams.AvailableCommand {
	ae.availableCommandsMu.RLock()
	defer ae.availableCommandsMu.RUnlock()
	return ae.availableCommands
}

// CachedModeState holds the last-known session mode state for re-sending on subscribe.
type CachedModeState struct {
	CurrentModeID  string
	AvailableModes []streams.SessionModeInfo
}

// CachedModelState holds the last-known session model state for re-sending on subscribe.
type CachedModelState struct {
	CurrentModelID string
	Models         []streams.SessionModelInfo
	ConfigOptions  []streams.ConfigOption
	ConfigSource   string
	ConfigID       string
	// ConfigOptionsSettled is true after startup config application has a
	// complete provider snapshot, including snapshots with no options.
	ConfigOptionsSettled bool
}

type configSettlement struct {
	configID        string
	providerDefault *CachedModelState
}

// SetModeState caches the session mode state on this execution.
func (ae *AgentExecution) SetModeState(state *CachedModeState) {
	ae.modeStateMu.Lock()
	defer ae.modeStateMu.Unlock()
	ae.modeState = state
}

// GetModeState returns the cached session mode state.
func (ae *AgentExecution) GetModeState() *CachedModeState {
	ae.modeStateMu.RLock()
	defer ae.modeStateMu.RUnlock()
	return ae.modeState
}

// SetModelState caches the session model state on this execution.
func (ae *AgentExecution) SetModelState(state *CachedModelState) {
	ae.modelStateMu.Lock()
	defer ae.modelStateMu.Unlock()
	ae.modelState = state
	ae.captureProviderDefaultModelState(state)
	ae.cacheAuthoritativeConfigResponse(state)
}

// SettleConfigOptions pairs the initial provider defaults with the live state
// after the final startup RPC. When that response is still in flight, it keeps
// both values until the stream dispatcher receives the matching update.
func (ae *AgentExecution) SettleConfigOptions(
	configID string,
	providerDefault *CachedModelState,
) (*CachedModelState, *CachedModelState, bool) {
	ae.modelStateMu.Lock()
	defer ae.modelStateMu.Unlock()
	providerDefault = cloneCachedModelState(providerDefault)
	if providerDefault == nil {
		providerDefault = cloneCachedModelState(ae.providerDefaultModelState)
	}
	if configID == "" {
		if ae.modelState == nil {
			ae.pendingConfigSettlement = &configSettlement{providerDefault: providerDefault}
			return nil, nil, false
		}
		state := cloneCachedModelState(ae.modelState)
		if providerDefault == nil {
			providerDefault = state
		}
		return providerDefault, state, true
	}
	if response := ae.consumeAuthoritativeConfigResponse(configID); response != nil {
		if providerDefault == nil {
			providerDefault = response
		}
		return providerDefault, cloneCachedModelState(ae.modelState), true
	}
	ae.pendingConfigSettlement = &configSettlement{
		configID: configID, providerDefault: providerDefault,
	}
	return nil, nil, false
}

// SetModelStateApplyingSettlement caches provider state and applies a pending
// startup settlement exactly once when stream delivery lagged initialization.
func (ae *AgentExecution) SetModelStateApplyingSettlement(state *CachedModelState) (*CachedModelState, bool) {
	ae.modelStateMu.Lock()
	defer ae.modelStateMu.Unlock()
	ae.modelState = state
	ae.captureProviderDefaultModelState(state)
	ae.cacheAuthoritativeConfigResponse(state)

	settlement := ae.pendingConfigSettlement
	if settlement == nil {
		return state, false
	}
	if settlement.configID == "" {
		ae.pendingConfigSettlement = nil
		state.ConfigOptionsSettled = true
		if settlement.providerDefault != nil {
			settled := cloneCachedModelState(settlement.providerDefault)
			settled.ConfigOptionsSettled = true
			return settled, true
		}
		return state, true
	}
	response := ae.consumeAuthoritativeConfigResponse(settlement.configID)
	if response == nil {
		return state, false
	}
	ae.pendingConfigSettlement = nil
	state.ConfigOptionsSettled = true
	if settlement.providerDefault != nil {
		settled := cloneCachedModelState(settlement.providerDefault)
		settled.ConfigOptionsSettled = true
		return settled, true
	}
	if ae.providerDefaultModelState != nil {
		settled := cloneCachedModelState(ae.providerDefaultModelState)
		settled.ConfigOptionsSettled = true
		return settled, true
	}
	response.ConfigOptionsSettled = true
	return response, true
}

func (ae *AgentExecution) captureProviderDefaultModelState(state *CachedModelState) {
	if ae.providerDefaultModelState == nil && state != nil && len(state.ConfigOptions) > 0 {
		ae.providerDefaultModelState = cloneCachedModelState(state)
	}
}

func (ae *AgentExecution) cacheAuthoritativeConfigResponse(state *CachedModelState) {
	if state == nil || state.ConfigSource != "provider_response" || state.ConfigID == "" {
		return
	}
	if ae.authoritativeConfigResponses == nil {
		ae.authoritativeConfigResponses = make(map[string]*CachedModelState)
	}
	ae.authoritativeConfigResponses[state.ConfigID] = cloneCachedModelState(state)
}

func (ae *AgentExecution) consumeAuthoritativeConfigResponse(configID string) *CachedModelState {
	response := ae.authoritativeConfigResponses[configID]
	delete(ae.authoritativeConfigResponses, configID)
	return response
}

func cloneCachedModelState(state *CachedModelState) *CachedModelState {
	if state == nil {
		return nil
	}
	cloned := &CachedModelState{
		CurrentModelID:       state.CurrentModelID,
		Models:               append([]streams.SessionModelInfo(nil), state.Models...),
		ConfigOptions:        append([]streams.ConfigOption(nil), state.ConfigOptions...),
		ConfigSource:         state.ConfigSource,
		ConfigID:             state.ConfigID,
		ConfigOptionsSettled: state.ConfigOptionsSettled,
	}
	for i := range cloned.ConfigOptions {
		cloned.ConfigOptions[i].Options = append(
			[]streams.ConfigOptionValue(nil),
			state.ConfigOptions[i].Options...,
		)
	}
	return cloned
}

// GetModelState returns the cached session model state.
func (ae *AgentExecution) GetModelState() *CachedModelState {
	ae.modelStateMu.RLock()
	defer ae.modelStateMu.RUnlock()
	return cloneCachedModelState(ae.modelState)
}

// SetAuthMethods caches the auth methods on this execution.
func (ae *AgentExecution) SetAuthMethods(methods []streams.AuthMethodInfo) {
	ae.authMethodsMu.Lock()
	defer ae.authMethodsMu.Unlock()
	ae.authMethods = methods
}

// GetAuthMethods returns the cached auth methods for this execution.
func (ae *AgentExecution) GetAuthMethods() []streams.AuthMethodInfo {
	ae.authMethodsMu.RLock()
	defer ae.authMethodsMu.RUnlock()
	return ae.authMethods
}

// SetSessionSpan stores the session-level trace span on the execution.
func (ae *AgentExecution) SetSessionSpan(span trace.Span) {
	ae.sessionSpanMu.Lock()
	defer ae.sessionSpanMu.Unlock()
	ae.sessionSpan = span
}

// SessionTraceContext returns a context carrying the session span for creating child spans.
// Uses context.Background() so the span lifetime is independent of request cancellation.
// Returns plain context.Background() when no session span is set (no-op safe).
func (ae *AgentExecution) SessionTraceContext() context.Context {
	ae.sessionSpanMu.RLock()
	defer ae.sessionSpanMu.RUnlock()
	if ae.sessionSpan == nil {
		return context.Background()
	}
	return trace.ContextWithSpan(context.Background(), ae.sessionSpan)
}

// EndSessionSpan ends the session-level trace span if one exists. Idempotent.
func (ae *AgentExecution) EndSessionSpan() {
	ae.sessionSpanMu.Lock()
	defer ae.sessionSpanMu.Unlock()
	if ae.sessionSpan != nil {
		ae.sessionSpan.End()
		ae.sessionSpan = nil
	}
}

// RepoLaunchSpec describes one repository for a multi-repo task launch.
// Mirrors the per-repo launch fields that LaunchRequest historically carried at
// the top level. When LaunchRequest.Repositories is set, each entry produces
// one prepared worktree under the shared TaskDirName.
type RepoLaunchSpec struct {
	TaskRepositoryID       string
	RepositoryID           string
	RepositoryPath         string
	RepositoryURL          string // Clone URL for remote executors that need to clone
	RepoName               string // Repository name used as subdirectory inside TaskDirName
	BaseBranch             string
	DefaultBranch          string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch         string
	PRNumber               int // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	RemoteContribution     *models.RemoteContribution
	WorktreeID             string // Existing worktree ID to reuse (skip creation if set)
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	WorktreeBranchTicket   string
	PullBeforeWorktree     bool
	RemoteSyncHandled      bool
	// RefreshRepository is an optional provider-authenticated refresh deferred
	// until worktree materialization. A valid reusable worktree bypasses it.
	RefreshRepository          func(context.Context) error
	RefreshRepositoryWithState func(context.Context) (repoclone.RemoteRefState, error)
	RemoteRefState             repoclone.RemoteRefState
	RepoSetupScript            string // Repository-level setup script (optional)
	RepoCleanupScript          string // Repository-level cleanup script (optional)
	CopyFiles                  string // Comma-separated paths/globs to copy from the source repo (gitignored .env / config files)
	ContributionDestination    *models.ContributionDestination
	ComparisonTarget           *models.ComparisonTarget
	// BranchSlug, when set, suffixes the worktree directory as
	// {RepoName}-{BranchSlug} so multi-branch tasks (same repo, multiple
	// branches) don't collide.
	BranchSlug string
	// BranchIdentitySlug is the stable branch key used for worktree reuse and
	// persisted environment metadata. It may differ from BranchSlug when a
	// primary branch keeps the flat legacy path.
	BranchIdentitySlug string
}

// WorkspaceFolderSpec is a durable host folder attachment projected into both
// fresh launches and workspace-only resume construction.
type WorkspaceFolderSpec struct {
	Name      string
	LocalPath string
}

// WorkspaceRepositorySpec is the durable host-side source needed to recreate
// a task's owned repository entry after a restart.
type WorkspaceRepositorySpec struct {
	RepositoryID           string
	RepositoryPath         string
	RepoName               string
	BaseBranch             string
	DefaultBranch          string
	CheckoutBranch         string
	ComparisonTarget       *models.ComparisonTarget
	WorktreeID             string
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	PullBeforeWorktree     bool
	RemoteSyncHandled      bool
	BranchSlug             string
	BranchIdentitySlug     string
}

// RouteOverride carries a fully resolved provider profile for one
// routing-driven launch. Empty fields mean "use the base profile value"
// — so when the dispatcher does NOT supply an override, launch behavior
// is byte-identical to today.
type RouteOverride struct {
	ExecutionProfileID string
	ProviderID         string
	Model              string
	Tier               string
	Mode               string
	Flags              []string
	Env                map[string]string
}

// LaunchRequest contains parameters for launching an agent
type LaunchRequest struct {
	TaskID            string
	WorkspaceID       string // Kandev workspace ID — used to build the scratch dir for repo-less tasks
	SessionID         string
	TaskEnvironmentID string // Env this session belongs to (shared across sessions in same task)
	// WorkspaceReuseRequired selects attach-only environment preparation.
	WorkspaceReuseRequired bool
	TaskTitle              string // Human-readable task title for semantic worktree naming
	// AgentProfileID is the stable Office identity for routed Office launches.
	// For non-Office launches it is also the concrete execution profile.
	AgentProfileID string
	// ExecutionProfileID selects the complete CLI runtime profile. Empty keeps
	// backward-compatible behavior by using AgentProfileID.
	ExecutionProfileID string
	StartAgent         bool                // Transfer launch activity through initial startup/prompt
	TurnID             string              // Durable Kandev turn for the initial prompt, when present
	WorkspacePath      string              // Host path to workspace (original repository path)
	TaskDescription    string              // Task description to send via ACP prompt
	Attachments        []MessageAttachment // Attachments (images/files) for the initial prompt
	Env                map[string]string   // Additional env vars
	// ApprovedSecretEnvKeys contains repository binding keys that SSH may
	// forward in addition to its managed credential allowlist.
	ApprovedSecretEnvKeys []string
	// EnvironmentDefinitions preserve source identity until lifecycle has added
	// every managed runtime value and can perform the final strict resolution.
	EnvironmentDefinitions        []runtimeenv.Definition
	EnvironmentResolutionRequired bool
	// EnvironmentFinalized marks the immutable effective snapshot passed to the
	// runtime. Later lifecycle steps must not merge profile or credential values
	// over this snapshot.
	EnvironmentFinalized bool
	ACPSessionID         string // ACP session ID to resume, if available
	Metadata             map[string]interface{}
	ModelOverride        string         // If set, use this model instead of the profile's model
	RouteOverride        *RouteOverride // If set, overrides agent_id/model/mode/etc per provider routing

	// Ephemeral tasks (quick chat) get fallback workspace directories when no repo is configured.
	// Non-ephemeral tasks without a workspace path will not receive a fallback directory.
	IsEphemeral bool

	// IsPassthrough is the session's mode snapshot taken when the session was
	// created (TaskSession.IsPassthrough). When the launch request originates
	// from an existing session, this is the source of truth for the launch
	// path so a profile that toggles CLIPassthrough after the session was
	// created does not strand the session in the wrong mode. Non-session
	// launches (e.g. the low-level controller.LaunchAgent path) leave this
	// false and fall back to live profile resolution.
	IsPassthrough bool

	// Executor configuration - determines which runtime to use
	ExecutorType        string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig      map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID string            // Previous execution ID for runtime reconnect
	McpMode             string            // MCP tool mode: "task" (default), "config", or "office"
	McpProviders        []string          // Normalized provider capabilities attached to the task
	McpProfile          *mcpprofile.Context

	// Environment preparation
	SetupScript string // Setup script to run before agent starts

	// CopyFiles is the per-repository copy_files spec resolved from
	// Repository.CopyFiles by the orchestrator. For worktree executors the
	// worktree.Manager applies it host-side during Create. For remote
	// executors (Docker, Sprites) the launch path ships the bytes via
	// agentctl after CreateInstance. Empty disables the feature.
	CopyFiles string

	// Worktree configuration
	UseWorktree            bool   // Whether to use a Git worktree for isolation
	WorktreeID             string // Existing worktree ID to reuse (skip creation if set)
	RepositoryID           string // Repository ID for worktree tracking
	TaskRepositoryID       string // Exact task_repositories row for worktree recovery
	RepositoryPath         string // Path to the main repository (for worktree creation)
	BaseBranch             string // Base branch for the worktree (e.g., "main")
	DefaultBranch          string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch         string // Branch to fetch and checkout after worktree creation (e.g., PR head branch)
	PRNumber               int    // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	RemoteContribution     *models.RemoteContribution
	ComparisonTarget       *models.ComparisonTarget
	WorktreeBranchPrefix   string // Branch prefix for worktree branches
	WorktreeBranchTemplate string // Branch name template for worktree branches
	WorktreeBranchTicket   string // External ticket value for branch templates
	PullBeforeWorktree     bool   // Whether to pull from remote before creating the worktree
	RemoteSyncHandled      bool   // Authenticated provider refresh already completed
	// RefreshRepository is an optional provider-authenticated refresh deferred
	// until worktree materialization. A valid reusable worktree bypasses it.
	RefreshRepository          func(context.Context) error
	RefreshRepositoryWithState func(context.Context) (repoclone.RemoteRefState, error)
	RemoteRefState             repoclone.RemoteRefState
	ContributionDestination    *models.ContributionDestination

	// Task directory mode: place worktree at ~/.kandev/tasks/{TaskDirName}/{RepoName}/
	TaskDirName string // Semantic task directory name (e.g. "fix-bug_ab12")
	RepoName    string // Repository name used as subdirectory inside the task directory
	BranchSlug  string // Optional branch directory suffix for multi-branch tasks
	// BranchIdentitySlug is the stable branch key used for single-repo reuse.
	// It may be non-empty when BranchSlug is empty to preserve a flat path.
	BranchIdentitySlug string

	// Repositories carries one entry per repository when the launch is multi-repo.
	// When non-empty it is the source of truth; the legacy single-repo top-level
	// fields above are populated from Repositories[0] for callers that have not
	// yet been updated.
	Repositories     []RepoLaunchSpec
	WorkspaceFolders []WorkspaceFolderSpec

	// managedGoCachePath is resolved once before local preparation so setup
	// scripts and the runtime instance cannot observe different settings.
	managedGoCachePath string
}

// RepoSpecs returns the per-repo launch specs for this request. When
// Repositories is set it is returned verbatim; otherwise a length-1 list is
// synthesized from the legacy top-level single-repo fields. Returns an empty
// slice for repo-less launches (e.g. quick chat).
func (r *LaunchRequest) RepoSpecs() []RepoLaunchSpec {
	if len(r.Repositories) > 0 {
		return r.Repositories
	}
	if r.RepositoryID == "" && r.RepositoryPath == "" {
		return nil
	}
	return []RepoLaunchSpec{{
		TaskRepositoryID:           r.TaskRepositoryID,
		RepositoryID:               r.RepositoryID,
		RepositoryPath:             r.RepositoryPath,
		RepoName:                   r.RepoName,
		BaseBranch:                 r.BaseBranch,
		DefaultBranch:              r.DefaultBranch,
		CheckoutBranch:             r.CheckoutBranch,
		PRNumber:                   r.PRNumber,
		RemoteContribution:         r.RemoteContribution,
		ComparisonTarget:           r.ComparisonTarget,
		ContributionDestination:    r.ContributionDestination,
		WorktreeID:                 r.WorktreeID,
		WorktreeBranchPrefix:       r.WorktreeBranchPrefix,
		WorktreeBranchTemplate:     r.WorktreeBranchTemplate,
		WorktreeBranchTicket:       r.WorktreeBranchTicket,
		PullBeforeWorktree:         r.PullBeforeWorktree,
		RemoteSyncHandled:          r.RemoteSyncHandled,
		RefreshRepository:          r.RefreshRepository,
		RefreshRepositoryWithState: r.RefreshRepositoryWithState,
		RemoteRefState:             r.RemoteRefState,
		CopyFiles:                  r.CopyFiles,
		BranchSlug:                 r.BranchSlug,
		BranchIdentitySlug:         r.BranchIdentitySlug,
	}}
}

// MessageAttachment represents an image or file attachment for agent prompts.
type MessageAttachment struct {
	AttachmentID string // file-backed descriptor ID; Data is empty for staged files
	Type         string // "image", "audio", or "resource"
	Data         string // base64-encoded data
	MimeType     string // MIME type
	Name         string // optional filename for resource attachments
	SizeBytes    int64  // raw byte size for file-backed descriptors
	DeliveryMode string // "prompt" (native/default) or "path"
}

// AttachmentReader opens an authorized, claimed backend attachment for
// streaming into the active agent execution. Implementations must enforce the
// task/session ownership checks before returning any bytes.
type AttachmentReader interface {
	OpenClaimed(ctx context.Context, id, taskID, sessionID string) (io.ReadCloser, string, string, int64, error)
}

// CredentialsManager interface for credential retrieval
type CredentialsManager interface {
	GetCredentialValue(ctx context.Context, key string) (value string, err error)
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID     string
	ProfileName   string
	AgentID       string
	AgentName     string // e.g., "auggie", "claude", "codex"
	Model         string // applied through ACP model selection at session start
	Mode          string // applied via ACP session/set_mode at session start (empty = use agent default)
	ConfigOptions map[string]string
	// FallbackModel is the optional single ACP model ID to switch to when
	// Model is unavailable. Ignored when AutoFallback is enabled.
	FallbackModel string
	// AutoFallback opts the profile into the legacy automatic-fallback
	// behavior (session-start best-effort).
	AutoFallback        bool
	AllowIndexing       bool // Deprecated: legacy, kept so existing call sites compile; launch path reads CLIFlags.
	CLIPassthrough      bool
	NativeSessionResume bool // Agent supports ACP session/load for resume
	SupportsMCP         bool
	// CLIFlags is the resolved user-configurable list of CLI flags for this
	// profile. Passed verbatim to cliflags.Resolve at launch time.
	CLIFlags []settingsmodels.CLIFlag
	// CommandPrefix is an optional launcher prefix (e.g. "greywall --")
	// shell-tokenised and prepended to the agent command at launch time.
	CommandPrefix string
	// EnvVars are user-configured environment variables for this profile.
	EnvVars []settingsmodels.ProfileEnvVar

	// Deprecated: legacy permission fields, no longer consulted by the launch
	// path. Kept so existing call sites compile during the transition.
	AutoApprove                bool
	DangerouslySkipPermissions bool
}

// ProfileResolver resolves agent profile IDs to profile information
type ProfileResolver interface {
	ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)
}

// BootMessageService creates and updates boot messages displayed in chat during agent startup.
type BootMessageService interface {
	CreateMessage(ctx context.Context, req *BootMessageRequest) (*models.Message, error)
	UpdateMessage(ctx context.Context, message *models.Message) error
}

// BootMessageRequest contains parameters for creating a boot message.
type BootMessageRequest struct {
	TaskSessionID string
	TaskID        string
	Content       string
	AuthorType    string
	Type          string
	Metadata      map[string]interface{}
}

// McpConfigProvider returns MCP configuration for a given agent profile ID.
type McpConfigProvider interface {
	GetConfigByProfileID(ctx context.Context, profileID string) (*mcpconfig.ProfileConfig, error)
}

// WorkspaceInfo contains information about a task's workspace for on-demand execution creation
type WorkspaceInfo struct {
	TaskID                string
	SessionID             string // Task session ID (from task_sessions table)
	TaskEnvironmentID     string // Env this session belongs to (shared across sessions in same task)
	WorkspacePath         string // Path to the workspace/repository
	WorkspaceFolders      []WorkspaceFolderSpec
	WorkspaceRepositories []WorkspaceRepositorySpec
	TaskDirName           string
	WorkspaceID           string
	AgentProfileID        string // Stable Office agent identity (or the execution profile for legacy sessions)
	ExecutionProfileID    string // Concrete CLI profile selected for this execution
	ExecutorProfileID     string // Concrete executor profile selected for this execution
	AgentID               string // Agent type ID (e.g., "auggie", "codex") - required for runtime creation
	ACPSessionID          string // Agent's session ID for conversation resumption (from session metadata)
	// SessionMode is the persisted session permission mode (e.g. "acceptEdits")
	// from session metadata, declared via the set_session_mode workflow action or
	// a user toggle. Applied as a mode override at ACP session init so a fresh
	// launch starts in the declared mode before the first prompt. See issue #1183.
	SessionMode string
	// RuntimeModel/RuntimeConfigOptions are restored ACP session settings built
	// from provider state plus explicit user overrides. They take precedence over
	// profile defaults when resuming or recreating a session.
	RuntimeModel            string
	RuntimeConfigOptions    map[string]string
	RuntimeConfigOptionsSet bool

	// Executor-aware fields for correct runtime selection and remote reconnection
	ExecutorType     string                 // Executor type (e.g., "local_pc", "sprites")
	RuntimeName      agentruntime.Runtime   // Runtime name from ExecutorRunning record
	AgentExecutionID string                 // Previous execution ID (for remote reconnect)
	Metadata         map[string]interface{} // Additional metadata (reconnect flags)
}

// WorkspaceInfoProvider provides workspace information for tasks
type WorkspaceInfoProvider interface {
	// GetWorkspaceInfoForSession returns workspace info for a specific task session
	GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*WorkspaceInfo, error)
	// GetWorkspaceInfoForEnvironment returns workspace info for a task environment.
	GetWorkspaceInfoForEnvironment(ctx context.Context, taskEnvironmentID string) (*WorkspaceInfo, error)
}

// RecoveredExecution contains info about an execution recovered from a runtime.
type RecoveredExecution struct {
	ExecutionID        string
	TaskID             string
	SessionID          string
	ContainerID        string
	AgentProfileID     string
	ExecutionProfileID string
}

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// PromptStopReasonDispatched is the StopReason returned when SendPrompt was
// called in dispatch-only mode and returned after the agent acknowledged the
// prompt instead of waiting for the turn to complete.
const PromptStopReasonDispatched = "dispatched"
