package secrets

import "time"

// Secret represents stored secret metadata (without the value).
type Secret struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// SecretWithValue is used for create/update operations.
type SecretWithValue struct {
	Secret
	Value string `json:"value,omitempty"`
}

// SecretListItem is returned by list endpoints — never contains the value.
type SecretListItem struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	HasValue  bool      `json:"has_value" db:"has_value"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CreateSecretRequest is the request body for creating a secret.
type CreateSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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
