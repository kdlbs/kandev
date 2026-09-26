package executor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
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

// Keep the historical executor error names as aliases while the lifecycle and
// orchestrator share one source-aware resolver.
type EnvironmentConflictError = runtimeenv.ConflictError
type EnvironmentSecretError = runtimeenv.SecretError

func (e *Executor) taskEnvironmentSources(
	workspaceID string,
	managed map[string]string,
	profileEnvVars []models.ProfileEnvVar,
	repositories []*repoInfo,
) ([]environmentSource, error) {
	// Avoid adding independent lengths to build the capacity. The sum can
	// overflow before allocation when values originate in request-sized data.
	sources := make([]environmentSource, 0)
	managedKeys := make([]string, 0, len(managed))
	for key := range managed {
		managedKeys = append(managedKeys, key)
	}
	sort.Strings(managedKeys)
	for _, key := range managedKeys {
		sources = append(sources, environmentSource{key: key, literal: managed[key], origin: runtimeenv.OriginManagedRuntime})
	}
	for _, envVar := range profileEnvVars {
		if envVar.SecretID == "" && envVar.Value == "" {
			continue
		}
		sources = append(sources, environmentSource{
			key: envVar.Key, literal: envVar.Value, secretID: envVar.SecretID, origin: runtimeenv.OriginExecutorProfile,
		})
	}
	for _, info := range repositories {
		if info == nil || info.Repository == nil || len(info.Repository.SecretBindings) == 0 {
			continue
		}
		if info.Repository.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("repository environment belongs to a different workspace")
		}
		origin := runtimeenv.RepositoryOrigin(info.Repository.Name)
		for _, binding := range info.Repository.SecretBindings {
			sources = append(sources, environmentSource{
				key: binding.Key, secretID: binding.SecretID, origin: origin, workspaceID: workspaceID,
			})
		}
	}
	return sources, nil
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
	var preferredShellApplied bool
	req.Env, preferredShellApplied = e.applyPreferredShellEnvWithStatus(ctx, req.ExecutorType, req.Env)
	if preferredShellApplied {
		profileEnvVars = withoutPreferredShellProfileEnvVars(profileEnvVars)
	}
	sources, err := e.taskEnvironmentSources(req.WorkspaceID, req.Env, profileEnvVars, repositories)
	if err != nil {
		return fmt.Errorf("build task environment sources: %w", err)
	}
	req.EnvironmentDefinitions = make([]runtimeenv.Definition, 0, len(sources))
	for _, source := range sources {
		req.EnvironmentDefinitions = append(req.EnvironmentDefinitions, runtimeenv.Definition{
			Key: source.key, Literal: source.literal, SecretID: source.secretID,
			Origin: source.origin, WorkspaceID: source.workspaceID,
		})
	}
	// This shape-only preflight keeps repository/repository and profile/repository
	// conflicts from reaching the agent manager. Lifecycle repeats validation and
	// performs the actual reveal after it has added managed runtime definitions.
	if err := runtimeenv.Validate(req.EnvironmentDefinitions); err != nil {
		return fmt.Errorf("validate task environment sources: %w", err)
	}
	req.EnvironmentResolutionRequired = true
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
	req.ApprovedSecretEnvKeys = make([]string, 0, len(keys))
	for key := range keys {
		req.ApprovedSecretEnvKeys = append(req.ApprovedSecretEnvKeys, key)
	}
	sort.Strings(req.ApprovedSecretEnvKeys)
	return nil
}

func withoutPreferredShellProfileEnvVars(values []models.ProfileEnvVar) []models.ProfileEnvVar {
	filtered := make([]models.ProfileEnvVar, 0, len(values))
	for _, value := range values {
		if isPreferredShellEnvKey(value.Key) {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func isPreferredShellEnvKey(key string) bool {
	return key == "SHELL" || key == "AGENTCTL_SHELL_COMMAND"
}

func (e *Executor) revealGlobalSecret(ctx context.Context, secretID string) (string, error) {
	if scoped, ok := e.secretStore.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, secretID)
	}
	return e.secretStore.Reveal(ctx, secretID)
}
