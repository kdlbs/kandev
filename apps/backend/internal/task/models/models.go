package models

import (
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Task represents a task in the database
type Task struct {
	ID           string                 `json:"id"`
	WorkspaceID  string                 `json:"workspace_id"`
	BoardID      string                 `json:"board_id"`
	ColumnID     string                 `json:"column_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	State        v1.TaskState           `json:"state"`
	Priority     int                    `json:"priority"`
	Position     int                    `json:"position"` // Order within column
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Repositories []*TaskRepository      `json:"repositories,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Board represents a Kanban board
type Board struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Workspace represents a workspace
type Workspace struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	OwnerID               string    `json:"owner_id"`
	DefaultExecutorID     *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string   `json:"default_agent_profile_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// Column represents a column in a board
type Column struct {
	ID        string       `json:"id"`
	BoardID   string       `json:"board_id"`
	Name      string       `json:"name"`
	Position  int          `json:"position"`
	State     v1.TaskState `json:"state"` // Maps column to task state
	Color     string       `json:"color"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// TaskRepository represents a repository associated with a task
type TaskRepository struct {
	ID           string                 `json:"id"`
	TaskID       string                 `json:"task_id"`
	RepositoryID string                 `json:"repository_id"`
	BaseBranch   string                 `json:"base_branch"`
	Position     int                    `json:"position"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
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
	// MessageTypeProgress is for progress updates
	MessageTypeProgress MessageType = "progress"
	// MessageTypeError is for error messages
	MessageTypeError MessageType = "error"
	// MessageTypeStatus is for status changes: started, completed, failed
	MessageTypeStatus MessageType = "status"
	// MessageTypePermissionRequest is for agent permission requests
	MessageTypePermissionRequest MessageType = "permission_request"
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

// ToAPI converts internal Message to API type
func (m *Message) ToAPI() *v1.Message {
	messageType := string(m.Type)
	if messageType == "" {
		messageType = string(MessageTypeMessage)
	}
	result := &v1.Message{
		ID:            m.ID,
		TaskSessionID: m.TaskSessionID,
		TaskID:        m.TaskID,
		TurnID:        m.TurnID,
		AuthorType:    string(m.AuthorType),
		AuthorID:      m.AuthorID,
		Content:       m.Content,
		Type:          messageType,
		Metadata:      m.Metadata,
		RequestsInput: m.RequestsInput,
		CreatedAt:     m.CreatedAt,
	}
	return result
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

// TaskSession represents a persistent agent execution session for a task.
// This replaces the in-memory TaskExecution tracking and survives backend restarts.
type TaskSession struct {
	ID                   string                 `json:"id"`
	TaskID               string                 `json:"task_id"`
	AgentExecutionID     string                 `json:"agent_execution_id"` // Docker container/agent execution
	ContainerID          string                 `json:"container_id"`       // Docker container ID for cleanup
	AgentProfileID       string                 `json:"agent_profile_id"`   // ID of the agent profile used
	ExecutorID           string                 `json:"executor_id"`
	EnvironmentID        string                 `json:"environment_id"`
	RepositoryID         string                 `json:"repository_id"`       // Primary repository (for backward compatibility)
	BaseBranch           string                 `json:"base_branch"`         // Primary base branch (for backward compatibility)
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
}

// ToAPI converts internal TaskSession to API type
// TODO: Add v1.TaskSession type to pkg/api/v1/
func (s *TaskSession) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                 s.ID,
		"task_id":            s.TaskID,
		"agent_execution_id": s.AgentExecutionID,
		"container_id":       s.ContainerID,
		"agent_profile_id":   s.AgentProfileID,
		"executor_id":        s.ExecutorID,
		"environment_id":     s.EnvironmentID,
		"repository_id":      s.RepositoryID,
		"base_branch":        s.BaseBranch,
		"worktrees":          s.Worktrees,
		"state":              string(s.State),
		"started_at":         s.StartedAt,
		"updated_at":         s.UpdatedAt,
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
	ExecutorTypeLocalPC      ExecutorType = "local_pc"
	ExecutorTypeLocalDocker  ExecutorType = "local_docker"
	ExecutorTypeRemoteDocker ExecutorType = "remote_docker"
	ExecutorTypeRemoteVPS    ExecutorType = "remote_vps"
	ExecutorTypeK8s          ExecutorType = "k8s"
)

const (
	ExecutorIDLocalPC      = "exec-local-pc"
	ExecutorIDLocalDocker  = "exec-local-docker"
	ExecutorIDRemoteDocker = "exec-remote-docker"
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
	ID               string     `json:"id"`
	SessionID        string     `json:"session_id"`
	TaskID           string     `json:"task_id"`
	ExecutorID       string     `json:"executor_id"`
	Runtime          string     `json:"runtime,omitempty"`
	Status           string     `json:"status"`
	Resumable        bool       `json:"resumable"`
	ResumeToken      string     `json:"resume_token,omitempty"`
	AgentExecutionID string     `json:"agent_execution_id,omitempty"`
	ContainerID      string     `json:"container_id,omitempty"`
	AgentctlURL      string     `json:"agentctl_url,omitempty"`
	AgentctlPort     int        `json:"agentctl_port,omitempty"`
	PID              int        `json:"pid,omitempty"`
	WorktreeID       string     `json:"worktree_id,omitempty"`
	WorktreePath     string     `json:"worktree_path,omitempty"`
	WorktreeBranch   string     `json:"worktree_branch,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
		BoardID:      t.BoardID,
		Title:        t.Title,
		Description:  t.Description,
		State:        t.State,
		Priority:     t.Priority,
		Repositories: repositories,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		Metadata:     t.Metadata,
	}
}
