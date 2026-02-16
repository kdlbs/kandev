package v1

import "time"

// TaskState represents the state of a task
type TaskState string

const (
	TaskStateTODO            TaskState = "TODO"
	TaskStateCreated         TaskState = "CREATED"
	TaskStateScheduling      TaskState = "SCHEDULING"
	TaskStateInProgress      TaskState = "IN_PROGRESS"
	TaskStateReview          TaskState = "REVIEW"
	TaskStateBlocked         TaskState = "BLOCKED"
	TaskStateWaitingForInput TaskState = "WAITING_FOR_INPUT"
	TaskStateCompleted       TaskState = "COMPLETED"
	TaskStateFailed          TaskState = "FAILED"
	TaskStateCancelled       TaskState = "CANCELLED"
)

// TaskSessionState represents the state of an agent session.
type TaskSessionState string

const (
	TaskSessionStateCreated         TaskSessionState = "CREATED"
	TaskSessionStateStarting        TaskSessionState = "STARTING"
	TaskSessionStateRunning         TaskSessionState = "RUNNING"
	TaskSessionStateWaitingForInput TaskSessionState = "WAITING_FOR_INPUT"
	TaskSessionStateCompleted       TaskSessionState = "COMPLETED"
	TaskSessionStateFailed          TaskSessionState = "FAILED"
	TaskSessionStateCancelled       TaskSessionState = "CANCELLED"
)

// MessageType represents a normalized session message type.
type MessageType string

const (
	MessageTypeMessage  MessageType = "message"
	MessageTypeContent  MessageType = "content"
	MessageTypeToolCall MessageType = "tool_call"
	MessageTypeProgress MessageType = "progress"
	MessageTypeLog      MessageType = "log"
	MessageTypeError    MessageType = "error"
	MessageTypeStatus   MessageType = "status"
	MessageTypeThinking MessageType = "thinking"
	MessageTypeTodo     MessageType = "todo"
)

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

// Task represents a Kanban task
type Task struct {
	ID           string                 `json:"id"`
	WorkspaceID  string                 `json:"workspace_id"`
	WorkflowID   string                 `json:"workflow_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	State        TaskState              `json:"state"`
	Priority     int                    `json:"priority"`
	Repositories []TaskRepository       `json:"repositories,omitempty"`
	CreatedBy    string                 `json:"created_by"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TaskRepositoryInput for creating/updating task repositories
type TaskRepositoryInput struct {
	RepositoryID string `json:"repository_id" binding:"required"`
	BaseBranch   string `json:"base_branch" binding:"required"`
}

// CreateTaskRequest for creating a new task
type CreateTaskRequest struct {
	Title          string                 `json:"title" binding:"required,max=500"`
	Description    string                 `json:"description" binding:"required"`
	Priority       int                    `json:"priority" binding:"min=0,max=10"`
	Repositories   []TaskRepositoryInput  `json:"repositories,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	StartAgent     bool                   `json:"start_agent,omitempty"`
	AgentProfileID string                 `json:"agent_profile_id,omitempty"`
}

// UpdateTaskRequest for updating an existing task
type UpdateTaskRequest struct {
	Title        *string                `json:"title,omitempty" binding:"omitempty,max=500"`
	Description  *string                `json:"description,omitempty"`
	Priority     *int                   `json:"priority,omitempty" binding:"omitempty,min=0,max=10"`
	Repositories []TaskRepositoryInput  `json:"repositories,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskStateRequest for changing task state
type UpdateTaskStateRequest struct {
	State TaskState `json:"state" binding:"required"`
}

// TaskEvent for task history/audit
type TaskEvent struct {
	ID        int64                  `json:"id"`
	TaskID    string                 `json:"task_id"`
	EventType string                 `json:"event_type"`
	OldState  *TaskState             `json:"old_state,omitempty"`
	NewState  *TaskState             `json:"new_state,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedBy *string                `json:"created_by,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// MessageAttachment represents an attachment (image, file, etc.) to a message
type MessageAttachment struct {
	Type     string `json:"type"`      // "image"
	Data     string `json:"data"`      // Base64-encoded data
	MimeType string `json:"mime_type"` // MIME type (e.g., "image/png")
}

// ContextFileMeta represents a context file reference attached to a message
type ContextFileMeta struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// Message represents a message in a task session (user or agent)
type Message struct {
	ID            string                 `json:"id"`
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id,omitempty"`
	TurnID        string                 `json:"turn_id,omitempty"` // FK to task_session_turns
	AuthorType    string                 `json:"author_type"`       // "user" or "agent"
	Type          string                 `json:"type,omitempty"`
	AuthorID      string                 `json:"author_id,omitempty"`
	Content       string                 `json:"content"`
	RequestsInput bool                   `json:"requests_input"` // True if agent is requesting user input
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// CreateMessageRequest for adding a message to a task session
type CreateMessageRequest struct {
	TaskSessionID string                 `json:"session_id" binding:"required"`
	Content       string                 `json:"content" binding:"required"`
	AuthorType    string                 `json:"author_type,omitempty"` // Defaults to "user" if not specified
	Type          string                 `json:"type,omitempty"`
	RequestsInput bool                   `json:"requests_input,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// PermissionOption represents a permission choice presented to the user
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always
}

// PermissionRequest represents an agent's request for user permission
type PermissionRequest struct {
	RequestID   string             `json:"request_id"`            // Unique ID for this request (JSON-RPC ID)
	TaskID      string             `json:"task_id"`               // Task the agent is working on
	InstanceID  string             `json:"instance_id"`           // Agent instance ID
	SessionID   string             `json:"session_id"`            // ACP session ID
	ToolCallID  string             `json:"tool_call_id"`          // Tool call requesting permission
	Title       string             `json:"title"`                 // Human-readable title
	Description string             `json:"description,omitempty"` // Additional context
	Options     []PermissionOption `json:"options"`               // Available choices
	CreatedAt   time.Time          `json:"created_at"`
}

// PermissionResponse represents the user's response to a permission request
type PermissionResponse struct {
	RequestID string `json:"request_id" binding:"required"` // The request being responded to
	OptionID  string `json:"option_id,omitempty"`           // Selected option (if not cancelled)
	Cancelled bool   `json:"cancelled,omitempty"`           // True if user cancelled
}
