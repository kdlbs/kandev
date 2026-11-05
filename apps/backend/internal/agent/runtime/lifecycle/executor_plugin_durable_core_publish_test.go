package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A core-prepared durable resume has no provider-recorded repository set, so the
// publication must say so and carry the host's workspace path.
func TestDurableResumeOfACorePreparedWorkspacePublishesAsCorePrepared(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()

	require.Len(t, fixture.store.published, 1)
	published := fixture.store.published[0]
	require.True(t, published.CorePreparedWorkspace)
	require.Equal(t, pluginExecutorWorkspacePath, published.WorkspacePath)
	require.Empty(t, published.Repos)
}

// A provider-prepared resume keeps publishing the provider's recorded set.
func TestDurableResumeOfAProviderPreparedWorkspaceDoesNotPublishAsCorePrepared(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	inventory.WorkspacePreparation = pluginWorkspacePreparationProvider
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	_, _ = fixture.recover()
	for _, published := range fixture.store.published {
		require.False(t, published.CorePreparedWorkspace)
	}
}
