package lifecycle

import (
	"github.com/kandev/kandev/internal/task/models"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinLaunchMetadataUsesOnlyTrustedProfile(t *testing.T) {
	profile := builtinDispatchProfile(p.PackageCore)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	for _, source := range []string{"metadata", "executor"} {
		req := &LaunchRequest{Metadata: map[string]any{}, ExecutorConfig: map[string]string{}}
		if source == "metadata" {
			req.Metadata[p.BuiltinBindingMetadataKey] = encoded
		} else {
			req.ExecutorConfig[p.BuiltinBindingMetadataKey] = encoded
		}
		metadata := buildLaunchMetadata(req, "", "", "")
		require.NotContains(t, metadata, p.BuiltinBindingMetadataKey, source)
		req.McpProfile = &profile
		metadata = buildLaunchMetadata(req, "", "", "")
		require.Equal(t, encoded, metadata[p.BuiltinBindingMetadataKey])
	}
}

func TestBuiltinBindingPersistsOnlyWithinItsSession(t *testing.T) {
	require.True(t, ShouldPersistMetadataKey(p.BuiltinBindingMetadataKey))
	require.True(t, IsSessionScopedMetadataKey(p.BuiltinBindingMetadataKey))
}

func TestBuiltinBindingDoesNotInheritPriorExecution(t *testing.T) {
	prior := &models.ExecutorRunning{Metadata: map[string]any{p.BuiltinBindingMetadataKey: "old binding", "context_window": 100}}
	running := buildRunningFromExecution(&AgentExecution{ID: "new"}, prior)
	require.NotContains(t, running.Metadata, p.BuiltinBindingMetadataKey)
	require.Equal(t, 100, running.Metadata["context_window"])
}
