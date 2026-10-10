package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

const (
	ExecutorProviderIDConfigKey = "provider_identity"
	ExecutorProviderIDPrefix    = "exec-plugin-"
	ExecutorProfileSecretPrefix = "$kandev-secret:"
)

// ExecutorProfileSecretReference encodes a vault ID in the existing scalar
// profile map without adding a schema column or exposing cleartext.
func ExecutorProfileSecretReference(secretID string) string {
	return ExecutorProfileSecretPrefix + secretID
}

// ExecutorProfileSecretID decodes a vault reference from the scalar profile
// map. Empty or non-reference values return ok=false.
func ExecutorProfileSecretID(value string) (secretID string, ok bool) {
	if len(value) <= len(ExecutorProfileSecretPrefix) || value[:len(ExecutorProfileSecretPrefix)] != ExecutorProfileSecretPrefix {
		return "", false
	}
	return value[len(ExecutorProfileSecretPrefix):], true
}

// RedactExecutorProfileConfig removes internal vault references from a
// profile projection and returns only whether each field has a configured
// secret. Both returned maps are safe for ordinary API reads.
func RedactExecutorProfileConfig(config map[string]string) (map[string]string, map[string]bool) {
	redacted := make(map[string]string, len(config))
	configured := make(map[string]bool)
	for key, value := range config {
		if _, ok := ExecutorProfileSecretID(value); ok {
			configured[key] = true
			continue
		}
		redacted[key] = value
	}
	return redacted, configured
}

// ExecutorProviderExecutorID returns a stable, host-owned executor ID for the
// provider identity. Hashing keeps plugin-controlled names out of DB keys.
func ExecutorProviderExecutorID(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return ExecutorProviderIDPrefix + hex.EncodeToString(digest[:16])
}

// ExecutorProvider describes the host-owned executor entry contributed by an
// installed plugin. It contains only manifest metadata and runtime
// availability; provider resource state and credentials are never included.
type ExecutorProvider struct {
	ExecutorID             string                       `json:"executor_id"`
	Identity               string                       `json:"identity"`
	PluginID               string                       `json:"plugin_id"`
	InstallationID         string                       `json:"installation_id"`
	Key                    string                       `json:"key"`
	ContractVersion        int                          `json:"contract_version"`
	SupportedStateVersions []int                        `json:"-"`
	DisplayName            string                       `json:"display_name"`
	Description            string                       `json:"description"`
	LocalizedMessages      map[string]string            `json:"localized_messages,omitempty"`
	ProfileSchema          map[string]any               `json:"profile_schema"`
	ResourceStateSchema    map[string]any               `json:"-"`
	Capabilities           ExecutorProviderCapabilities `json:"capabilities"`
	Available              bool                         `json:"available"`
	AvailabilityCause      string                       `json:"availability_cause,omitempty"`
}

// ExecutorProviderCapabilities is the provider's manifest capability ceiling.
type ExecutorProviderCapabilities struct {
	Terminal            bool   `json:"terminal"`
	Files               bool   `json:"files"`
	Git                 bool   `json:"git"`
	EmbeddedEditor      bool   `json:"embedded_editor"`
	Preview             bool   `json:"preview"`
	Reattach            bool   `json:"reattach"`
	Retention           string `json:"retention"`
	MaximumLifetimeSecs int64  `json:"maximum_lifetime_seconds,omitempty"`
}

// PluginExecutorEnvironmentStatus is the safe, host-owned status projection
// exposed to task clients. Provider resource state and credentials stay private.
type PluginExecutorEnvironmentStatus struct {
	State          string `json:"state"`
	Retention      string `json:"retention"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	DeadlinePassed bool   `json:"deadline_passed"`
	Reason         string `json:"reason,omitempty"`
}

// ExecutorProviderFieldError is a safe, localized validation result from a
// provider. It never contains a submitted secret value.
type ExecutorProviderFieldError struct {
	Field     string `json:"field"`
	Code      string `json:"code"`
	MessageID string `json:"message_id"`
}

// ExecutorProviderProfile is the transient validation payload. SecretValues
// are revealed only for the duration of a provider validation RPC.
type ExecutorProviderProfile struct {
	ProfileID    string
	Config       map[string]string
	SecretValues map[string]string
}

// ExecutorProviderLaunchProfile is a transient, host-authorized snapshot used
// to provision or reattach one remote executor environment. SecretValues are
// never persisted; SecretReferences are the same-profile vault bindings.
type ExecutorProviderLaunchProfile struct {
	Provider            ExecutorProvider
	ProfileID           string
	OwnershipGeneration int64
	Config              map[string]string
	SecretValues        map[string]string
	SecretReferences    map[string]string
}

// ExecutorProviderCatalog is implemented by the plugin service and injected
// into the task service at composition time to avoid a package cycle.
type ExecutorProviderCatalog interface {
	ListExecutorProviders(context.Context) ([]ExecutorProvider, error)
	ValidateExecutorProviderProfile(context.Context, string, ExecutorProviderProfile) ([]ExecutorProviderFieldError, error)
}
