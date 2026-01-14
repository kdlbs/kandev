package models

import (
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Task represents a task in the database
type Task struct {
	ID            string                 `json:"id"`
	WorkspaceID   string                 `json:"workspace_id"`
	BoardID       string                 `json:"board_id"`
	ColumnID      string                 `json:"column_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	State         v1.TaskState           `json:"state"`
	Priority      int                    `json:"priority"`
	AgentType     string                 `json:"agent_type,omitempty"`
	RepositoryURL string                 `json:"repository_url,omitempty"`
	Branch        string                 `json:"branch,omitempty"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Position      int                    `json:"position"` // Order within column
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
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

// CommentAuthorType represents who authored a comment
type CommentAuthorType string

const (
	// CommentAuthorUser indicates a comment from a human user
	CommentAuthorUser CommentAuthorType = "user"
	// CommentAuthorAgent indicates a comment from an AI agent
	CommentAuthorAgent CommentAuthorType = "agent"
)

// CommentType represents the type of comment content
type CommentType string

const (
	// CommentTypeMessage is the default type for user/agent regular messages
	CommentTypeMessage CommentType = "message"
	// CommentTypeContent is for agent response content
	CommentTypeContent CommentType = "content"
	// CommentTypeToolCall is when agent uses a tool
	CommentTypeToolCall CommentType = "tool_call"
	// CommentTypeProgress is for progress updates
	CommentTypeProgress CommentType = "progress"
	// CommentTypeError is for error messages
	CommentTypeError CommentType = "error"
	// CommentTypeStatus is for status changes: started, completed, failed
	CommentTypeStatus CommentType = "status"
)

// Comment represents a comment on a task
type Comment struct {
	ID             string                 `json:"id"`
	TaskID         string                 `json:"task_id"`
	AuthorType     CommentAuthorType      `json:"author_type"`
	AuthorID       string                 `json:"author_id,omitempty"` // User ID or Agent Instance ID
	Content        string                 `json:"content"`
	Type           CommentType            `json:"type,omitempty"` // Defaults to "message" for backward compatibility
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	RequestsInput  bool                   `json:"requests_input"` // True if agent is requesting user input
	ACPSessionID   string                 `json:"acp_session_id,omitempty"`
	AgentSessionID string                 `json:"agent_session_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ToAPI converts internal Comment to API type
func (c *Comment) ToAPI() *v1.Comment {
	commentType := string(c.Type)
	if commentType == "" {
		commentType = string(CommentTypeMessage)
	}
	result := &v1.Comment{
		ID:            c.ID,
		TaskID:        c.TaskID,
		AuthorType:    string(c.AuthorType),
		AuthorID:      c.AuthorID,
		Content:       c.Content,
		Type:          commentType,
		Metadata:      c.Metadata,
		RequestsInput: c.RequestsInput,
		CreatedAt:     c.CreatedAt,
	}
	if c.AgentSessionID != "" {
		result.AgentSessionID = c.AgentSessionID
	}
	return result
}

// AgentSessionStatus represents the status of an agent session
type AgentSessionStatus string

const (
	// AgentSessionStatusPending - session created but agent not started
	AgentSessionStatusPending AgentSessionStatus = "pending"
	// AgentSessionStatusRunning - agent is actively running
	AgentSessionStatusRunning AgentSessionStatus = "running"
	// AgentSessionStatusWaiting - agent waiting for user input
	AgentSessionStatusWaiting AgentSessionStatus = "waiting"
	// AgentSessionStatusCompleted - agent finished successfully
	AgentSessionStatusCompleted AgentSessionStatus = "completed"
	// AgentSessionStatusFailed - agent failed with error
	AgentSessionStatusFailed AgentSessionStatus = "failed"
	// AgentSessionStatusStopped - agent was manually stopped
	AgentSessionStatusStopped AgentSessionStatus = "stopped"
)

// AgentSession represents a persistent agent execution session for a task.
// This replaces the in-memory TaskExecution tracking and survives backend restarts.
type AgentSession struct {
	ID              string                 `json:"id"`
	TaskID          string                 `json:"task_id"`
	AgentInstanceID string                 `json:"agent_instance_id"` // Docker container/agent instance
	ContainerID     string                 `json:"container_id"`      // Docker container ID for cleanup
	AgentType       string                 `json:"agent_type"`        // e.g., "augment-agent"
	ACPSessionID    string                 `json:"acp_session_id"`    // ACP protocol session for resumption
	ExecutorID      string                 `json:"executor_id"`
	EnvironmentID   string                 `json:"environment_id"`
	Status          AgentSessionStatus     `json:"status"`
	Progress        int                    `json:"progress"` // 0-100
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	StartedAt       time.Time              `json:"started_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ToAPI converts internal AgentSession to API type
// TODO: Add v1.AgentSession type to pkg/api/v1/
func (s *AgentSession) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                s.ID,
		"task_id":           s.TaskID,
		"agent_instance_id": s.AgentInstanceID,
		"container_id":      s.ContainerID,
		"agent_type":        s.AgentType,
		"acp_session_id":    s.ACPSessionID,
		"executor_id":       s.ExecutorID,
		"environment_id":    s.EnvironmentID,
		"status":            string(s.Status),
		"progress":          s.Progress,
		"started_at":        s.StartedAt,
		"updated_at":        s.UpdatedAt,
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
	Provider       string    `json:"provider"`
	ProviderRepoID string    `json:"provider_repo_id"`
	ProviderOwner  string    `json:"provider_owner"`
	ProviderName   string    `json:"provider_name"`
	DefaultBranch  string    `json:"default_branch"`
	SetupScript    string    `json:"setup_script"`
	CleanupScript  string    `json:"cleanup_script"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	Config    map[string]string `json:"config,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
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
}

// ToAPI converts internal Task to API type
func (t *Task) ToAPI() *v1.Task {
	var agentType *string
	if t.AgentType != "" {
		agentType = &t.AgentType
	}

	var repositoryURL *string
	if t.RepositoryURL != "" {
		repositoryURL = &t.RepositoryURL
	}

	var branch *string
	if t.Branch != "" {
		branch = &t.Branch
	}

	var assignedAgentID *string
	if t.AssignedTo != "" {
		assignedAgentID = &t.AssignedTo
	}

	return &v1.Task{
		ID:              t.ID,
		WorkspaceID:     t.WorkspaceID,
		BoardID:         t.BoardID,
		Title:           t.Title,
		Description:     t.Description,
		State:           t.State,
		Priority:        t.Priority,
		AgentType:       agentType,
		RepositoryURL:   repositoryURL,
		Branch:          branch,
		AssignedAgentID: assignedAgentID,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		Metadata:        t.Metadata,
	}
}
