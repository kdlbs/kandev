package lifecycle

import (
	"context"
	"errors"
	"fmt"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/gitconfigenv"
	"go.uber.org/zap"
)

var ErrProfileSecretUnavailable = errors.New("BLOCKED_PROFILE_SECRET")

// mergeAgentProfileEnv fills missing keys in env from the agent profile's
// env_vars. Existing keys in env (office tokens, executor profile env, etc.)
// are never overwritten.
func (m *Manager) mergeAgentProfileEnv(ctx context.Context, profileID string, env map[string]string) error {
	return m.mergeAgentProfileEnvWithPartial(ctx, profileID, env, false)
}

// mergeAgentProfileEnvWithPartial is mergeAgentProfileEnv with the partial-vs-
// fail-closed choice exposed. The ACP session-launch env builder passes
// partial=true so a broken secret on one profile var drops that var instead of
// blanking the agent's whole environment (AC-004.1); every other caller keeps
// the fail-closed default.
func (m *Manager) mergeAgentProfileEnvWithPartial(ctx context.Context, profileID string, env map[string]string, partial bool) error {
	if profileID == "" || env == nil || m.profileResolver == nil {
		return nil
	}
	info, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil || info == nil {
		return err
	}
	return m.mergeAgentProfileEnvFromInfoWithPartial(ctx, info, env, partial)
}

func (m *Manager) mergeAgentProfileEnvFromInfo(ctx context.Context, info *AgentProfileInfo, env map[string]string) error {
	return m.mergeAgentProfileEnvFromInfoWithPartial(ctx, info, env, false)
}

func (m *Manager) mergeAgentProfileEnvFromInfoWithPartial(ctx context.Context, info *AgentProfileInfo, env map[string]string, partial bool) error {
	if info == nil || env == nil || len(info.EnvVars) == 0 {
		return nil
	}
	resolved, err := m.resolveAgentProfileEnvVarsWithPartial(ctx, info.EnvVars, partial)
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
	return m.resolveAgentProfileEnvVarsWithPartial(ctx, envVars, false)
}

// resolveAgentProfileEnvVarsWithPartial resolves profile env entries. When
// partial is true a single secret-store miss or failed reveal drops only that
// entry (with a warn log) and the remaining entries are still delivered — a
// broken secret reference on one variable must not blank the agent's whole
// environment at session launch. When partial is false the whole environment is
// aborted (the passthrough-restart and terminal-env callers rely on that
// fail-closed behavior). Cancellation errors always propagate.
func (m *Manager) resolveAgentProfileEnvVarsWithPartial(
	ctx context.Context, envVars []settingsmodels.ProfileEnvVar, partial bool,
) (map[string]string, error) {
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
				if partial {
					m.logger.Warn("agent profile env var references a secret but no secret store is configured; dropping entry",
						zap.String("env_key", key))
					continue
				}
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
				if partial {
					m.logger.Warn("failed to reveal secret for agent profile env var; dropping entry",
						zap.String("env_key", key), zap.Error(err))
					continue
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
