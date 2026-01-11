package v1

import "time"

// Workspace represents a workspace
type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateWorkspaceRequest for creating a new workspace
type CreateWorkspaceRequest struct {
	Name        string  `json:"name" binding:"required,max=255"`
	Description *string `json:"description,omitempty"`
}

// UpdateWorkspaceRequest for updating an existing workspace
type UpdateWorkspaceRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,max=255"`
	Description *string `json:"description,omitempty"`
}
