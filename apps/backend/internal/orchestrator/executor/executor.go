// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
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
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
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
	// Task environment
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
	// Session history + plan (for context handover)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
}

// Common errors
const defaultBaseBranch = "main"

var (
	ErrNoAgentProfileID        = errors.New("task has no agent_profile_id configured")
	ErrExecutionNotFound       = errors.New("execution not found")
	ErrExecutionAlreadyRunning = errors.New("execution already running")
	ErrNoCloneURL              = errors.New("repository has no clone URL: provider owner and name are required")
	ErrTaskArchived            = errors.New("task is archived")
	ErrStaleExecution          = errors.New("stale execution: no live execution in memory")
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

	// SetMcpMode changes the MCP tool mode on an existing execution's agentctl instance.
	// Used when a session transitions to config mode after the workspace was prepared with default mode.
	SetMcpMode(ctx context.Context, executionID string, mode string) error

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

	// CleanupStaleExecutionBySessionID stops the runtime instance and removes a stale
	// execution from the in-memory tracking store. Used when the agent process has
	// exited but the execution entry was not cleaned up (e.g. prepared workspace
	// where agent was never started, or session resume after crash).
	CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error

	// EnsureWorkspaceExecutionForSession ensures an agentctl execution exists for a
	// session so that workspace operations (file tree, terminals, git) are accessible.
	// Used for restoring workspace access on terminal-state sessions.
	EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error

	// GetExecutionIDForSession returns the execution ID for a session from the
	// in-memory execution store. Returns empty string and error if not found.
	// Used to detect stale AgentExecutionID values in the database after restart.
	GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error)

	// GetGitLog retrieves the git log for a session from baseCommit to HEAD.
	// If targetBranch is provided, uses dynamic merge-base calculation for accurate filtering.
	// Used for archive snapshot capture. Returns nil, nil if no execution exists.
	GetGitLog(ctx context.Context, sessionID, baseCommit string, limit int, targetBranch string) (*client.GitLogResult, error)

	// GetCumulativeDiff retrieves the cumulative diff for a session from baseCommit to the
	// working tree (including uncommitted/unstaged changes). Used for archive snapshot capture.
	// Note: archive snapshots will capture uncommitted working-tree state.
	// Returns nil, nil if no execution exists.
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
	TaskEnvironmentID   string // Env owning this session (shared across sessions in the same task)
	TaskTitle           string // Human-readable task title for semantic worktree naming
	AgentProfileID      string
	RepositoryURL       string
	Branch              string
	TaskDescription     string                 // Task description to send via ACP prompt
	Attachments         []v1.MessageAttachment // Attachments for the initial prompt (images/files)
	Priority            int
	Metadata            map[string]interface{}
	Env                 map[string]string
	ACPSessionID        string            // ACP session ID to resume, if available
	ModelOverride       string            // If set, use this model instead of the profile's model
	ExecutorType        string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig      map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID string            // Previous execution ID for runtime reconnect
	McpMode             string            // MCP tool mode: "config" activates config-mode tools
	IsEphemeral         bool              // Ephemeral task (quick chat) — enables fallback workspace creation

	// Setup script from executor profile (runs in execution environment before agent starts)
	SetupScript string

	// Worktree configuration for concurrent agent execution
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

	// Repositories carries one entry per repository when the launch is multi-repo.
	// When non-empty it is the source of truth and the legacy single-repo
	// top-level fields above are populated from Repositories[0] for backwards
	// compatibility with code paths that have not yet been updated.
	Repositories []RepoSpec
}

// RepoSpec describes one repository for a multi-repo task launch from the
// orchestrator. Mirrors lifecycle.RepoLaunchSpec; kept as a separate type so
// the orchestrator package does not need to import lifecycle types into its
// public API.
type RepoSpec struct {
	RepositoryID         string
	RepositoryPath       string
	RepositoryURL        string
	RepoName             string
	BaseBranch           string
	CheckoutBranch       string
	WorktreeID           string
	WorktreeBranchPrefix string
	PullBeforeWorktree   bool
	RepoSetupScript      string
	RepoCleanupScript    string
}

// McpModeConfig activates config-mode MCP tools (workflow steps, agents, MCP
// config, tasks). Used when plan_mode is enabled on a session.
const McpModeConfig = "config"

// LaunchOptions contains optional parameters for LaunchPreparedSession.
type LaunchOptions struct {
	AgentProfileID string
	ExecutorID     string
	Prompt         string
	WorkflowStepID string
	StartAgent     bool
	McpMode        string // MCP tool mode: McpModeConfig activates config-mode tools
	Attachments    []v1.MessageAttachment
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
	PrepareResult    *lifecycle.EnvPrepareResult `json:"-"` // Carried from lifecycle.Launch for synchronous persistence

	// Worktrees is the per-repository preparer result list when the launch is
	// multi-repo. Empty for single-repo launches; the legacy WorktreeID/Path/
	// Branch fields above mirror Worktrees[0] in that case.
	Worktrees []RepoWorktreeResult
}

// RepoWorktreeResult mirrors lifecycle.RepoWorktreeResult for the orchestrator
// API surface. One entry per repository prepared during a multi-repo launch.
type RepoWorktreeResult struct {
	RepositoryID   string
	WorktreeID     string
	WorktreeBranch string
	WorktreePath   string
	MainRepoGitDir string
	ErrorMessage   string
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
	// PrepareResult carries the env preparation result for deferred persistence
	PrepareResult *lifecycle.EnvPrepareResult
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

// LaunchFailedFunc is called when session launch fails before the agent starts.
// Useful for creating user-facing status messages tied to launch errors.
type LaunchFailedFunc func(ctx context.Context, taskID, sessionID string, err error)

// PrimarySessionSetFunc is called when the first session for a task is marked
// primary. This lets the orchestrator publish a task.updated event so the
// frontend receives the primary_session_id.
type PrimarySessionSetFunc func(ctx context.Context, taskID, sessionID string)

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

	// Callback for session launch failures (pre-start). Allows orchestrator
	// to emit user-friendly guidance for known failure patterns.
	onLaunchFailed LaunchFailedFunc

	// Callback when the first session for a task is marked primary.
	onPrimarySessionSet PrimarySessionSetFunc

	// Per-session locks to prevent concurrent resume/launch operations on the same session.
	// This prevents race conditions when the backend restarts and multiple resume requests
	// arrive simultaneously (e.g., from frontend auto-resume).
	sessionLocks sync.Map // map[string]*sync.Mutex

	// Per-task locks for env persistence — concurrent launches for the same
	// task race in persistTaskEnvironment (each sees existingEnv == nil and
	// each calls CreateTaskEnvironment). The unique index on
	// task_environments(task_id) catches the second insert, but this lock
	// closes the window before the constraint trips so the first launch
	// succeeds and the second reuses its env.
	taskEnvLocks sync.Map // map[string]*sync.Mutex

	// Optional cloner for provider-backed repos without a local path.
	repoCloner  RepoCloner
	repoUpdater RepoUpdater
}

// taskEnvLock returns the per-task mutex for env persistence, creating one on
// demand. Mirrors the sessionLocks pattern.
func (e *Executor) taskEnvLock(taskID string) *sync.Mutex {
	mu, _ := e.taskEnvLocks.LoadOrStore(taskID, &sync.Mutex{})
	return mu.(*sync.Mutex)
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

// SetOnPrimarySessionSet sets a callback for when the first session for a task
// is marked primary. This publishes a task.updated event so the frontend
// receives primary_session_id.
func (e *Executor) SetOnPrimarySessionSet(fn PrimarySessionSetFunc) {
	e.onPrimarySessionSet = fn
}

// SetOnLaunchFailed sets a callback for launch failures that happen before
// the agent process has started.
func (e *Executor) SetOnLaunchFailed(fn LaunchFailedFunc) {
	e.onLaunchFailed = fn
}

// SetCapabilities sets the executor type capabilities provider.
func (e *Executor) SetCapabilities(c ExecutorTypeCapabilities) {
	e.capabilities = c
}
