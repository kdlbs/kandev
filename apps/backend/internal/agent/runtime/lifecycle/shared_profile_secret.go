package lifecycle

import (
	"context"
	"errors"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
)

// SharedProfileSecretGrantorResolver returns the active identity of the user
// who shared an agent-profile secret binding. It reports false when the user
// is missing, disabled, in an unusable organization, or authentication is off.
type SharedProfileSecretGrantorResolver func(ctx context.Context, userID string) (authn.Identity, bool)

// SharedProfileSecretError explains why a shared agent-profile secret binding
// could not be used by the starting user. Reason is a fixed phrase and never
// carries a secret identifier or value.
type SharedProfileSecretError struct {
	Reason string
}

func (e *SharedProfileSecretError) Error() string {
	return "shared agent profile secret is unavailable: " + e.Reason
}

const (
	sharedSecretReasonUnauthenticated = "the starter is not a signed-in user; start the task as a signed-in member"
	sharedSecretReasonUnsupported     = "this backend does not support shared profile secrets"
	sharedSecretReasonGrantorInactive = "the user who shared it is no longer active; " +
		"its owner must be re-enabled or another member must bind their own secret"
	sharedSecretReasonCrossOrg    = "the starter is not in the organization of the user who shared it"
	sharedSecretReasonOwnerMissed = "the user who shared it no longer owns a global secret with this binding; " +
		"its owner must re-select and share the secret again"
)

// SetSharedProfileSecretGrantorResolver wires the account check used before a
// starting user may resolve an agent-profile secret its owner shared. Without
// it, shared bindings resolve only for their owner.
func (m *Manager) SetSharedProfileSecretGrantorResolver(resolve SharedProfileSecretGrantorResolver) {
	m.sharedSecretGrantor = resolve
}

// revealAgentProfileSecret resolves an agent-profile secret binding. The
// starting user's own visibility is tried first. Only when the secret is not
// visible to that user and the binding carries an owner grant does the
// shared path run, and it requires an authenticated starter, an active
// grantor in the same organization, and a global secret the grantor still
// owns.
func (m *Manager) revealAgentProfileSecret(ctx context.Context, secretID, sharedByUserID string) (string, error) {
	value, err := revealGlobalSecret(ctx, m.secretStore, secretID)
	if err == nil || sharedByUserID == "" || !errors.Is(err, secrets.ErrNotFound) {
		return value, err
	}
	return m.revealSharedProfileSecret(ctx, secretID, sharedByUserID)
}

func (m *Manager) revealSharedProfileSecret(ctx context.Context, secretID, sharedByUserID string) (string, error) {
	actor, ok := authn.IdentityFromContext(ctx)
	if !ok || actor.Synthetic || actor.UserID == "" {
		return "", &SharedProfileSecretError{Reason: sharedSecretReasonUnauthenticated}
	}
	store, ok := m.secretStore.(secrets.OwnerGrantedSecretStore)
	if !ok || m.sharedSecretGrantor == nil {
		return "", &SharedProfileSecretError{Reason: sharedSecretReasonUnsupported}
	}
	grantor, ok := m.sharedSecretGrantor(ctx, sharedByUserID)
	if !ok || grantor.Synthetic || grantor.UserID != sharedByUserID {
		return "", &SharedProfileSecretError{Reason: sharedSecretReasonGrantorInactive}
	}
	if grantor.OrgID != actor.OrgID {
		return "", &SharedProfileSecretError{Reason: sharedSecretReasonCrossOrg}
	}
	value, err := store.RevealGlobalOwnedBy(ctx, secretID, sharedByUserID)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", &SharedProfileSecretError{Reason: sharedSecretReasonOwnerMissed}
	}
	return value, err
}

// sharedProfileSecretGrants indexes the owner grants carried by agent-profile
// bindings by key and secret, so a definition is resolved through a grant only
// when it injects exactly the shared secret under exactly the shared key.
func sharedProfileSecretGrants(envVars []settingsmodels.ProfileEnvVar) map[string]string {
	grants := make(map[string]string)
	for _, envVar := range envVars {
		if envVar.SharedByUserID == "" || envVar.SecretID == "" || envVar.Key == "" {
			continue
		}
		grants[sharedGrantKey(envVar.Key, envVar.SecretID)] = envVar.SharedByUserID
	}
	return grants
}

func sharedGrantKey(key, secretID string) string {
	return key + "\x00" + secretID
}

// withoutSecretShares drops owner grants from bindings resolved outside an
// agent profile, such as executor-profile environments.
func withoutSecretShares(envVars []settingsmodels.ProfileEnvVar) []settingsmodels.ProfileEnvVar {
	out := make([]settingsmodels.ProfileEnvVar, len(envVars))
	for i, envVar := range envVars {
		envVar.SharedByUserID = ""
		out[i] = envVar
	}
	return out
}

// environmentRevealer resolves task environment definitions, honoring the
// agent profile's owner grants for global secret definitions only.
func (m *Manager) environmentRevealer(profileInfo *AgentProfileInfo) runtimeenv.RevealFunc {
	if profileInfo == nil {
		return m.resolveEnvironmentDefinition
	}
	grants := sharedProfileSecretGrants(profileInfo.EnvVars)
	if len(grants) == 0 {
		return m.resolveEnvironmentDefinition
	}
	return func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		grantor := grants[sharedGrantKey(definition.Key, definition.SecretID)]
		if grantor == "" || definition.WorkspaceID != "" || m.secretStore == nil {
			return m.resolveEnvironmentDefinition(ctx, definition)
		}
		return m.revealAgentProfileSecret(ctx, definition.SecretID, grantor)
	}
}

// sharedProfileSecretHint appends the actionable reason for an agent-profile
// secret failure without exposing the secret identifier or value.
func sharedProfileSecretHint(err error) string {
	var sharedErr *SharedProfileSecretError
	if errors.As(err, &sharedErr) {
		return sharedErr.Reason
	}
	if errors.Is(err, secrets.ErrNotFound) {
		return "the secret is missing, not global, or owned by another user; " +
			"if another user owns it, they can enable shared use for this binding"
	}
	return ""
}
