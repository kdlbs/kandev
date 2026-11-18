package lifecycle

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProviderBindingCannotBeInheritedByFreshLaunch(t *testing.T) {
	keys := []string{p.ProviderMCPBindingMetadataKey, p.ProviderMCPEffectsClosedMetadataKey, p.ProviderMCPOperationMetadataKey, p.ProviderMCPRuntimeProfileMetadataKey}
	supplied := map[string]interface{}{"retained_setting": "kept"}
	for _, key := range keys {
		supplied[key] = map[string]any{"untrusted": true}
	}
	executorConfig := map[string]string{}
	for _, key := range keys {
		executorConfig[key] = "untrusted"
	}
	launch := buildLaunchMetadata(&LaunchRequest{Metadata: supplied, ExecutorConfig: executorConfig}, "", "", "")
	workspace := copyExecutionCreateMetadata(supplied)
	fresh := (&ExecutorInstance{InstanceID: "new", Metadata: supplied}).ToAgentExecution(&ExecutorCreateRequest{Metadata: supplied})
	for _, key := range keys {
		require.NotContains(t, launch, key, "task metadata cannot grant a provider scope")
		require.NotContains(t, workspace, key)
		_, exists := fresh.metadataValue(key)
		require.False(t, exists, "a new process owner cannot inherit another process's acceptance")
	}
	require.Equal(t, "kept", launch["retained_setting"])
	require.Equal(t, "kept", workspace["retained_setting"])
	require.Contains(t, supplied, p.ProviderMCPBindingMetadataKey, "preparation must not mutate its source")
}
