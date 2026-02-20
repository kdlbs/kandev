package service

import (
	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/task/models"
)

// Request types

// TaskRepositoryInput for creating/updating task repositories
type TaskRepositoryInput struct {
	RepositoryID  string `json:"repository_id"`
	BaseBranch    string `json:"base_branch"`
	LocalPath     string `json:"local_path,omitempty"`
	Name          string `json:"name,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// CreateTaskRequest contains the data for creating a new task
type CreateTaskRequest struct {
	WorkspaceID    string                 `json:"workspace_id"`
	WorkflowID     string                 `json:"workflow_id"`
	WorkflowStepID string                 `json:"workflow_step_id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Priority       int                    `json:"priority"`
	State          *v1.TaskState          `json:"state,omitempty"`
	Repositories   []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position       int                    `json:"position"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest contains the data for updating a task
type UpdateTaskRequest struct {
	Title        *string                `json:"title,omitempty"`
	Description  *string                `json:"description,omitempty"`
	Priority     *int                   `json:"priority,omitempty"`
	State        *v1.TaskState          `json:"state,omitempty"`
	Repositories []TaskRepositoryInput  `json:"repositories,omitempty"`
	Position     *int                   `json:"position,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CreateWorkflowRequest contains the data for creating a new workflow
type CreateWorkflowRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

// UpdateWorkflowRequest contains the data for updating a workflow
type UpdateWorkflowRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateWorkspaceRequest contains the data for creating a new workspace
type CreateWorkspaceRequest struct {
	Name                  string  `json:"name"`
	Description           string  `json:"description"`
	OwnerID               string  `json:"owner_id"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}

// UpdateWorkspaceRequest contains the data for updating a workspace
type UpdateWorkspaceRequest struct {
	Name                  *string `json:"name,omitempty"`
	Description           *string `json:"description,omitempty"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}

// CreateRepositoryRequest contains the data for creating a new repository
type CreateRepositoryRequest struct {
	WorkspaceID          string `json:"workspace_id"`
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	LocalPath            string `json:"local_path"`
	Provider             string `json:"provider"`
	ProviderRepoID       string `json:"provider_repo_id"`
	ProviderOwner        string `json:"provider_owner"`
	ProviderName         string `json:"provider_name"`
	DefaultBranch        string `json:"default_branch"`
	WorktreeBranchPrefix string `json:"worktree_branch_prefix"`
	PullBeforeWorktree   *bool  `json:"pull_before_worktree"`
	SetupScript          string `json:"setup_script"`
	CleanupScript        string `json:"cleanup_script"`
	DevScript            string `json:"dev_script"`
}

// UpdateRepositoryRequest contains the data for updating a repository
type UpdateRepositoryRequest struct {
	Name                 *string `json:"name,omitempty"`
	SourceType           *string `json:"source_type,omitempty"`
	LocalPath            *string `json:"local_path,omitempty"`
	Provider             *string `json:"provider,omitempty"`
	ProviderRepoID       *string `json:"provider_repo_id,omitempty"`
	ProviderOwner        *string `json:"provider_owner,omitempty"`
	ProviderName         *string `json:"provider_name,omitempty"`
	DefaultBranch        *string `json:"default_branch,omitempty"`
	WorktreeBranchPrefix *string `json:"worktree_branch_prefix,omitempty"`
	PullBeforeWorktree   *bool   `json:"pull_before_worktree,omitempty"`
	SetupScript          *string `json:"setup_script,omitempty"`
	CleanupScript        *string `json:"cleanup_script,omitempty"`
	DevScript            *string `json:"dev_script,omitempty"`
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
	ExecutorID  string            `json:"executor_id"`
	Name        string            `json:"name"`
	IsDefault   bool              `json:"is_default"`
	Config      map[string]string `json:"config,omitempty"`
	SetupScript string            `json:"setup_script"`
}

// UpdateExecutorProfileRequest contains the data for updating an executor profile
type UpdateExecutorProfileRequest struct {
	Name        *string           `json:"name,omitempty"`
	IsDefault   *bool             `json:"is_default,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	SetupScript *string           `json:"setup_script,omitempty"`
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
	Content       string                 `json:"content"`
	AuthorType    string                 `json:"author_type,omitempty"` // "user" or "agent", defaults to "user"
	AuthorID      string                 `json:"author_id,omitempty"`
	RequestsInput bool                   `json:"requests_input,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}
