package lifecycle

import (
	"context"
	"errors"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

// ResolveUtilityProfileEnvironment resolves the already authorized profile snapshot.
func (m *Manager) ResolveUtilityProfileEnvironment(ctx context.Context, agentName string, values []settingsmodels.ProfileEnvVar) (map[string]string, error) {
	agentConfig, ok := m.registry.Get(agentName)
	if !ok {
		return nil, errors.New("utility agent is unavailable")
	}
	var defaults map[string]string
	if rt := agentConfig.Runtime(); rt != nil {
		defaults = rt.Env
	}
	return runtimeenv.ResolveProfile(ctx, values, defaults, func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		if m.secretStore == nil {
			return "", errors.New("secret store unavailable")
		}
		if scoped, ok := m.secretStore.(secrets.ScopedSecretStore); ok {
			return scoped.RevealGlobal(ctx, definition.SecretID)
		}
		if err := secrets.ValidateGlobalReference(ctx, m.secretStore, definition.SecretID); err != nil {
			return "", err
		}
		return m.secretStore.Reveal(ctx, definition.SecretID)
	})
}
