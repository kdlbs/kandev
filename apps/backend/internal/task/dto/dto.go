package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type WorkflowDTO struct {
	ID             string  `json:"id"`
	WorkspaceID    string  `json:"workspace_id"`
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	Prompt         *string `json:"prompt,omitempty"`
	AgentProfileID string  `json:"agent_profile_id,omitempty"`
	SortOrder      int     `json:"sort_order"`
	Hidden         bool    `json:"hidden,omitempty"`
	// Style is a Phase 2 (ADR-0004) UX hint read by the frontend ONLY.
	// Allowed values: "kanban" | "office" | "custom".
	Style string `json:"style,omitempty"`
	// Source records where the workflow definition came from ("manual" |
	// "github"); SourcePath is the repo-relative file for synced workflows.
	Source     string    `json:"source,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type WorkspaceDTO struct {
	ID                          string    `json:"id"`
	Name                        string    `json:"name"`
	Description                 *string   `json:"description,omitempty"`
	OwnerID                     string    `json:"owner_id"`
	DefaultExecutorID           *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID        *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID       *string   `json:"default_agent_profile_id,omitempty"`
	DefaultConfigAgentProfileID *string   `json:"default_config_agent_profile_id,omitempty"`
	TaskPrefix                  string    `json:"task_prefix,omitempty"`
	TaskSequence                int       `json:"task_sequence,omitempty"`
	OfficeWorkflowID            string    `json:"office_workflow_id,omitempty"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type RepositoryDTO struct {
	ID                     string                       `json:"id"`
	WorkspaceID            string                       `json:"workspace_id"`
	Name                   string                       `json:"name"`
	SourceType             string                       `json:"source_type"`
	LocalPath              string                       `json:"local_path"`
	Provider               string                       `json:"provider"`
	ProviderRepoID         string                       `json:"provider_repo_id"`
	ProviderHost           string                       `json:"provider_host"`
	ProviderScope          string                       `json:"provider_scope"`
	ProviderOwner          string                       `json:"provider_owner"`
	ProviderName           string                       `json:"provider_name"`
	RemoteURL              string                       `json:"remote_url"`
	DefaultBranch          string                       `json:"default_branch"`
	WorktreeBranchPrefix   string                       `json:"worktree_branch_prefix"`
	WorktreeBranchTemplate string                       `json:"worktree_branch_template"`
	PullBeforeWorktree     bool                         `json:"pull_before_worktree"`
	SetupScript            string                       `json:"setup_script"`
	CleanupScript          string                       `json:"cleanup_script"`
	DevScript              string                       `json:"dev_script"`
	CopyFiles              string                       `json:"copy_files"`
	SecretBindings         []RepositorySecretBindingDTO `json:"secret_bindings,omitempty"`
	CreatedAt              time.Time                    `json:"created_at"`
	UpdatedAt              time.Time                    `json:"updated_at"`
	Scripts                []RepositoryScriptDTO        `json:"scripts,omitempty"`
}

type RepositorySecretBindingDTO struct {
	Key      string `json:"key"`
	SecretID string `json:"secret_id"`
}

// RepositorySetDTO is a named group of workspace repositories on the wire.
// Repositories is always an array, never null: the web store indexes it without
// a nil check, and a set whose members were all deleted is legitimately empty.
type RepositorySetDTO struct {
	ID           string                 `json:"id"`
	WorkspaceID  string                 `json:"workspace_id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Repositories []RepositorySetItemDTO `json:"repositories"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// RepositorySetItemDTO is one repository's membership, in apply order.
type RepositorySetItemDTO struct {
	RepositoryID string `json:"repository_id"`
	Position     int    `json:"position"`
}

type RepositoryBranchPolicyDTO struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	BaseBranch        string    `json:"base_branch"`
	BranchTemplate    string    `json:"branch_template"`
	PullRequestTarget string    `json:"pull_request_target"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RepositoryScriptDTO struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ShellOutputSnapshotResponse is the on-demand full output for one shell message.
type ShellOutputSnapshotResponse struct {
	MessageID string                         `json:"message_id"`
	Status    string                         `json:"status"`
	UpdatedAt time.Time                      `json:"updated_at"`
	Output    models.ShellExecOutputSnapshot `json:"output"`
}

type ExecutorDTO struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Type      models.ExecutorType   `json:"type"`
	Status    models.ExecutorStatus `json:"status"`
	IsSystem  bool                  `json:"is_system"`
	Resumable bool                  `json:"resumable"`
	Config    map[string]string     `json:"config,omitempty"`
	Profiles  []ExecutorProfileDTO  `json:"profiles,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type ExecutorProfileDTO struct {
	ID            string                 `json:"id"`
	ExecutorID    string                 `json:"executor_id"`
	ExecutorType  string                 `json:"executor_type,omitempty"`
	ExecutorName  string                 `json:"executor_name,omitempty"`
	Name          string                 `json:"name"`
	McpPolicy     string                 `json:"mcp_policy,omitempty"`
	Config        map[string]string      `json:"config,omitempty"`
	PrepareScript string                 `json:"prepare_script"`
	CleanupScript string                 `json:"cleanup_script"`
	EnvVars       []models.ProfileEnvVar `json:"env_vars,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type ListExecutorProfilesResponse struct {
	Profiles []ExecutorProfileDTO `json:"profiles"`
	Total    int                  `json:"total"`
}

type EnvironmentDTO struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Kind         models.EnvironmentKind `json:"kind"`
	IsSystem     bool                   `json:"is_system"`
	WorktreeRoot string                 `json:"worktree_root,omitempty"`
	ImageTag     string                 `json:"image_tag,omitempty"`
	Dockerfile   string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string      `json:"build_config,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type TaskDTO struct {
	ID                          string                   `json:"id"`
	WorkspaceID                 string                   `json:"workspace_id"`
	WorkflowID                  string                   `json:"workflow_id"`
	WorkflowStepID              string                   `json:"workflow_step_id"`
	Title                       string                   `json:"title"`
	Description                 string                   `json:"description"`
	State                       v1.TaskState             `json:"state"`
	Priority                    string                   `json:"priority"`
	WIPAdmitted                 bool                     `json:"wip_admitted"`
	QueuedForStepID             string                   `json:"queued_for_step_id,omitempty"`
	QueuedAt                    *time.Time               `json:"queued_at,omitempty"`
	Repositories                []TaskRepositoryDTO      `json:"repositories,omitempty"`
	WorkspaceFolders            []TaskWorkspaceFolderDTO `json:"workspace_folders,omitempty"`
	Position                    int                      `json:"position"`
	PrimarySessionID            *string                  `json:"primary_session_id,omitempty"`
	SessionCount                *int                     `json:"session_count,omitempty"`
	ReviewStatus                models.ReviewStatus      `json:"review_status,omitempty"`
	PrimaryExecutorID           *string                  `json:"primary_executor_id,omitempty"`
	PrimaryExecutorType         *string                  `json:"primary_executor_type,omitempty"`
	PrimaryExecutorName         *string                  `json:"primary_executor_name,omitempty"`
	PrimaryAgentName            *string                  `json:"primary_agent_name,omitempty"`
	PrimaryWorkingDirectory     *string                  `json:"primary_working_directory,omitempty"`
	PrimarySessionState         *string                  `json:"primary_session_state,omitempty"`
	PrimarySessionPendingAction *string                  `json:"primary_session_pending_action"`
	TaskPendingAction           *string                  `json:"task_pending_action"`
	// ForegroundActivity is the task-level MOST-ACTIVE-WINS activity aggregate
	// across the task's sessions: "generating" when
	// any session is generating, "background" when none is generating but at
	// least one RUNNING session is holding a turn open for background work, and
	// empty (omitted) when no session is running — in which case task-level
	// surfaces fall through to the coarse task state (done / waiting / failed).
	// Computed on the backend and carried on the task record so every task-level
	// surface reads one authoritative value; stamped by EnrichTaskForegroundActivity.
	ForegroundActivity  v1.ForegroundActivity  `json:"foreground_activity,omitempty"`
	ActiveSubagentCount int                    `json:"active_subagent_count"`
	IsRemoteExecutor    bool                   `json:"is_remote_executor,omitempty"`
	ParentID            string                 `json:"parent_id,omitempty"`
	Autopilot           bool                   `json:"autopilot"`
	ArchivedAt          *time.Time             `json:"archived_at,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	// Interrupted reports that the task's session was mid-turn when the backend
	// died and has not been resumed since. Derived from the interrupted_at
	// metadata key at DTO conversion time (see FromTaskWithSessionInfo).
	Interrupted bool `json:"interrupted,omitempty"`
	// AutoStartFailed reports that a workflow step's auto_start_agent on_enter
	// action failed to launch a run for this task. Derived from the
	// auto_start_failed metadata key at DTO conversion time (see
	// FromTaskWithSessionInfo).
	AutoStartFailed bool `json:"auto_start_failed,omitempty"`

	// Dependency projection. Derived on every read from task_blockers plus each
	// related task's own state — never persisted, because a stale copy would be
	// read by the auto-start gate. Stamped by EnrichTaskDependencies.
	Blocked bool `json:"blocked,omitempty"`
	// BlockedReason is "pending", "failed", or "unknown"; omitted when the task
	// is not blocked.
	BlockedReason string `json:"blocked_reason,omitempty"`
	// DependsOn lists direct predecessors, DependsOn/Blocks are not transitive.
	DependsOn []TaskDependencyRefDTO `json:"depends_on,omitempty"`
	// Blocks lists direct dependents so the dependency chip can render both
	// directions without a second round trip.
	Blocks []TaskDependencyRefDTO `json:"blocks,omitempty"`
	// StartWhenUnblocked reports that a launch intent is waiting on dependency
	// resolution. Read-only here; set through the create request or the picker.
	StartWhenUnblocked bool `json:"start_when_unblocked,omitempty"`

	// Office extensions
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id,omitempty"`
	Origin                 string `json:"origin,omitempty"`
	ProjectID              string `json:"project_id,omitempty"`
	Labels                 string `json:"labels,omitempty"`
	Identifier             string `json:"identifier,omitempty"`
	// ExternalID is a caller-supplied identity used for task create-
	// idempotency (docs/specs/tasks/requirements/external-id-idempotency.md). Omitted
	// when the task holds none.
	ExternalID string `json:"external_id,omitempty"`
	// IsFromOffice is the authoritative "this task is owned by office"
	// flag. Computed by the task repo at read time as
	// (project_id != '' OR workflow_id == workspace.office_workflow_id).
	// True for office tasks even when they have no project yet; false for
	// kanban-board tasks. Use this to gate office-only UI (e.g. the
	// "Open in office view" topbar link).
	IsFromOffice bool `json:"is_from_office,omitempty"`

	// PRs lists the GitHub pull requests associated with this task, when the
	// github service is wired and any association exists. Surfaced through the
	// task-listing MCP tools so agents can reason about PR status (e.g. find
	// tasks whose PRs are merged). Omitted when empty.
	PRs []v1.TaskPRSummary `json:"prs,omitempty"`

	// StatusSummary is the bounded task-level projection consumed by task rows.
	// It is loaded in batches and is absent when no projection exists yet; the
	// existing coarse fields above remain the compatibility fallback.
	StatusSummary *statussummary.TaskStatusSummary `json:"status_summary,omitempty"`
	// StatusSummaryInvalidated distinguishes a known-stale summary from an
	// ordinarily omitted partial projection so clients clear their cache and
	// expose the coarse compatibility fallback.
	StatusSummaryInvalidated bool `json:"status_summary_invalidated,omitempty"`
}

type TaskRepositoryDTO struct {
	ID                            string                 `json:"id"`
	TaskID                        string                 `json:"task_id"`
	RepositoryID                  string                 `json:"repository_id"`
	BaseBranch                    string                 `json:"base_branch"`
	CheckoutBranch                string                 `json:"checkout_branch,omitempty"`
	BranchPolicyID                string                 `json:"branch_policy_id,omitempty"`
	BranchPolicyName              string                 `json:"branch_policy_name,omitempty"`
	BranchPolicyBaseBranch        string                 `json:"branch_policy_base_branch,omitempty"`
	BranchPolicyBranchTemplate    string                 `json:"branch_policy_branch_template,omitempty"`
	BranchPolicyPullRequestTarget string                 `json:"branch_policy_pull_request_target,omitempty"`
	Position                      int                    `json:"position"`
	Metadata                      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt                     time.Time              `json:"created_at"`
	UpdatedAt                     time.Time              `json:"updated_at"`
}

// TaskWorkspaceFolderDTO is the API projection of a durable non-Git source.
type TaskWorkspaceFolderDTO struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	LocalPath   string    `json:"local_path"`
	DisplayName string    `json:"display_name"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskSessionDTO struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	// Name is the user-supplied session tab label. Serialized without
	// omitempty so a cleared name ("") overwrites stale client state.
	Name              string `json:"name"`
	AgentExecutionID  string `json:"agent_execution_id,omitempty"`
	ContainerID       string `json:"container_id,omitempty"`
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ExecutorID        string `json:"executor_id,omitempty"`
	ExecutorProfileID string `json:"executor_profile_id,omitempty"`
	EnvironmentID     string `json:"environment_id,omitempty"`
	RepositoryID      string `json:"repository_id,omitempty"`
	BaseBranch        string `json:"base_branch,omitempty"`
	BaseCommitSHA     string `json:"base_commit_sha,omitempty"`
	WorktreeID        string `json:"worktree_id,omitempty"`
	WorktreePath      string `json:"worktree_path,omitempty"`
	WorktreeBranch    string `json:"worktree_branch,omitempty"`
	// WorkspacePath is the effective task root used by Files and chat links;
	// WorktreePath remains the flattened primary repository path.
	WorkspacePath string `json:"workspace_path,omitempty"`
	// Worktrees lists all session worktrees (one per repo on multi-repo tasks);
	// the flattened Worktree* fields above carry only the first for backward
	// compatibility. Entries use the stable session-worktree wire shape
	// (session_id + worktree identity) shared with TaskSession.ToAPI.
	Worktrees            []map[string]interface{} `json:"worktrees,omitempty"`
	State                models.TaskSessionState  `json:"state"`
	ErrorMessage         string                   `json:"error_message,omitempty"`
	Metadata             map[string]interface{}   `json:"metadata,omitempty"`
	AgentProfileSnapshot map[string]interface{}   `json:"agent_profile_snapshot,omitempty"`
	ExecutorSnapshot     map[string]interface{}   `json:"executor_snapshot,omitempty"`
	EnvironmentSnapshot  map[string]interface{}   `json:"environment_snapshot,omitempty"`
	RepositorySnapshot   map[string]interface{}   `json:"repository_snapshot,omitempty"`
	StartedAt            time.Time                `json:"started_at"`
	CompletedAt          *time.Time               `json:"completed_at,omitempty"`
	UpdatedAt            time.Time                `json:"updated_at"`
	// Workflow fields
	IsPrimary         bool                `json:"is_primary"`
	IsPassthrough     bool                `json:"is_passthrough"`
	ReviewStatus      models.ReviewStatus `json:"review_status,omitempty"`
	TaskEnvironmentID string              `json:"task_environment_id,omitempty"`
	// ForegroundActivity mirrors the in-memory fine-grained busy substate so a
	// fresh page-load / second tab sees live background work without waiting for
	// a WS flip (ADR-0049). Generating is emitted only for RUNNING sessions;
	// background may remain present after the foreground turn settles.
	// Not persisted — populated at the serialization boundary by
	// EnrichForegroundActivity, never by FromTaskSession.
	ForegroundActivity v1.ForegroundActivity `json:"foreground_activity,omitempty"`
	// CancellationPending mirrors the orchestrator's runtime cancellation
	// projection. It is always serialized so false clears stale client state.
	CancellationPending bool `json:"cancellation_pending"`
	// CancellationRevision identifies the process-local cancellation transition
	// generation that produced CancellationPending. It is always serialized so
	// clients can reject delayed snapshots from older generations.
	CancellationRevision uint64 `json:"cancellation_revision"`
	// SupportsSteering is true when a send right now would be delivered into the
	// still-generating turn (mid-turn steering) rather than blocked/queued.
	// Derived live at serialization from the connected agent's negotiated
	// capability plus the runtime flag; never persisted (see mid-turn-steering
	// spec). The composer uses it to promise delivery, not folding.
	SupportsSteering bool `json:"supports_steering,omitempty"`
	// PendingAction is the compact per-session projection used when the
	// session transcript is not loaded in the client.
	PendingAction         *string                       `json:"pending_action"`
	PendingActionRevision *models.PendingActionRevision `json:"pending_action_revision,omitempty"`
	ActiveSubagentCount   int                           `json:"active_subagent_count"`
	// LastReadMessageID is the session's Slack-style read cursor — the id of
	// the newest message the frontend has marked as read. Used by the
	// transcript to position the unread ("New") divider.
	LastReadMessageID string `json:"last_read_message_id,omitempty"`
	// Usage/cost rollup (docs/specs/task-cost-ledger/spec.md AC-28, AC-29).
	// Deliberately not on TaskSessionSummaryDTO - the summary projection used
	// by cross-task views is not widened by this card.
	CostSubcents   int64 `json:"cost_subcents"`
	TokensIn       int64 `json:"tokens_in"`
	TokensCachedIn int64 `json:"tokens_cached_in"`
	TokensOut      int64 `json:"tokens_out"`
}

// TaskSessionSummaryDTO is a lightweight version of TaskSessionDTO without snapshot fields.
// Used for list endpoints where snapshots are not needed, reducing response size by ~40-60%.
type TaskSessionSummaryDTO struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	// Name is the user-supplied session tab label. Serialized without
	// omitempty so a cleared name ("") overwrites stale client state.
	Name              string `json:"name"`
	AgentExecutionID  string `json:"agent_execution_id,omitempty"`
	ContainerID       string `json:"container_id,omitempty"`
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ExecutorID        string `json:"executor_id,omitempty"`
	ExecutorProfileID string `json:"executor_profile_id,omitempty"`
	EnvironmentID     string `json:"environment_id,omitempty"`
	RepositoryID      string `json:"repository_id,omitempty"`
	BaseBranch        string `json:"base_branch,omitempty"`
	BaseCommitSHA     string `json:"base_commit_sha,omitempty"`
	WorktreeID        string `json:"worktree_id,omitempty"`
	WorktreePath      string `json:"worktree_path,omitempty"`
	WorktreeBranch    string `json:"worktree_branch,omitempty"`
	// WorkspacePath is the effective task root used by Files and chat links;
	// WorktreePath remains the flattened primary repository path.
	WorkspacePath string `json:"workspace_path,omitempty"`
	// Worktrees lists all session worktrees (one per repo on multi-repo tasks);
	// the flattened Worktree* fields above carry only the first for backward
	// compatibility. Entries use the stable session-worktree wire shape.
	Worktrees         []map[string]interface{} `json:"worktrees,omitempty"`
	State             models.TaskSessionState  `json:"state"`
	ErrorMessage      string                   `json:"error_message,omitempty"`
	Metadata          map[string]interface{}   `json:"metadata,omitempty"`
	StartedAt         time.Time                `json:"started_at"`
	CompletedAt       *time.Time               `json:"completed_at,omitempty"`
	UpdatedAt         time.Time                `json:"updated_at"`
	IsPrimary         bool                     `json:"is_primary"`
	IsPassthrough     bool                     `json:"is_passthrough"`
	ReviewStatus      models.ReviewStatus      `json:"review_status,omitempty"`
	TaskEnvironmentID string                   `json:"task_environment_id,omitempty"`
	// ForegroundActivity mirrors the in-memory fine-grained busy substate
	// (ADR-0049); see TaskSessionDTO.
	ForegroundActivity v1.ForegroundActivity `json:"foreground_activity,omitempty"`
	// CancellationPending mirrors the runtime cancellation projection and is
	// always serialized so false clears stale client state.
	CancellationPending bool `json:"cancellation_pending"`
	// CancellationRevision identifies the process-local cancellation transition
	// generation represented by CancellationPending.
	CancellationRevision uint64 `json:"cancellation_revision"`
	// SupportsSteering mirrors TaskSessionDTO.SupportsSteering for list endpoints.
	SupportsSteering bool `json:"supports_steering,omitempty"`
	// PendingAction is the compact per-session projection used when the
	// session transcript is not loaded in the client.
	PendingAction         *string                       `json:"pending_action"`
	PendingActionRevision *models.PendingActionRevision `json:"pending_action_revision,omitempty"`
	ActiveSubagentCount   int                           `json:"active_subagent_count"`
	LastReadMessageID     string                        `json:"last_read_message_id,omitempty"`
	// CommandCount is the number of tool_call messages on this session,
	// surfaced inline in the timeline entry header ("ran N commands").
	// Populated by ListTaskSessions; defaults to 0 for callers that don't
	// resolve it.
	CommandCount int `json:"command_count"`
}

// ListTaskSessionSummariesResponse is the list response using summary DTOs.
type ListTaskSessionSummariesResponse struct {
	Sessions []TaskSessionSummaryDTO `json:"sessions"`
	Total    int                     `json:"total"`
}

type GetTaskSessionResponse struct {
	Session TaskSessionDTO `json:"session"`
}

// MarkSessionReadResponse is intentionally narrow: it carries only the
// updated read cursor, never a full session snapshot. A full-session
// response would be frozen at request time and, if applied verbatim by a
// client, could overwrite a newer field (e.g. state) that arrived via a
// concurrent WebSocket update while this request was still in flight.
type MarkSessionReadResponse struct {
	SessionID         string `json:"session_id"`
	LastReadMessageID string `json:"last_read_message_id"`
}

type ListTaskSessionsResponse struct {
	Sessions []TaskSessionDTO `json:"sessions"`
	Total    int              `json:"total"`
}

type WorkflowSnapshotDTO struct {
	Workflow WorkflowDTO       `json:"workflow"`
	Steps    []WorkflowStepDTO `json:"steps"`
	Tasks    []TaskDTO         `json:"tasks"`
}

type ListMessagesResponse struct {
	Messages []*v1.Message `json:"messages"`
	Total    int           `json:"total"`
	HasMore  bool          `json:"has_more"`
	Cursor   string        `json:"cursor"`
}

// MessageSearchHit is a lightweight match returned by a session message search.
type MessageSearchHit struct {
	ID         string    `json:"id"`
	TurnID     string    `json:"turn_id,omitempty"`
	AuthorType string    `json:"author_type"`
	Type       string    `json:"type"`
	Snippet    string    `json:"snippet"`
	CreatedAt  time.Time `json:"created_at"`
}

// SearchMessagesResponse contains hits from a session message search.
type SearchMessagesResponse struct {
	Hits  []MessageSearchHit `json:"hits"`
	Total int                `json:"total"`
}

type TurnDTO struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	TaskID      string                 `json:"task_id"`
	StartedAt   string                 `json:"started_at"`
	CompletedAt *string                `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type ListTurnsResponse struct {
	Turns []TurnDTO `json:"turns"`
	Total int       `json:"total"`
}

type ListWorkflowsResponse struct {
	Workflows []WorkflowDTO `json:"workflows"`
	Total     int           `json:"total"`
}

type ListWorkspacesResponse struct {
	Workspaces []WorkspaceDTO `json:"workspaces"`
	Total      int            `json:"total"`
}

type ListRepositoriesResponse struct {
	Repositories []RepositoryDTO `json:"repositories"`
	Total        int             `json:"total"`
}

type ListRepositorySetsResponse struct {
	RepositorySets []RepositorySetDTO `json:"repository_sets"`
	Total          int                `json:"total"`
}

type ListRepositoryBranchPoliciesResponse struct {
	Policies []RepositoryBranchPolicyDTO `json:"repository_branch_policies"`
	Total    int                         `json:"total"`
}

type ListRepositoryScriptsResponse struct {
	Scripts []RepositoryScriptDTO `json:"scripts"`
	Total   int                   `json:"total"`
}

type ListExecutorsResponse struct {
	Executors []ExecutorDTO `json:"executors"`
	Total     int           `json:"total"`
}

type ListEnvironmentsResponse struct {
	Environments []EnvironmentDTO `json:"environments"`
	Total        int              `json:"total"`
}

type BranchDTO struct {
	Name   string `json:"name"`
	Type   string `json:"type"`   // "local" or "remote"
	Remote string `json:"remote"` // remote name (e.g., "origin") for remote branches
}

type RepositoryBranchesResponse struct {
	Branches      []BranchDTO `json:"branches"`
	Total         int         `json:"total"`
	CurrentBranch string      `json:"current_branch,omitempty"`
	// FetchedAt is the timestamp of the most recent `git fetch` for this
	// repository (RFC3339). Empty when no refresh has been requested in the
	// process lifetime.
	FetchedAt string `json:"fetched_at,omitempty"`
	// FetchError is the human-readable error from the last fetch attempt for
	// this request, if one was attempted and failed. Empty otherwise.
	FetchError string `json:"fetch_error,omitempty"`
}

// LocalRepositoryStatusResponse reports current branch + dirty paths for a
// local repository on disk (no session required). Used by the task-create
// dialog to preflight the fresh-branch flow on local executors.
type LocalRepositoryStatusResponse struct {
	CurrentBranch string   `json:"current_branch"`
	DirtyFiles    []string `json:"dirty_files"`
}

type LocalRepositoryDTO struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type RepositoryDiscoveryResponse struct {
	Roots        []string             `json:"roots"`
	Repositories []LocalRepositoryDTO `json:"repositories"`
	Total        int                  `json:"total"`
}

type RepositoryPathValidationResponse struct {
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	IsGitRepo     bool   `json:"is_git"`
	Allowed       bool   `json:"allowed"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Message       string `json:"message,omitempty"`
}

type ListTasksResponse struct {
	Tasks []TaskDTO `json:"tasks"`
	Total int       `json:"total"`
}

type SuccessResponse struct {
	Success bool `json:"success"`
}

func FromWorkflow(workflow *models.Workflow) WorkflowDTO {
	var description *string
	if workflow.Description != "" {
		description = &workflow.Description
	}
	var prompt *string
	if workflow.Prompt != "" {
		prompt = &workflow.Prompt
	}

	return WorkflowDTO{
		ID:             workflow.ID,
		WorkspaceID:    workflow.WorkspaceID,
		Name:           workflow.Name,
		Description:    description,
		Prompt:         prompt,
		AgentProfileID: workflow.AgentProfileID,
		SortOrder:      workflow.SortOrder,
		Hidden:         workflow.Hidden,
		Style:          workflow.Style,
		Source:         workflow.Source,
		SourcePath:     workflow.SourcePath,
		CreatedAt:      workflow.CreatedAt,
		UpdatedAt:      workflow.UpdatedAt,
	}
}

func FromWorkspace(workspace *models.Workspace) WorkspaceDTO {
	var description *string
	if workspace.Description != "" {
		description = &workspace.Description
	}

	return WorkspaceDTO{
		ID:                          workspace.ID,
		Name:                        workspace.Name,
		Description:                 description,
		OwnerID:                     workspace.OwnerID,
		DefaultExecutorID:           workspace.DefaultExecutorID,
		DefaultEnvironmentID:        workspace.DefaultEnvironmentID,
		DefaultAgentProfileID:       workspace.DefaultAgentProfileID,
		DefaultConfigAgentProfileID: workspace.DefaultConfigAgentProfileID,
		TaskPrefix:                  workspace.TaskPrefix,
		TaskSequence:                workspace.TaskSequence,
		OfficeWorkflowID:            workspace.OfficeWorkflowID,
		CreatedAt:                   workspace.CreatedAt,
		UpdatedAt:                   workspace.UpdatedAt,
	}
}

func FromRepository(repository *models.Repository) RepositoryDTO {
	bindings := make([]RepositorySecretBindingDTO, 0, len(repository.SecretBindings))
	for _, binding := range repository.SecretBindings {
		bindings = append(bindings, RepositorySecretBindingDTO{Key: binding.Key, SecretID: binding.SecretID})
	}
	return RepositoryDTO{
		ID:                     repository.ID,
		WorkspaceID:            repository.WorkspaceID,
		Name:                   repository.Name,
		SourceType:             repository.SourceType,
		LocalPath:              repository.LocalPath,
		Provider:               repository.Provider,
		ProviderRepoID:         repository.ProviderRepoID,
		ProviderHost:           repository.ProviderHost,
		ProviderScope:          repository.ProviderScope,
		ProviderOwner:          repository.ProviderOwner,
		ProviderName:           repository.ProviderName,
		RemoteURL:              repository.RemoteURL,
		DefaultBranch:          repository.DefaultBranch,
		WorktreeBranchPrefix:   repository.WorktreeBranchPrefix,
		WorktreeBranchTemplate: repository.WorktreeBranchTemplate,
		PullBeforeWorktree:     repository.PullBeforeWorktree,
		SetupScript:            repository.SetupScript,
		CleanupScript:          repository.CleanupScript,
		DevScript:              repository.DevScript,
		CopyFiles:              repository.CopyFiles,
		SecretBindings:         bindings,
		CreatedAt:              repository.CreatedAt,
		UpdatedAt:              repository.UpdatedAt,
	}
}

func FromRepositorySet(set *models.RepositorySet) RepositorySetDTO {
	items := make([]RepositorySetItemDTO, 0, len(set.Items))
	for _, item := range set.Items {
		items = append(items, RepositorySetItemDTO{
			RepositoryID: item.RepositoryID,
			Position:     item.Position,
		})
	}
	return RepositorySetDTO{
		ID:           set.ID,
		WorkspaceID:  set.WorkspaceID,
		Name:         set.Name,
		Description:  set.Description,
		Repositories: items,
		CreatedAt:    set.CreatedAt,
		UpdatedAt:    set.UpdatedAt,
	}
}

func FromRepositoryBranchPolicy(policy *models.RepositoryBranchPolicy) RepositoryBranchPolicyDTO {
	return RepositoryBranchPolicyDTO{
		ID: policy.ID, RepositoryID: policy.RepositoryID, Name: policy.Name,
		Description: policy.Description, BaseBranch: policy.BaseBranch,
		BranchTemplate: policy.BranchTemplate, PullRequestTarget: policy.PullRequestTarget,
		CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt,
	}
}

func FromRepositoryScript(script *models.RepositoryScript) RepositoryScriptDTO {
	return RepositoryScriptDTO{
		ID:           script.ID,
		RepositoryID: script.RepositoryID,
		Name:         script.Name,
		Command:      script.Command,
		Position:     script.Position,
		CreatedAt:    script.CreatedAt,
		UpdatedAt:    script.UpdatedAt,
	}
}

func FromExecutor(executor *models.Executor) ExecutorDTO {
	return ExecutorDTO{
		ID:        executor.ID,
		Name:      executor.Name,
		Type:      executor.Type,
		Status:    executor.Status,
		IsSystem:  executor.IsSystem,
		Resumable: executor.Resumable,
		Config:    executor.Config,
		CreatedAt: executor.CreatedAt,
		UpdatedAt: executor.UpdatedAt,
	}
}

func FromExecutorProfile(profile *models.ExecutorProfile) ExecutorProfileDTO {
	return ExecutorProfileDTO{
		ID:            profile.ID,
		ExecutorID:    profile.ExecutorID,
		Name:          profile.Name,
		McpPolicy:     profile.McpPolicy,
		Config:        profile.Config,
		PrepareScript: profile.PrepareScript,
		CleanupScript: profile.CleanupScript,
		EnvVars:       profile.EnvVars,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	}
}

// FromExecutorProfileWithExecutor converts an ExecutorProfile model to a DTO
// with executor type and name populated.
func FromExecutorProfileWithExecutor(profile *models.ExecutorProfile, executor *models.Executor) ExecutorProfileDTO {
	d := FromExecutorProfile(profile)
	if executor != nil {
		d.ExecutorType = string(executor.Type)
		d.ExecutorName = executor.Name
	}
	return d
}

func FromEnvironment(environment *models.Environment) EnvironmentDTO {
	return EnvironmentDTO{
		ID:           environment.ID,
		Name:         environment.Name,
		Kind:         environment.Kind,
		IsSystem:     environment.IsSystem,
		WorktreeRoot: environment.WorktreeRoot,
		ImageTag:     environment.ImageTag,
		Dockerfile:   environment.Dockerfile,
		BuildConfig:  environment.BuildConfig,
		CreatedAt:    environment.CreatedAt,
		UpdatedAt:    environment.UpdatedAt,
	}
}

func FromLocalRepository(repo service.LocalRepository) LocalRepositoryDTO {
	return LocalRepositoryDTO{
		Path:          repo.Path,
		Name:          repo.Name,
		DefaultBranch: repo.DefaultBranch,
	}
}

func FromTask(task *models.Task) TaskDTO {
	return FromTaskWithPrimarySession(task, nil)
}

// FromTaskWithPrimarySession converts a task model to a TaskDTO, including the primary session ID.
func FromTaskWithPrimarySession(task *models.Task, primarySessionID *string) TaskDTO {
	return FromTaskWithSessionInfo(task, primarySessionID, nil, models.ReviewStatusNone, nil, nil, nil, nil, nil, nil, nil)
}

// FromTaskWithSessionInfo converts a task model to a TaskDTO, including session information.
func FromTaskWithSessionInfo(
	task *models.Task,
	primarySessionID *string,
	sessionCount *int,
	reviewStatus models.ReviewStatus,
	primaryExecutorID *string,
	primaryExecutorType *string,
	primaryExecutorName *string,
	primaryAgentName *string,
	primaryWorkingDirectory *string,
	primarySessionState *string,
	primarySessionPendingAction *string,
) TaskDTO {
	// Convert repositories
	var repositories []TaskRepositoryDTO
	for _, repo := range task.Repositories {
		repositories = append(repositories, TaskRepositoryDTO{
			ID:                            repo.ID,
			TaskID:                        repo.TaskID,
			RepositoryID:                  repo.RepositoryID,
			BaseBranch:                    repo.BaseBranch,
			CheckoutBranch:                repo.CheckoutBranch,
			BranchPolicyID:                repo.BranchPolicyID,
			BranchPolicyName:              repo.BranchPolicyName,
			BranchPolicyBaseBranch:        repo.BranchPolicyBaseBranch,
			BranchPolicyBranchTemplate:    repo.BranchPolicyBranchTemplate,
			BranchPolicyPullRequestTarget: repo.BranchPolicyPullRequestTarget,
			Position:                      repo.Position,
			Metadata:                      repo.Metadata,
			CreatedAt:                     repo.CreatedAt,
			UpdatedAt:                     repo.UpdatedAt,
		})
	}
	var workspaceFolders []TaskWorkspaceFolderDTO
	for _, folder := range task.WorkspaceFolders {
		workspaceFolders = append(workspaceFolders, TaskWorkspaceFolderDTO{
			ID:          folder.ID,
			TaskID:      folder.TaskID,
			LocalPath:   folder.LocalPath,
			DisplayName: folder.DisplayName,
			Position:    folder.Position,
			CreatedAt:   folder.CreatedAt,
			UpdatedAt:   folder.UpdatedAt,
		})
	}

	return TaskDTO{
		ID:                          task.ID,
		WorkspaceID:                 task.WorkspaceID,
		WorkflowID:                  task.WorkflowID,
		WorkflowStepID:              task.WorkflowStepID,
		Title:                       task.Title,
		Description:                 task.Description,
		State:                       task.State,
		Priority:                    task.Priority,
		WIPAdmitted:                 task.WIPAdmitted,
		QueuedForStepID:             task.QueuedForStepID,
		QueuedAt:                    task.QueuedAt,
		Repositories:                repositories,
		WorkspaceFolders:            workspaceFolders,
		Position:                    task.Position,
		PrimarySessionID:            primarySessionID,
		SessionCount:                sessionCount,
		ReviewStatus:                reviewStatus,
		PrimaryExecutorID:           primaryExecutorID,
		PrimaryExecutorType:         primaryExecutorType,
		PrimaryExecutorName:         primaryExecutorName,
		PrimaryAgentName:            primaryAgentName,
		PrimaryWorkingDirectory:     primaryWorkingDirectory,
		PrimarySessionState:         primarySessionState,
		PrimarySessionPendingAction: primarySessionPendingAction,
		IsRemoteExecutor:            primaryExecutorType != nil && models.IsRemoteExecutorType(models.ExecutorType(*primaryExecutorType)),
		ParentID:                    task.ParentID,
		Autopilot:                   task.Autopilot,
		ArchivedAt:                  task.ArchivedAt,
		CreatedAt:                   task.CreatedAt,
		UpdatedAt:                   task.UpdatedAt,
		Metadata:                    models.PublicTaskMetadata(task.Metadata),
		Interrupted:                 task.Metadata[models.MetaKeyInterruptedAt] != nil,
		AutoStartFailed:             task.Metadata[models.MetaKeyAutoStartFailed] != nil,
		// Office extensions. AssigneeAgentProfileID is a read-time
		// projection from workflow_step_participants (ADR 0005 Wave F);
		// the repo's task SELECTs hydrate it via a correlated subquery.
		AssigneeAgentProfileID: task.AssigneeAgentProfileID,
		Origin:                 task.Origin,
		ProjectID:              task.ProjectID,
		Labels:                 task.Labels,
		Identifier:             task.Identifier,
		ExternalID:             task.ExternalID,
		IsFromOffice:           task.IsFromOffice,
	}
}

// FromTaskSessionSummary converts a session model to a summary DTO (no snapshot fields).
func FromTaskSessionSummary(session *models.TaskSession) TaskSessionSummaryDTO {
	result := TaskSessionSummaryDTO{
		ID:                session.ID,
		TaskID:            session.TaskID,
		Name:              session.Name,
		AgentExecutionID:  session.AgentExecutionID,
		ContainerID:       session.ContainerID,
		AgentProfileID:    session.AgentProfileID,
		ExecutorID:        session.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		EnvironmentID:     session.EnvironmentID,
		RepositoryID:      session.RepositoryID,
		BaseBranch:        session.BaseBranch,
		BaseCommitSHA:     session.BaseCommitSHA,
		WorkspacePath:     session.WorkspacePath,
		State:             session.State,
		ErrorMessage:      session.ErrorMessage,
		Metadata:          session.Metadata,
		StartedAt:         session.StartedAt,
		CompletedAt:       session.CompletedAt,
		UpdatedAt:         session.UpdatedAt,
		IsPrimary:         session.IsPrimary,
		IsPassthrough:     session.IsPassthrough,
		ReviewStatus:      session.ReviewStatus,
		TaskEnvironmentID: session.TaskEnvironmentID,
		LastReadMessageID: session.LastReadMessageID,
	}
	if worktrees := session.WorktreesAPI(); len(worktrees) > 0 {
		result.WorktreeID = session.Worktrees[0].WorktreeID
		result.WorktreePath = session.Worktrees[0].WorktreePath
		result.WorktreeBranch = session.Worktrees[0].WorktreeBranch
		result.Worktrees = worktrees
	}
	return result
}

func FromTaskSession(session *models.TaskSession) TaskSessionDTO {
	result := TaskSessionDTO{
		ID:                   session.ID,
		TaskID:               session.TaskID,
		Name:                 session.Name,
		AgentExecutionID:     session.AgentExecutionID,
		ContainerID:          session.ContainerID,
		AgentProfileID:       session.AgentProfileID,
		ExecutorID:           session.ExecutorID,
		ExecutorProfileID:    session.ExecutorProfileID,
		EnvironmentID:        session.EnvironmentID,
		RepositoryID:         session.RepositoryID,
		BaseBranch:           session.BaseBranch,
		BaseCommitSHA:        session.BaseCommitSHA,
		WorkspacePath:        session.WorkspacePath,
		State:                session.State,
		ErrorMessage:         session.ErrorMessage,
		Metadata:             session.Metadata,
		AgentProfileSnapshot: session.AgentProfileSnapshot,
		ExecutorSnapshot:     session.ExecutorSnapshot,
		EnvironmentSnapshot:  session.EnvironmentSnapshot,
		RepositorySnapshot:   session.RepositorySnapshot,
		StartedAt:            session.StartedAt,
		CompletedAt:          session.CompletedAt,
		UpdatedAt:            session.UpdatedAt,
		// Workflow fields
		IsPrimary:         session.IsPrimary,
		IsPassthrough:     session.IsPassthrough,
		ReviewStatus:      session.ReviewStatus,
		TaskEnvironmentID: session.TaskEnvironmentID,
		LastReadMessageID: session.LastReadMessageID,
		CostSubcents:      session.CostSubcents,
		TokensIn:          session.TokensIn,
		TokensCachedIn:    session.TokensCachedIn,
		TokensOut:         session.TokensOut,
	}
	if worktrees := session.WorktreesAPI(); len(worktrees) > 0 {
		result.WorktreeID = session.Worktrees[0].WorktreeID
		result.WorktreePath = session.Worktrees[0].WorktreePath
		result.WorktreeBranch = session.Worktrees[0].WorktreeBranch
		result.Worktrees = worktrees
	}
	return result
}

// ForegroundActivityProvider surfaces the in-memory fine-grained busy substate
// for a session (ADR-0049). It is satisfied by the
// orchestrator; the serialization layer depends only on this narrow seam so it
// takes no hard orchestrator dependency and can be faked in tests.
type ForegroundActivityProvider interface {
	ForegroundActivity(sessionID string) v1.ForegroundActivity
}

// CancellationPendingProvider surfaces the orchestrator's runtime cancellation
// projection without coupling task serialization to the orchestrator package.
type CancellationPendingProvider interface {
	CancellationPending(sessionID string) bool
}

// CancellationPendingSnapshotProvider is the atomic form of the cancellation
// projection used by serialization boundaries that need ordering identity.
// Implementations must read the boolean and revision from one critical section.
type CancellationPendingSnapshotProvider interface {
	CancellationPendingSnapshot(sessionID string) (pending bool, revision uint64)
}

type ActiveSubagentCountProvider interface {
	ActiveSubagentCount(sessionID string) int
}

// SteerEligibleProvider reports whether a send to a session right now would be
// delivered as a mid-turn steer. Optional: a provider that does not implement it
// simply never advertises steering, which is the conservative default.
type SteerEligibleProvider interface {
	SteerEligible(sessionID string, state models.TaskSessionState) bool
}

// EnrichForegroundActivity stamps the live fine-grained busy substate onto a full
// session DTO. Generating is emitted only for RUNNING sessions; detached
// background activity remains meaningful after the coarse state settles.
func EnrichForegroundActivity(dto *TaskSessionDTO, provider ForegroundActivityProvider) {
	if dto == nil || provider == nil {
		return
	}
	if activity, ok := sessionForegroundActivity(dto.ID, dto.State, provider); ok {
		dto.ForegroundActivity = activity
	}
	dto.ActiveSubagentCount = activeSubagentCount(dto.ID, provider)
	dto.SupportsSteering = steerEligible(dto.ID, dto.State, provider)
}

// EnrichForegroundActivitySummary is EnrichForegroundActivity for the lightweight
// summary DTO used by the list endpoints.
func EnrichForegroundActivitySummary(dto *TaskSessionSummaryDTO, provider ForegroundActivityProvider) {
	if dto == nil || provider == nil {
		return
	}
	if activity, ok := sessionForegroundActivity(dto.ID, dto.State, provider); ok {
		dto.ForegroundActivity = activity
	}
	dto.ActiveSubagentCount = activeSubagentCount(dto.ID, provider)
	dto.SupportsSteering = steerEligible(dto.ID, dto.State, provider)
}

func activeSubagentCount(sessionID string, provider ForegroundActivityProvider) int {
	countProvider, ok := provider.(ActiveSubagentCountProvider)
	if !ok {
		return 0
	}
	return countProvider.ActiveSubagentCount(sessionID)
}

// steerEligible reports whether the live provider would deliver a send to this
// session as a mid-turn steer. Derived here at the serialization boundary from
// the live in-memory provider and never persisted, so a restart with no
// connected execution serializes false.
func steerEligible(sessionID string, state models.TaskSessionState, provider ForegroundActivityProvider) bool {
	steerProvider, ok := provider.(SteerEligibleProvider)
	if !ok {
		return false
	}
	return steerProvider.SteerEligible(sessionID, state)
}

// WorkflowStepDTO represents a workflow step for API responses
type WorkflowStepDTO struct {
	ID                    string         `json:"id"`
	WorkflowID            string         `json:"workflow_id"`
	Name                  string         `json:"name"`
	Position              int            `json:"position"`
	Color                 string         `json:"color"`
	Prompt                string         `json:"prompt,omitempty"`
	Events                *StepEventsDTO `json:"events,omitempty"`
	AllowManualMove       bool           `json:"allow_manual_move"`
	IsStartStep           bool           `json:"is_start_step"`
	ShowInCommandPanel    bool           `json:"show_in_command_panel"`
	AutoArchiveAfterHours int            `json:"auto_archive_after_hours,omitempty"`
	AgentProfileID        string         `json:"agent_profile_id,omitempty"`
	WIPLimit              int            `json:"wip_limit"`
	PullFromStepID        string         `json:"pull_from_step_id,omitempty"`
	// StageType is a Phase 2 (ADR-0004) semantic hint for the frontend.
	// Allowed values: "work" | "review" | "approval" | "custom".
	StageType                  string    `json:"stage_type,omitempty"`
	AutoAdvanceRequiresSignal  bool      `json:"auto_advance_requires_signal"`
	CancelTriggersTurnComplete bool      `json:"cancel_triggers_turn_complete"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// StepEventsDTO represents step events for API responses
type StepEventsDTO struct {
	OnEnter             []StepActionDTO `json:"on_enter,omitempty"`
	OnTurnStart         []StepActionDTO `json:"on_turn_start,omitempty"`
	OnTurnComplete      []StepActionDTO `json:"on_turn_complete,omitempty"`
	OnExit              []StepActionDTO `json:"on_exit,omitempty"`
	OnComment           []StepActionDTO `json:"on_comment,omitempty"`
	OnBlockerResolved   []StepActionDTO `json:"on_blocker_resolved,omitempty"`
	OnChildrenCompleted []StepActionDTO `json:"on_children_completed,omitempty"`
	OnApprovalResolved  []StepActionDTO `json:"on_approval_resolved,omitempty"`
	OnHeartbeat         []StepActionDTO `json:"on_heartbeat,omitempty"`
	OnBudgetAlert       []StepActionDTO `json:"on_budget_alert,omitempty"`
	OnAgentError        []StepActionDTO `json:"on_agent_error,omitempty"`
}

// StepActionDTO represents a step action for API responses
type StepActionDTO struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// MoveTaskResponse includes the task and the target workflow step info
type MoveTaskResponse struct {
	Task         TaskDTO         `json:"task"`
	WorkflowStep WorkflowStepDTO `json:"workflow_step"`
}

// Session Workflow Review DTOs

// ApproveSessionRequest is the request to approve a session's current step
type ApproveSessionRequest struct {
	SessionID string `json:"-"` // From URL path
}

// ApproveSessionResponse is the response after approving a session
type ApproveSessionResponse struct {
	Success      bool            `json:"success"`
	Session      TaskSessionDTO  `json:"session"`
	WorkflowStep WorkflowStepDTO `json:"workflow_step,omitempty"` // New step after approval
}

// TaskPlanDTO represents a task plan for API responses
type TaskPlanDTO struct {
	ID                             string     `json:"id"`
	TaskID                         string     `json:"task_id"`
	Title                          string     `json:"title"`
	Content                        string     `json:"content"`
	CreatedBy                      string     `json:"created_by"`
	CreatedAt                      time.Time  `json:"created_at"`
	UpdatedAt                      time.Time  `json:"updated_at"`
	ImplementationStartedAt        *time.Time `json:"implementation_started_at,omitempty"`
	ImplementationStartedSessionID *string    `json:"implementation_started_session_id,omitempty"`
	ImplementationStartedBy        *string    `json:"implementation_started_by,omitempty"`
}

// TaskPlanFromModel converts a TaskPlan model to a TaskPlanDTO.
func TaskPlanFromModel(plan *models.TaskPlan) *TaskPlanDTO {
	if plan == nil {
		return nil
	}
	return &TaskPlanDTO{
		ID:                             plan.ID,
		TaskID:                         plan.TaskID,
		Title:                          plan.Title,
		Content:                        plan.Content,
		CreatedBy:                      plan.CreatedBy,
		CreatedAt:                      plan.CreatedAt,
		UpdatedAt:                      plan.UpdatedAt,
		ImplementationStartedAt:        plan.ImplementationStartedAt,
		ImplementationStartedSessionID: plan.ImplementationStartedSessionID,
		ImplementationStartedBy:        plan.ImplementationStartedBy,
	}
}

// TaskPlanRevisionDTO represents a plan revision for API responses.
// Content is optional so list responses can omit it (fetched on demand).
type TaskPlanRevisionDTO struct {
	ID             string `json:"id"`
	TaskID         string `json:"task_id"`
	RevisionNumber int    `json:"revision_number"`
	Title          string `json:"title"`
	Content        string `json:"content,omitempty"`
	// ContentLength is the character (rune) count of Content, computed here
	// before TaskPlanRevisionMetaFromModel blanks Content for list/WS payloads
	// — so list rows can show a size without fetching full content.
	ContentLength      int       `json:"content_length"`
	AuthorKind         string    `json:"author_kind"`
	AuthorName         string    `json:"author_name"`
	RevertOfRevisionID *string   `json:"revert_of_revision_id,omitempty"`
	WorkflowStepID     string    `json:"workflow_step_id,omitempty"`
	WorkflowStepName   string    `json:"workflow_step_name,omitempty"`
	WorkflowStepColor  string    `json:"workflow_step_color,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TaskPlanRevisionFromModel converts a TaskPlanRevision model with content.
func TaskPlanRevisionFromModel(rev *models.TaskPlanRevision) *TaskPlanRevisionDTO {
	if rev == nil {
		return nil
	}
	return &TaskPlanRevisionDTO{
		ID:                 rev.ID,
		TaskID:             rev.TaskID,
		RevisionNumber:     rev.RevisionNumber,
		Title:              rev.Title,
		Content:            rev.Content,
		ContentLength:      models.PlanContentLength(rev.Content),
		AuthorKind:         rev.AuthorKind,
		AuthorName:         rev.AuthorName,
		RevertOfRevisionID: rev.RevertOfRevisionID,
		WorkflowStepID:     rev.WorkflowStepID,
		WorkflowStepName:   rev.WorkflowStepName,
		WorkflowStepColor:  rev.WorkflowStepColor,
		CreatedAt:          rev.CreatedAt,
		UpdatedAt:          rev.UpdatedAt,
	}
}

// TaskPlanRevisionMetaFromModel converts without content (for list payloads/WS broadcasts).
func TaskPlanRevisionMetaFromModel(rev *models.TaskPlanRevision) *TaskPlanRevisionDTO {
	meta := TaskPlanRevisionFromModel(rev)
	if meta != nil {
		meta.Content = ""
	}
	return meta
}

const turnTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

// FromTurn converts a Turn model to a TurnDTO.
func FromTurn(turn *models.Turn) TurnDTO {
	var completedAt *string
	if turn.CompletedAt != nil {
		formatted := turn.CompletedAt.UTC().Format(turnTimestampLayout)
		completedAt = &formatted
	}

	return TurnDTO{
		ID:          turn.ID,
		SessionID:   turn.TaskSessionID,
		TaskID:      turn.TaskID,
		StartedAt:   turn.StartedAt.UTC().Format(turnTimestampLayout),
		CompletedAt: completedAt,
		Metadata:    turn.Metadata,
		CreatedAt:   turn.CreatedAt.UTC().Format(turnTimestampLayout),
		UpdatedAt:   turn.UpdatedAt.UTC().Format(turnTimestampLayout),
	}
}
