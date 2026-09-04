package service

import (
	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/task/models"
)

// Request types

// TaskRepositoryInput for creating/updating task repositories
type TaskRepositoryInput struct {
	RepositoryID   string `json:"repository_id"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch,omitempty"`
	BranchPolicyID string `json:"branch_policy_id,omitempty"`
	PRNumber       int    `json:"pr_number,omitempty"` // GitHub PR number when CheckoutBranch is a PR head; persisted into task_repositories.metadata["pr_number"].
	LocalPath      string `json:"local_path,omitempty"`
	Name           string `json:"name,omitempty"`
	DefaultBranch  string `json:"default_branch,omitempty"`
	GitHubURL      string `json:"github_url,omitempty"`
	RemoteURL      string `json:"remote_url,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ProviderHost   string `json:"provider_host,omitempty"`
	ProviderScope  string `json:"provider_scope,omitempty"`
	ProviderRepoID string `json:"provider_repo_id,omitempty"`
	ProviderOwner  string `json:"provider_owner,omitempty"`
	ProviderName   string `json:"provider_name,omitempty"`

	// PreserveBaseBranch keeps an effective branch produced after policy
	// resolution (for example, the branch created by the local fresh-branch
	// flow) when task repositories are recreated. It is internal-only: normal
	// task creation must always anchor a policy-backed repository to the
	// policy's immutable base branch.
	PreserveBaseBranch bool `json:"-"`

	// RemoteContribution is server-authored after provider resolution. It is
	// intentionally excluded from JSON request surfaces; callers must not be
	// able to forge a writable source binding.
	RemoteContribution *models.RemoteContribution `json:"-"`
	// ContributionDestination is server-authored during managed Improve
	// Kandev task preparation. It is never accepted from REST, WebSocket, or
	// MCP JSON request bodies.
	ContributionDestination *models.ContributionDestination `json:"-"`
	TrustedRemote           bool                            `json:"-"`

	// ResolveProviderDefaults opts the GitHub-URL resolution path into a
	// synchronous default-branch probe (git ls-remote --symref) when neither
	// the input nor the existing workspace repo carries one. Set only by
	// callers that have no downstream backfill (e.g. add_branch_to_task on a
	// live worktree-executor task — no executor relaunch will run
	// backfillRepoDefaultBranch). Left zero by create_task so the pinned
	// "empty default_branch is filled at clone time" contract stays intact.
	ResolveProviderDefaults bool `json:"-"`

	// TrustedProviderDescriptor is an internal-only marker for a complete
	// provider descriptor already authorized by the plugin host. It is never
	// accepted from REST/MCP JSON; callers must still supply every identity and
	// exact credential-free clone URL field above.
	//
	// A descriptor that arrives without this marker uses the normal built-in
	// resolver. Plugin descriptors must come through the authenticated plugin
	// Host Tasks.Create path, which sets this marker only after validating the
	// active plugin's provider ownership and clone origin.
	TrustedProviderDescriptor bool                           `json:"-"`
	BranchPolicySnapshot      *models.RepositoryBranchPolicy `json:"-"`
}

// CreateTaskRequest contains the data for creating a new task
type CreateTaskRequest struct {
	WorkspaceID    string                 `json:"workspace_id"`
	WorkflowID     string                 `json:"workflow_id"`
	WorkflowStepID string                 `json:"workflow_step_id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	AutoTitle      bool                   `json:"auto_title,omitempty"`
	Priority       string                 `json:"priority"`
	State          *v1.TaskState          `json:"state,omitempty"`
	Repositories   []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position       int                    `json:"position"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	DeferredLaunch map[string]interface{} `json:"deferred_launch,omitempty"`
	// RecordAgentProfileRecentUse opts this deferred launch into task_create
	// profile-history attribution. Only the authenticated HTTP/WS selector
	// surfaces set it; programmatic callers such as MCP must leave it false.
	RecordAgentProfileRecentUse bool `json:"-"`
	PlanMode                    bool `json:"plan_mode,omitempty"`

	// StartAgent reports that the caller intends to launch an agent for this
	// task right away. It only steers step resolution (see resolveWorkflowStep)
	// — the launch itself is the caller's job, after CreateTask returns.
	StartAgent    bool   `json:"start_agent,omitempty"`
	IsEphemeral   bool   `json:"is_ephemeral,omitempty"` // Ephemeral tasks are hidden from kanban, used for quick chat
	ParentID      string `json:"parent_id,omitempty"`
	Autopilot     bool   `json:"autopilot,omitempty"`
	WorkspacePath string `json:"workspace_path,omitempty"` // Optional host folder for repo-less tasks

	// ExternalID is a caller-supplied identity used for create-idempotency
	// (docs/specs/tasks/requirements/external-id-idempotency.md). Accepted on REST
	// and MCP; empty means no idempotency key.
	ExternalID string `json:"external_id,omitempty"`

	// Office extensions
	AssigneeAgentProfileID string   `json:"assignee_agent_profile_id,omitempty"`
	Origin                 string   `json:"origin,omitempty"`
	ProjectID              string   `json:"project_id,omitempty"`
	Labels                 string   `json:"labels,omitempty"`
	BlockedBy              []string `json:"blocked_by,omitempty"`

	// StartWhenUnblocked records the requested agent start as a deferred launch
	// intent that dependency resolution consumes, instead of launching now.
	//
	// nil means "derive from the request's start intent": a create that asked to
	// start an agent AND declared BlockedBy is describing a chain step, not a
	// task to run immediately. Automated callers pass start_agent=true by habit,
	// so deriving is what makes an agent-built chain run in order rather than
	// launching every step at once.
	StartWhenUnblocked *bool `json:"start_when_unblocked,omitempty"`
}

// UpdateTaskRequest contains the data for updating a task
type UpdateTaskRequest struct {
	Title          *string                `json:"title,omitempty"`
	Description    *string                `json:"description,omitempty"`
	Priority       *string                `json:"priority,omitempty"`
	State          *v1.TaskState          `json:"state,omitempty"`
	WorkflowStepID *string                `json:"workflow_step_id,omitempty"`
	Repositories   []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position       *int                   `json:"position,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	// ParentID nests the task under another task ("subtask"). A nil pointer
	// leaves the relationship untouched; a pointer to "" clears it (un-nests
	// back to a root task); a non-empty value nests it under that parent.
	ParentID *string `json:"parent_id,omitempty"`
}

// CreateWorkflowRequest contains the data for creating a new workflow
type CreateWorkflowRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	Prompt             string  `json:"prompt,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
	// Hidden marks the workflow as system-only; excluded from management UI and pickers.
	Hidden bool `json:"hidden,omitempty"`
}

// UpdateWorkflowRequest contains the data for updating a workflow
type UpdateWorkflowRequest struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	Prompt         *string `json:"prompt,omitempty"`
	AgentProfileID *string `json:"agent_profile_id,omitempty"`
}

// CreateWorkspaceRequest contains the data for creating a new workspace
type CreateWorkspaceRequest struct {
	Name                        string  `json:"name"`
	Description                 string  `json:"description"`
	OwnerID                     string  `json:"owner_id"`
	DefaultExecutorID           *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID        *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID       *string `json:"default_agent_profile_id,omitempty"`
	DefaultConfigAgentProfileID *string `json:"default_config_agent_profile_id,omitempty"`
	BootstrapKanbanWorkflow     bool
}

// UpdateWorkspaceRequest contains the data for updating a workspace
type UpdateWorkspaceRequest struct {
	Name                        *string `json:"name,omitempty"`
	Description                 *string `json:"description,omitempty"`
	DefaultExecutorID           *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID        *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID       *string `json:"default_agent_profile_id,omitempty"`
	DefaultConfigAgentProfileID *string `json:"default_config_agent_profile_id,omitempty"`
}

// FindOrCreateRepositoryRequest contains the data for finding or creating a repository by provider info.
type FindOrCreateRepositoryRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Provider       string `json:"provider"`
	ProviderHost   string `json:"provider_host"`
	ProviderScope  string `json:"provider_scope"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	ProviderRepoID string `json:"provider_repo_id"`
	RemoteURL      string `json:"remote_url"`
	DefaultBranch  string `json:"default_branch"`
	LocalPath      string `json:"local_path"`
}

// CreateRepositoryRequest contains the data for creating a new repository
type CreateRepositoryRequest struct {
	WorkspaceID            string                         `json:"workspace_id"`
	Name                   string                         `json:"name"`
	SourceType             string                         `json:"source_type"`
	LocalPath              string                         `json:"local_path"`
	Provider               string                         `json:"provider"`
	ProviderRepoID         string                         `json:"provider_repo_id"`
	ProviderHost           string                         `json:"provider_host"`
	ProviderScope          string                         `json:"provider_scope"`
	ProviderOwner          string                         `json:"provider_owner"`
	ProviderName           string                         `json:"provider_name"`
	RemoteURL              string                         `json:"remote_url"`
	DefaultBranch          string                         `json:"default_branch"`
	WorktreeBranchPrefix   string                         `json:"worktree_branch_prefix"`
	WorktreeBranchTemplate string                         `json:"worktree_branch_template"`
	PullBeforeWorktree     *bool                          `json:"pull_before_worktree"`
	SetupScript            string                         `json:"setup_script"`
	CleanupScript          string                         `json:"cleanup_script"`
	DevScript              string                         `json:"dev_script"`
	CopyFiles              string                         `json:"copy_files"`
	SecretBindings         []RepositorySecretBindingInput `json:"secret_bindings,omitempty"`
}

// RepositorySecretBindingInput is the write-only reference shape accepted by
// repository mutations. Secret values never cross this boundary.
type RepositorySecretBindingInput struct {
	Key      string `json:"key"`
	SecretID string `json:"secret_id"`
}

// InitializeLocalRepositoryRequest contains the data for creating and registering a new local repository.
type InitializeLocalRepositoryRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	ParentPath  string `json:"parent_path"`
}

// UpdateRepositoryRequest contains the data for updating a repository
type UpdateRepositoryRequest struct {
	Name                   *string `json:"name,omitempty"`
	SourceType             *string `json:"source_type,omitempty"`
	LocalPath              *string `json:"local_path,omitempty"`
	Provider               *string `json:"provider,omitempty"`
	ProviderRepoID         *string `json:"provider_repo_id,omitempty"`
	ProviderHost           *string `json:"provider_host,omitempty"`
	ProviderScope          *string `json:"provider_scope,omitempty"`
	ProviderOwner          *string `json:"provider_owner,omitempty"`
	ProviderName           *string `json:"provider_name,omitempty"`
	DefaultBranch          *string `json:"default_branch,omitempty"`
	WorktreeBranchPrefix   *string `json:"worktree_branch_prefix,omitempty"`
	WorktreeBranchTemplate *string `json:"worktree_branch_template,omitempty"`
	PullBeforeWorktree     *bool   `json:"pull_before_worktree,omitempty"`
	SetupScript            *string `json:"setup_script,omitempty"`
	CleanupScript          *string `json:"cleanup_script,omitempty"`
	DevScript              *string `json:"dev_script,omitempty"`
	CopyFiles              *string `json:"copy_files,omitempty"`
	// SecretBindings uses nil to preserve the current set and a non-nil empty
	// slice to clear it.
	SecretBindings *[]RepositorySecretBindingInput `json:"secret_bindings,omitempty"`
}

// CreateExecutorRequest contains the data for creating an executor
type CreateExecutorRequest struct {
	Name      string                `json:"name"`
	Type      models.ExecutorType   `json:"type"`
	Status    models.ExecutorStatus `json:"status"`
	IsSystem  bool                  `json:"is_system"`
	Resumable bool                  `json:"resumable"`
	Config    map[string]string     `json:"config,omitempty"`
}

// UpdateExecutorRequest contains the data for updating an executor
type UpdateExecutorRequest struct {
	Name      *string                `json:"name,omitempty"`
	Type      *models.ExecutorType   `json:"type,omitempty"`
	Status    *models.ExecutorStatus `json:"status,omitempty"`
	Resumable *bool                  `json:"resumable,omitempty"`
	Config    map[string]string      `json:"config,omitempty"`
}

// CreateExecutorProfileRequest contains the data for creating an executor profile
type CreateExecutorProfileRequest struct {
	ExecutorID    string                 `json:"executor_id"`
	Name          string                 `json:"name"`
	McpPolicy     string                 `json:"mcp_policy"`
	Config        map[string]string      `json:"config,omitempty"`
	PrepareScript string                 `json:"prepare_script"`
	CleanupScript string                 `json:"cleanup_script"`
	EnvVars       []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

// UpdateExecutorProfileRequest contains the data for updating an executor profile
type UpdateExecutorProfileRequest struct {
	Name          *string                `json:"name,omitempty"`
	McpPolicy     *string                `json:"mcp_policy,omitempty"`
	Config        map[string]string      `json:"config,omitempty"`
	PrepareScript *string                `json:"prepare_script,omitempty"`
	CleanupScript *string                `json:"cleanup_script,omitempty"`
	EnvVars       []models.ProfileEnvVar `json:"env_vars,omitempty"`
}

// CreateEnvironmentRequest contains the data for creating an environment
type CreateEnvironmentRequest struct {
	Name         string                 `json:"name"`
	Kind         models.EnvironmentKind `json:"kind"`
	WorktreeRoot string                 `json:"worktree_root,omitempty"`
	ImageTag     string                 `json:"image_tag,omitempty"`
	Dockerfile   string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string      `json:"build_config,omitempty"`
}

// UpdateEnvironmentRequest contains the data for updating an environment
type UpdateEnvironmentRequest struct {
	Name         *string                 `json:"name,omitempty"`
	Kind         *models.EnvironmentKind `json:"kind,omitempty"`
	WorktreeRoot *string                 `json:"worktree_root,omitempty"`
	ImageTag     *string                 `json:"image_tag,omitempty"`
	Dockerfile   *string                 `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string       `json:"build_config,omitempty"`
}

// ListMessagesRequest contains options for paginated message listing
type ListMessagesRequest struct {
	TaskSessionID string
	Limit         int
	Before        string
	After         string
	Sort          string
}

// CreateRepositoryScriptRequest contains the data for creating a repository script
type CreateRepositoryScriptRequest struct {
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	Command      string `json:"command"`
	Position     int    `json:"position"`
}

// UpdateRepositoryScriptRequest contains the data for updating a repository script
type UpdateRepositoryScriptRequest struct {
	Name     *string `json:"name,omitempty"`
	Command  *string `json:"command,omitempty"`
	Position *int    `json:"position,omitempty"`
}

// CreateMessageRequest contains the data for creating a new message
type CreateMessageRequest struct {
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id,omitempty"`
	TurnID        string                 `json:"turn_id"`
	CompletedTurn bool                   `json:"-"`
	Content       string                 `json:"content"`
	AuthorType    string                 `json:"author_type,omitempty"` // "user" or "agent", defaults to "user"
	AuthorID      string                 `json:"author_id,omitempty"`
	RequestsInput bool                   `json:"requests_input,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}
