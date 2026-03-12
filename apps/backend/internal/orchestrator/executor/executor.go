// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// executorStore is the minimal repository interface required by the Executor.
type executorStore interface {
	// Task
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	// Task↔repo junction
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)
	// Session
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	UpdateTaskSessionBaseCommit(ctx context.Context, id string, baseCommitSHA string) error
	SetSessionPrimary(ctx context.Context, sessionID string) error
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	// Session worktree
	CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error
	// Repository entity
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	// Executor
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	// Workspace
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
}

// Common errors
const defaultBaseBranch = "main"

var (
	ErrNoAgentProfileID        = errors.New("task has no agent_profile_id configured")
	ErrExecutionNotFound       = errors.New("execution not found")
	ErrExecutionAlreadyRunning = errors.New("execution already running")
	ErrNoCloneURL              = errors.New("repository has no clone URL: provider owner and name are required")
)

// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// AgentManagerClient is an interface for the Agent Manager service
// This will be implemented via gRPC or HTTP client
type AgentManagerClient interface {
	// LaunchAgent creates a new agentctl instance for a task (agent not started yet)
	LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error)

	// StartAgentProcess starts the agent subprocess for an execution.
	// The command is built internally based on the execution's agent profile.
	StartAgentProcess(ctx context.Context, agentExecutionID string) error

	// StopAgent stops a running agent
	StopAgent(ctx context.Context, agentExecutionID string, force bool) error
	StopAgentWithReason(ctx context.Context, agentExecutionID string, reason string, force bool) error

	// PromptAgent sends a prompt to a running agent
	// Returns PromptResult indicating if the agent needs input
	// Attachments (images) are passed to the agent if provided
	PromptAgent(ctx context.Context, agentExecutionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error)

	// CancelAgent interrupts the current agent turn without terminating the process.
	CancelAgent(ctx context.Context, sessionID string) error

	// RespondToPermission sends a response to a permission request
	RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error

	// IsAgentRunningForSession checks if an agent is actually running for a session
	// This probes the actual agent (Docker container or standalone process) rather than relying on cached state
	IsAgentRunningForSession(ctx context.Context, sessionID string) bool

	// ResolveAgentProfile resolves an agent profile ID to profile information
	ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)

	// SetExecutionDescription updates the task description in an existing execution's metadata.
	// Used when starting an agent on a session whose workspace was already launched.
	SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error

	// RestartAgentProcess stops the agent subprocess and starts a fresh one with a new ACP session,
	// clearing the agent's conversation context. The execution environment (container/agentctl) is preserved.
	RestartAgentProcess(ctx context.Context, agentExecutionID string) error

	// ResetAgentContext resets the agent's conversation context using the fastest available strategy.
	// For ACP agents, this creates a new session without restarting the process.
	// For other agents, this falls back to RestartAgentProcess.
	ResetAgentContext(ctx context.Context, agentExecutionID string) error

	// SetSessionModelBySessionID attempts an in-place model switch via ACP session/set_model.
	// Returns an error if the agent doesn't support in-place model switching.
	SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error

	// WasSessionInitialized reports whether the given execution completed session initialization.
	// Used to distinguish launch-phase failures from normal prompt failures.
	WasSessionInitialized(executionID string) bool

	// GetSessionAuthMethods returns cached auth methods for a session's execution.
	// Used to include auth method hints in error recovery messages.
	GetSessionAuthMethods(sessionID string) []streams.AuthMethodInfo

	// IsPassthroughSession checks if the given session is running in passthrough (PTY) mode.
	IsPassthroughSession(ctx context.Context, sessionID string) bool

	// WritePassthroughStdin writes data to the agent's PTY stdin for passthrough sessions.
	WritePassthroughStdin(ctx context.Context, sessionID string, data string) error

	// MarkPassthroughRunning marks a passthrough execution as running.
	MarkPassthroughRunning(sessionID string) error

	// GetRemoteRuntimeStatusBySession returns remote runtime status metadata for a session
	// (used by UI cloud indicators). Returns nil,nil when unavailable.
	GetRemoteRuntimeStatusBySession(ctx context.Context, sessionID string) (*RemoteRuntimeStatus, error)

	// PollRemoteStatusForRecords performs a one-time remote status poll for the given
	// executor records. Used during startup to populate remote status cache before any
	// sessions are lazily resumed.
	PollRemoteStatusForRecords(ctx context.Context, records []RemoteStatusPollRequest)

	// CleanupStaleExecutionBySessionID removes a stale execution from the in-memory
	// tracking store. Used when the agent process has exited but the execution entry
	// was not cleaned up (e.g. prepared workspace where agent was never started).
	CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error

	// EnsureWorkspaceExecutionForSession ensures an agentctl execution exists for a
	// session so that workspace operations (file tree, terminals, git) are accessible.
	// Used for restoring workspace access on terminal-state sessions.
	EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error

	// GetGitLog retrieves the git log for a session from baseCommit to HEAD.
	// Used for archive snapshot capture. Returns nil, nil if no execution exists.
	GetGitLog(ctx context.Context, sessionID, baseCommit string, limit int) (*client.GitLogResult, error)

	// GetCumulativeDiff retrieves the cumulative diff for a session from baseCommit to HEAD.
	// Used for archive snapshot capture. Returns nil, nil if no execution exists.
	GetCumulativeDiff(ctx context.Context, sessionID, baseCommit string) (*client.CumulativeDiffResult, error)

	// GetGitStatus retrieves the current git status for a session.
	// Returns nil, nil if no execution exists.
	GetGitStatus(ctx context.Context, sessionID string) (*client.GitStatusResult, error)

	// WaitForAgentctlReady waits for the agentctl HTTP server to be ready for a session.
	// This must be called before other agentctl operations (git status, shell, etc.).
	WaitForAgentctlReady(ctx context.Context, sessionID string) error
}

// RemoteRuntimeStatus mirrors runtime status details needed by orchestrator/UI.
type RemoteRuntimeStatus struct {
	RuntimeName   string
	RemoteName    string
	State         string
	CreatedAt     *time.Time
	LastCheckedAt time.Time
	ErrorMessage  string
}

// RemoteStatusPollRequest contains the fields from ExecutorRunning needed for remote status polling.
type RemoteStatusPollRequest struct {
	SessionID        string
	Runtime          string
	AgentExecutionID string
	ContainerID      string
	Metadata         map[string]interface{}
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID                  string
	ProfileName                string
	AgentID                    string
	AgentName                  string
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	CLIPassthrough             bool
	NativeSessionResume        bool // Agent supports ACP session/load for resume
	SupportsMCP                bool
}

// LaunchAgentRequest contains parameters for launching an agent
type LaunchAgentRequest struct {
	TaskID              string
	SessionID           string
	TaskTitle           string // Human-readable task title for semantic worktree naming
	AgentProfileID      string
	RepositoryURL       string
	Branch              string
	TaskDescription     string // Task description to send via ACP prompt
	Priority            int
	Metadata            map[string]interface{}
	Env                 map[string]string
	ACPSessionID        string            // ACP session ID to resume, if available
	ModelOverride       string            // If set, use this model instead of the profile's model
	ExecutorType        string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig      map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID string            // Previous execution ID for runtime reconnect

	// Setup script from executor profile (runs in execution environment before agent starts)
	SetupScript string

	// Worktree configuration for concurrent agent execution
	UseWorktree          bool   // Whether to use a Git worktree for isolation
	RepositoryID         string // Repository ID for worktree tracking
	RepositoryPath       string // Path to the main repository (for worktree creation)
	BaseBranch           string // Base branch for the worktree (e.g., "main")
	CheckoutBranch       string // Branch to fetch and checkout after worktree creation (e.g., PR head branch)
	WorktreeBranchPrefix string // Branch prefix for worktree branches
	PullBeforeWorktree   bool   // Whether to pull from remote before creating the worktree
}

// LaunchOptions contains optional parameters for LaunchPreparedSession.
type LaunchOptions struct {
	AgentProfileID string
	ExecutorID     string
	Prompt         string
	WorkflowStepID string
	StartAgent     bool
}

// LaunchAgentResponse contains the result of launching an agent
type LaunchAgentResponse struct {
	AgentExecutionID string
	ContainerID      string
	Status           v1.AgentStatus
	WorktreeID       string
	WorktreePath     string
	WorktreeBranch   string
	Metadata         map[string]interface{}
}

// TaskExecution tracks an active task execution
type TaskExecution struct {
	TaskID           string
	AgentExecutionID string
	AgentProfileID   string
	StartedAt        time.Time
	SessionState     v1.TaskSessionState
	LastUpdate       time.Time
	// SessionID is the database ID of the agent session
	SessionID string
	// Worktree info for the agent
	WorktreePath   string
	WorktreeBranch string
}

// FromTaskSession converts a models.TaskSession to TaskExecution
func FromTaskSession(s *models.TaskSession) *TaskExecution {
	execution := &TaskExecution{
		TaskID:           s.TaskID,
		AgentExecutionID: s.AgentExecutionID,
		AgentProfileID:   s.AgentProfileID,
		StartedAt:        s.StartedAt,
		SessionState:     agentSessionStateToV1(s.State),
		LastUpdate:       s.UpdatedAt,
		SessionID:        s.ID,
	}
	if len(s.Worktrees) > 0 {
		execution.WorktreePath = s.Worktrees[0].WorktreePath
		execution.WorktreeBranch = s.Worktrees[0].WorktreeBranch
	}
	return execution
}

// agentSessionStateToV1 converts models.TaskSessionState to v1.TaskSessionState
func agentSessionStateToV1(state models.TaskSessionState) v1.TaskSessionState {
	return v1.TaskSessionState(state)
}

// TaskStateChangeFunc is called when the executor needs to update a task's state.
// When set, it replaces direct repo.UpdateTaskState calls so the caller can
// publish events (e.g. WebSocket notifications) alongside the DB update.
type TaskStateChangeFunc func(ctx context.Context, taskID string, state v1.TaskState) error

// SessionStateChangeFunc is called when the executor needs to update a session's state.
// When set, it replaces direct repo.UpdateTaskSessionState calls so the caller can
// publish events (e.g. WebSocket notifications) alongside the DB update.
type SessionStateChangeFunc func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error

// AgentStartFailedFunc is called when the agent process fails to start.
// It receives the task/session/execution IDs and the error. If the callback
// returns true, it has handled the failure (e.g., as a recoverable auth error)
// and the executor should skip its default FAILED state updates.
type AgentStartFailedFunc func(ctx context.Context, taskID, sessionID, agentExecutionID string, err error) (handled bool)

// ExecutorTypeCapabilities provides behavioral queries about executor types.
// Implemented by the lifecycle manager using its backend registry.
type ExecutorTypeCapabilities interface {
	RequiresCloneURL(executorType string) bool
	ShouldApplyPreferredShell(executorType string) bool
}

// Executor manages agent execution for tasks
type Executor struct {
	agentManager AgentManagerClient
	repo         executorStore
	secretStore  secrets.SecretStore
	shellPrefs   ShellPreferenceProvider
	capabilities ExecutorTypeCapabilities
	logger       *logger.Logger

	// Configuration
	retryLimit int
	retryDelay time.Duration

	// Callback for task state changes that need event publishing.
	// Set by the orchestrator to route through the task service layer.
	onTaskStateChange TaskStateChangeFunc

	// Callback for session state changes that need event publishing.
	// Set by the orchestrator to route through updateTaskSessionState which
	// updates the DB and publishes WebSocket events.
	onSessionStateChange SessionStateChangeFunc

	// Callback for agent process start failures. When set, the executor
	// delegates failure handling to this callback, allowing the orchestrator
	// to detect auth errors and treat them as recoverable.
	onAgentStartFailed AgentStartFailedFunc

	// Per-session locks to prevent concurrent resume/launch operations on the same session.
	// This prevents race conditions when the backend restarts and multiple resume requests
	// arrive simultaneously (e.g., from frontend auto-resume).
	sessionLocks sync.Map // map[string]*sync.Mutex

	// Optional cloner for provider-backed repos without a local path.
	repoCloner  RepoCloner
	repoUpdater RepoUpdater
}

// RepoCloner clones remote repositories to local disk.
type RepoCloner interface {
	EnsureCloned(ctx context.Context, cloneURL, owner, name string) (string, error)
	// BuildCloneURL constructs a protocol-aware clone URL for the given provider/owner/name.
	BuildCloneURL(provider, owner, name string) (string, error)
}

// RepoUpdater updates repository records in the database.
type RepoUpdater interface {
	UpdateRepositoryLocalPath(ctx context.Context, repositoryID, localPath string) error
}

// ExecutorConfig holds configuration for the Executor
type ExecutorConfig struct {
	ShellPrefs  ShellPreferenceProvider
	SecretStore secrets.SecretStore
}

type ShellPreferenceProvider interface {
	PreferredShell(ctx context.Context) (string, error)
}

// NewExecutor creates a new executor
func NewExecutor(agentManager AgentManagerClient, repo executorStore, log *logger.Logger, cfg ExecutorConfig) *Executor {
	return &Executor{
		agentManager: agentManager,
		repo:         repo,
		secretStore:  cfg.SecretStore,
		shellPrefs:   cfg.ShellPrefs,
		logger:       log.WithFields(zap.String("component", "executor")),
		retryLimit:   3,
		retryDelay:   5 * time.Second,
	}
}

// SetOnTaskStateChange sets a callback for task state changes.
// This allows the orchestrator to route state changes through the task service layer
// which publishes WebSocket events. Without this, async goroutines would only update
// the database, leaving the frontend out of sync.
func (e *Executor) SetOnTaskStateChange(fn TaskStateChangeFunc) {
	e.onTaskStateChange = fn
}

// SetOnSessionStateChange sets a callback for session state changes.
// This allows the orchestrator to route state changes through updateTaskSessionState
// which updates the DB and publishes WebSocket events to the frontend.
func (e *Executor) SetOnSessionStateChange(fn SessionStateChangeFunc) {
	e.onSessionStateChange = fn
}

// SetRepoCloner sets the cloner used to clone provider-backed repositories on launch.
func (e *Executor) SetRepoCloner(cloner RepoCloner, updater RepoUpdater) {
	e.repoCloner = cloner
	e.repoUpdater = updater
}

// SetOnAgentStartFailed sets a callback for agent process start failures.
// This allows the orchestrator to intercept auth errors and treat them as
// recoverable instead of terminal failures.
func (e *Executor) SetOnAgentStartFailed(fn AgentStartFailedFunc) {
	e.onAgentStartFailed = fn
}

// SetCapabilities sets the executor type capabilities provider.
func (e *Executor) SetCapabilities(c ExecutorTypeCapabilities) {
	e.capabilities = c
}
