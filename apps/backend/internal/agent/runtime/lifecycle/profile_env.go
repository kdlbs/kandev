package lifecycle

import (
	"context"
	"errors"
	"fmt"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/githubauth"
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

// composeExecutionRuntimeEnvironment updates an existing execution snapshot
// with a per-run overlay. Host GitHub helpers are Kandev-owned generated
// entries and are removed before composition so a later request can replace
// or remove them without hiding inherited user configuration.
func composeExecutionRuntimeEnvironment(base, overlay map[string]string) (map[string]string, error) {
	removeObsoleteManagedCredentialEnvironment(base)
	filtered, err := gitconfigenv.Filter(base, func(index int, entries []gitconfigenv.Entry) bool {
		return !githubauth.IsHostGitHubCredentialHelperEntry(entries[index].Key, entries[index].Value)
	})
	if err != nil {
		return nil, fmt.Errorf("remove generated host GitHub helper: %w", err)
	}
	return gitconfigenv.Merge(filtered, overlay)
}

func removeObsoleteManagedCredentialEnvironment(env map[string]string) {
	for _, key := range []string{
		githubauth.CredentialBrokerURLEnv,
		githubauth.CredentialHelperPathEnv,
		githubauth.CredentialCLIShimDirEnv,
		githubauth.CredentialCLIBashEnvEnv,
		githubauth.CredentialParentBashEnv,
		githubauth.CredentialLeaseEnv,
		githubauth.CredentialReissueCapabilityEnv,
		githubauth.CredentialTaskIDEnv,
		githubauth.CredentialSessionIDEnv,
		githubauth.CredentialRepositoryEnv,
		githubauth.CredentialOwnerEnv,
		githubauth.CredentialRepoEnv,
		githubauth.CredentialHostEnv,
		githubauth.CredentialScopesEnv,
	} {
		delete(env, key)
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
