// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

// AgentCtlPort is the default agentctl control port.
const AgentCtlPort = ports.AgentCtl

// AgentExecution represents a running agent execution
type AgentExecution struct {
	ID                string
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
	ContinueCommand      string               // Command for follow-up prompts (one-shot agents like Amp)
	RuntimeName          agentruntime.Runtime // Name of the runtime used (e.g., "docker", "standalone")
	Status               v1.AgentStatus
	StartedAt            time.Time
	FinishedAt           *time.Time
	ExitCode             *int
	ErrorMessage         string
	Metadata             map[string]interface{}
	promptGeneration     uint64
	promptLifecycleMu    sync.Mutex

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
	PassthroughProcessID string    // Process ID in the interactive runner (empty if not in passthrough mode)
	PassthroughStartedAt time.Time // When the current passthrough process was launched; used to detect fast-fail exits and skip auto-restart loops
	// passthroughLaunchUsedResume is true if the current passthrough process was
	// launched via ResumePassthroughSession with the resume flag attached. The
	// fast-fail handler reads this to decide whether to retry once with a fresh
	// command (no resume flag) — covers the "No conversation found to continue"
	// case where the CLI's local conversation history is gone after a backend
	// restart.
	passthroughLaunchUsedResume bool
	// passthroughResumeFailed sticks once a resume launch fast-fails, so that
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

	// Streaming message tracking - IDs of the current in-progress messages being streamed
	// These are set when we create a streaming message and cleared on tool_call/complete
	currentMessageID  string
	currentThinkingID string

	// History-based context injection for agents without native session resume (e.g. Auggie).
	// historyEnabled gates recording and injection; set from SessionConfig.HistoryContextInjection.
	// needsResumeContext is set to true when the session has history that should be injected.
	// resumeContextInjected is set to true after context has been injected into a prompt.
	historyEnabled        bool
	needsResumeContext    bool
	resumeContextInjected bool

	// sessionInitialized is set to true after InitializeAndPrompt completes successfully.
	// Used to distinguish launch-phase failures from normal prompt failures.
	sessionInitialized bool

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

	// Closed when the current SendPrompt returns, so CancelAgent can wait
	// for the in-flight prompt to finish before the caller retries.
	promptFinished   chan struct{}
	promptFinishedMu sync.Mutex

	// Last time an agent event was received (for stall detection)
	lastActivityAt   time.Time
	lastActivityAtMu sync.Mutex

	// Fires once on the first agent event to publish AgentRunning.
	firstActivityOnce sync.Once

	// Session-level trace span for grouping all operations under one trace
	sessionSpan   trace.Span
	sessionSpanMu sync.RWMutex
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
	StopReason string
	IsError    bool
	Error      string
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
		if settlement.providerDefault != nil {
			return cloneCachedModelState(settlement.providerDefault), true
		}
		return state, true
	}
	response := ae.consumeAuthoritativeConfigResponse(settlement.configID)
	if response == nil {
		return state, false
	}
	ae.pendingConfigSettlement = nil
	if settlement.providerDefault != nil {
		return cloneCachedModelState(settlement.providerDefault), true
	}
	if ae.providerDefaultModelState != nil {
		return cloneCachedModelState(ae.providerDefaultModelState), true
	}
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
		CurrentModelID: state.CurrentModelID,
		Models:         append([]streams.SessionModelInfo(nil), state.Models...),
		ConfigOptions:  append([]streams.ConfigOption(nil), state.ConfigOptions...),
		ConfigSource:   state.ConfigSource,
		ConfigID:       state.ConfigID,
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
	RepositoryID           string
	RepositoryPath         string
	RepositoryURL          string // Clone URL for remote executors that need to clone
	RepoName               string // Repository name used as subdirectory inside TaskDirName
	BaseBranch             string
	DefaultBranch          string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch         string
	PRNumber               int    // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	WorktreeID             string // Existing worktree ID to reuse (skip creation if set)
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	WorktreeBranchTicket   string
	PullBeforeWorktree     bool
	RepoSetupScript        string // Repository-level setup script (optional)
	RepoCleanupScript      string // Repository-level cleanup script (optional)
	CopyFiles              string // Comma-separated paths/globs to copy from the source repo (gitignored .env / config files)
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
	WorktreeID             string
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	PullBeforeWorktree     bool
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
	TaskTitle         string // Human-readable task title for semantic worktree naming
	// AgentProfileID is the stable Office identity for routed Office launches.
	// For non-Office launches it is also the concrete execution profile.
	AgentProfileID string
	// ExecutionProfileID selects the complete CLI runtime profile. Empty keeps
	// backward-compatible behavior by using AgentProfileID.
	ExecutionProfileID string
	StartAgent         bool                // Transfer launch activity through initial startup/prompt
	WorkspacePath      string              // Host path to workspace (original repository path)
	TaskDescription    string              // Task description to send via ACP prompt
	Attachments        []MessageAttachment // Attachments (images/files) for the initial prompt
	Env                map[string]string   // Additional env vars
	ACPSessionID       string              // ACP session ID to resume, if available
	Metadata           map[string]interface{}
	ModelOverride      string         // If set, use this model instead of the profile's model
	RouteOverride      *RouteOverride // If set, overrides agent_id/model/mode/etc per provider routing

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
	RepositoryPath         string // Path to the main repository (for worktree creation)
	BaseBranch             string // Base branch for the worktree (e.g., "main")
	DefaultBranch          string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch         string // Branch to fetch and checkout after worktree creation (e.g., PR head branch)
	PRNumber               int    // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	WorktreeBranchPrefix   string // Branch prefix for worktree branches
	WorktreeBranchTemplate string // Branch name template for worktree branches
	WorktreeBranchTicket   string // External ticket value for branch templates
	PullBeforeWorktree     bool   // Whether to pull from remote before creating the worktree

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
		RepositoryID:           r.RepositoryID,
		RepositoryPath:         r.RepositoryPath,
		RepoName:               r.RepoName,
		BaseBranch:             r.BaseBranch,
		DefaultBranch:          r.DefaultBranch,
		CheckoutBranch:         r.CheckoutBranch,
		PRNumber:               r.PRNumber,
		WorktreeID:             r.WorktreeID,
		WorktreeBranchPrefix:   r.WorktreeBranchPrefix,
		WorktreeBranchTemplate: r.WorktreeBranchTemplate,
		WorktreeBranchTicket:   r.WorktreeBranchTicket,
		PullBeforeWorktree:     r.PullBeforeWorktree,
		CopyFiles:              r.CopyFiles,
		BranchSlug:             r.BranchSlug,
		BranchIdentitySlug:     r.BranchIdentitySlug,
	}}
}

// MessageAttachment represents an image or file attachment for agent prompts.
type MessageAttachment struct {
	Type         string // "image", "audio", or "resource"
	Data         string // base64-encoded data
	MimeType     string // MIME type
	Name         string // optional filename for resource attachments
	DeliveryMode string // "prompt" (native/default) or "path"
}

// CredentialsManager interface for credential retrieval
type CredentialsManager interface {
	GetCredentialValue(ctx context.Context, key string) (value string, err error)
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID           string
	ProfileName         string
	AgentID             string
	AgentName           string // e.g., "auggie", "claude", "codex"
	Model               string // applied through ACP model selection at session start
	Mode                string // applied via ACP session/set_mode at session start (empty = use agent default)
	ConfigOptions       map[string]string
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
