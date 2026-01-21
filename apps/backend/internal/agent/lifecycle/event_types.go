// Package lifecycle provides event payload types for agent lifecycle events.
package lifecycle

import "time"

// AgentEventPayload is the payload for agent lifecycle events (started, stopped, ready, completed, failed).
type AgentEventPayload struct {
	InstanceID     string     `json:"instance_id"`
	TaskID         string     `json:"task_id"`
	AgentProfileID string     `json:"agent_profile_id"`
	ContainerID    string     `json:"container_id,omitempty"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Progress       int        `json:"progress"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
}

// AgentctlEventPayload is the payload for agentctl lifecycle events (starting, ready, error).
type AgentctlEventPayload struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// ACPSessionCreatedPayload is the payload when an ACP session is created.
type ACPSessionCreatedPayload struct {
	TaskID          string `json:"task_id"`
	AgentInstanceID string `json:"agent_instance_id"`
	ACPSessionID    string `json:"acp_session_id"`
}

// AgentStreamEventData contains the nested event data within AgentStreamEventPayload.
type AgentStreamEventData struct {
	Type          string                 `json:"type"`
	ACPSessionID  string                 `json:"acp_session_id,omitempty"`
	Text          string                 `json:"text,omitempty"`
	ToolCallID    string                 `json:"tool_call_id,omitempty"`
	ToolName      string                 `json:"tool_name,omitempty"`
	ToolTitle     string                 `json:"tool_title,omitempty"`
	ToolStatus    string                 `json:"tool_status,omitempty"`
	ToolArgs      map[string]interface{} `json:"tool_args,omitempty"`
	ToolResult    interface{}            `json:"tool_result,omitempty"`
	Error         string                 `json:"error,omitempty"`
	SessionStatus string                 `json:"session_status,omitempty"` // "resumed" or "new" for session_status events
	Data          interface{}            `json:"data,omitempty"`

	// Streaming message fields (for "message_streaming" type)
	// MessageID is the ID of the message being streamed (empty for first chunk, set for appends)
	MessageID string `json:"message_id,omitempty"`
	// IsAppend indicates whether this is an append to an existing message (true) or a new message (false)
	IsAppend bool `json:"is_append,omitempty"`
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

// GitStatusEventPayload is the payload for git status updates.
type GitStatusEventPayload struct {
	TaskID       string      `json:"task_id"`
	SessionID    string      `json:"session_id"`
	AgentID      string      `json:"agent_id"`
	Branch       string      `json:"branch"`
	RemoteBranch string      `json:"remote_branch,omitempty"`
	Modified     []string    `json:"modified"`
	Added        []string    `json:"added"`
	Deleted      []string    `json:"deleted"`
	Untracked    []string    `json:"untracked"`
	Renamed      []string    `json:"renamed"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	Files        interface{} `json:"files,omitempty"` // map[string]FileInfo from agentctl
	Timestamp    string      `json:"timestamp"`
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
