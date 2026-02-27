// Package lifecycle manages agent execution lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.opentelemetry.io/otel/trace"
)

// Default agentctl port
const AgentCtlPort = 9999

// AgentExecution represents a running agent execution
type AgentExecution struct {
	ID              string
	TaskID          string
	SessionID       string
	AgentProfileID  string
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

	// agentctl client for this execution
	agentctl *agentctl.Client

	// Unified workspace stream for shell I/O, git status, and file changes
	workspaceStream   *agentctl.WorkspaceStream
	workspaceStreamMu sync.RWMutex

	// Standalone mode info (when not using Docker)
	standaloneInstanceID string // Instance ID in standalone agentctl
	standalonePort       int    // Port of the standalone execution

	// Passthrough mode info (CLI passthrough without ACP)
	PassthroughProcessID string // Process ID in the interactive runner (empty if not in passthrough mode)

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

	// Available commands from the agent (for slash command menu)
	availableCommands   []streams.AvailableCommand
	availableCommandsMu sync.RWMutex

	// Channel signaled by handleAgentEvent(complete) or stream disconnect to unblock SendPrompt.
	// Buffered (size 1) so the sender never blocks.
	promptDoneCh chan PromptCompletionSignal

	// Last time an agent event was received (for stall detection)
	lastActivityAt   time.Time
	lastActivityAtMu sync.Mutex

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
	WorkspacePath   string            // Host path to workspace (original repository path)
	TaskDescription string            // Task description to send via ACP prompt
	Env             map[string]string // Additional env vars
	ACPSessionID    string            // ACP session ID to resume, if available
	Metadata        map[string]interface{}
	ModelOverride   string // If set, use this model instead of the profile's model

	// Executor configuration - determines which runtime to use
	ExecutorType        string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig      map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID string            // Previous execution ID for runtime reconnect

	// Environment preparation
	SetupScript string // Setup script to run before agent starts

	// Worktree configuration
	UseWorktree          bool   // Whether to use a Git worktree for isolation
	RepositoryID         string // Repository ID for worktree tracking
	RepositoryPath       string // Path to the main repository (for worktree creation)
	BaseBranch           string // Base branch for the worktree (e.g., "main")
	WorktreeBranchPrefix string // Branch prefix for worktree branches
	PullBeforeWorktree   bool   // Whether to pull from remote before creating the worktree
}

// CredentialsManager interface for credential retrieval
type CredentialsManager interface {
	GetCredentialValue(ctx context.Context, key string) (value string, err error)
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID                  string
	ProfileName                string
	AgentID                    string
	AgentName                  string // e.g., "auggie", "claude", "codex"
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	AllowIndexing              bool
	CLIPassthrough             bool
	NativeSessionResume        bool // Agent supports ACP session/load for resume
	SupportsMCP                bool
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
