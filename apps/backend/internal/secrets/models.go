package secrets

import "time"

// SecretCategory groups secrets by purpose.
type SecretCategory string

const (
	CategoryAPIKey       SecretCategory = "api_key"
	CategoryServiceToken SecretCategory = "service_token"
	CategorySSHKey       SecretCategory = "ssh_key"
	CategoryCustom       SecretCategory = "custom"
)

// ValidCategories is the set of allowed categories.
var ValidCategories = map[SecretCategory]bool{
	CategoryAPIKey:       true,
	CategoryServiceToken: true,
	CategorySSHKey:       true,
	CategoryCustom:       true,
}

// Secret represents stored secret metadata (without the value).
type Secret struct {
	ID        string            `json:"id" db:"id"`
	Name      string            `json:"name" db:"name"`
	EnvKey    string            `json:"env_key" db:"env_key"`
	Category  SecretCategory    `json:"category" db:"category"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" db:"updated_at"`
}

// SecretWithValue is used for create/update operations.
type SecretWithValue struct {
	Secret
	Value string `json:"value,omitempty"`
}

// SecretListItem is returned by list endpoints — never contains the value.
type SecretListItem struct {
	ID        string            `json:"id" db:"id"`
	Name      string            `json:"name" db:"name"`
	EnvKey    string            `json:"env_key" db:"env_key"`
	Category  SecretCategory    `json:"category" db:"category"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	HasValue  bool              `json:"has_value" db:"has_value"`
	CreatedAt time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" db:"updated_at"`
}

// CreateSecretRequest is the request body for creating a secret.
type CreateSecretRequest struct {
	Name     string            `json:"name"`
	EnvKey   string            `json:"env_key"`
	Value    string            `json:"value"`
	Category SecretCategory    `json:"category,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// UpdateSecretRequest is the request body for updating a secret.
type UpdateSecretRequest struct {
	Name     *string           `json:"name,omitempty"`
	Value    *string           `json:"value,omitempty"`
	Category *SecretCategory   `json:"category,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RevealSecretResponse is returned by the reveal endpoint.
type RevealSecretResponse struct {
	Value string `json:"value"`
}
