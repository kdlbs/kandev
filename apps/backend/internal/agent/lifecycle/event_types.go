// Package lifecycle provides event payload types for agent lifecycle events.
package lifecycle

import (
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// AgentEventPayload is the payload for agent lifecycle events (started, stopped, ready, completed, failed).
type AgentEventPayload struct {
	AgentExecutionID string     `json:"agent_execution_id"`
	TaskID           string     `json:"task_id"`
	SessionID        string     `json:"session_id,omitempty"`
	AgentProfileID   string     `json:"agent_profile_id"`
	ContainerID      string     `json:"container_id,omitempty"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	ExitCode         *int       `json:"exit_code,omitempty"`
}

// AgentctlEventPayload is the payload for agentctl lifecycle events (starting, ready, error).
type AgentctlEventPayload struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	ErrorMessage     string `json:"error_message,omitempty"`
	WorktreeID       string `json:"worktree_id,omitempty"`
	WorktreePath     string `json:"worktree_path,omitempty"`
	WorktreeBranch   string `json:"worktree_branch,omitempty"`
}

// ACPSessionCreatedPayload is the payload when an ACP session is created.
type ACPSessionCreatedPayload struct {
	TaskID          string `json:"task_id"`
	SessionID       string `json:"session_id"`
	AgentInstanceID string `json:"agent_instance_id"`
	ACPSessionID    string `json:"acp_session_id"`
}

// AgentStreamEventData contains the nested event data within AgentStreamEventPayload.
type AgentStreamEventData struct {
	Type          string      `json:"type"`
	ACPSessionID  string      `json:"acp_session_id,omitempty"`
	Text          string      `json:"text,omitempty"`
	ToolCallID    string      `json:"tool_call_id,omitempty"`
	ToolName      string      `json:"tool_name,omitempty"`
	ToolTitle     string      `json:"tool_title,omitempty"`
	ToolStatus    string      `json:"tool_status,omitempty"`
	Error         string      `json:"error,omitempty"`
	SessionStatus string      `json:"session_status,omitempty"` // "resumed" or "new" for session_status events
	Data          interface{} `json:"data,omitempty"`

	// ParentToolCallID identifies the parent Task tool call when this event
	// comes from a subagent. Used for visual nesting in the UI.
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`

	// Normalized contains the typed tool payload data.
	// This is used to populate message metadata with structured tool information.
	Normalized *streams.NormalizedPayload `json:"normalized,omitempty"`

	// Streaming message fields (for "message_streaming" and "thinking_streaming" types)
	// MessageID is the ID of the message being streamed (empty for first chunk, set for appends)
	MessageID string `json:"message_id,omitempty"`
	// IsAppend indicates whether this is an append to an existing message (true) or a new message (false)
	IsAppend bool `json:"is_append,omitempty"`
	// MessageType distinguishes between "message" and "thinking" content types
	MessageType string `json:"message_type,omitempty"`

	// AvailableCommands contains the slash commands available from the agent.
	// Populated when Type is "available_commands".
	AvailableCommands []streams.AvailableCommand `json:"available_commands,omitempty"`
}

// AgentStreamEventPayload is the payload for agent stream events (WebSocket streaming).
type AgentStreamEventPayload struct {
	Type      string                `json:"type"` // Always "agent/event"
	Timestamp string                `json:"timestamp"`
	AgentID   string                `json:"agent_id"`
	TaskID    string                `json:"task_id"`
	SessionID string                `json:"session_id"` // Task session ID
	Data      *AgentStreamEventData `json:"data"`
}

// GitEventType discriminates the type of git event
type GitEventType string

const (
	GitEventTypeStatusUpdate    GitEventType = "status_update"
	GitEventTypeCommitCreated   GitEventType = "commit_created"
	GitEventTypeCommitsReset    GitEventType = "commits_reset"
	GitEventTypeSnapshotCreated GitEventType = "snapshot_created"
)

// GitEventPayload is a unified payload for all git-related WebSocket events.
// Uses discriminated union pattern with Type field.
type GitEventPayload struct {
	Type      GitEventType `json:"type"`
	TaskID    string       `json:"task_id,omitempty"`
	SessionID string       `json:"session_id"`
	AgentID   string       `json:"agent_id,omitempty"`
	Timestamp string       `json:"timestamp"`

	// For status_update
	Status *GitStatusData `json:"status,omitempty"`

	// For commit_created
	Commit *GitCommitData `json:"commit,omitempty"`

	// For commits_reset
	Reset *GitResetData `json:"reset,omitempty"`

	// For snapshot_created
	Snapshot *GitSnapshotData `json:"snapshot,omitempty"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p GitEventPayload) GetSessionID() string {
	return p.SessionID
}

type GitStatusData struct {
	Branch       string      `json:"branch"`
	RemoteBranch string      `json:"remote_branch,omitempty"`
	HeadCommit   string      `json:"head_commit,omitempty"`
	BaseCommit   string      `json:"base_commit,omitempty"`
	Modified     []string    `json:"modified"`
	Added        []string    `json:"added"`
	Deleted      []string    `json:"deleted"`
	Untracked    []string    `json:"untracked"`
	Renamed      []string    `json:"renamed"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	Files        interface{} `json:"files,omitempty"`
}

type GitCommitData struct {
	ID           string `json:"id,omitempty"`
	CommitSHA    string `json:"commit_sha"`
	ParentSHA    string `json:"parent_sha"`
	Message      string `json:"commit_message"`
	AuthorName   string `json:"author_name"`
	AuthorEmail  string `json:"author_email"`
	FilesChanged int    `json:"files_changed"`
	Insertions   int    `json:"insertions"`
	Deletions    int    `json:"deletions"`
	CommittedAt  string `json:"committed_at"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type GitResetData struct {
	PreviousHead string `json:"previous_head"`
	CurrentHead  string `json:"current_head"`
	DeletedCount int    `json:"deleted_count"`
}

type GitSnapshotData struct {
	ID           string      `json:"id"`
	SessionID    string      `json:"session_id"`
	SnapshotType string      `json:"snapshot_type"`
	Branch       string      `json:"branch"`
	RemoteBranch string      `json:"remote_branch"`
	HeadCommit   string      `json:"head_commit"`
	BaseCommit   string      `json:"base_commit"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	Files        interface{} `json:"files,omitempty"`
	TriggeredBy  string      `json:"triggered_by"`
	CreatedAt    string      `json:"created_at"`
}

// FileChangeEventPayload is the payload for file change notifications.
type FileChangeEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p FileChangeEventPayload) GetSessionID() string {
	return p.SessionID
}

// PermissionOption represents a single permission option in a permission request.
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// PermissionRequestEventPayload is the payload when an agent requests permission.
type PermissionRequestEventPayload struct {
	Type          string                 `json:"type"` // Always "permission_request"
	Timestamp     string                 `json:"timestamp"`
	AgentID       string                 `json:"agent_id"`
	TaskID        string                 `json:"task_id"`
	SessionID     string                 `json:"session_id"`
	PendingID     string                 `json:"pending_id"`
	ToolCallID    string                 `json:"tool_call_id"`
	Title         string                 `json:"title"`
	Options       []PermissionOption     `json:"options"`
	ActionType    string                 `json:"action_type"`
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`
}

// ShellOutputEventPayload is the payload for shell output events.
type ShellOutputEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Type      string `json:"type"` // Always "output" for shell output events
	Data      string `json:"data"`
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ShellOutputEventPayload) GetSessionID() string {
	return p.SessionID
}

// ShellExitEventPayload is the payload for shell exit events.
type ShellExitEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Type      string `json:"type"` // Always "exit" for shell exit events
	Code      int    `json:"code"` // Exit code
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ShellExitEventPayload) GetSessionID() string {
	return p.SessionID
}

// ProcessOutputEventPayload is the payload for process output events.
type ProcessOutputEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	ProcessID string `json:"process_id"`
	Kind      string `json:"kind"`
	Stream    string `json:"stream"` // stdout|stderr
	Data      string `json:"data"`
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ProcessOutputEventPayload) GetSessionID() string {
	return p.SessionID
}

// ProcessStatusEventPayload is the payload for process status events.
type ProcessStatusEventPayload struct {
	SessionID  string `json:"session_id"`
	ProcessID  string `json:"process_id"`
	Kind       string `json:"kind"`
	ScriptName string `json:"script_name,omitempty"`
	Status     string `json:"status"`
	Command    string `json:"command,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ProcessStatusEventPayload) GetSessionID() string {
	return p.SessionID
}

// ContextWindowEventPayload is the payload for context window update events.
type ContextWindowEventPayload struct {
	TaskID                 string  `json:"task_id"`
	SessionID              string  `json:"session_id"`
	AgentID                string  `json:"agent_id"`
	ContextWindowSize      int64   `json:"context_window_size"`
	ContextWindowUsed      int64   `json:"context_window_used"`
	ContextWindowRemaining int64   `json:"context_window_remaining"`
	ContextEfficiency      float64 `json:"context_efficiency"`
	Timestamp              string  `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ContextWindowEventPayload) GetSessionID() string {
	return p.SessionID
}

// AvailableCommandsEventPayload is the payload for available commands update events.
type AvailableCommandsEventPayload struct {
	TaskID            string                    `json:"task_id"`
	SessionID         string                    `json:"session_id"`
	AgentID           string                    `json:"agent_id"`
	AvailableCommands []streams.AvailableCommand `json:"available_commands"`
	Timestamp         string                    `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p AvailableCommandsEventPayload) GetSessionID() string {
	return p.SessionID
}
