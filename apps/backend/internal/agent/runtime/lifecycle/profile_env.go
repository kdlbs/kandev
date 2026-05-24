package lifecycle

import (
	"context"

	"go.uber.org/zap"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// metadataKeyProfileEnvResolved caches resolved profile env vars on an execution
// so configureAndStartAgent does not re-resolve secrets on the same launch.
const metadataKeyProfileEnvResolved = "profile_env_resolved"

// mergeAgentProfileEnv fills missing keys in env from the agent profile's
// env_vars. Existing keys in env (office tokens, executor profile env, etc.)
// are never overwritten.
func (m *Manager) mergeAgentProfileEnv(ctx context.Context, profileID string, env map[string]string) {
	if profileID == "" || env == nil || m.profileResolver == nil {
		return
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return
	}
	m.mergeAgentProfileEnvFromInfo(ctx, info, env)
}

func (m *Manager) mergeAgentProfileEnvFromInfo(ctx context.Context, info *AgentProfileInfo, env map[string]string) {
	if info == nil || env == nil || len(info.EnvVars) == 0 {
		return
	}
	resolved := m.resolveAgentProfileEnvVars(ctx, info.EnvVars)
	mergeEnvFillMissing(env, resolved)
}

func (m *Manager) cacheResolvedProfileEnv(execution *AgentExecution, resolved map[string]string) {
	if execution == nil || len(resolved) == 0 {
		return
	}
	if execution.Metadata == nil {
		execution.Metadata = make(map[string]interface{})
	}
	execution.Metadata[metadataKeyProfileEnvResolved] = cloneStringMap(resolved)
}

func (m *Manager) mergeAgentProfileEnvForExecution(ctx context.Context, execution *AgentExecution, env map[string]string) {
	if execution != nil {
		if cached, ok := execution.Metadata[metadataKeyProfileEnvResolved].(map[string]string); ok && len(cached) > 0 {
			mergeEnvFillMissing(env, cached)
			return
		}
	}
	if execution == nil {
		return
	}
	m.mergeAgentProfileEnv(ctx, execution.AgentProfileID, env)
}

func mergeEnvFillMissing(dst, src map[string]string) {
	if len(src) == 0 || dst == nil {
		return
	}
	for k, v := range src {
		if v == "" {
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}

// resolveAgentProfileEnvVars resolves profile env entries. SecretID wins over
// Value; if secret resolution fails, the entry is skipped rather than falling
// back. Literal Value is used only when SecretID is empty, and empty keys are
// ignored.
func (m *Manager) resolveAgentProfileEnvVars(ctx context.Context, envVars []settingsmodels.ProfileEnvVar) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(envVars))
	for _, ev := range envVars {
		key := ev.Key
		if key == "" {
			continue
		}
		if ev.SecretID != "" {
			if m.secretStore == nil {
				m.logger.Warn("secret store not configured for profile env var",
					zap.String("key", key),
					zap.String("secret_id", ev.SecretID))
				continue
			}
			value, err := m.secretStore.Reveal(ctx, ev.SecretID)
			if err != nil {
				m.logger.Warn("failed to resolve secret for profile env var",
					zap.String("key", key),
					zap.String("secret_id", ev.SecretID),
					zap.Error(err))
				continue
			}
			resolved[key] = value
			continue
		}
		if ev.Value != "" {
			resolved[key] = ev.Value
		}
	}
	return resolved
}
