// Package api provides HTTP handlers for the task service API.
package api

import "time"

// CreateTaskRequest for creating a task
type CreateTaskRequest struct {
	BoardID     string                 `json:"board_id" binding:"required"`
	ColumnID    string                 `json:"column_id" binding:"required"`
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"`
	AgentType   string                 `json:"agent_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskRequest for updating a task
type UpdateTaskRequest struct {
	Title       *string                `json:"title,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	AgentType   *string                `json:"agent_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateTaskStateRequest for changing task state
type UpdateTaskStateRequest struct {
	State string `json:"state" binding:"required"`
}

// MoveTaskRequest for moving a task to a different column
type MoveTaskRequest struct {
	ColumnID string `json:"column_id" binding:"required"`
	Position int    `json:"position"`
}

// CreateBoardRequest for creating a board
type CreateBoardRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateBoardRequest for updating a board
type UpdateBoardRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateColumnRequest for creating a column
type CreateColumnRequest struct {
	Name     string `json:"name" binding:"required"`
	Position int    `json:"position"`
	State    string `json:"state"` // Maps to task state: TODO, IN_PROGRESS, DONE
}

// Response types

// TaskResponse represents a task in API responses
type TaskResponse struct {
	ID          string                 `json:"id"`
	BoardID     string                 `json:"board_id"`
	ColumnID    string                 `json:"column_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	State       string                 `json:"state"`
	Priority    int                    `json:"priority"`
	AgentType   string                 `json:"agent_type,omitempty"`
	Position    int                    `json:"position"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// BoardResponse represents a board in API responses
type BoardResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ColumnResponse represents a column in API responses
type ColumnResponse struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

// TasksListResponse for listing tasks
type TasksListResponse struct {
	Tasks []*TaskResponse `json:"tasks"`
	Total int             `json:"total"`
}

// BoardsListResponse for listing boards
type BoardsListResponse struct {
	Boards []*BoardResponse `json:"boards"`
	Total  int              `json:"total"`
}

// ColumnsListResponse for listing columns
type ColumnsListResponse struct {
	Columns []*ColumnResponse `json:"columns"`
	Total   int               `json:"total"`
}

