package lifecycle

import (
	"context"
	"errors"
	"fmt"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/gitconfigenv"
)

var ErrProfileSecretUnavailable = errors.New("BLOCKED_PROFILE_SECRET")

// mergeAgentProfileEnv fills missing keys in env from the agent profile's
// env_vars. Existing keys in env (office tokens, executor profile env, etc.)
// are never overwritten.
func (m *Manager) mergeAgentProfileEnv(ctx context.Context, profileID string, env map[string]string) error {
	if profileID == "" || env == nil || m.profileResolver == nil {
		return nil
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return err
	}
	return m.mergeAgentProfileEnvFromInfo(ctx, info, env)
}

func (m *Manager) mergeAgentProfileEnvFromInfo(ctx context.Context, info *AgentProfileInfo, env map[string]string) error {
	if info == nil || env == nil || len(info.EnvVars) == 0 {
		return nil
	}
	resolved, err := m.resolveAgentProfileEnvVars(ctx, info.EnvVars)
	if err != nil {
		return err
	}
	mergeEnvFillMissing(env, resolved)
	return nil
}

func (m *Manager) mergeAgentProfileEnvForExecution(ctx context.Context, execution *AgentExecution, env map[string]string) error {
	if execution == nil {
		return nil
	}
	return m.mergeAgentProfileEnv(ctx, execution.AgentProfileID, env)
}

func mergeEnvFillMissing(dst, src map[string]string) {
	if len(src) == 0 || dst == nil {
		return
	}
	for k, v := range src {
		if v == "" || gitconfigenv.IsIndexedKey(k) {
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
	merged, err := gitconfigenv.Merge(src, dst)
	if err == nil {
		gitconfigenv.CopyIndexed(dst, merged)
	}
}

// resolveAgentProfileEnvVars resolves profile env entries. SecretID wins over
// Value. A missing secret store or failed reveal aborts the whole profile
// environment rather than falling back to a literal value or partial map.
func (m *Manager) resolveAgentProfileEnvVars(ctx context.Context, envVars []settingsmodels.ProfileEnvVar) (map[string]string, error) {
	if len(envVars) == 0 {
		return nil, nil
	}
	resolved := make(map[string]string, len(envVars))
	for _, ev := range envVars {
		key := ev.Key
		if key == "" {
			continue
		}
		if ev.SecretID != "" {
			if m.secretStore == nil {
				return nil, profileSecretError(key)
			}
			value, err := m.revealGlobalSecret(ctx, ev.SecretID)
			if err != nil {
				// Preserve caller cancellation identity. The sanitized sentinel is
				// for secret failures only; callers use context errors to stop work
				// without misclassifying a cancelled request as bad configuration.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				return nil, profileSecretError(key)
			}
			resolved[key] = value
			continue
		}
		if ev.Value != "" {
			resolved[key] = ev.Value
		}
	}
	return resolved, nil
}

func profileSecretError(key string) error {
	return fmt.Errorf("%w: env key %q unavailable", ErrProfileSecretUnavailable, key)
}

func (m *Manager) revealGlobalSecret(ctx context.Context, secretID string) (string, error) {
	return revealGlobalSecret(ctx, m.secretStore, secretID)
}
