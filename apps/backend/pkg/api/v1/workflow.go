package v1

import "time"

// Workflow represents a task workflow
type Workflow struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateWorkflowRequest for creating a new workflow
type CreateWorkflowRequest struct {
	Name        string  `json:"name" binding:"required,max=255"`
	Description *string `json:"description,omitempty"`
}

// UpdateWorkflowRequest for updating a workflow
type UpdateWorkflowRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,max=255"`
	Description *string `json:"description,omitempty"`
}
