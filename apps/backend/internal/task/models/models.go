package models

import (
	"errors"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrExecutorRunningNotFound is returned when no executor running record exists for a session.
var ErrExecutorRunningNotFound = errors.New("executor running not found")

// ListMessagesOptions defines pagination options for listing messages
type ListMessagesOptions struct {
	Limit  int
	Before string
	After  string
	Sort   string
}

// Task metadata keys used for deferred agent start (e.g., task.moved → handleTaskMovedNoSession).
const (
	MetaKeyAgentProfileID    = "agent_profile_id"
	MetaKeyExecutorProfileID = "executor_profile_id"
)

// Task represents a task in the database
type Task struct {
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspace_id"`
	WorkflowID     string                 `json:"workflow_id"`
	WorkflowStepID string                 `json:"workflow_step_id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	State          v1.TaskState           `json:"state"`
	Priority       int                    `json:"priority"`
	Position       int                    `json:"position"` // Order within workflow step
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Repositories   []*TaskRepository      `json:"repositories,omitempty"`
	IsEphemeral    bool                   `json:"is_ephemeral"`        // Ephemeral tasks are not shown in kanban, used for quick chat
	ParentID       string                 `json:"parent_id,omitempty"` // FK to parent task for subtasks
	ArchivedAt     *time.Time             `json:"archived_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// Workflow represents a task workflow
type Workflow struct {
	ID                 string    `json:"id"`
	WorkspaceID        string    `json:"workspace_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	AgentProfileID     string    `json:"agent_profile_id,omitempty"`
	WorkflowTemplateID *string   `json:"workflow_template_id,omitempty"`
	SortOrder          int       `json:"sort_order"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Workspace represents a workspace
type Workspace struct {
	ID                          string    `json:"id"`
	Name                        string    `json:"name"`
	Description                 string    `json:"description"`
	OwnerID                     string    `json:"owner_id"`
	DefaultExecutorID           *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID        *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID       *string   `json:"default_agent_profile_id,omitempty"`
	DefaultConfigAgentProfileID *string   `json:"default_config_agent_profile_id,omitempty"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

// TaskRepository represents a repository associated with a task
type TaskRepository struct {
	ID             string                 `json:"id"`
	TaskID         string                 `json:"task_id"`
	RepositoryID   string                 `json:"repository_id"`
	BaseBranch     string                 `json:"base_branch"`
	CheckoutBranch string                 `json:"checkout_branch,omitempty"`
	Position       int                    `json:"position"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// MessageAuthorType represents who authored a message
type MessageAuthorType string

const (
	// MessageAuthorUser indicates a message from a human user
	MessageAuthorUser MessageAuthorType = "user"
	// MessageAuthorAgent indicates a message from an AI agent
	MessageAuthorAgent MessageAuthorType = "agent"
)

// MessageType represents the type of message content
type MessageType string

const (
	// MessageTypeMessage is the default type for user/agent regular messages
	MessageTypeMessage MessageType = "message"
	// MessageTypeContent is for agent response content
	MessageTypeContent MessageType = "content"
	// MessageTypeToolCall is when agent uses a tool
	MessageTypeToolCall MessageType = "tool_call"
	// MessageTypeToolEdit is for file edit operations with diff visualization
	MessageTypeToolEdit MessageType = "tool_edit"
	// MessageTypeToolRead is for file read operations
	MessageTypeToolRead MessageType = "tool_read"
	// MessageTypeToolExecute is for command execution operations
	MessageTypeToolExecute MessageType = "tool_execute"
	// MessageTypeProgress is for progress updates
	MessageTypeProgress MessageType = "progress"
	// MessageTypeLog is for agent log messages (info, debug, warning, etc.)
	MessageTypeLog MessageType = "log"
	// MessageTypeError is for error messages
	MessageTypeError MessageType = "error"
	// MessageTypeStatus is for status changes: started, completed, failed
	MessageTypeStatus MessageType = "status"
	// MessageTypePermissionRequest is for agent permission requests
	MessageTypePermissionRequest MessageType = "permission_request"
	// MessageTypeClarificationRequest is for agent clarification questions
	MessageTypeClarificationRequest MessageType = "clarification_request"
	// MessageTypeScriptExecution is for setup/cleanup script execution messages
	MessageTypeScriptExecution MessageType = "script_execution"
	// MessageTypeThinking is for agent thinking/reasoning content
	MessageTypeThinking MessageType = "thinking"
	// MessageTypeAgentPlan is for agent native plan content (e.g. ExitPlanMode)
	MessageTypeAgentPlan MessageType = "agent_plan"
	// MessageTypeTodo is for agent todo/task list updates
	MessageTypeTodo MessageType = "todo"
)

// Message represents a message in a task session
type Message struct {
	ID            string                 `json:"id"`
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id,omitempty"`
	TurnID        string                 `json:"turn_id"` // FK to task_session_turns
	AuthorType    MessageAuthorType      `json:"author_type"`
	AuthorID      string                 `json:"author_id,omitempty"` // User ID or Agent Execution ID
	Content       string                 `json:"content"`
	Type          MessageType            `json:"type,omitempty"` // Defaults to "message"
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RequestsInput bool                   `json:"requests_input"` // True if agent is requesting user input
	CreatedAt     time.Time              `json:"created_at"`
}

// ToAPI converts internal Message to API type.
// Only true system-injected content (wrapped in <kandev-system> tags) is stripped
// from the visible content sent to the UI.
func (m *Message) ToAPI() *v1.Message {
	messageType := string(m.Type)
	if messageType == "" {
		messageType = string(MessageTypeMessage)
	}
	hasHidden := sysprompt.HasSystemContent(m.Content)
	meta := m.Metadata
	if hasHidden {
		if meta == nil {
			meta = make(map[string]interface{})
		} else {
			// Copy to avoid mutating original
			meta = copyMetadata(meta)
		}
		meta["has_hidden_prompts"] = true
	}
	result := &v1.Message{
		ID:            m.ID,
		TaskSessionID: m.TaskSessionID,
		TaskID:        m.TaskID,
		TurnID:        m.TurnID,
		AuthorType:    string(m.AuthorType),
		AuthorID:      m.AuthorID,
		Content:       sysprompt.StripSystemContent(m.Content),
		Type:          messageType,
		Metadata:      meta,
		RequestsInput: m.RequestsInput,
		CreatedAt:     m.CreatedAt,
	}
	if hasHidden {
		result.RawContent = m.Content
	}
	return result
}

// copyMetadata returns a shallow copy of a metadata map.
func copyMetadata(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	maps.Copy(cp, m)
	return cp
}

// Turn represents a single prompt/response cycle within a task session.
// A turn starts when a user sends a prompt and ends when the agent completes,
// cancels, or errors.
type Turn struct {
	ID            string                 `json:"id"`
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// TaskSessionState represents the state of an agent session
type TaskSessionState string

const (
	// TaskSessionStateCreated - session created but agent not started
	TaskSessionStateCreated TaskSessionState = "CREATED"
	// TaskSessionStateStarting - agent is starting up
	TaskSessionStateStarting TaskSessionState = "STARTING"
	// TaskSessionStateRunning - agent is actively running
	TaskSessionStateRunning TaskSessionState = "RUNNING"
	// TaskSessionStateWaitingForInput - agent waiting for user input
	TaskSessionStateWaitingForInput TaskSessionState = "WAITING_FOR_INPUT"
	// TaskSessionStateCompleted - agent finished successfully
	TaskSessionStateCompleted TaskSessionState = "COMPLETED"
	// TaskSessionStateFailed - agent failed with error
	TaskSessionStateFailed TaskSessionState = "FAILED"
	// TaskSessionStateCancelled - agent was manually stopped
	TaskSessionStateCancelled TaskSessionState = "CANCELLED"
)

// TaskSessionWorktree represents the association between a task session and a worktree
type TaskSessionWorktree struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	WorktreeID   string    `json:"worktree_id"`
	RepositoryID string    `json:"repository_id"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`

	// Worktree details stored on this association
	WorktreePath   string `json:"worktree_path,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
}

// SessionBranchInfo is a lightweight projection of a session with its worktree branch.
// Used by the PR watch reconciler to find sessions that may need PR watches.
type SessionBranchInfo struct {
	SessionID string
	TaskID    string
	Branch    string
}

// TaskSession represents a persistent agent execution session for a task.
// This replaces the in-memory TaskExecution tracking and survives backend restarts.
type TaskSession struct {
	ID                   string                 `json:"id"`
	TaskID               string                 `json:"task_id"`
	AgentExecutionID     string                 `json:"agent_execution_id"` // Docker container/agent execution
	ContainerID          string                 `json:"container_id"`       // Docker container ID for cleanup
	AgentProfileID       string                 `json:"agent_profile_id"`   // ID of the agent profile used
	ExecutorID           string                 `json:"executor_id"`
	ExecutorProfileID    string                 `json:"executor_profile_id"`
	EnvironmentID        string                 `json:"environment_id"`
	RepositoryID         string                 `json:"repository_id"`       // Primary repository (for backward compatibility)
	BaseBranch           string                 `json:"base_branch"`         // Primary base branch (for backward compatibility)
	BaseCommitSHA        string                 `json:"base_commit_sha"`     // Git commit SHA at session start (for cumulative diff)
	Worktrees            []*TaskSessionWorktree `json:"worktrees,omitempty"` // Associated worktrees
	AgentProfileSnapshot map[string]interface{} `json:"agent_profile_snapshot,omitempty"`
	ExecutorSnapshot     map[string]interface{} `json:"executor_snapshot,omitempty"`
	EnvironmentSnapshot  map[string]interface{} `json:"environment_snapshot,omitempty"`
	RepositorySnapshot   map[string]interface{} `json:"repository_snapshot,omitempty"`
	State                TaskSessionState       `json:"state"`
	ErrorMessage         string                 `json:"error_message,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	StartedAt            time.Time              `json:"started_at"`
	CompletedAt          *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt            time.Time              `json:"updated_at"`

	// Environment reference
	TaskEnvironmentID string `json:"task_environment_id,omitempty"` // FK to task_environments for shared env

	// Workflow-related fields
	IsPrimary     bool    `json:"is_primary"`              // Whether this is the primary session for the task
	IsPassthrough bool    `json:"is_passthrough"`          // Whether this session uses passthrough (PTY) mode
	ReviewStatus  *string `json:"review_status,omitempty"` // pending, approved
}

// ToAPI converts internal TaskSession to API type
// TODO: Add v1.TaskSession type to pkg/api/v1/
func (s *TaskSession) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                  s.ID,
		"task_id":             s.TaskID,
		"agent_execution_id":  s.AgentExecutionID,
		"container_id":        s.ContainerID,
		"agent_profile_id":    s.AgentProfileID,
		"executor_id":         s.ExecutorID,
		"executor_profile_id": s.ExecutorProfileID,
		"environment_id":      s.EnvironmentID,
		"repository_id":       s.RepositoryID,
		"base_branch":         s.BaseBranch,
		"base_commit_sha":     s.BaseCommitSHA,
		"worktrees":           s.Worktrees,
		"state":               string(s.State),
		"started_at":          s.StartedAt,
		"updated_at":          s.UpdatedAt,
	}
	// For backward compatibility, populate worktree_path and worktree_branch from first worktree
	if len(s.Worktrees) > 0 {
		result["worktree_path"] = s.Worktrees[0].WorktreePath
		result["worktree_branch"] = s.Worktrees[0].WorktreeBranch
	}
	if s.ErrorMessage != "" {
		result["error_message"] = s.ErrorMessage
	}
	if s.CompletedAt != nil {
		result["completed_at"] = s.CompletedAt
	}
	if s.Metadata != nil {
		result["metadata"] = s.Metadata
	}
	if s.AgentProfileSnapshot != nil {
		result["agent_profile_snapshot"] = s.AgentProfileSnapshot
	}
	if s.ExecutorSnapshot != nil {
		result["executor_snapshot"] = s.ExecutorSnapshot
	}
	if s.EnvironmentSnapshot != nil {
		result["environment_snapshot"] = s.EnvironmentSnapshot
	}
	if s.RepositorySnapshot != nil {
		result["repository_snapshot"] = s.RepositorySnapshot
	}
	result["is_passthrough"] = s.IsPassthrough
	if s.TaskEnvironmentID != "" {
		result["task_environment_id"] = s.TaskEnvironmentID
	}
	return result
}

// Repository represents a workspace repository
type Repository struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	SourceType  string `json:"source_type"`
	// LocalPath is the path to a local checkout; for provider-backed repos, this is
	// populated after the repo is cloned/synced on the agent host.
	LocalPath string `json:"local_path"`
	// Provider fields describe the upstream source (e.g. github/gitlab) for future syncing.
	Provider             string     `json:"provider"`
	ProviderRepoID       string     `json:"provider_repo_id"`
	ProviderOwner        string     `json:"provider_owner"`
	ProviderName         string     `json:"provider_name"`
	DefaultBranch        string     `json:"default_branch"`
	WorktreeBranchPrefix string     `json:"worktree_branch_prefix"`
	PullBeforeWorktree   bool       `json:"pull_before_worktree"`
	SetupScript          string     `json:"setup_script"`
	CleanupScript        string     `json:"cleanup_script"`
	DevScript            string     `json:"dev_script"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
}

// RepositoryScript represents a custom script for a repository
type RepositoryScript struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ExecutorType represents the executor runtime type.
type ExecutorType string

const (
	ExecutorTypeLocal        ExecutorType = "local"
	ExecutorTypeWorktree     ExecutorType = "worktree"
	ExecutorTypeLocalDocker  ExecutorType = "local_docker"
	ExecutorTypeRemoteDocker ExecutorType = "remote_docker"
	ExecutorTypeSprites      ExecutorType = "sprites"
	ExecutorTypeMockRemote   ExecutorType = "mock_remote"
)

// IsRemoteExecutorType reports whether the given executor type represents
// a remote execution environment.
func IsRemoteExecutorType(t ExecutorType) bool {
	switch t {
	case ExecutorTypeSprites, ExecutorTypeRemoteDocker, ExecutorTypeMockRemote:
		return true
	default:
		return false
	}
}

// IsContainerizedExecutorType reports whether the given executor type runs
// in a container/sandbox where shells must be executed inside the container
// via agentctl, not on the host.
func IsContainerizedExecutorType(t ExecutorType) bool {
	switch t {
	case ExecutorTypeLocalDocker, ExecutorTypeSprites, ExecutorTypeRemoteDocker, ExecutorTypeMockRemote:
		return true
	default:
		return false
	}
}

// IsAlwaysResumableRuntime reports whether the given runtime string represents
// an executor that can always be resumed even without an explicit resume token.
func IsAlwaysResumableRuntime(runtime string) bool {
	return ExecutorType(runtime) == ExecutorTypeSprites
}

const (
	ExecutorIDLocal       = "exec-local"
	ExecutorIDWorktree    = "exec-worktree"
	ExecutorIDLocalDocker = "exec-local-docker"
	ExecutorIDSprites     = "exec-sprites"
)

// ExecutorStatus represents executor availability.
type ExecutorStatus string

const (
	ExecutorStatusActive   ExecutorStatus = "active"
	ExecutorStatusDisabled ExecutorStatus = "disabled"
)

// Executor represents an execution target.
type Executor struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      ExecutorType      `json:"type"`
	Status    ExecutorStatus    `json:"status"`
	IsSystem  bool              `json:"is_system"`
	Resumable bool              `json:"resumable"`
	Config    map[string]string `json:"config,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	DeletedAt *time.Time        `json:"deleted_at,omitempty"`
}

// ExecutorRunning tracks an active executor instance for a session.
type ExecutorRunning struct {
	ID               string                 `json:"id"`
	SessionID        string                 `json:"session_id"`
	TaskID           string                 `json:"task_id"`
	ExecutorID       string                 `json:"executor_id"`
	Runtime          string                 `json:"runtime,omitempty"`
	Status           string                 `json:"status"`
	Resumable        bool                   `json:"resumable"`
	ResumeToken      string                 `json:"resume_token,omitempty"`
	LastMessageUUID  string                 `json:"last_message_uuid,omitempty"`
	AgentExecutionID string                 `json:"agent_execution_id,omitempty"`
	ContainerID      string                 `json:"container_id,omitempty"`
	AgentctlURL      string                 `json:"agentctl_url,omitempty"`
	AgentctlPort     int                    `json:"agentctl_port,omitempty"`
	PID              int                    `json:"pid,omitempty"`
	WorktreeID       string                 `json:"worktree_id,omitempty"`
	WorktreePath     string                 `json:"worktree_path,omitempty"`
	WorktreeBranch   string                 `json:"worktree_branch,omitempty"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	LastSeenAt       *time.Time             `json:"last_seen_at,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// ProfileEnvVar represents an environment variable for an executor profile.
// Either Value (plain text) or SecretID (reference to a secret) should be set, not both.
type ProfileEnvVar struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	SecretID string `json:"secret_id,omitempty"`
}

// ExecutorProfile represents a named configuration preset for an executor.
type ExecutorProfile struct {
	ID            string            `json:"id"`
	ExecutorID    string            `json:"executor_id"`
	Name          string            `json:"name"`
	McpPolicy     string            `json:"mcp_policy,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
	PrepareScript string            `json:"prepare_script,omitempty"`
	CleanupScript string            `json:"cleanup_script,omitempty"`
	EnvVars       []ProfileEnvVar   `json:"env_vars,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// EnvironmentKind represents the runtime type for environments.
type EnvironmentKind string

const (
	EnvironmentKindLocalPC     EnvironmentKind = "local_pc"
	EnvironmentKindDockerImage EnvironmentKind = "docker_image"
)

const (
	EnvironmentIDLocal = "env-local"
)

// Environment represents a runtime environment configuration.
type Environment struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Kind         EnvironmentKind   `json:"kind"`
	IsSystem     bool              `json:"is_system"`
	WorktreeRoot string            `json:"worktree_root,omitempty"`
	ImageTag     string            `json:"image_tag,omitempty"`
	Dockerfile   string            `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	DeletedAt    *time.Time        `json:"deleted_at,omitempty"`
}

// TaskEnvironmentStatus represents the lifecycle state of a task execution environment.
type TaskEnvironmentStatus string

const (
	TaskEnvironmentStatusCreating TaskEnvironmentStatus = "creating"
	TaskEnvironmentStatusReady    TaskEnvironmentStatus = "ready"
	TaskEnvironmentStatusStopped  TaskEnvironmentStatus = "stopped"
	TaskEnvironmentStatusFailed   TaskEnvironmentStatus = "failed"
)

// TaskEnvironment represents a per-task execution environment instance.
// It owns the workspace (worktree/container/sandbox) and the agentctl control server.
// Multiple sessions can share the same TaskEnvironment.
type TaskEnvironment struct {
	ID                string                `json:"id"`
	TaskID            string                `json:"task_id"`
	RepositoryID      string                `json:"repository_id"`
	ExecutorType      string                `json:"executor_type"`
	ExecutorID        string                `json:"executor_id"`
	ExecutorProfileID string                `json:"executor_profile_id"`
	AgentExecutionID  string                `json:"agent_execution_id"` // agentctl execution handle
	ControlPort       int                   `json:"control_port"`       // agentctl control port
	Status            TaskEnvironmentStatus `json:"status"`

	// Type-specific fields
	WorktreeID     string `json:"worktree_id,omitempty"`
	WorktreePath   string `json:"worktree_path,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	WorkspacePath  string `json:"workspace_path,omitempty"`
	ContainerID    string `json:"container_id,omitempty"`
	SandboxID      string `json:"sandbox_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToAPI converts internal TaskEnvironment to API map.
func (te *TaskEnvironment) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                  te.ID,
		"task_id":             te.TaskID,
		"repository_id":       te.RepositoryID,
		"executor_type":       te.ExecutorType,
		"executor_id":         te.ExecutorID,
		"executor_profile_id": te.ExecutorProfileID,
		"status":              string(te.Status),
		"workspace_path":      te.WorkspacePath,
		"created_at":          te.CreatedAt,
		"updated_at":          te.UpdatedAt,
	}
	if te.AgentExecutionID != "" {
		result["agent_execution_id"] = te.AgentExecutionID
	}
	if te.ControlPort != 0 {
		result["control_port"] = te.ControlPort
	}
	if te.WorktreeID != "" {
		result["worktree_id"] = te.WorktreeID
	}
	if te.WorktreePath != "" {
		result["worktree_path"] = te.WorktreePath
	}
	if te.WorktreeBranch != "" {
		result["worktree_branch"] = te.WorktreeBranch
	}
	if te.ContainerID != "" {
		result["container_id"] = te.ContainerID
	}
	if te.SandboxID != "" {
		result["sandbox_id"] = te.SandboxID
	}
	return result
}

// TaskPlan represents a plan associated with a task
type TaskPlan struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedBy string    `json:"created_by"` // "agent" or "user"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionFileReview tracks per-file review state within a session
type SessionFileReview struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	FilePath   string     `json:"file_path"`
	Reviewed   bool       `json:"reviewed"`
	DiffHash   string     `json:"diff_hash"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ToAPI converts internal Task to API type
func (t *Task) ToAPI() *v1.Task {
	// Convert TaskRepository models to API types
	var repositories []v1.TaskRepository
	for _, repo := range t.Repositories {
		repositories = append(repositories, v1.TaskRepository{
			ID:           repo.ID,
			TaskID:       repo.TaskID,
			RepositoryID: repo.RepositoryID,
			BaseBranch:   repo.BaseBranch,
			Position:     repo.Position,
			Metadata:     repo.Metadata,
			CreatedAt:    repo.CreatedAt,
			UpdatedAt:    repo.UpdatedAt,
		})
	}

	return &v1.Task{
		ID:           t.ID,
		WorkspaceID:  t.WorkspaceID,
		WorkflowID:   t.WorkflowID,
		Title:        t.Title,
		Description:  t.Description,
		State:        t.State,
		Priority:     t.Priority,
		Repositories: repositories,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		Metadata:     t.Metadata,
		IsEphemeral:  t.IsEphemeral,
		ParentID:     t.ParentID,
	}
}
