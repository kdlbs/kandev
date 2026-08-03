package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type environmentSource struct {
	key         string
	literal     string
	secretID    string
	origin      string
	workspaceID string
}

type environmentSourceResolver func(context.Context, environmentSource) (string, error)

// EnvironmentConflictError identifies an ambiguous environment key without
// including either plaintext values or secret identifiers.
type EnvironmentConflictError struct {
	Key     string
	Origins []string
}

func (e *EnvironmentConflictError) Error() string {
	return fmt.Sprintf("environment key %q has conflicting definitions from %s", e.Key, strings.Join(e.Origins, ", "))
}

// EnvironmentSecretError identifies a secret that could not be resolved while
// retaining a redacted error boundary for callers and logs.
type EnvironmentSecretError struct {
	Key    string
	Origin string
	err    error
}

func (e *EnvironmentSecretError) Error() string {
	return fmt.Sprintf("environment key %q from %s could not be resolved", e.Key, e.Origin)
}

func (e *EnvironmentSecretError) Unwrap() error { return e.err }

func resolveEnvironmentSources(
	ctx context.Context, sources []environmentSource, resolve environmentSourceResolver,
) (map[string]string, error) {
	ordered := append([]environmentSource(nil), sources...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].key != ordered[j].key {
			return ordered[i].key < ordered[j].key
		}
		if ordered[i].origin != ordered[j].origin {
			return ordered[i].origin < ordered[j].origin
		}
		return ordered[i].secretID < ordered[j].secretID
	})

	selected := make(map[string]environmentSource, len(ordered))
	origins := make(map[string]map[string]struct{}, len(ordered))
	for _, source := range ordered {
		if strings.TrimSpace(source.key) == "" {
			continue
		}
		if prior, exists := selected[source.key]; exists {
			if environmentSourceIdentity(prior) != environmentSourceIdentity(source) {
				conflictOrigins := make([]string, 0, len(origins[source.key])+1)
				for origin := range origins[source.key] {
					conflictOrigins = append(conflictOrigins, origin)
				}
				conflictOrigins = append(conflictOrigins, source.origin)
				sort.Strings(conflictOrigins)
				return nil, &EnvironmentConflictError{Key: source.key, Origins: conflictOrigins}
			}
			origins[source.key][source.origin] = struct{}{}
			continue
		}
		selected[source.key] = source
		origins[source.key] = map[string]struct{}{source.origin: {}}
	}

	resolved := make(map[string]string, len(selected))
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		source := selected[key]
		value := source.literal
		if source.secretID != "" {
			var err error
			value, err = resolve(ctx, source)
			if err != nil {
				return nil, &EnvironmentSecretError{Key: source.key, Origin: source.origin, err: err}
			}
		}
		resolved[key] = value
	}
	return resolved, nil
}

func environmentSourceIdentity(source environmentSource) string {
	if source.secretID != "" {
		return "secret:" + source.secretID
	}
	hash := sha256.Sum256([]byte(source.literal))
	return "literal:" + hex.EncodeToString(hash[:])
}

// resolveTaskEnvironment merges managed values, executor profile definitions,
// and all attached repository bindings. Secret values are revealed only after
// the origin/conflict pass has completed.
func (e *Executor) resolveTaskEnvironment(
	ctx context.Context,
	workspaceID string,
	managed map[string]string,
	profileEnvVars []models.ProfileEnvVar,
	repositories []*repoInfo,
) (map[string]string, error) {
	sources := make([]environmentSource, 0, len(managed)+len(profileEnvVars))
	managedKeys := make([]string, 0, len(managed))
	for key := range managed {
		managedKeys = append(managedKeys, key)
	}
	sort.Strings(managedKeys)
	for _, key := range managedKeys {
		sources = append(sources, environmentSource{key: key, literal: managed[key], origin: "managed runtime"})
	}
	for _, envVar := range profileEnvVars {
		sources = append(sources, environmentSource{
			key: envVar.Key, literal: envVar.Value, secretID: envVar.SecretID, origin: "executor profile",
		})
	}
	for _, info := range repositories {
		if info == nil || info.Repository == nil || len(info.Repository.SecretBindings) == 0 {
			continue
		}
		if info.Repository.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("repository environment belongs to a different workspace")
		}
		origin := "repository"
		if name := strings.TrimSpace(info.Repository.Name); name != "" {
			origin = "repository " + name
		}
		for _, binding := range info.Repository.SecretBindings {
			sources = append(sources, environmentSource{
				key: binding.Key, secretID: binding.SecretID, origin: origin, workspaceID: workspaceID,
			})
		}
	}
	return resolveEnvironmentSources(ctx, sources, e.resolveEnvironmentSource)
}

func (e *Executor) resolveLaunchEnvironment(
	ctx context.Context,
	req *LaunchAgentRequest,
	profileEnvVars []models.ProfileEnvVar,
	repositories []*repoInfo,
) error {
	if req == nil {
		return errors.New("launch request is required")
	}
	req.Env = e.applyPreferredShellEnv(ctx, req.ExecutorType, req.Env)
	resolved, err := e.resolveTaskEnvironment(ctx, req.WorkspaceID, req.Env, profileEnvVars, repositories)
	if err != nil {
		return fmt.Errorf("resolve task environment: %w", err)
	}
	req.Env = resolved
	keys := make(map[string]struct{})
	for _, info := range repositories {
		if info == nil || info.Repository == nil {
			continue
		}
		for _, binding := range info.Repository.SecretBindings {
			if strings.TrimSpace(binding.Key) != "" {
				keys[binding.Key] = struct{}{}
			}
		}
	}
	if len(keys) > 0 {
		req.ApprovedSecretEnvKeys = make([]string, 0, len(keys))
		for key := range keys {
			req.ApprovedSecretEnvKeys = append(req.ApprovedSecretEnvKeys, key)
		}
		sort.Strings(req.ApprovedSecretEnvKeys)
	}
	return nil
}

func (e *Executor) resolveEnvironmentSource(ctx context.Context, source environmentSource) (string, error) {
	if e.secretStore == nil {
		return "", errors.New("secret store unavailable")
	}
	if source.workspaceID != "" {
		if scoped, ok := e.secretStore.(secrets.ScopedSecretStore); ok {
			return scoped.RevealForWorkspace(ctx, source.secretID, source.workspaceID)
		}
	}
	return e.revealGlobalSecret(ctx, source.secretID)
}

func (e *Executor) revealGlobalSecret(ctx context.Context, secretID string) (string, error) {
	if scoped, ok := e.secretStore.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, secretID)
	}
	return e.secretStore.Reveal(ctx, secretID)
}
