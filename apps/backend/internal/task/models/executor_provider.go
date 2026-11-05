package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	ExecutorID             string `json:"executor_id"`
	Identity               string `json:"identity"`
	PluginID               string `json:"plugin_id"`
	InstallationID         string `json:"installation_id"`
	Key                    string `json:"key"`
	ContractVersion        int    `json:"contract_version"`
	SupportedStateVersions []int  `json:"-"`
	// RequiredCapabilities and StateBudgetBytes are the manifest's host
	// capability declaration; an empty list means the provider is not gated.
	RequiredCapabilities []ExecutorProviderCapabilityRequirement `json:"-"`
	StateBudgetBytes     int                                     `json:"-"`
	// DestroyTimeoutSeconds is the manifest's destroy deadline; zero means the default.
	DestroyTimeoutSeconds int `json:"-"`
	// RuntimeAPIURL is host_public (or empty) or provider_forwarded.
	RuntimeAPIURL string `json:"-"`
	// WorkspacePreparation is core (or empty) or provider.
	WorkspacePreparation string                       `json:"-"`
	DisplayName          string                       `json:"display_name"`
	Description          string                       `json:"description"`
	LocalizedMessages    map[string]string            `json:"localized_messages,omitempty"`
	ProfileSchema        map[string]any               `json:"profile_schema"`
	ResourceStateSchema  map[string]any               `json:"-"`
	Capabilities         ExecutorProviderCapabilities `json:"capabilities"`
	Available            bool                         `json:"available"`
	AvailabilityCause    string                       `json:"availability_cause,omitempty"`
}

// ExecutorProviderCapabilityRequirement is one host capability a provider
// manifest requires, by registry name and version.
type ExecutorProviderCapabilityRequirement struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// ErrExecutorCapabilityMismatch is the stable `capability_mismatch` refusal:
// the manifest, the host registry, and the running provider's accepted echo do
// not agree.
var ErrExecutorCapabilityMismatch = errors.New("capability_mismatch")

// ErrExecutorConflict is the stable `conflict` refusal of a provider response
// that carries a field of a capability it did not declare, or a workspace
// descriptor that differs from the one recorded for the environment generation.
var ErrExecutorConflict = errors.New("conflict")

// ErrExecutorInvalidConfig is the stable `invalid_config` refusal of a provider
// callback whose shape is malformed: an unknown phase, a phase of the other
// claim kind, or a missing sequence.
var ErrExecutorInvalidConfig = errors.New("invalid_config")

// ErrExecutorClaimNotFound is the stable `not_found` refusal of a provider
// callback that names no active claim for its environment.
var ErrExecutorClaimNotFound = errors.New("not_found")

// ErrExecutorStateTooLarge is the host-originated `state_too_large` refusal for
// provider state above its budget or an envelope above its cap. A provider
// cannot return it.
var ErrExecutorStateTooLarge = errors.New("state_too_large")

// ErrExecutorSecretInState is the `secret_in_state` refusal of a provider
// response whose state contains the exact bytes of a secret value revealed for
// the call that produced it.
var ErrExecutorSecretInState = errors.New("secret_in_state")

// ErrExecutorProfileInvalidType is the `invalid_type` refusal of a stored profile
// value that does not parse as its schema type, raised before any provider call.
var ErrExecutorProfileInvalidType = errors.New("invalid_type")

// ErrExecutorProfileTooLarge is the host-originated `profile_too_large` refusal
// of a profile whose provider-facing config and secret IDs exceed the budget.
var ErrExecutorProfileTooLarge = errors.New("profile_too_large")

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
	// SecretReferences maps a secret property to the store ID that holds it.
	SecretReferences map[string]string
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

// ExecutorProfileContractBudgetBytes bounds the provider-facing profile config
// plus the secret store IDs that the host records for a provider that declares
// a capability.
const ExecutorProfileContractBudgetBytes = 4096

// ExecutorProfileContractBytes is the one measure used at profile save, launch
// and resume: the compact JSON length of the provider-facing config object
// (sorted keys, no HTML escaping) plus the compact JSON array of the secret
// store IDs. A nil or empty ID list contributes nothing.
func ExecutorProfileContractBytes(publicConfig map[string]string, secretIDs []string) int {
	total := compactJSONLength(publicConfig)
	if len(secretIDs) > 0 {
		total += compactJSONLength(secretIDs)
	}
	return total
}

func compactJSONLength(value any) int {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return 0
	}
	return buffer.Len() - 1 // Encode appends a newline
}

const (
	// PluginClaimKindCleanup and PluginClaimKindRecovery name the two claim
	// kinds a provider checkpoint can be scoped to.
	PluginClaimKindCleanup  = "cleanup"
	PluginClaimKindRecovery = "recovery"
)

// PluginExecutorClaimCheckpoint binds one provider checkpoint to the active
// claim it was issued under. Sequence and Digest are the provider's ordering
// token and the digest of the checkpoint payload.
type PluginExecutorClaimCheckpoint struct {
	Claim    *TaskEnvironmentRecoveryClaim
	Kind     string
	Sequence uint64
	Digest   string
}

// ErrExecutorPublishMismatch is the refusal of a ready publication whose
// evidence does not match the stored operation. The environment is retained;
// the wrapped PluginPublishMismatchError names the reason.
var ErrExecutorPublishMismatch = errors.New("publish_mismatch")

// Reasons a ready publication is refused. The set is closed.
const (
	PluginPublishReasonOperation  = "operation"
	PluginPublishReasonOwner      = "owner"
	PluginPublishReasonSession    = "session"
	PluginPublishReasonGeneration = "generation"
	PluginPublishReasonRuntime    = "runtime_identity"
	PluginPublishReasonWorkspace  = "workspace_descriptor"
	PluginPublishReasonRepos      = "repository_inventory"
	PluginPublishReasonPhase      = "phase"
	PluginPublishReasonRevision   = "revision"
	PluginPublishReasonStatus     = "environment_status"
)

// PluginPublishMismatchError carries the named reason of a refused publication.
type PluginPublishMismatchError struct{ Reason string }

func (e *PluginPublishMismatchError) Error() string {
	return ErrExecutorPublishMismatch.Error() + ": " + e.Reason
}

func (e *PluginPublishMismatchError) Is(target error) bool {
	return target == ErrExecutorPublishMismatch
}

// PublishRecoveredTaskEnvironmentParams is the verified evidence for flipping a
// creating environment to ready under the recovery claim. It carries no
// provider data; the repository re-checks every field against the stored
// envelope in the publishing transaction.
type PublishRecoveredTaskEnvironmentParams struct {
	Claim               *TaskEnvironmentRecoveryClaim
	SessionID           string
	EnvironmentID       string
	OperationID         string
	RuntimeIdentity     string
	OwnershipGeneration int64
	// ExpectedRevision is the envelope revision the caller read after writing
	// the ready phase.
	ExpectedRevision uint64
	// Repos is the repository projection that replaces the environment's rows.
	Repos []*TaskEnvironmentRepo
	// CorePreparedWorkspace marks an environment whose workspace core prepares
	// (workspace_preparation: core). Core populates it through agentctl only after
	// the instance exists, which a recovery never does, so the publication refuses
	// a task with repositories and publishes only a repo-less one. When the
	// provider recorded no workspace descriptor it uses WorkspacePath.
	CorePreparedWorkspace bool
	// WorkspacePath is the workspace path the host uses when the envelope records
	// no descriptor; it is read only for a CorePreparedWorkspace.
	WorkspacePath string
}
