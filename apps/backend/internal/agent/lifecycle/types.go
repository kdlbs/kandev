// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

// AgentCtlPort is the default agentctl control port.
const AgentCtlPort = ports.AgentCtl

// AgentExecution represents a running agent execution
type AgentExecution struct {
	ID              string
	TaskID          string
	SessionID       string
	AgentProfileID  string
	AgentID         string // Agent type ID (e.g., "claude-acp", "codex") — used for fallback auth methods
	ContainerID     string
	ContainerIP     string // IP address of the container for agentctl communication
	WorkspacePath   string // Path to the workspace (worktree or repository path)
	ACPSessionID    string // ACP session ID to resume, if available
	AgentCommand    string // Command to start the agent subprocess
	ContinueCommand string // Command for follow-up prompts (one-shot agents like Amp)
	RuntimeName     string // Name of the runtime used (e.g., "docker", "standalone")
	Status          v1.AgentStatus
	StartedAt       time.Time
	FinishedAt      *time.Time
	ExitCode        *int
	ErrorMessage    string
	Metadata        map[string]interface{}

	// PrepareResult carries the environment preparation result back to the caller
	// so it can be persisted synchronously before UpdateTaskSession clobbers metadata.
	PrepareResult *EnvPrepareResult `json:"-"`

	// agentctl client for this execution
	agentctl *agentctl.Client

	// Unified workspace stream for shell I/O, git status, and file changes
	workspaceStream   *agentctl.WorkspaceStream
	workspaceStreamMu sync.RWMutex

	// Standalone mode info (when not using Docker)
	standaloneInstanceID string // Instance ID in standalone agentctl
	standalonePort       int    // Port of the standalone execution

	// Passthrough mode info (CLI passthrough without ACP)
	PassthroughProcessID string    // Process ID in the interactive runner (empty if not in passthrough mode)
	PassthroughStartedAt time.Time // When the current passthrough process was launched; used to detect fast-fail exits and skip auto-restart loops

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
	modelState   *CachedModelState
	modelStateMu sync.RWMutex

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

// SetWorkspaceStream sets the unified workspace stream for this execution
func (ae *AgentExecution) SetWorkspaceStream(ws *agentctl.WorkspaceStream) {
	ae.workspaceStreamMu.Lock()
	defer ae.workspaceStreamMu.Unlock()
	ae.workspaceStream = ws
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
}

// GetModelState returns the cached session model state.
func (ae *AgentExecution) GetModelState() *CachedModelState {
	ae.modelStateMu.RLock()
	defer ae.modelStateMu.RUnlock()
	return ae.modelState
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

// LaunchRequest contains parameters for launching an agent
type LaunchRequest struct {
	TaskID          string
	SessionID       string
	TaskTitle       string // Human-readable task title for semantic worktree naming
	AgentProfileID  string
	WorkspacePath   string              // Host path to workspace (original repository path)
	TaskDescription string              // Task description to send via ACP prompt
	Attachments     []MessageAttachment // Attachments (images/files) for the initial prompt
	Env             map[string]string   // Additional env vars
	ACPSessionID    string              // ACP session ID to resume, if available
	Metadata        map[string]interface{}
	ModelOverride   string // If set, use this model instead of the profile's model

	// Ephemeral tasks (quick chat) get fallback workspace directories when no repo is configured.
	// Non-ephemeral tasks without a workspace path will not receive a fallback directory.
	IsEphemeral bool

	// Executor configuration - determines which runtime to use
	ExecutorType        string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig      map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID string            // Previous execution ID for runtime reconnect
	McpMode             string            // MCP tool mode: "config" activates config-mode tools

	// Environment preparation
	SetupScript string // Setup script to run before agent starts

	// Worktree configuration
	UseWorktree          bool   // Whether to use a Git worktree for isolation
	WorktreeID           string // Existing worktree ID to reuse (skip creation if set)
	RepositoryID         string // Repository ID for worktree tracking
	RepositoryPath       string // Path to the main repository (for worktree creation)
	BaseBranch           string // Base branch for the worktree (e.g., "main")
	CheckoutBranch       string // Branch to fetch and checkout after worktree creation (e.g., PR head branch)
	WorktreeBranchPrefix string // Branch prefix for worktree branches
	PullBeforeWorktree   bool   // Whether to pull from remote before creating the worktree

	// Task directory mode: place worktree at ~/.kandev/tasks/{TaskDirName}/{RepoName}/
	TaskDirName string // Semantic task directory name (e.g. "fix-bug_ab12")
	RepoName    string // Repository name used as subdirectory inside the task directory
}

// MessageAttachment represents an image or file attachment for agent prompts.
type MessageAttachment struct {
	Type     string // "image", "audio", or "resource"
	Data     string // base64-encoded data
	MimeType string // MIME type
	Name     string // optional filename for resource attachments
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
	Model               string // applied via ACP session/set_model at session start
	Mode                string // applied via ACP session/set_mode at session start (empty = use agent default)
	AllowIndexing       bool   // Deprecated: legacy, kept so existing call sites compile; launch path reads CLIFlags.
	CLIPassthrough      bool
	NativeSessionResume bool // Agent supports ACP session/load for resume
	SupportsMCP         bool
	// CLIFlags is the resolved user-configurable list of CLI flags for this
	// profile. Passed verbatim to cliflags.Resolve at launch time.
	CLIFlags []settingsmodels.CLIFlag

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
	TaskID         string
	SessionID      string // Task session ID (from task_sessions table)
	WorkspacePath  string // Path to the workspace/repository
	AgentProfileID string // Optional - agent profile for the task
	AgentID        string // Agent type ID (e.g., "auggie", "codex") - required for runtime creation
	ACPSessionID   string // Agent's session ID for conversation resumption (from session metadata)

	// Executor-aware fields for correct runtime selection and remote reconnection
	ExecutorType     string                 // Executor type (e.g., "local_pc", "sprites")
	RuntimeName      string                 // Runtime name from ExecutorRunning record
	AgentExecutionID string                 // Previous execution ID (for remote reconnect)
	Metadata         map[string]interface{} // Additional metadata (reconnect flags)
}

// WorkspaceInfoProvider provides workspace information for tasks
type WorkspaceInfoProvider interface {
	// GetWorkspaceInfoForSession returns workspace info for a specific task session
	GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*WorkspaceInfo, error)
}

// RecoveredExecution contains info about an execution recovered from a runtime.
type RecoveredExecution struct {
	ExecutionID    string
	TaskID         string
	SessionID      string
	ContainerID    string
	AgentProfileID string
}

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}
