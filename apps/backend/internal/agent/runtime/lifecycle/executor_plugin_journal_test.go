package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginExecutorInstanceCarriesDurableJournalUnderRecordedWorkspace(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, capabilityLaunchProvider(), workspaceResource(testBackendOnlyWorkspace, "worker-7"))
	harness.request.DurableJournalOwnerID = "session-plugin-1"

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	require.NoError(t, err)
	require.Equal(t, testBackendOnlyWorkspace+"/.kandev/agentctl-journals/session-plugin-1/delivery.bbolt", harness.instance.DurableJournalPath)
	require.Equal(t, testBackendOnlyWorkspace, harness.instance.WorkspacePath)
}

func TestPluginExecutorInstanceHasNoJournalWithoutOwnerOrDescriptor(t *testing.T) {
	t.Run("no owner", func(t *testing.T) {
		harness := newWorkspaceLaunchHarness(t, capabilityLaunchProvider(), workspaceResource(testBackendOnlyWorkspace, "worker-7"))
		_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
		require.NoError(t, err)
		require.Empty(t, harness.instance.DurableJournalPath)
	})
	t.Run("provider declares no workspace descriptor", func(t *testing.T) {
		harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
		harness.request.DurableJournalOwnerID = "session-plugin-1"
		_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
		require.NoError(t, err)
		require.Empty(t, harness.instance.DurableJournalPath)
	})
}
