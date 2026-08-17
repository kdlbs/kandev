package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys pins the runtime
// environment invariant at the launch-definition boundary: every
// Agent.Runtime().Env entry becomes a definition, but a higher-precedence
// definition already present for that key wins.
func TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	require.True(t, ok)
	runtimeEnv := agentConfig.Runtime().Env
	require.NotEmpty(t, runtimeEnv, "the fixture agent must declare runtime env for this to be meaningful")

	var claimed string
	for key := range runtimeEnv {
		claimed = key
		break
	}

	definitions := []runtimeenv.Definition{
		{Key: claimed, Literal: "profile-wins", Origin: runtimeenv.OriginAgentProfile},
	}
	appendAgentRuntimeDefaults(&definitions, agentConfig)

	byKey := map[string]runtimeenv.Definition{}
	for _, d := range definitions {
		require.NotContains(t, byKey, d.Key, "duplicate definition for %q", d.Key)
		byKey[d.Key] = d
	}
	require.Len(t, byKey, len(runtimeEnv),
		"every Agent.Runtime().Env key must reach the launch environment exactly once")
	require.Equal(t, "profile-wins", byKey[claimed].Literal,
		"an existing higher-precedence definition must not be overwritten by the agent default")
	require.Equal(t, runtimeenv.OriginAgentProfile, byKey[claimed].Origin)
	for key, value := range runtimeEnv {
		if key == claimed {
			continue
		}
		require.Equal(t, value, byKey[key].Literal, "runtime env %q lost its value", key)
		require.Equal(t, runtimeenv.OriginManagedAgentDefaults, byKey[key].Origin)
	}
}

func TestAppendAgentRuntimeDefaultsIgnoresMissingConfig(t *testing.T) {
	definitions := []runtimeenv.Definition{{Key: "EXISTING", Literal: "value"}}

	appendAgentRuntimeDefaults(&definitions, nil)

	require.Len(t, definitions, 1)
}

func TestHasEnvironmentDefinition(t *testing.T) {
	definitions := []runtimeenv.Definition{
		{Key: "PATH", Literal: "/usr/bin"},
		{Key: "ANTHROPIC_API_KEY", SecretID: "secret-1"},
	}

	require.True(t, hasEnvironmentDefinition(definitions, "PATH"))
	require.True(t, hasEnvironmentDefinition(definitions, "ANTHROPIC_API_KEY"))
	require.False(t, hasEnvironmentDefinition(definitions, "HTTPS_PROXY"))
	require.False(t, hasEnvironmentDefinition(nil, "PATH"))
}

func TestAppendMapDefinitionsIsDeterministicallyOrdered(t *testing.T) {
	var definitions []runtimeenv.Definition

	appendMapDefinitions(&definitions, map[string]string{
		"ZED": "3", "ALPHA": "1", "MIKE": "2",
	}, runtimeenv.OriginAgentProfile)

	require.Equal(t, []runtimeenv.Definition{
		{Key: "ALPHA", Literal: "1", Origin: runtimeenv.OriginAgentProfile},
		{Key: "MIKE", Literal: "2", Origin: runtimeenv.OriginAgentProfile},
		{Key: "ZED", Literal: "3", Origin: runtimeenv.OriginAgentProfile},
	}, definitions, "map iteration order must not leak into the launch environment")

	var empty []runtimeenv.Definition
	appendMapDefinitions(&empty, nil, runtimeenv.OriginAgentProfile)
	require.Empty(t, empty)
}

// TestAppendProfileDefinitionsDropsUnusableEntries pins the filter: a blank key
// or an entry with neither a literal nor a secret would produce an env var the
// agent cannot use.
func TestAppendProfileDefinitionsDropsUnusableEntries(t *testing.T) {
	var definitions []runtimeenv.Definition

	appendProfileDefinitions(&definitions, []settingsmodels.ProfileEnvVar{
		{Key: "  ", Value: "blank key"},
		{Key: "EMPTY"},
		{Key: "LITERAL", Value: "value"},
		{Key: "SECRET", SecretID: "secret-1"},
	}, runtimeenv.OriginAgentProfile)

	require.Equal(t, []runtimeenv.Definition{
		{Key: "LITERAL", Literal: "value", Origin: runtimeenv.OriginAgentProfile},
		{Key: "SECRET", SecretID: "secret-1", Origin: runtimeenv.OriginAgentProfile},
	}, definitions)
}

func TestApprovedSecretEnvironmentKeysOnlyIncludesWorkspaceScoped(t *testing.T) {
	approved := approvedSecretEnvironmentKeys([]runtimeenv.Definition{
		{Key: "GLOBAL_SECRET", SecretID: "secret-1"},
		{Key: "REPO_TOKEN", SecretID: "secret-2", WorkspaceID: "ws-1"},
		{Key: "  ", SecretID: "secret-3", WorkspaceID: "ws-1"},
		{Key: "ANOTHER_TOKEN", SecretID: "secret-4", WorkspaceID: "ws-1"},
		{Key: "REPO_TOKEN", SecretID: "secret-5", WorkspaceID: "ws-2"},
	})

	require.Equal(t, []string{"ANOTHER_TOKEN", "REPO_TOKEN"}, approved,
		"only workspace-scoped bindings may be forwarded, deduplicated and sorted")
}

func TestResolveEnvironmentDefinitionLiteralNeedsNoSecretStore(t *testing.T) {
	mgr := newTestManager(t)

	value, err := mgr.resolveEnvironmentDefinition(context.Background(),
		runtimeenv.Definition{Key: "PATH", Literal: "/usr/bin"})

	require.NoError(t, err)
	require.Equal(t, "/usr/bin", value)
}

func TestResolveEnvironmentDefinitionRequiresSecretStore(t *testing.T) {
	mgr := newTestManager(t)
	mgr.secretStore = nil

	_, err := mgr.resolveEnvironmentDefinition(context.Background(),
		runtimeenv.Definition{Key: "TOKEN", SecretID: "secret-1"})

	require.ErrorContains(t, err, "secret store unavailable")
}

// TestResolveEnvironmentDefinitionRejectsUnscopedStoreForWorkspaceSecret pins
// the fail-closed rule: a workspace-scoped binding must never fall back to a
// global reveal, which would cross the workspace boundary.
func TestResolveEnvironmentDefinitionRejectsUnscopedStoreForWorkspaceSecret(t *testing.T) {
	mgr := newTestManager(t)
	mgr.secretStore = &mockSecretStore{}

	_, err := mgr.resolveEnvironmentDefinition(context.Background(),
		runtimeenv.Definition{Key: "REPO_TOKEN", SecretID: "secret-1", WorkspaceID: "ws-1"})

	require.ErrorContains(t, err, "workspace-scoped secret storage is unavailable")
}
