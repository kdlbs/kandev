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
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

// Comment represents a comment on a task
type Comment struct {
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	AuthorType      CommentAuthorType `json:"author_type"`
	AuthorID        string            `json:"author_id,omitempty"` // User ID or Agent Instance ID
	Content         string            `json:"content"`
	RequestsInput   bool              `json:"requests_input"`   // True if agent is requesting user input
	ACPSessionID    string            `json:"acp_session_id,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// ToAPI converts internal Comment to API type
func (c *Comment) ToAPI() *v1.Comment {
	return &v1.Comment{
		ID:            c.ID,
		TaskID:        c.TaskID,
		AuthorType:    string(c.AuthorType),
		AuthorID:      c.AuthorID,
		Content:       c.Content,
		RequestsInput: c.RequestsInput,
		CreatedAt:     c.CreatedAt,
	}
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
