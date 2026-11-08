package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
)

// profileSecretGrants maps a binding (key and secret) to the user who shared
// that secret with every authorized starter of the agent profile.
type profileSecretGrants map[string]string

func profileSecretGrantKey(key, secretID string) string {
	return strings.TrimSpace(key) + "\x00" + secretID
}

// resolveProfileSecretGrants decides the owner grant of each requested secret
// binding. An omitted shared flag keeps the previous grant of an unchanged
// binding, false revokes it, and true either keeps the previous grant or
// records the caller, who must own the global secret. Every retained grant is
// re-checked against its grantor so a binding never outlives the grantor's
// ownership of the secret.
func (c *Controller) resolveProfileSecretGrants(
	ctx context.Context,
	previous []models.ProfileEnvVar,
	requested []dto.ProfileEnvVarDTO,
) (profileSecretGrants, error) {
	previousGrants := profileSecretGrants{}
	for _, envVar := range previous {
		if envVar.SharedByUserID != "" && envVar.SecretID != "" {
			previousGrants[profileSecretGrantKey(envVar.Key, envVar.SecretID)] = envVar.SharedByUserID
		}
	}
	grants := profileSecretGrants{}
	for i, envVar := range requested {
		grantor, err := c.resolveProfileSecretGrant(ctx, i, envVar, previousGrants)
		if err != nil {
			return nil, err
		}
		if grantor != "" {
			grants[profileSecretGrantKey(envVar.Key, envVar.SecretID)] = grantor
		}
	}
	return grants, nil
}

func (c *Controller) resolveProfileSecretGrant(
	ctx context.Context,
	index int,
	envVar dto.ProfileEnvVarDTO,
	previousGrants profileSecretGrants,
) (string, error) {
	shared := envVar.Shared
	if shared != nil && !*shared {
		return "", nil
	}
	if strings.TrimSpace(envVar.SecretID) == "" {
		if shared != nil {
			return "", fmt.Errorf("%w: env_vars[%d]: only a secret binding can be shared", ErrInvalidProfileEnvVars, index)
		}
		return "", nil
	}
	grantor := previousGrants[profileSecretGrantKey(envVar.Key, envVar.SecretID)]
	if grantor == "" && shared == nil {
		return "", nil
	}
	if grantor == "" {
		caller, ok := authn.IdentityFromContext(ctx)
		if !ok || caller.Synthetic || caller.UserID == "" {
			return "", fmt.Errorf("%w: env_vars[%d]: sharing a secret requires a signed-in user who owns it",
				ErrInvalidProfileEnvVars, index)
		}
		grantor = caller.UserID
	}
	if err := c.verifyProfileSecretGrant(ctx, envVar.SecretID, grantor); err != nil {
		return "", fmt.Errorf("%w: env_vars[%d]: %s", ErrInvalidProfileEnvVars, index, err.Error())
	}
	return grantor, nil
}

// verifyProfileSecretGrant reports whether grantor owns the global secret.
// The message never names the secret or its owner.
func (c *Controller) verifyProfileSecretGrant(ctx context.Context, secretID, grantor string) error {
	store, ok := c.secretStore.(secrets.OwnerGrantedSecretStore)
	if !ok {
		return fmt.Errorf("this backend does not support shared profile secrets")
	}
	if _, err := store.GetGlobalOwnedBy(ctx, secretID, grantor); err != nil {
		return fmt.Errorf("only the owner of a global secret can share it through a profile")
	}
	return nil
}

// apply records the resolved grants on the bindings that still inject the
// same secret under the same key; every other binding carries no grant.
func (g profileSecretGrants) apply(envVars []models.ProfileEnvVar) []models.ProfileEnvVar {
	for i := range envVars {
		envVars[i].SharedByUserID = ""
		if envVars[i].SecretID != "" {
			envVars[i].SharedByUserID = g[profileSecretGrantKey(envVars[i].Key, envVars[i].SecretID)]
		}
	}
	return envVars
}

func (g profileSecretGrants) covers(envVar dto.ProfileEnvVarDTO) bool {
	return g[profileSecretGrantKey(envVar.Key, envVar.SecretID)] != ""
}

// rejectProfileSecretShares refuses shared bindings on paths that do not
// resolve owner grants, such as nested profiles created with a new agent.
func rejectProfileSecretShares(envVars []dto.ProfileEnvVarDTO) error {
	for i, envVar := range envVars {
		if envVar.Shared != nil && *envVar.Shared {
			return fmt.Errorf("%w: env_vars[%d]: create the agent first, then share the secret on its profile",
				ErrInvalidProfileEnvVars, i)
		}
	}
	return nil
}
