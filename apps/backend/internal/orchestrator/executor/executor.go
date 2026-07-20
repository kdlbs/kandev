// Package executor manages agent execution for tasks.
package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/cloneauth"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// executorStore is the minimal repository interface required by the Executor.
type executorStore interface {
	// Task
	GetTask(ctx context.Context, id string) (*models.Task, error)
	// UpdateTaskStateIfNotArchived atomically transitions state unless the
	// task is archived (archived_at IS NULL). Used instead of a plain
	// unconditional UpdateTaskState for every runtime-driven task-state write
	// (IN_PROGRESS on start/resume, FAILED on launch error) so a late write
	// can never race an archive that lands after an earlier (non-transactional)
	// archived-state check. Returns the pre-update state and whether a row
	// was modified.
	UpdateTaskStateIfNotArchived(ctx context.Context, id string, state v1.TaskState) (v1.TaskState, bool, error)
	// UpdateTaskStateIfCurrentIn atomically transitions state only when the
	// current state is in allowed AND the task is not archived (archived_at
	// IS NULL). Used instead of UpdateTaskState for guarded REVIEW writes so
	// a late write can never race an archive that lands after an earlier
	// (non-transactional) archived-state check. Returns the pre-update state
	// and whether a row was modified.
	UpdateTaskStateIfCurrentIn(ctx context.Context, id string, state v1.TaskState, allowed []v1.TaskState) (v1.TaskState, bool, error)
	// Task↔repo junction
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	// Session
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionIfCurrentState(ctx context.Context, session *models.TaskSession, expected models.TaskSessionState) (bool, error)
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	UpdateTaskSessionBaseCommit(ctx context.Context, id string, baseCommitSHA string) error
	GetTaskSessionByTaskAndAgent(ctx context.Context, taskID, agentInstanceID string) (*models.TaskSession, error)
	SetSessionPrimary(ctx context.Context, sessionID string) error
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	// Session worktree
	CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error)
	// Repository entity
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	// Executor
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	HasExecutorRunningRow(ctx context.Context, sessionID string) (bool, error)
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error
	UpdateExecutorRunningStatus(ctx context.Context, sessionID, status string) error
	// Workspace
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	// Task environment
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
	CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
	UpdateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
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
	ErrAgentCommandMissing     = errors.New("existing execution has no agent command configured")
	// ErrSessionStateSuperseded means a runtime registered successfully, but a
	// concurrent terminal session transition won the persistence race. Callers
	// must not start the process and must arbitrate exact-execution teardown
	// ownership before deciding whether to force-stop the registered runtime.
	ErrSessionStateSuperseded = errors.New("session state superseded by terminal transition")
)

// SessionStateSupersededError records the terminal state that rejected a
// stale runtime persistence write.
type SessionStateSupersededError struct {
	SessionID string
	State     models.TaskSessionState
}

func (e *SessionStateSupersededError) Error() string {
	return fmt.Sprintf("%s: session %s is %s", ErrSessionStateSuperseded, e.SessionID, e.State)
}

func (e *SessionStateSupersededError) Unwrap() error { return ErrSessionStateSuperseded }

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

	// IsAgentCommandConfigured reports whether an execution is ready for
	// StartAgentProcess or still needs workspace-only promotion via LaunchAgent.
	IsAgentCommandConfigured(agentExecutionID string) bool

	// StopAgent stops a running agent
	StopAgent(ctx context.Context, agentExecutionID string, force bool) error
	StopAgentWithReason(ctx context.Context, agentExecutionID string, reason string, force bool) error

	// PromptAgent sends a prompt to a running agent
	// Returns PromptResult indicating if the agent needs input
	// Attachments (images) are passed to the agent if provided
	// When dispatchOnly is true, returns once the prompt is accepted by agentctl
	// without waiting for the agent's turn to complete.
	PromptAgent(ctx context.Context, agentExecutionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*PromptResult, error)

	// CancelAgent interrupts the current agent turn without terminating the process.
	CancelAgent(ctx context.Context, sessionID string) error

	// RespondToPermission sends a response to a permission request
	RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error

	// IsAgentRunningForSession checks if an agent is actually running for a session
	// This probes the actual agent (Docker container or standalone process) rather than relying on cached state
	IsAgentRunningForSession(ctx context.Context, sessionID string) bool

	// IsAgentReadyForPrompt checks if the session can accept an immediate prompt.
	// This is stricter than IsAgentRunningForSession because ACP sessions can be
	// running before session initialization and stream setup have completed.
	IsAgentReadyForPrompt(ctx context.Context, sessionID string) bool

	// ResolveAgentProfile resolves an agent profile ID to profile information
	ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)

	// SetExecutionDescription updates the task description in an existing execution's metadata.
	// Used when starting an agent on a session whose workspace was already launched.
	SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error

	// SetExecutionEnv updates per-run env vars in an existing execution before subprocess start.
	SetExecutionEnv(ctx context.Context, agentExecutionID string, env map[string]string) error

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

	// SetSessionModelBySessionID attempts an in-place model switch via ACP model selection.
	// Returns an error if the agent doesn't support in-place model switching.
	SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error

	// SetSessionModeBySessionID applies a session permission mode (e.g. "default",
	// "acceptEdits") to the running agent via ACP session/set_mode. Returns an
	// error when no agent is running for the session. See issue #1183.
	SetSessionModeBySessionID(ctx context.Context, sessionID, modeID string) error

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

	// ResolvePassthroughConfig returns the resolved PassthroughConfig for a session's agent.
	// Used by the orchestrator to route chat-compose prompts and Stop button presses into
	// the agent's PTY stdin (with the correct submit sequence) instead of through ACP.
	ResolvePassthroughConfig(ctx context.Context, sessionID string) (agents.PassthroughConfig, error)

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

	// GetGitStatus retrieves the current (cached) git status for a session.
	// Returns nil, nil if no execution exists.
	GetGitStatus(ctx context.Context, sessionID string) (*client.GitStatusResult, error)

	// GetGitStatusFresh retrieves a fresh (non-cached) git status for a session.
	// Bypasses the workspace tracker's poll cache. Use after the agent commits.
	// Returns nil, nil if no execution exists.
	GetGitStatusFresh(ctx context.Context, sessionID string) (*client.GitStatusResult, error)

	// WaitForAgentctlReady waits for the agentctl HTTP server to be ready for a session.
	// This must be called before other agentctl operations (git status, shell, etc.).
	WaitForAgentctlReady(ctx context.Context, sessionID string) error
}

// RemoteRuntimeStatus mirrors runtime status details needed by orchestrator/UI.
type RemoteRuntimeStatus struct {
	RuntimeName   agentruntime.Runtime
	RemoteName    string
	State         string
	CreatedAt     *time.Time
	LastCheckedAt time.Time
	ErrorMessage  string
}

// RemoteStatusPollRequest contains the fields from ExecutorRunning needed for remote status polling.
type RemoteStatusPollRequest struct {
	SessionID        string
	Runtime          agentruntime.Runtime
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
	Mode                       string
	ConfigOptions              map[string]string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	CLIPassthrough             bool
	NativeSessionResume        bool // Agent supports ACP session/load for resume
	SupportsMCP                bool
}

// LaunchAgentRequest contains parameters for launching an agent
type LaunchAgentRequest struct {
	TaskID            string
	WorkspaceID       string // Kandev workspace ID — used to build scratch dir for repo-less tasks
	SessionID         string
	TaskEnvironmentID string // Env owning this session (shared across sessions in the same task)
	TaskTitle         string // Human-readable task title for semantic worktree naming
	AgentProfileID    string
	// OfficeAgentProfileID is the stable Office identity. AgentProfileID stays
	// the concrete execution profile inside the executor for compatibility.
	OfficeAgentProfileID string
	StartAgent           bool // Keep lifecycle activity through initial startup/prompt
	RepositoryURL        string
	Branch               string
	TaskDescription      string                 // Task description to send via ACP prompt
	Attachments          []v1.MessageAttachment // Attachments for the initial prompt (images/files)
	Priority             string
	Metadata             map[string]interface{}
	Env                  map[string]string
	ACPSessionID         string            // ACP session ID to resume, if available
	ModelOverride        string            // If set, use this model instead of the profile's model
	ExecutorType         string            // Executor type (e.g., "local", "worktree", "local_docker") - determines runtime
	ExecutorConfig       map[string]string // Executor config (docker_host, git_token, etc.)
	PreviousExecutionID  string            // Previous execution ID for runtime reconnect
	McpMode              string            // MCP tool mode: "task" (default), "config", or "office"
	IsEphemeral          bool              // Ephemeral task (quick chat) — enables fallback workspace creation
	WorkspacePath        string            // Optional host folder for repo-less tasks (overrides scratch fallback)

	// IsPassthrough is the session's mode snapshot (TaskSession.IsPassthrough)
	// at session-creation time. Forwarded to the lifecycle manager so
	// StartAgentProcess routes to the passthrough vs ACP path based on the
	// session's original mode, not on live profile state — preventing
	// existing sessions from getting stranded when a profile's
	// CLIPassthrough flag is toggled after the session was created.
	IsPassthrough bool

	// Setup script from executor profile (runs in execution environment before agent starts)
	SetupScript string

	// CopyFiles is the per-repository copy_files spec resolved from
	// Repository.CopyFiles. Used by the worktree path (host-side copy via
	// worktree.Manager) and by remote-executor paths (Docker, Sprites)
	// which ship the bytes via agentctl.
	CopyFiles string

	// Worktree configuration for concurrent agent execution
	UseWorktree            bool   // Whether to use a Git worktree for isolation
	WorktreeID             string // Existing worktree ID to reuse (skip creation if set)
	RepositoryID           string // Repository ID for worktree tracking
	RepositoryPath         string // Path to the main repository (for worktree creation)
	BaseBranch             string // Base branch for the worktree (e.g., "main")
	DefaultBranch          string // Repository's default_branch, used as a fallback when BaseBranch is missing
	CheckoutBranch         string // Branch to fetch and checkout after worktree creation (e.g., PR head branch)
	PRNumber               int    // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	WorktreeBranchPrefix   string // Branch prefix for worktree branches
	WorktreeBranchTemplate string // Branch name template for worktree branches
	WorktreeBranchTicket   string // External ticket value for branch templates
	PullBeforeWorktree     bool   // Whether to pull from remote before creating the worktree

	// Task directory mode: place worktree at ~/.kandev/tasks/{TaskDirName}/{RepoName}/
	TaskDirName string // Semantic task directory name (e.g. "fix-bug_ab12")
	RepoName    string // Repository name used as subdirectory inside the task directory
	// BranchSlug, when non-empty, suffixes the top-level single-repo path.
	BranchSlug string
	// BranchIdentitySlug is the stable branch key for top-level single-repo
	// reuse. It may be non-empty when BranchSlug is empty to preserve a flat path.
	BranchIdentitySlug string

	// Repositories carries one entry per repository when the launch is multi-repo.
	// When non-empty it is the source of truth and the legacy single-repo
	// top-level fields above are populated from Repositories[0] for backwards
	// compatibility with code paths that have not yet been updated.
	Repositories []RepoSpec

	// RouteOverride carries a provider-routing override resolved by the
	// office scheduler. nil when routing is disabled or this is a kanban
	// launch.
	RouteOverride *RouteOverride
}

// RepoSpec describes one repository for a multi-repo task launch from the
// orchestrator. Mirrors lifecycle.RepoLaunchSpec; kept as a separate type so
// the orchestrator package does not need to import lifecycle types into its
// public API.
type RepoSpec struct {
	RepositoryID           string
	RepositoryPath         string
	RepositoryURL          string
	RepoName               string
	BaseBranch             string
	DefaultBranch          string // Repository's default_branch, used as fallback when BaseBranch is missing
	CheckoutBranch         string
	PRNumber               int // GitHub PR number when CheckoutBranch is a PR head; enables refs/pull/<N>/head fetch for fork PRs.
	WorktreeID             string
	WorktreeBranchPrefix   string
	WorktreeBranchTemplate string
	WorktreeBranchTicket   string
	PullBeforeWorktree     bool
	RepoSetupScript        string
	RepoCleanupScript      string
	CopyFiles              string
	// BranchSlug, when non-empty, suffixes the repo dir so the same repo can
	// host multiple branch worktrees as siblings within one task. Set by the
	// orchestrator when buildRepoSpecs detects multiple rows sharing a
	// RepositoryID; empty otherwise to preserve the single-branch layout.
	BranchSlug string

	// BranchIdentitySlug is the stable branch key used for worktree reuse and
	// persisted environment metadata. It may be non-empty even when BranchSlug
	// is empty so the primary branch can keep the legacy flat path.
	BranchIdentitySlug string
}

// McpModeConfig activates config-mode MCP tools (workflow steps, agents, MCP
// config, tasks). Used when plan_mode is enabled on a session.
const McpModeConfig = "config"

// McpModeOffice restricts the MCP toolset for office (autonomous) agents to
// interaction + plan tools. Office agents manage tasks via the kandev CLI
// (exposed through agentctl + $KANDEV_CLI), not MCP — see
// docs/specs/office-agent-cli/spec.md.
const McpModeOffice = "office"

// LaunchOptions contains optional parameters for LaunchPreparedSession.
type LaunchOptions struct {
	AgentProfileID       string
	OfficeAgentProfileID string
	ExecutorID           string
	Prompt               string
	WorkflowStepID       string
	StartAgent           bool
	McpMode              string // MCP tool mode: empty task default, McpModeConfig, or McpModeOffice
	Attachments          []v1.MessageAttachment
	Env                  map[string]string
	// RouteOverride carries a provider-routing override resolved by the
	// office scheduler. When nil, launch behavior is identical to today.
	RouteOverride *RouteOverride
}

// RouteOverride is the orchestrator-side mirror of routing.Candidate
// fields that need to flow into the lifecycle launch.
type RouteOverride struct {
	ExecutionProfileID string
	ProviderID         string
	Model              string
	Tier               string
	Mode               string
	Flags              []string
	Env                map[string]string
}

// LaunchContext is the orchestrator-side mirror of the Office launch
// context (prompt, executor selection, workflow step, attachments,
// plan-mode flag, env, profile). Routed launches use this so they
// preserve the Office-built prompt and configuration that the legacy
// path receives via StartTaskWithEnv.
type LaunchContext struct {
	ExecutorID        string
	ExecutorProfileID string
	Priority          string
	Prompt            string
	WorkflowStepID    string
	PlanMode          bool
	Attachments       []v1.MessageAttachment
	Env               map[string]string
}

// LaunchAgentResponse contains the result of launching an agent
type LaunchAgentResponse struct {
	AgentExecutionID string
	ContainerID      string
	Status           v1.AgentStatus
	WorktreeID       string
	WorktreePath     string
	WorktreeBranch   string
	WorkspacePath    string // Effective workspace path (may differ from WorktreePath for quick-chat sessions)
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
	BranchSlug     string
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

// TaskRuntimeStateReconcileFunc updates task state only while the originating
// session still owns an eligible runtime state.
type TaskRuntimeStateReconcileFunc func(ctx context.Context, taskID, sessionID string, state v1.TaskState) error

// SessionStateChangeFunc is called when the executor needs to update a session's state.
// When set, it replaces direct repo.UpdateTaskSessionState calls so the caller can
// publish events (e.g. WebSocket notifications) alongside the DB update.
type SessionStateChangeFunc func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error

// SessionStateTransitionFunc performs a guarded session-state transition and
// reports whether the requested state was accepted plus the final observed
// state. Coordinator stop uses this stricter callback so terminal races and
// persistence failures cannot be mistaken for accepted cancellation.
type SessionStateTransitionFunc func(ctx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) (changed bool, finalState models.TaskSessionState, err error)

// SessionStartingFunc is called when the executor has prepared/resumed an
// execution and needs to mark the session STARTING while preserving other
// session-row updates such as metadata. promoteTask controls whether the
// callback should also move the parent task to IN_PROGRESS immediately.
type SessionStartingFunc func(ctx context.Context, taskID string, session *models.TaskSession, promoteTask bool) error

// ExecutionCleanupClaimFunc atomically claims forced cleanup for one exact
// session execution. It returns true when the executor owns cleanup and false
// when another teardown path already owns that execution.
type ExecutionCleanupClaimFunc func(sessionID, agentExecutionID string) bool

// ExecutionStopOwnerRegistrationFunc records that an explicit teardown path
// owns one exact session execution. Registration is advisory: the explicit
// stop still runs, while orphan cleanup uses the record to avoid duplicating it.
type ExecutionStopOwnerRegistrationFunc func(sessionID, agentExecutionID string, force bool)

// TaskReviewStateReconcileFunc is called when runtime work stopped and the
// parent task should move to REVIEW only if no session is still STARTING/RUNNING.
type TaskReviewStateReconcileFunc func(ctx context.Context, taskID, completedSessionID string)

// AgentStartFailedFunc is called when the agent process fails to start.
// It receives the task/session/execution IDs and the error. fromResume is true
// when the failure occurred during a background session resume (rather than a
// user-initiated start), letting the orchestrator suppress user-facing toasts
// for transient bootstrap errors. If the callback returns true, it has handled
// the failure (e.g., as a recoverable auth error) and the executor should skip
// its default FAILED state updates.
type AgentStartFailedFunc func(ctx context.Context, taskID, sessionID, agentExecutionID string, err error, fromResume bool) (handled bool)

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

// GitLabCredentialResolver returns the configured origin and credential for
// exactly one workspace. Implementations must not fall back across workspaces.
type GitLabCredentialResolver interface {
	ResolveGitLabExecutionCredentials(ctx context.Context, workspaceID string) (host, token string, err error)
}

// Executor manages agent execution for tasks
type Executor struct {
	agentManager      AgentManagerClient
	repo              executorStore
	secretStore       secrets.SecretStore
	shellPrefs        ShellPreferenceProvider
	capabilities      ExecutorTypeCapabilities
	gitlabCredentials GitLabCredentialResolver
	logger            *logger.Logger

	// Configuration
	retryLimit int
	retryDelay time.Duration

	// Callback for task state changes that need event publishing.
	// Set by the orchestrator to route through the task service layer.
	onTaskStateChange TaskStateChangeFunc

	// Session-aware task state callback used after agent-process start settles.
	onTaskRuntimeStateReconcile TaskRuntimeStateReconcileFunc

	// Callback for session state changes that need event publishing.
	// Set by the orchestrator to route through updateTaskSessionState which
	// updates the DB and publishes WebSocket events.
	onSessionStateChange SessionStateChangeFunc

	// Strict session-state callback used by operations that need to distinguish
	// accepted writes from terminal/no-op races.
	onSessionStateTransition SessionStateTransitionFunc

	// Callback for STARTING writes that carry full session-row changes. Set by
	// the orchestrator so launch/resume/model-switch transitions serialize with
	// runtime task-state reconciliation.
	onSessionStarting SessionStartingFunc

	// Callback for exact-execution forced cleanup arbitration. Set by the
	// orchestrator so coordinator graceful stop and launch cleanup cannot both
	// tear down the same execution.
	onExecutionCleanupClaim ExecutionCleanupClaimFunc

	// Callback for registering explicit exact-execution teardown ownership before
	// a legacy stop persists CANCELLED. The requested stop always runs.
	onExecutionStopOwnerRegistration ExecutionStopOwnerRegistrationFunc

	// Callback for REVIEW reconciliation after runtime start failures. Set by the
	// orchestrator so failed-start writes share the same serialized guard as
	// normal turn completion.
	onTaskReviewStateReconcile TaskReviewStateReconcileFunc

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
	EnsureClonedWithAuth(ctx context.Context, cloneURL, owner, name, credentialHost, token string) (string, error)
	// BuildCloneURL constructs a protocol-aware clone URL for the given provider/owner/name.
	BuildCloneURLWithHost(provider, host, owner, name string) (string, error)
}

type authenticatedRepoCloner interface {
	EnsureClonedWithBasicAuth(ctx context.Context, cloneURL, owner, name, username, password string) (string, error)
}

func (e *Executor) ensureClonedWithWorkspaceAuth(
	ctx context.Context, repo *models.Repository, cloneURL string,
) (string, error) {
	if strings.EqualFold(repo.Provider, "gitlab") {
		credentialHost, token := "", ""
		if e.gitlabCredentials != nil {
			credentialHost, token, _ = e.gitlabCredentials.ResolveGitLabExecutionCredentials(ctx, repo.WorkspaceID)
		}
		return e.repoCloner.EnsureClonedWithAuth(
			ctx, cloneURL, repo.ProviderOwner, repo.ProviderName, credentialHost, token,
		)
	}
	if repo.Provider != "azure_devops" || !strings.HasPrefix(cloneURL, "https://") {
		return e.repoCloner.EnsureClonedWithAuth(ctx, cloneURL, repo.ProviderOwner, repo.ProviderName, "", "")
	}
	authCloner, ok := e.repoCloner.(authenticatedRepoCloner)
	if !ok || e.secretStore == nil {
		return "", fmt.Errorf("azure DevOps repository clone authentication is unavailable")
	}
	pat, err := e.secretStore.Reveal(ctx, cloneauth.AzureDevOpsPATKey(repo.WorkspaceID))
	if err != nil {
		return "", fmt.Errorf("read Azure DevOps clone credential: %w", err)
	}
	// Azure DevOps PAT authentication ignores the username; any non-empty value works.
	return authCloner.EnsureClonedWithBasicAuth(ctx, cloneURL, repo.ProviderOwner, repo.ProviderName, "kandev", pat)
}

// RepoUpdater updates repository records in the database.
type RepoUpdater interface {
	UpdateRepositoryLocalPath(ctx context.Context, repositoryID, localPath string) error
	// UpdateRepositoryDefaultBranch persists the integration branch detected
	// from the local clone. Called after a fresh provider-backed clone when
	// the repository row was created without an upstream-derived value
	// (e.g. via the MCP create_task path that takes a bare github URL).
	UpdateRepositoryDefaultBranch(ctx context.Context, repositoryID, defaultBranch string) error
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

// SetOnTaskRuntimeStateReconcile sets the session-aware runtime task-state callback.
func (e *Executor) SetOnTaskRuntimeStateReconcile(fn TaskRuntimeStateReconcileFunc) {
	e.onTaskRuntimeStateReconcile = fn
}

// SetOnSessionStateChange sets a callback for session state changes.
// This allows the orchestrator to route state changes through updateTaskSessionState
// which updates the DB and publishes WebSocket events to the frontend.
func (e *Executor) SetOnSessionStateChange(fn SessionStateChangeFunc) {
	e.onSessionStateChange = fn
}

// SetOnSessionStateTransition sets the guarded session-state callback used by
// detailed lifecycle operations.
func (e *Executor) SetOnSessionStateTransition(fn SessionStateTransitionFunc) {
	e.onSessionStateTransition = fn
}

// SetOnSessionStarting sets a callback for full session-row STARTING updates.
func (e *Executor) SetOnSessionStarting(fn SessionStartingFunc) {
	e.onSessionStarting = fn
}

// SetOnExecutionCleanupClaim sets the exact-execution forced cleanup arbiter.
func (e *Executor) SetOnExecutionCleanupClaim(fn ExecutionCleanupClaimFunc) {
	e.onExecutionCleanupClaim = fn
}

// SetOnExecutionStopOwnerRegistration sets the explicit-stop ownership registrar.
func (e *Executor) SetOnExecutionStopOwnerRegistration(fn ExecutionStopOwnerRegistrationFunc) {
	e.onExecutionStopOwnerRegistration = fn
}

// SetOnTaskReviewStateReconcile sets the guarded task REVIEW reconciliation
// callback used after resume/start failures.
func (e *Executor) SetOnTaskReviewStateReconcile(fn TaskReviewStateReconcileFunc) {
	e.onTaskReviewStateReconcile = fn
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

// SetGitLabCredentialResolver wires workspace-scoped GitLab execution auth.
func (e *Executor) SetGitLabCredentialResolver(resolver GitLabCredentialResolver) {
	e.gitlabCredentials = resolver
}
