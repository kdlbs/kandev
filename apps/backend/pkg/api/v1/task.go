package v1

import "time"

// TaskState represents the state of a task
type TaskState string

const (
	TaskStateTODO            TaskState = "TODO"
	TaskStateInProgress      TaskState = "IN_PROGRESS"
	TaskStateReview          TaskState = "REVIEW"
	TaskStateBlocked         TaskState = "BLOCKED"
	TaskStateWaitingForInput TaskState = "WAITING_FOR_INPUT"
	TaskStateCompleted       TaskState = "COMPLETED"
	TaskStateFailed          TaskState = "FAILED"
	TaskStateCancelled       TaskState = "CANCELLED"
)

// Task represents a Kanban task
type Task struct {
	ID              string                 `json:"id"`
	WorkspaceID     string                 `json:"workspace_id"`
	BoardID         string                 `json:"board_id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	State           TaskState              `json:"state"`
	Priority        int                    `json:"priority"`
	AgentType       *string                `json:"agent_type,omitempty"`
	RepositoryURL   *string                `json:"repository_url,omitempty"`
	Branch          *string                `json:"branch,omitempty"`
	AssignedAgentID *string                `json:"assigned_agent_id,omitempty"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// CreateTaskRequest for creating a new task
type CreateTaskRequest struct {
	Title         string                 `json:"title" binding:"required,max=500"`
	Description   string                 `json:"description" binding:"required"`
	Priority      int                    `json:"priority" binding:"min=0,max=10"`
	AgentType     *string                `json:"agent_type,omitempty"`
	RepositoryURL *string                `json:"repository_url,omitempty"`
	Branch        *string                `json:"branch,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest for updating an existing task
type UpdateTaskRequest struct {
	Title         *string                `json:"title,omitempty" binding:"omitempty,max=500"`
	Description   *string                `json:"description,omitempty"`
	Priority      *int                   `json:"priority,omitempty" binding:"omitempty,min=0,max=10"`
	AgentType     *string                `json:"agent_type,omitempty"`
	RepositoryURL *string                `json:"repository_url,omitempty"`
	Branch        *string                `json:"branch,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
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

// Comment represents a comment on a task (user or agent)
type Comment struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	AuthorType    string    `json:"author_type"` // "user" or "agent"
	AuthorID      string    `json:"author_id,omitempty"`
	Content       string    `json:"content"`
	RequestsInput bool      `json:"requests_input"` // True if agent is requesting user input
	CreatedAt     time.Time `json:"created_at"`
}

// CreateCommentRequest for adding a comment to a task
type CreateCommentRequest struct {
	TaskID        string `json:"task_id" binding:"required"`
	Content       string `json:"content" binding:"required"`
	AuthorType    string `json:"author_type,omitempty"` // Defaults to "user" if not specified
	RequestsInput bool   `json:"requests_input,omitempty"`
}
