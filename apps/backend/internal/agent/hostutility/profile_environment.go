package hostutility

import (
	"context"
	"errors"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// ProfileEnvironmentResolver resolves secrets from an authorized profile snapshot.
type ProfileEnvironmentResolver func(context.Context, string, []settingsmodels.ProfileEnvVar) (map[string]string, error)

// SetProfileEnvironmentResolver wires the lifecycle-owned secret resolver.
func (m *Manager) SetProfileEnvironmentResolver(resolver ProfileEnvironmentResolver) {
	m.profileEnvironmentResolver = resolver
}

func (m *Manager) resolveProfileEnvironment(ctx context.Context, agentName string, values []settingsmodels.ProfileEnvVar) (map[string]string, error) {
	if m.profileEnvironmentResolver != nil {
		env, err := m.profileEnvironmentResolver(ctx, agentName, values)
		if err == nil && env == nil {
			env = make(map[string]string)
		}
		return env, err
	}
	agentConfig, ok := m.registry.Get(agentName)
	if !ok {
		return nil, errors.New("utility agent is unavailable")
	}
	var defaults map[string]string
	if rt := agentConfig.Runtime(); rt != nil {
		defaults = rt.Env
	}
	return runtimeenv.ResolveProfile(ctx, values, defaults, nil)
}
