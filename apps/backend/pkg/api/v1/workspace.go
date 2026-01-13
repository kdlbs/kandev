package v1

import "time"

// Workspace represents a workspace
type Workspace struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Description           *string   `json:"description,omitempty"`
	OwnerID               string    `json:"owner_id"`
	DefaultExecutorID     *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string   `json:"default_agent_profile_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// CreateWorkspaceRequest for creating a new workspace
type CreateWorkspaceRequest struct {
	Name                  string  `json:"name" binding:"required,max=255"`
	Description           *string `json:"description,omitempty"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}

// UpdateWorkspaceRequest for updating an existing workspace
type UpdateWorkspaceRequest struct {
	Name                  *string `json:"name,omitempty" binding:"omitempty,max=255"`
	Description           *string `json:"description,omitempty"`
	DefaultExecutorID     *string `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID  *string `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID *string `json:"default_agent_profile_id,omitempty"`
}
