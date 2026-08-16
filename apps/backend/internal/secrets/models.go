package secrets

import (
	"encoding/json"
	"time"
)

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

// CopySecretRequest is the request body for copy/move operations. Name uses
// presence semantics: a JSON-omitted name means "use the source secret's
// name"; an explicit null, empty, or whitespace-only name is a validation
// error.
type CopySecretRequest struct {
	Scope       SecretScope `json:"target_scope"`
	WorkspaceID string      `json:"target_workspace_id"`
	Name        *string     `json:"name,omitempty"`

	nameSet  bool
	nameNull bool
}

// UnmarshalJSON implements presence-aware decoding so handlers can distinguish
// an omitted name (default to the source name) from an explicit null (a
// validation error). Go's standard decoding collapses both to a nil pointer,
// so this is the only way to honor the contract. Every field, including the
// presence markers, is reset on each decode so a reused request struct cannot
// leak state between messages.
func (r *CopySecretRequest) UnmarshalJSON(data []byte) error {
	// Reset every field (including the presence markers) BEFORE parsing so a
	// failed decode can never leave stale state from an earlier message, and
	// assign the receiver only after EVERY sub-decode succeeds so a semantic
	// failure (e.g. `name: 123`) cannot leave a partially populated receiver.
	r.Scope = ""
	r.WorkspaceID = ""
	r.Name = nil
	r.nameSet = false
	r.nameNull = false
	var aux struct {
		Scope       SecretScope     `json:"target_scope"`
		WorkspaceID string          `json:"target_workspace_id"`
		Name        json.RawMessage `json:"name"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var name *string
	nameSet := len(aux.Name) > 0
	nameNull := false
	if nameSet {
		if string(aux.Name) == "null" {
			nameNull = true
		} else {
			var value string
			if err := json.Unmarshal(aux.Name, &value); err != nil {
				return err
			}
			name = &value
		}
	}
	r.Scope = aux.Scope
	r.WorkspaceID = aux.WorkspaceID
	r.Name = name
	r.nameSet = nameSet
	r.nameNull = nameNull
	return nil
}
