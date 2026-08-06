package secrets

import "time"

// SecretScope identifies the ownership boundary for a secret.
type SecretScope string

const (
	// ScopeGlobal is visible to the current user in every workspace. When
	// authentication is disabled it is install-global.
	ScopeGlobal SecretScope = "global"
	// ScopeWorkspace is visible only inside its owning workspace.
	ScopeWorkspace SecretScope = "workspace"
)

// SecretListOptions controls a metadata-only secret listing.
type SecretListOptions struct {
	Scope         SecretScope
	WorkspaceID   string
	IncludeGlobal bool
}

// Secret represents stored secret metadata (without the value).
type Secret struct {
	ID          string      `json:"id" db:"id"`
	Name        string      `json:"name" db:"name"`
	Scope       SecretScope `json:"scope" db:"scope"`
	WorkspaceID string      `json:"workspace_id,omitempty" db:"workspace_id"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
}

// SecretWithValue is used for create/update operations.
type SecretWithValue struct {
	Secret
	Value string `json:"value,omitempty"`
}

// SecretListItem is returned by list endpoints — never contains the value.
type SecretListItem struct {
	ID          string      `json:"id" db:"id"`
	Name        string      `json:"name" db:"name"`
	Scope       SecretScope `json:"scope" db:"scope"`
	WorkspaceID string      `json:"workspace_id,omitempty" db:"workspace_id"`
	HasValue    bool        `json:"has_value" db:"has_value"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
}

// CreateSecretRequest is the request body for creating a secret.
type CreateSecretRequest struct {
	Name        string      `json:"name"`
	Value       string      `json:"value"`
	Scope       SecretScope `json:"scope,omitempty"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
}

// UpdateSecretRequest is the request body for updating a secret.
type UpdateSecretRequest struct {
	Name  *string `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

// RevealSecretResponse is returned by the reveal endpoint.
type RevealSecretResponse struct {
	Value string `json:"value"`
}
